// Owned, small, conditional-library fixtures. Never import into the runner/browser.
import assert from 'node:assert/strict'
import {mkdtemp, rm, writeFile} from 'node:fs/promises'
import {createServer} from 'node:http'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import axios from 'axios'
import resolveConfig from 'axios/unsafe/helpers/resolveConfig.js'
import {ssrRenderAttrs} from '@vue/server-renderer'
import NodeFormData from 'form-data'
import postcss from 'postcss'
import {customRandom, nanoid} from 'nanoid'
import {customAlphabet as insecureAlphabet} from 'nanoid/non-secure'
import sourceMap from 'source-map-js'
import api, {rootApi} from '../../src/api/client.js'

const {SourceMapConsumer}=sourceMap
const kind=process.argv[2]
assert.ok(process.send, 'fixture requires owned IPC parent')

function withInheritedCredentials(run) {
  const keys=['username','password'], previous=keys.map(key=>Object.getOwnPropertyDescriptor(Object.prototype,key))
  try {
    for(const key of keys)Object.defineProperty(Object.prototype,key,{value:`OWN-POLLUTED-${key}`,configurable:true,writable:true})
    return run()
  } finally {
    keys.forEach((key,i)=>previous[i]?Object.defineProperty(Object.prototype,key,previous[i]):delete Object.prototype[key])
  }
}
const basic=(username,password)=>`Basic ${Buffer.from(`${username}:${password}`).toString('base64')}`
const flatMap=()=>({version:3,sources:['fixture.js'],sourcesContent:['owned fixture'],names:[],mappings:'AAAA'})
function indexedMap(depth=1,line=0,column=0) {
  let map=flatMap()
  for(let i=0;i<depth;i++)map={version:3,sections:[{offset:{line,column},map}]}
  return map
}
async function postcssMap(unbased) {
  const owned=await mkdtemp(join(tmpdir(),'openreader-postcss-advisory-'))
  try {
    const marker=unbased?'OWN-OUTSIDE-MAP-MARKER':'OWN-LEGITIMATE-MAP-MARKER'
    const file=join(owned,'fixture.map')
    await writeFile(file,JSON.stringify({...flatMap(),sourcesContent:[marker]}))
    const css=`a{color:red}\n/*# sourceMappingURL=${unbased?file:'fixture.map'} */`
    const result=await postcss([]).process(css,{from:unbased?undefined:join(owned,'fixture.css'),map:{inline:false,annotation:false}})
    assert.ok(result.css.includes('a{color:red}'))
    const output=JSON.stringify(result.map?.toJSON())||''
    assert.equal(output.includes(marker),!unbased,'unbased sourceMappingURL must not disclose an unrelated owned map')
  } finally {await rm(owned,{recursive:true,force:true})}
}
async function appClientChecks(which) {
  const storage=new Map([['openreader_token','fixture-token-A']]),events=[]
  const previousStorage=Object.getOwnPropertyDescriptor(globalThis,'localStorage')
  const previousWindow=Object.getOwnPropertyDescriptor(globalThis,'window')
  Object.defineProperty(globalThis,'localStorage',{configurable:true,value:{getItem:key=>storage.get(key)||null,
    setItem:(key,value)=>storage.set(key,String(value)),removeItem:key=>storage.delete(key)}})
  Object.defineProperty(globalThis,'window',{configurable:true,value:{dispatchEvent:event=>events.push(event)}})
  let lateSeen,releaseLate,cancelSeen
  const seenLate=new Promise(resolve=>{lateSeen=resolve}),lateGate=new Promise(resolve=>{releaseLate=resolve})
  const seenCancel=new Promise(resolve=>{cancelSeen=resolve})
  const server=createServer(async(req,res)=>{
    const chunks=[]
    for await(const chunk of req)chunks.push(chunk)
    if(req.url.includes('/slow')){if(req.url.includes('cancel'))cancelSeen();return}
    if(req.url.includes('/late-reject')){lateSeen();await lateGate}
    res.setHeader('Content-Type','application/json; charset=utf-8')
    if(req.url.includes('reject')||req.url.includes('/auth/'))res.statusCode=401
    res.end(JSON.stringify({url:req.url,headers:req.headers,body:Buffer.concat(chunks).toString('utf8'),error:res.statusCode===401?'fixture unauthorized':undefined}))
  })
  const previous=[api,rootApi].map(client=>({baseURL:client.defaults.baseURL,adapter:client.defaults.adapter,proxy:client.defaults.proxy}))
  try {
    assert.equal(api.defaults.baseURL,'/api');assert.equal(rootApi.defaults.baseURL,undefined)
    assert.equal(api.defaults.timeout,12000);assert.equal(rootApi.defaults.timeout,12000)
    await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve))
    const base=`http://127.0.0.1:${server.address().port}`
    api.defaults.baseURL=`${base}/api`;rootApi.defaults.baseURL=base
    for(const client of [api,rootApi]){client.defaults.adapter='http';client.defaults.proxy=false}
    if(which==='app-client-normal') {
      const get=await api.get('/probe')
      assert.equal(get.status,200);assert.equal(get.data.url,'/api/probe')
      assert.equal(get.data.headers.authorization,'Bearer fixture-token-A')
      const root=await rootApi.get('/webdav/fixture');assert.equal(root.data.url,'/webdav/fixture')
      const explicit=await rootApi.get('/webdav/explicit',{headers:{Authorization:basic('fixture','pass')}})
      assert.equal(explicit.data.headers.authorization,basic('fixture','pass'))
      const json=await api.post('/echo',{title:'中文',n:1})
      assert.deepEqual(JSON.parse(json.data.body),{title:'中文',n:1})
      const form=new FormData();form.append('title','书名');form.append('file',new Blob(['正文'],{type:'text/plain'}),'fixture.txt')
      const upload=await api.post('/upload',form)
      assert.match(upload.data.headers['content-type'],/^multipart\/form-data; boundary=/)
      for(const text of ['name="title"','书名','name="file"','fixture.txt','正文'])assert.ok(upload.data.body.includes(text))
      assert.equal(events.length,0)
    } else if(which==='app-client-session') {
      for(const route of ['/auth/login','/auth/register']) {
        await assert.rejects(api.post(route,{}),error=>error.response.status===401&&error.response.data.error==='fixture unauthorized')
        assert.equal(storage.get('openreader_token'),'fixture-token-A');assert.equal(events.length,0)
      }
      const rejected=assert.rejects(api.get('/late-reject'),error=>error.response.status===401)
      await seenLate;storage.set('openreader_token','fixture-token-B');releaseLate();await rejected
      assert.equal(storage.get('openreader_token'),'fixture-token-B');assert.equal(events.length,0)
      await assert.rejects(api.get('/reject'),error=>error.response.status===401&&error.isAxiosError)
      assert.equal(storage.has('openreader_token'),false);assert.equal(events.length,1)
      assert.equal(events[0].type,'openreader:auth-required')
      assert.deepEqual(events[0].detail,{reason:'session',rejectedToken:'fixture-token-B'})
      await assert.rejects(rootApi.get('/reject'),error=>error.response.status===401)
      assert.equal(events.length,1,'unauthenticated repeat must not create another invalidation')
    } else {
      const controller=new AbortController()
      const rejection=assert.rejects(api.get('/slow?cancel',{signal:controller.signal}),error=>axios.isCancel(error)&&error.code==='ERR_CANCELED')
      await seenCancel;controller.abort();await rejection
      await assert.rejects(api.get('/slow?timeout',{timeout:100}),error=>error.code==='ECONNABORTED'&&error.isAxiosError)
      assert.equal(storage.get('openreader_token'),'fixture-token-A');assert.equal(events.length,0)
    }
  } finally {
    releaseLate();server.closeAllConnections()
    if(server.listening)await new Promise(resolve=>server.close(resolve))
    ;[api,rootApi].forEach((client,i)=>Object.assign(client.defaults,previous[i]))
    if(previousStorage)Object.defineProperty(globalThis,'localStorage',previousStorage);else delete globalThis.localStorage
    if(previousWindow)Object.defineProperty(globalThis,'window',previousWindow);else delete globalThis.window
  }
}
async function run() {
  switch(kind) {
    // Positive harness lifecycle controls, never counted as dependency-vulnerability reds.
    case 'harness-loop':for(;;){} // parent must kill and await close
    case 'harness-output':process.stdout.write(Buffer.alloc(70_000,120));return
    case 'axios-auth-inherited':return withInheritedCredentials(()=>assert.equal(resolveConfig({url:'/owned',auth:{}}).headers.get('Authorization'),basic('','')))
    case 'axios-auth-own':return withInheritedCredentials(()=>assert.equal(resolveConfig({url:'/owned',auth:{username:'own',password:'pass'}}).headers.get('Authorization'),basic('own','pass')))
    case 'axios-depth': {
      const form=new FormData();form.append('a'+'[a]'.repeat(300),'owned')
      assert.throws(()=>axios.formToJSON(form),error=>error.code==='ERR_FORM_DATA_DEPTH_EXCEEDED');return
    }
    case 'axios-form-normal': {
      const form=new FormData();form.append('title','中文');form.append('items[0][title]','第一章')
      assert.deepEqual(axios.formToJSON(form),{title:'中文',items:[{title:'第一章'}]});return
    }
    case 'vue-cr-attr':assert.equal(ssrRenderAttrs({'data-fixture\rhidden':''}),'');return
    case 'vue-normal':assert.equal(ssrRenderAttrs({'data-fixture':'中文<&"'}),' data-fixture="中文&lt;&amp;&quot;"');return
    case 'form-data-field':
    case 'form-data-filename': {
      const form=new NodeFormData(),bad='owned"\r\nX-Owned-Fixture: yes\r\nignored="'
      form.append(kind==='form-data-field'?bad:'file','owned',{filename:kind==='form-data-filename'?bad:'normal.txt'})
      assert.ok(!form.getBuffer().toString().includes('\r\nX-Owned-Fixture: yes\r\n'),'unescaped multipart header injection');return
    }
    case 'form-data-normal': {
      const form=new NodeFormData();form.append('title','中文');form.append('file',Buffer.from('正文'),{filename:'中文.txt'})
      const body=form.getBuffer().toString()
      for(const text of ['name="title"','中文','filename="中文.txt"','正文'])assert.ok(body.includes(text));return
    }
    case 'postcss-unbased-map':return postcssMap(true)
    case 'postcss-normal':return postcssMap(false)
    case 'nanoid-zero': {
      let calls=0
      const generate=customRandom('ab',4,size=>{if(++calls>32)throw new Error(`owned random work budget exceeded (${calls}>32)`);return new Uint8Array(size)})
      assert.equal(generate(0),'');assert.ok(calls<=1);return
    }
    case 'nanoid-negative': {
      const original=Math.random;let calls=0
      try {
        Math.random=()=>{if(++calls>32)throw new Error(`owned random work budget exceeded (${calls}>32)`);return 0.5}
        assert.equal(insecureAlphabet('ab',4)(-1),'');assert.ok(calls<=1)
      } finally {Math.random=original}
      return
    }
    case 'nanoid-normal':assert.match(nanoid(8),/^[\w-]{8}$/);assert.match(insecureAlphabet('ab',4)(),/^[ab]{4}$/);return
    case 'source-map-getter': {
      const consumer=new SourceMapConsumer(indexedMap(6));let inner=consumer,reads=0
      for(let i=0;i<6;i++)inner=inner._sections[0].consumer
      const sources=inner.sources
      Object.defineProperty(inner,'sources',{get(){reads++;return sources}})
      assert.deepEqual(consumer.sources,['fixture.js'])
      assert.ok(reads<=14,`six tiny nested sections caused ${reads} innermost reads, budget14`);return
    }
    case 'source-map-offset-negative':assert.throws(()=>new SourceMapConsumer(indexedMap(1,-1)),/offset/i);return
    case 'source-map-offset-fraction':assert.throws(()=>new SourceMapConsumer(indexedMap(1,1.5)),/offset/i);return
    case 'source-map-offset-huge':assert.throws(()=>new SourceMapConsumer(indexedMap(1,10_000_001)),/offset/i);return
    case 'source-map-offset-nested':assert.throws(()=>new SourceMapConsumer(indexedMap(3,5_000_000)),/offset/i);return
    case 'source-map-normal': {
      const consumer=new SourceMapConsumer(indexedMap(2,2))
      assert.deepEqual(consumer.sources,['fixture.js'])
      // Query within the mapped segment. The old library's separate exact-boundary column0
      // lookup issue is recorded in the contract, not recast as this advisory or locked in.
      assert.equal(consumer.originalPositionFor({line:5,column:1}).source,'fixture.js');return
    }
    case 'app-client-normal':
    case 'app-client-session':
    case 'app-client-cancel-timeout':return appClientChecks(kind)
    default:throw new Error(`unknown owned fixture ${kind}`)
  }
}
await new Promise((resolve,reject)=>process.send({type:'entered-work',kind},error=>error?reject(error):resolve()))
try {
  await run()
  await new Promise((resolve,reject)=>process.send({type:'result',ok:true,kind},error=>error?reject(error):resolve()))
} catch(error) {
  process.exitCode=1
  await new Promise(resolve=>process.send({type:'result',ok:false,kind,error:String(error),stack:error.stack},resolve))
} finally {process.disconnect()}
