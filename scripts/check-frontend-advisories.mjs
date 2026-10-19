#!/usr/bin/env node
import assert from 'node:assert/strict'
import {createHash} from 'node:crypto'
import {mkdtemp,readFile,writeFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {dirname,join,resolve} from 'node:path'
import {fileURLToPath} from 'node:url'
import {runTool} from './check-go-advisories.mjs'

const registry='https://registry.npmjs.org/'
const includes=['prod','dev','optional','peer']
export const npmAuditArgs=Object.freeze(['audit','--json','--package-lock-only','--audit-level=low',
  ...includes.map(kind=>`--include=${kind}`),`--registry=${registry}`])
const levels=['info','low','moderate','high','critical']
const object=value=>value!==null&&typeof value==='object'&&!Array.isArray(value)
const hash=bytes=>createHash('sha256').update(bytes).digest('hex')
const natural=(value,label)=>assert.ok(Number.isSafeInteger(value)&&value>=0,`invalid ${label}`)
const exactVersion=/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/

export function evaluateNpmAudit(raw,evidence) {
  assert.equal(evidence.exitCode,0,'npm audit did not terminate successfully')
  assert.equal(evidence.stderr,'','unexpected npm audit stderr requires review')
  assert.equal(evidence.registry,registry)
  assert.deepEqual(evidence.includes,includes,'all dependency classes must be included')
  assert.match(evidence.nodeVersion,/^v\d+\.\d+\.\d+$/)
  assert.match(evidence.npmVersion,/^\d+\.\d+\.\d+$/)
  for(const key of ['lock','manifest']) {
    assert.match(evidence[`${key}HashBefore`],/^[a-f0-9]{64}$/)
    assert.equal(evidence[`${key}HashAfter`],evidence[`${key}HashBefore`],`${key} changed during scan`)
  }
  const {manifest,lock}=evidence
  assert.ok(object(manifest)&&object(lock))
  assert.equal(manifest.name,'openreader-frontend');assert.equal(lock.name,manifest.name)
  assert.equal(lock.version,manifest.version);assert.equal(lock.lockfileVersion,3)
  assert.ok(object(lock.packages)&&object(lock.packages['']),'complete lock graph required')
  assert.equal(lock.packages[''].name,manifest.name,'lock root name mismatch')
  assert.equal(lock.packages[''].version,manifest.version,'lock root version mismatch')
  for(const key of ['dependencies','devDependencies']) {
    assert.ok(object(manifest[key])&&Object.keys(manifest[key]).length>0,`manifest ${key} missing`)
    assert.deepEqual(lock.packages[''][key],manifest[key],`lock root ${key} mismatch`)
  }
  const paths=Object.keys(lock.packages).filter(path=>path!=='')
  assert.ok(paths.length>0&&paths.every(path=>path.startsWith('node_modules/')),'unexpected lock structure')
  for(const path of paths) {
    assert.ok(object(lock.packages[path]),'invalid lock package')
    assert.match(lock.packages[path].version,exactVersion,'lock package version missing or unsupported')
  }
  if(manifest.overrides!==undefined) {
    assert.ok(object(manifest.overrides),'only concrete leaf constraints are supported')
    for(const [name,version] of Object.entries(manifest.overrides)) {
      assert.match(name,/^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/,'broad override is not permitted')
      assert.match(version,exactVersion,'only exact leaf constraints are supported')
      const selected=paths.filter(path=>path===`node_modules/${name}`||path.endsWith(`/node_modules/${name}`))
      assert.ok(selected.length>0&&selected.every(path=>lock.packages[path].version===version),'override contradicts lock')
    }
  }
  assert.equal(typeof raw,'string')
  const report=JSON.parse(raw)
  assert.ok(object(report)&&!report.error,'registry/tool error is not a clean report')
  assert.equal(report.auditReportVersion,2,'unsupported audit schema')
  assert.ok(object(report.vulnerabilities)&&object(report.metadata))
  const counts=report.metadata.vulnerabilities,dependencies=report.metadata.dependencies
  assert.ok(object(counts)&&object(dependencies),'audit metadata missing')
  assert.deepEqual(Object.keys(counts).sort(),[...levels,'total'].sort(),'unknown severity metadata')
  for(const key of [...levels,'total'])natural(counts[key],`${key} finding count`)
  assert.equal(counts.total,levels.reduce((n,key)=>n+counts[key],0),'inconsistent severity total')
  for(const key of ['prod','dev','optional','peer','peerOptional','total'])natural(dependencies[key],`${key} dependency count`)
  assert.ok(dependencies.prod>0&&dependencies.dev>0,'prod/dev scope evidence missing')
  assert.equal(dependencies.total,paths.length,'audit does not cover the full lock graph')
  const derived=Object.fromEntries(levels.map(level=>[level,0]))
  for(const [name,entry] of Object.entries(report.vulnerabilities)) {
    assert.ok(object(entry)&&entry.name===name,'invalid vulnerability entry')
    assert.ok(levels.includes(entry.severity),'unknown vulnerability severity')
    derived[entry.severity]++
  }
  for(const level of levels)assert.equal(derived[level],counts[level],`inconsistent ${level} entries`)
  assert.equal(counts.total,0,'npm-reported findings block release; no ignores')
  return {status:'pass',scope:'full-lock+prod+dev+optional+peer',findings:counts.total,
    lockPackages:paths.length,lockHash:evidence.lockHashBefore,manifestHash:evidence.manifestHashBefore,
    registry,nodeVersion:evidence.nodeVersion,npmVersion:evidence.npmVersion,observedAt:new Date().toISOString(),
    limitations:['Go/C/OS/runtime reachability not scanned','npm audit does not expose a Go-style database timestamp']}
}

async function main() {
  assert.equal(process.argv.length,2,'external results or bypass flags are not accepted')
  const frontend=join(dirname(fileURLToPath(import.meta.url)),'..','frontend')
  const output=await mkdtemp(join(tmpdir(),'openreader-npm-advisories-'))
  console.log(`Frontend advisory evidence: ${output}`)
  try {
    const manifestBefore=await readFile(join(frontend,'package.json'))
    const lockBefore=await readFile(join(frontend,'package-lock.json'))
    const npmVersion=(await runTool('npm',['--version'],{cwd:frontend})).trim()
    let scan
    try {scan=await runTool('npm',npmAuditArgs,{cwd:frontend,captureOutput:true})}
    catch(error) {
      await writeFile(join(output,'audit.json'),error.stdout||'')
      await writeFile(join(output,'audit.stderr'),error.stderr||'')
      await writeFile(join(output,'terminal.json'),JSON.stringify({exitCode:error.exitCode??null,signal:error.signal??null}))
      throw error
    }
    await writeFile(join(output,'audit.json'),scan.stdout)
    await writeFile(join(output,'audit.stderr'),scan.stderr)
    await writeFile(join(output,'terminal.json'),JSON.stringify({exitCode:scan.exitCode,signal:scan.signal}))
    const evidence={manifest:JSON.parse(manifestBefore),lock:JSON.parse(lockBefore),
      lockHashBefore:hash(lockBefore),lockHashAfter:hash(await readFile(join(frontend,'package-lock.json'))),
      manifestHashBefore:hash(manifestBefore),manifestHashAfter:hash(await readFile(join(frontend,'package.json'))),
      nodeVersion:process.version,npmVersion,registry,includes,exitCode:scan.exitCode,stderr:scan.stderr}
    await writeFile(join(output,'scope.json'),JSON.stringify({registry,includes,nodeVersion:process.version,npmVersion,
      lockHashBefore:evidence.lockHashBefore,lockHashAfter:evidence.lockHashAfter,
      manifestHashBefore:evidence.manifestHashBefore,manifestHashAfter:evidence.manifestHashAfter},null,2))
    const summary=evaluateNpmAudit(scan.stdout,evidence)
    await writeFile(join(output,'summary.json'),JSON.stringify(summary,null,2))
    console.log(JSON.stringify(summary,null,2))
  } catch(error) {
    await writeFile(join(output,'failure.txt'),String(error))
    throw error
  }
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  main().catch(error=>{console.error(`Frontend advisory gate failed: ${error.message}`);process.exitCode=1})
}
