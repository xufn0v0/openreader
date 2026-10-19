import assert from 'node:assert/strict'
import test from 'node:test'
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {runTool} from './check-go-advisories.mjs'
import {evaluateNpmAudit, npmAuditArgs} from './check-frontend-advisories.mjs'

function fixture() {
  const manifest={name:'openreader-frontend',version:'0.1.0',dependencies:{axios:'^1.20.0'},devDependencies:{postcss:'^8.5.23'}}
  const lock={name:manifest.name,version:manifest.version,lockfileVersion:3,packages:{
    '':structuredClone(manifest),'node_modules/axios':{version:'1.20.0'},'node_modules/postcss':{version:'8.5.23',dev:true}}}
  return {report:{auditReportVersion:2,vulnerabilities:{},metadata:{
    vulnerabilities:{info:0,low:0,moderate:0,high:0,critical:0,total:0},
    dependencies:{prod:1,dev:1,optional:0,peer:0,peerOptional:0,total:2}}},
    evidence:{manifest,lock,lockHashBefore:'a'.repeat(64),lockHashAfter:'a'.repeat(64),
      manifestHashBefore:'b'.repeat(64),manifestHashAfter:'b'.repeat(64),exitCode:0,
      nodeVersion:'v22.23.3',npmVersion:'10.9.6',registry:'https://registry.npmjs.org/',
      includes:['prod','dev','optional','peer'],stderr:''}}
}
function check(change=()=>{}) {
  const f=fixture();change(f)
  return evaluateNpmAudit(JSON.stringify(f.report),f.evidence)
}
test('valid complete zero-finding report records exact scope rather than whole-image clean',()=>{
  const result=check()
  assert.equal(result.status,'pass');assert.equal(result.findings,0);assert.equal(result.lockPackages,2)
  assert.equal(result.scope,'full-lock+prod+dev+optional+peer')
  assert.equal(result.lockHash,'a'.repeat(64))
  assert.ok(result.limitations.includes('Go/C/OS/runtime reachability not scanned'))
})
test('internal command explicitly includes all dependency classes and official registry',()=>{
  for(const arg of ['audit','--json','--package-lock-only','--include=prod','--include=dev',
    '--include=optional','--include=peer','--registry=https://registry.npmjs.org/'])assert.ok(npmAuditArgs.includes(arg))
  assert.ok(!npmAuditArgs.some(arg=>arg.startsWith('--omit')))
})
for(const raw of ['', '  ', '{}', '[]','null','{"auditReportVersion":2','{}garbage',
  '{"error":{"code":"ENOAUDIT","summary":"registry unavailable"}}']) {
  test(`invalid/empty/registry-error JSON fails closed: ${JSON.stringify(raw)}`,()=>assert.throws(()=>evaluateNpmAudit(raw,fixture().evidence)))
}
const invalid={
  'tool exit failure even with apparent zero findings':f=>f.evidence.exitCode=1,
  'unexpected successful stderr requires review':f=>f.evidence.stderr='npm warn unexpected scanner condition',
  'unknown audit schema':f=>f.report.auditReportVersion=3,
  'missing vulnerability object':f=>delete f.report.vulnerabilities,
  'array instead of vulnerability object':f=>f.report.vulnerabilities=[],
  'missing metadata':f=>delete f.report.metadata,
  'metadata claims unmatched findings':f=>f.report.metadata.vulnerabilities.high=f.report.metadata.vulnerabilities.total=1,
  'non-integral count':f=>f.report.metadata.vulnerabilities.low=0.5,
  'negative count':f=>f.report.metadata.vulnerabilities.low=-1,
  'unknown severity field':f=>f.report.metadata.vulnerabilities.extreme=0,
  'missing dependency totals':f=>delete f.report.metadata.dependencies,
  'incomplete graph scope':f=>f.report.metadata.dependencies.total=1,
  'no dev evidence':f=>f.report.metadata.dependencies.dev=0,
  'omitted dev command scope':f=>f.evidence.includes=['prod','optional','peer'],
  'omitted optional command scope':f=>f.evidence.includes=['prod','dev','peer'],
  'omitted peer command scope':f=>f.evidence.includes=['prod','dev','optional'],
  'lock changed while scan ran':f=>f.evidence.lockHashAfter='c'.repeat(64),
  'manifest changed while scan ran':f=>f.evidence.manifestHashAfter='c'.repeat(64),
  'invalid hash':f=>f.evidence.lockHashBefore=f.evidence.lockHashAfter='unknown',
  'manifest lock mismatch':f=>f.evidence.lock.packages[''].dependencies.axios='^1.7.7',
  'unexpected package':f=>f.evidence.lock.name='different-project',
  'unsupported lock schema':f=>f.evidence.lock.lockfileVersion=1,
  'lock root name mismatch':f=>f.evidence.lock.packages[''].name='other',
  'lock root version mismatch':f=>f.evidence.lock.packages[''].version='0.0.0',
  'lock package missing version':f=>delete f.evidence.lock.packages['node_modules/axios'].version,
  'broad wildcard override':f=>f.evidence.manifest.overrides={'*':'1.0.0'},
  'override contradicts lock':f=>f.evidence.manifest.overrides={postcss:'8.5.22'},
  'wrong registry':f=>f.evidence.registry='https://example.invalid/',
  'missing Node identity':f=>f.evidence.nodeVersion='',
  'missing npm identity':f=>f.evidence.npmVersion='',
}
for(const [name,change] of Object.entries(invalid))test(name,()=>assert.throws(()=>check(change)))
for(const severity of ['info','low','moderate','high','critical']) {
  test(`${severity} finding cannot pass or be hidden by audit-level`,()=>assert.throws(()=>check(f=>{
    f.report.vulnerabilities.axios={name:'axios',severity,via:[{source:123,title:'owned fixture'}]}
    f.report.metadata.vulnerabilities[severity]=1;f.report.metadata.vulnerabilities.total=1
  })))
}
test('vulnerability object contradicting zero counts fails closed',()=>assert.throws(()=>check(f=>{
  f.report.vulnerabilities.axios={name:'axios',severity:'high',via:['owned fixture']}
})))
test('concrete exact same-major leaf constraints must match actual locked nodes',()=>{
  assert.equal(check(f=>f.evidence.manifest.overrides={postcss:'8.5.23'}).status,'pass')
})
test('workflow triggers for npm gate files and checks them before native build',async()=>{
  const source=await readFile(new URL('../.github/workflows/docker-publish.yml',import.meta.url),'utf8')
  for(const path of ['scripts/check-frontend-advisories.mjs','scripts/check-frontend-advisories.test.mjs']) {
    assert.ok(source.slice(0,source.indexOf('workflow_dispatch:')).includes(`- ${path}`))
  }
  const tests=source.indexOf('run: node --test scripts/check-frontend-advisories.test.mjs')
  const scan=source.indexOf('run: node scripts/check-frontend-advisories.mjs')
  const build=source.indexOf('- name: Build native validation image')
  assert.ok(tests>0&&scan>tests&&build>scan)
  assert.ok(!source.slice(tests,build).includes('continue-on-error'))
})
test('CLI rejects externally supplied results before starting npm',async()=>{
  const script=fileURLToPath(new URL('./check-frontend-advisories.mjs',import.meta.url))
  await assert.rejects(runTool(process.execPath,[script,'owned-not-a-scan.json']),
    error=>error.exitCode===1&&error.stderr.includes('external results or bypass flags'))
})
