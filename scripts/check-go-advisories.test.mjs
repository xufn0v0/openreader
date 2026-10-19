import assert from 'node:assert/strict'
import test from 'node:test'
import {readFile} from 'node:fs/promises'
import {evaluateSourceScan, parseMessages, runTool} from './check-go-advisories.mjs'

const now = Date.parse('2026-10-16T03:00:00Z')
function fixture() {
  const packages = ['openreader/backend', 'golang.org/x/crypto/bcrypt']
  return {
    evidence: {goVersion:'go1.26.9', goos:'darwin', goarch:'arm64', cgoEnabled:'1',
      cryptoVersion:'v0.57.0', graphs:{host:packages, 'linux/amd64':packages, 'linux/arm64':packages}},
    messages: [
      {config:{protocol_version:'v1.0.0', scanner_name:'govulncheck', scanner_version:'v1.9.0',
        db:'https://vuln.go.dev', db_last_modified:'2026-10-08T22:31:09Z', go_version:'go1.26.9',
        scan_level:'symbol', scan_mode:'source'}},
      {SBOM:{go_version:'go1.26.9', roots:['openreader/backend'], modules:[
        {path:'openreader/backend'}, {path:'golang.org/x/crypto', version:'v0.57.0'}]}},
      {osv:{id:'GO-2026-5932', affected:[{package:{ecosystem:'Go', name:'golang.org/x/crypto'},
        ranges:[{type:'SEMVER',events:[{introduced:'0'}]}], ecosystem_specific:{imports:[
          {path:'golang.org/x/crypto/openpgp'}, {path:'golang.org/x/crypto/openpgp/packet'}]}}]}},
      {finding:{osv:'GO-2026-5932', trace:[{module:'golang.org/x/crypto',version:'v0.57.0'}]}},
    ],
  }
}
function check(change = () => {}) {
  const f = fixture()
  change(f)
  return evaluateSourceScan(f.messages.map(x=>JSON.stringify(x)).join('\n'),f.evidence,{now})
}
test('stream parser handles nested objects and escaped braces in strings', () => {
  const objects = [{progress:{message:'a } { " \\ b'}}, {osv:{x:[{y:1}]}}]
  assert.deepEqual(parseMessages(objects.map(x=>JSON.stringify(x,null,2)).join('\n')),objects)
})
for (const raw of ['', '  ', '{}junk', '{"config":', '[]', 'null', '{"a":1} {']) {
  test(`invalid/incomplete stream fails closed: ${JSON.stringify(raw)}`,()=>assert.throws(()=>parseMessages(raw)))
}
test('only precise absent OpenPGP module finding is not-applicable, not clean',()=>{
  const result=check()
  assert.equal(result.status,'pass-with-not-applicable')
  assert.deepEqual(result.notApplicable,['GO-2026-5932'])
  assert.equal(result.findings,1)
  assert.equal(result.goos,'darwin')
  assert.ok(result.limitations.includes('C/OS/Node/frontend not scanned'))
})
test('no findings is valid, OSV record alone is not a finding',()=>{
  const result=check(f=>f.messages.pop())
  assert.equal(result.status,'pass')
  assert.equal(result.findings,0)
})
test('official repeated identical OSV is idempotent',()=>{
  assert.equal(check(f=>f.messages.push(structuredClone(f.messages[2]))).advisories,1)
})
test('conflicting repeated OSV fails closed',()=>assert.throws(()=>check(f=>{
  const conflict=structuredClone(f.messages[2])
  conflict.osv.summary='different metadata'
  f.messages.push(conflict)
})))
const invalid = {
  'missing config': f=>f.messages.shift(),
  'duplicate config': f=>f.messages.push(f.messages[0]),
  'duplicate SBOM': f=>f.messages.push(f.messages[1]),
  'missing SBOM': f=>f.messages.splice(1,1),
  'empty roots': f=>f.messages[1].SBOM.roots=[],
  'unknown protocol': f=>f.messages[0].config.protocol_version='v2',
  'wrong scanner': f=>f.messages[0].config.scanner_version='v1.0.0',
  'foreign DB': f=>f.messages[0].config.db='https://example.invalid',
  'stale DB': f=>f.messages[0].config.db_last_modified='2026-09-01T00:00:00Z',
  'future DB': f=>f.messages[0].config.db_last_modified='2026-10-18T00:00:00Z',
  'invalid DB date': f=>f.messages[0].config.db_last_modified='never',
  'wrong Go runtime': f=>f.messages[0].config.go_version='go1.26.5',
  'wrong SBOM runtime': f=>f.messages[1].SBOM.go_version='go1.24.13',
  'binary precision cannot masquerade as source': f=>f.messages[0].config.scan_mode='binary',
  'module precision cannot masquerade as symbol': f=>f.messages[0].config.scan_level='module',
  'unknown message': f=>f.messages.push({error:{message:'incomplete'}}),
  'ambiguous message': f=>f.messages.push({progress:{message:'x'},finding:{}}),
  'unknown advisory': f=>f.messages[3].finding.osv='GO-2099-9999',
  'empty trace': f=>f.messages[3].finding.trace=[],
  'version mismatch': f=>f.messages[3].finding.trace[0].version='v0.1.0',
  'missing version': f=>delete f.messages[3].finding.trace[0].version,
  'SBOM version mismatch': f=>f.messages[1].SBOM.modules[1].version='v0.1.0',
  'different module': f=>f.messages[3].finding.trace[0].module='other/module',
  'package finding': f=>f.messages[3].finding.trace[0].package='golang.org/x/crypto/openpgp',
  'symbol finding': f=>f.messages[3].finding.trace[0].function='ReadMessage',
  'missing affected imports': f=>f.messages[2].osv.affected[0].ecosystem_specific.imports=[],
  'new affected non-OpenPGP package': f=>f.messages[2].osv.affected[0].ecosystem_specific.imports.push({path:'golang.org/x/crypto/bcrypt'}),
  'unexpected affected module': f=>f.messages[2].osv.affected[0].package.name='other/module',
  'new fixed version requires review': f=>f.messages[2].osv.affected[0].ranges[0].events.push({fixed:'0.58.0'}),
  'empty graph': f=>f.evidence.graphs.host=[],
  'missing target graph': f=>delete f.evidence.graphs['linux/amd64'],
  'missing main': f=>f.evidence.graphs.host=['golang.org/x/crypto/bcrypt'],
  'removed bcrypt': f=>f.evidence.graphs.host=['openreader/backend'],
  'CGO evidence absent': f=>f.evidence.cgoEnabled='0',
}
for(const [name,change] of Object.entries(invalid)) test(name,()=>assert.throws(()=>check(change)))
for(const graph of ['host','linux/amd64','linux/arm64']) {
  for(const path of ['golang.org/x/crypto/openpgp','golang.org/x/crypto/openpgp/packet']) {
    test(`${graph} imports ${path}: exemption revoked`,()=>assert.throws(()=>check(f=>{
      f.evidence.graphs[graph]=[...f.evidence.graphs[graph],path]
    })))
  }
}
test('exception expires and requires explicit review',()=>{
  const f=fixture()
  f.messages[0].config.db_last_modified='2026-11-15T00:00:00Z'
  assert.throws(()=>evaluateSourceScan(f.messages.map(x=>JSON.stringify(x)).join('\n'),f.evidence,
    {now:Date.parse('2026-11-16T00:00:00Z')}))
})
test('owned scanner process must finish successfully',async()=>{
  assert.equal(await runTool(process.execPath,['-e','process.stdout.write("ok")']), 'ok')
  await assert.rejects(runTool(process.execPath,['-e','process.stdout.write("{}");process.exit(7)']))
})
test('owned tool optionally preserves successful stderr and real terminal status',async()=>{
  const result=await runTool(process.execPath,['-e','process.stdout.write("ok");process.stderr.write("owned warning")'],{captureOutput:true})
  assert.deepEqual(result,{stdout:'ok',stderr:'owned warning',exitCode:0,signal:null})
})
test('timeout kills and reaps owned scanner process group',async()=>{
  await assert.rejects(runTool(process.execPath,['-e',
    'require("child_process").spawn(process.execPath,["-e","setInterval(()=>{},1000)"],{stdio:"inherit"});setInterval(()=>{},1000)'],
  {timeout:150}),/timeout/)
})
test('caller interruption kills and reaps owned scanner before rejecting',async()=>{
  const pending=runTool(process.execPath,['-e','setInterval(()=>{},1000)'],{timeout:250})
  const timer=setTimeout(()=>process.emit('SIGTERM'),50)
  try {await assert.rejects(pending,/interrupted/)} finally {clearTimeout(timer)}
})
test('output limit fails rather than truncating to apparent clean result',async()=>{
  await assert.rejects(runTool(process.execPath,['-e','process.stdout.write("x".repeat(100000))'],{maxBytes:100}),/output/)
})
test('tool stream preserves UTF-8 split across chunks',async()=>{
  const result=await runTool(process.execPath,['-e',
    'const b=Buffer.from("正文");process.stdout.write(b.subarray(0,1));setTimeout(()=>process.stdout.write(b.subarray(1)),50)'])
  assert.equal(result,'正文')
})
test('release workflow triggers for gate changes and runs both checks before native build/publish',async()=>{
  const source=await readFile(new URL('../.github/workflows/docker-publish.yml',import.meta.url),'utf8')
  for(const path of ['scripts/check-go-advisories.mjs','scripts/check-go-advisories.test.mjs']) {
    assert.ok(source.slice(0,source.indexOf('workflow_dispatch:')).includes(`- ${path}`))
  }
  const tests=source.indexOf('run: node --test scripts/check-go-advisories.test.mjs')
  const scan=source.indexOf('run: node scripts/check-go-advisories.mjs')
  const build=source.indexOf('- name: Build native validation image')
  assert.ok(tests>0 && scan>tests && build>scan)
  assert.ok(!source.slice(tests,build).includes('continue-on-error'))
})
