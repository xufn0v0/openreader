import assert from 'node:assert/strict'
import {fork} from 'node:child_process'
import {fileURLToPath} from 'node:url'
import test from 'node:test'

const worker=fileURLToPath(new URL('./fixtures/dependencyAdvisoryWorker.mjs',import.meta.url))
const cases=['axios-auth-inherited','axios-auth-own','axios-depth','axios-form-normal','vue-cr-attr','vue-normal',
  'form-data-field','form-data-filename','form-data-normal','postcss-unbased-map','postcss-normal',
  'nanoid-zero','nanoid-negative','nanoid-normal','source-map-getter','source-map-offset-negative',
  'source-map-offset-fraction','source-map-offset-huge','source-map-offset-nested','source-map-normal',
  'app-client-normal','app-client-session','app-client-cancel-timeout']

// No external URL/profile/input mode. Children own their fixture objects/files/loopback server.
async function ownedCase(kind,{workMs=3000,interruptOnEntered=false}={}) {
  const child=fork(worker,[kind],{execArgv:['--max-old-space-size=96'],stdio:['ignore','pipe','pipe','ipc']})
  let entered=false,result,lifecycleError,stderr='',bytes=0,timer
  function stop(message) {
    lifecycleError ||= message
    if(child.exitCode===null&&child.signalCode===null)child.kill('SIGKILL')
  }
  const interrupt=()=>stop('owned fixture interrupted')
  process.once('SIGINT',interrupt);process.once('SIGTERM',interrupt)
  const arm=(ms,label)=>{clearTimeout(timer);timer=setTimeout(()=>stop(label),ms)}
  arm(5000,'fixture startup deadline')
  child.on('error',error=>{lifecycleError ||= String(error)})
  child.on('message',message=>{
    if(message?.kind!==kind)return stop('unexpected fixture identity')
    if(message.type==='entered-work') {
      if(entered)return stop('duplicate entered-work')
      entered=true;arm(workMs,'actual fixture work deadline')
      if(interruptOnEntered)process.emit('SIGTERM')
    } else if(message.type==='result')result=message
    else stop('unknown fixture message')
  })
  for(const [stream,isError] of [[child.stdout,false],[child.stderr,true]])stream.on('data',chunk=>{
    bytes+=chunk.length
    if(bytes>65536)return stop('fixture output budget')
    if(isError)stderr+=chunk
  })
  const terminal=await new Promise(resolve=>child.once('close',(code,signal)=>resolve({code,signal})))
  clearTimeout(timer)
  process.removeListener('SIGINT',interrupt);process.removeListener('SIGTERM',interrupt)
  return {entered,result,lifecycleError,stderr,...terminal}
}
for(const kind of cases)test(`dependency advisory actual worker: ${kind}`,{timeout:12000},async t=>{
  const outcome=await ownedCase(kind)
  assert.equal(outcome.lifecycleError,undefined,`lifecycle failure (not a behavior verdict): ${JSON.stringify(outcome)}`)
  assert.equal(outcome.entered,true,`fixture did not enter actual work: ${JSON.stringify(outcome)}`)
  assert.ok(outcome.result,`no terminal behavior verdict: ${JSON.stringify(outcome)}`)
  assert.equal(outcome.signal,null)
  assert.equal(outcome.result.ok,true,`${kind}: ${outcome.result.error}\n${outcome.result.stack||''}`)
  assert.equal(outcome.code,0)
  if(outcome.stderr)t.diagnostic(`owned fixture stderr: ${outcome.stderr.trim()}`)
})
test('owned harness actually kills and closes an over-budget worker',{timeout:12000},async()=>{
  const out=await ownedCase('harness-loop',{workMs:50})
  assert.equal(out.entered,true);assert.equal(out.lifecycleError,'actual fixture work deadline')
  assert.equal(out.signal,'SIGKILL');assert.equal(out.result,undefined)
})
test('owned harness actually reaps worker on caller interruption',{timeout:12000},async()=>{
  const out=await ownedCase('harness-loop',{interruptOnEntered:true})
  assert.equal(out.entered,true);assert.equal(out.lifecycleError,'owned fixture interrupted')
  assert.equal(out.signal,'SIGKILL');assert.equal(out.result,undefined)
})
test('owned harness rejects excess output even if worker reports success',{timeout:12000},async()=>{
  const out=await ownedCase('harness-output')
  assert.equal(out.entered,true);assert.equal(out.lifecycleError,'fixture output budget')
  assert.ok(out.code!==null||out.signal!==null,'must have awaited terminal close')
})
