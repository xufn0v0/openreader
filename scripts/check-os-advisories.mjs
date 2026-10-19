#!/usr/bin/env node
import assert from 'node:assert/strict'
import {createHash} from 'node:crypto'
import {mkdtemp, readFile, rm, stat, writeFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {dirname, join, resolve} from 'node:path'
import {fileURLToPath} from 'node:url'
import {gunzipSync} from 'node:zlib'
import {runTool} from './check-go-advisories.mjs'

const day = 86_400_000
const secdbRoot = 'https://secdb.alpinelinux.org/v3.23/'
const releasePlatforms = Object.freeze({
  'linux/amd64': {architecture:'amd64', apkArch:'x86_64'},
  'linux/arm64': {architecture:'arm64', apkArch:'aarch64', variant:'v8'},
})
// Field letters apk-tools 3.0.8 writes for the pinned base. A new letter changes the schema: review first.
const databaseFields = new Set([...'CPVASITULomtcDpikrqFMRaZ'])
const namePattern = /^[A-Za-z0-9][A-Za-z0-9._+-]*$/
const versionPattern = /^[0-9A-Za-z._+~-]+$/
const checksumPattern = /^Q1[A-Za-z0-9+/]{27}=$/
const apkChecksum = data => `Q1${createHash('sha1').update(data).digest('base64')}`
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')

function normalizePath(raw) {
  assert.equal(typeof raw, 'string')
  const path = raw.replace(/^(\.\/)+/, '').replace(/\/+$/, '')
  assert.ok(path && !path.startsWith('/') && !path.includes('\0')
    && path.split('/').every(part => part && part !== '.' && part !== '..'), `unsafe archive path ${JSON.stringify(raw)}`)
  return path
}

function parsePax(data) {
  const records = {}
  for (let offset = 0; offset < data.length;) {
    const space = data.indexOf(0x20, offset)
    const digits = space > offset ? data.subarray(offset, space).toString('latin1') : ''
    assert.match(digits, /^[1-9]\d{0,9}$/, 'malformed pax record')
    const end = offset + Number(digits)
    assert.ok(end <= data.length && end > space + 1 && data[end - 1] === 0x0a, 'malformed pax record')
    const body = data.subarray(space + 1, end - 1).toString('utf8')
    const equals = body.indexOf('=')
    assert.ok(equals > 0, 'malformed pax record')
    records[body.slice(0, equals)] = body.slice(equals + 1)
    offset = end
  }
  return records
}

// Fail-closed POSIX/GNU tar reader: header checksums, sizes and the end-of-archive marker must be exact,
// unknown entry types are rejected, and file data stays covered by the image layer digests.
export function readTar(bytes) {
  assert.ok(Buffer.isBuffer(bytes) && bytes.length >= 1024 && bytes.length % 512 === 0, 'truncated or unaligned tar archive')
  const entries = []
  let pax = null, longName = null, longLink = null
  for (let offset = 0; ;) {
    assert.ok(offset + 512 <= bytes.length, 'missing tar end-of-archive marker')
    const header = bytes.subarray(offset, offset + 512)
    if (header.every(byte => byte === 0)) {
      assert.ok(offset + 1024 <= bytes.length && bytes.subarray(offset).every(byte => byte === 0), 'incomplete tar end-of-archive marker')
      assert.ok(pax === null && longName === null && longLink === null, 'dangling tar extension header')
      return entries
    }
    const field = (start, length) => header.subarray(start, start + length).toString('utf8').replace(/\0.*$/s, '')
    const number = (start, length) => {
      const value = field(start, length).trim()
      assert.match(value, /^[0-7]+$/, 'invalid tar numeric field')
      return Number.parseInt(value, 8)
    }
    let sum = 0
    for (let i = 0; i < 512; i++) sum += i >= 148 && i < 156 ? 0x20 : header[i]
    assert.equal(number(148, 8), sum, 'tar header checksum mismatch')
    const magic = header.subarray(257, 265).toString('latin1')
    const posix = magic.startsWith('ustar\0'), gnu = magic === 'ustar  \0'
    assert.ok(posix || gnu, 'unsupported tar format')
    const type = header[156] === 0 ? '0' : String.fromCharCode(header[156])
    const size = pax?.size === undefined ? number(124, 12) : Number(pax.size)
    assert.ok(Number.isSafeInteger(size) && size >= 0, 'invalid tar entry size')
    const start = offset + 512
    assert.ok(start + size <= bytes.length, 'truncated tar entry')
    const data = bytes.subarray(start, start + size)
    offset = start + Math.ceil(size / 512) * 512
    if (type === 'x') {assert.equal(pax, null, 'repeated pax header'); pax = parsePax(data); continue}
    if (type === 'g') {parsePax(data); continue}
    if (type === 'L') {assert.equal(longName, null, 'repeated long name'); longName = data.toString('utf8').replace(/\0+$/, ''); continue}
    if (type === 'K') {assert.equal(longLink, null, 'repeated long link'); longLink = data.toString('utf8').replace(/\0+$/, ''); continue}
    assert.ok('01234567'.includes(type), `unsupported tar entry type ${JSON.stringify(type)}`)
    const prefix = posix ? field(345, 155) : ''
    const raw = pax?.path ?? longName ?? (prefix ? `${prefix}/${field(0, 100)}` : field(0, 100))
    const link = pax?.linkpath ?? longLink ?? field(157, 100)
    pax = longName = longLink = null
    if (type === '5' && !raw.replace(/^(\.\/)+/, '').replace(/\/+$/, '')) continue
    entries.push({path:normalizePath(raw), type, data, link})
  }
}

// Apply image layers like the container runtime: OCI whiteouts and opaque directories hide only lower
// layers, a non-directory replaces any lower subtree, and hardlinks resolve to already-present files.
export function flattenLayers(layers) {
  assert.ok(Array.isArray(layers) && layers.length > 0, 'image has no layers')
  const files = new Map()
  const removeChildren = path => {
    for (const key of [...files.keys()]) if (!path || key.startsWith(`${path}/`)) files.delete(key)
  }
  for (const entries of layers) {
    for (const {path} of entries) {
      const slash = path.lastIndexOf('/'), parent = slash < 0 ? '' : path.slice(0, slash), name = path.slice(slash + 1)
      if (name === '.wh..wh..opq') removeChildren(parent)
      else if (name.startsWith('.wh.')) {
        const hidden = parent ? `${parent}/${name.slice(4)}` : name.slice(4)
        files.delete(hidden)
        removeChildren(hidden)
      }
    }
    for (const entry of entries) {
      if (entry.path.slice(entry.path.lastIndexOf('/') + 1).startsWith('.wh.')) continue
      let value
      if (entry.type === '0' || entry.type === '7') value = {type:'file', checksum:apkChecksum(entry.data), content:entry.data}
      else if (entry.type === '2') {
        assert.ok(entry.link, `empty symlink ${entry.path}`)
        value = {type:'symlink', checksum:apkChecksum(entry.link), link:entry.link}
      } else if (entry.type === '1') {
        const target = files.get(normalizePath(entry.link))
        assert.ok(target?.type === 'file', `dangling hardlink ${entry.path}`)
        value = {...target}
      } else if (entry.type === '5') value = {type:'dir'}
      else value = {type:'special'}
      if (value.type !== 'dir') removeChildren(entry.path)
      files.set(entry.path, value)
    }
  }
  return files
}

// `docker save` archive (OCI layout or legacy manifest.json): exactly one image, its config blob must
// match its digest, and every uncompressed layer must match the config diff_id before it is applied.
export function readImageArchive(bytes) {
  const members = new Map()
  for (const entry of readTar(bytes)) {
    if (entry.type !== '0' && entry.type !== '7') continue
    assert.ok(!members.has(entry.path), `duplicate archive member ${entry.path}`)
    members.set(entry.path, entry.data)
  }
  const member = path => {
    const data = members.get(normalizePath(path))
    assert.ok(data, `missing archive member ${path}`)
    return data
  }
  const manifest = JSON.parse(member('manifest.json').toString('utf8'))
  assert.ok(Array.isArray(manifest) && manifest.length === 1, 'exactly one image manifest required')
  const [{Config:configPath, Layers:layerPaths}] = manifest
  const configBytes = member(configPath)
  const digest = /^blobs\/sha256\/([0-9a-f]{64})$/.exec(normalizePath(configPath))
  assert.ok(digest && sha256(configBytes) === digest[1], 'image config does not match its digest')
  const config = JSON.parse(configBytes.toString('utf8'))
  const diffIds = config?.rootfs?.diff_ids
  assert.ok(config?.rootfs?.type === 'layers' && Array.isArray(layerPaths) && layerPaths.length > 0
    && Array.isArray(diffIds) && diffIds.length === layerPaths.length, 'image layers do not match config rootfs')
  const layers = layerPaths.map((path, index) => {
    let layer = member(path)
    if (layer[0] === 0x1f && layer[1] === 0x8b) layer = gunzipSync(layer, {maxOutputLength:1 << 30})
    else assert.ok(!(layer[0] === 0x28 && layer[1] === 0xb5 && layer[2] === 0x2f && layer[3] === 0xfd), 'unsupported zstd layer')
    assert.equal(`sha256:${sha256(layer)}`, diffIds[index], 'layer does not match its config diff_id')
    return readTar(layer)
  })
  return {config, files:flattenLayers(layers)}
}

export function parseApkDatabase(text, apkArch) {
  assert.equal(typeof text, 'string')
  assert.ok(text.trim() && text.endsWith('\n'), 'empty or truncated APK database')
  const packages = new Map(), owners = new Map()
  for (const block of text.trim().split(/\n{2,}/)) {
    const fields = {}, files = []
    let folder = null, file = null
    for (const line of block.split('\n')) {
      assert.match(line, /^[A-Za-z]:/, 'malformed APK database line')
      const key = line[0], value = line.slice(2)
      assert.ok(databaseFields.has(key), `unknown APK database field ${key}: review required`)
      if (key === 'F') {folder = normalizePath(value); file = null}
      else if (key === 'M') assert.ok(folder !== null && file === null, 'directory mode without directory')
      else if (key === 'R') {
        assert.ok(folder !== null, 'APK file without directory')
        assert.ok(!file || file.checksum, `APK file without checksum ${file?.path}`)
        file = {path:normalizePath(`${folder}/${value}`), checksum:null}
        files.push(file)
      } else if (key === 'a') assert.ok(file && file.checksum === null, 'file mode without file')
      else if (key === 'Z') {
        assert.ok(file && file.checksum === null, 'APK checksum without file')
        assert.match(value, checksumPattern, 'invalid APK file checksum')
        file.checksum = value
      } else {
        assert.ok(!Object.hasOwn(fields, key), `repeated APK field ${key}`)
        fields[key] = value
      }
    }
    for (const key of 'CPVAo') assert.ok(fields[key], `APK record missing ${key}`)
    for (const entry of files) assert.ok(entry.checksum, `APK file without checksum ${entry.path}`)
    assert.match(fields.C, checksumPattern, 'invalid APK package checksum')
    assert.match(fields.P, namePattern, 'invalid APK package name')
    assert.match(fields.o, namePattern, 'invalid APK origin')
    assert.match(fields.V, versionPattern, 'invalid APK version')
    assert.ok([apkArch, 'noarch'].includes(fields.A), `wrong APK architecture ${fields.P}/${fields.A} for ${apkArch}`)
    assert.ok(!packages.has(fields.P), `duplicate APK package ${fields.P}`)
    for (const entry of files) {
      assert.ok(!owners.has(entry.path), `duplicate APK file ${entry.path}`)
      owners.set(entry.path, fields.P)
    }
    packages.set(fields.P, {name:fields.P, version:fields.V, arch:fields.A, origin:fields.o, checksum:fields.C, files})
  }
  return packages
}

function parseOsRelease(text) {
  const values = {}
  for (const line of text.split('\n')) {
    if (!line.trim() || line.startsWith('#')) continue
    const match = /^([A-Z0-9_]+)=(?:"([^"]*)"|([^"\s]*))$/.exec(line)
    assert.ok(match, 'malformed os-release')
    values[match[1]] = match[2] ?? match[3]
  }
  return values
}

export function validateSecdb(name, input, now) {
  const modified = Date.parse(input?.lastModified)
  assert.ok(typeof input?.lastModified === 'string' && Number.isFinite(modified) && now - modified <= 30 * day
    && modified - now <= day, `invalid, stale or future ${name} secdb`)
  const document = input.document
  assert.ok(document && typeof document === 'object' && !Array.isArray(document), `invalid ${name} secdb`)
  assert.equal(document.distroversion, 'v3.23', `${name} secdb distribution changed`)
  assert.equal(document.reponame, name, `${name} secdb repository changed`)
  assert.equal(document.urlprefix, 'https://dl-cdn.alpinelinux.org/alpine', `${name} secdb URL prefix changed`)
  assert.equal(document.apkurl, '{{urlprefix}}/{{distroversion}}/{{reponame}}/{{arch}}/{{pkg.name}}-{{pkg.ver}}.apk', `${name} secdb schema changed`)
  assert.ok(Array.isArray(document.archs) && document.archs.includes('x86_64') && document.archs.includes('aarch64'),
    `${name} secdb architectures changed`)
  assert.ok(Array.isArray(document.packages) && document.packages.length > 0, `empty ${name} secdb`)
  const fixes = new Map(), names = new Set()
  for (const entry of document.packages) {
    const pkg = entry?.pkg
    assert.ok(pkg && typeof pkg.name === 'string' && namePattern.test(pkg.name), `invalid ${name} secdb package`)
    assert.ok(pkg.secfixes && typeof pkg.secfixes === 'object' && !Array.isArray(pkg.secfixes), `invalid secfixes for ${pkg.name}`)
    names.add(pkg.name)
    for (const [fixed, ids] of Object.entries(pkg.secfixes)) {
      assert.match(fixed, versionPattern, `invalid fixed version for ${pkg.name}`)
      assert.ok(Array.isArray(ids) && ids.length > 0 && ids.every(id => typeof id === 'string' && id && id.trim() === id),
        `invalid secfix identifiers for ${pkg.name}`)
      if (!fixes.has(pkg.name)) fixes.set(pkg.name, [])
      fixes.get(pkg.name).push({repository:name, fixed, ids})
    }
  }
  return {name, lastModified:new Date(modified).toISOString(), packages:document.packages.length, names, fixes,
    ...(input.sha256 ? {sha256:input.sha256, url:input.url} : {})}
}

function mergeFixes(databases) {
  const fixes = new Map()
  for (const database of databases) for (const [name, list] of database.fixes) fixes.set(name, [...(fixes.get(name) ?? []), ...list])
  return fixes
}

// Pairs are compared with the native apk-tools of the release image; never semver or lexical order.
export function planComparisons(databases, fixes) {
  const pairs = new Set()
  for (const packages of databases) for (const pkg of packages.values()) {
    for (const fix of fixes.get(pkg.origin) ?? []) pairs.add(`${pkg.version}|${fix.fixed}`)
  }
  return [...pairs].sort()
}

function validateBaseline(baseline) {
  assert.equal(baseline?.schema, 'openreader-os-apk-baseline/v1', 'unknown OS baseline schema')
  assert.match(baseline.baseImage, /^alpine:3\.23\.\d+@sha256:[0-9a-f]{64}$/, 'base image must stay a pinned 3.23 digest')
  assert.ok(baseline.distribution?.id === 'alpine' && /^3\.23\.\d+$/.test(baseline.distribution.versionId)
    && baseline.distribution.secdb === 'v3.23', 'invalid baseline distribution')
  assert.deepEqual(Object.keys(baseline.platforms ?? {}).sort(), Object.keys(releasePlatforms).sort(), 'baseline platforms changed')
  for (const [platform, value] of Object.entries(baseline.platforms)) {
    assert.equal(value.apkArch, releasePlatforms[platform].apkArch, `baseline architecture for ${platform}`)
    assert.ok(Object.keys(value.packages ?? {}).length > 0, `empty baseline for ${platform}`)
    for (const [name, pkg] of Object.entries(value.packages)) {
      assert.ok(namePattern.test(name) && versionPattern.test(pkg.version) && namePattern.test(pkg.origin)
        && checksumPattern.test(pkg.checksum), `invalid baseline package ${platform} ${name}`)
    }
  }
  for (const key of ['world', 'repositories', 'command']) {
    assert.ok(Array.isArray(baseline[key]) && baseline[key].length > 0 && baseline[key].every(item => typeof item === 'string' && item),
      `invalid baseline ${key}`)
  }
  assert.ok(baseline.repositories.every(item => item.startsWith('https://dl-cdn.alpinelinux.org/alpine/v3.23/')), 'unofficial baseline repository')
  for (const [path, value] of Object.entries(baseline.modifiedFiles ?? {})) {
    assert.ok(normalizePath(path) === path && namePattern.test(value.package) && checksumPattern.test(value.packageChecksum)
      && checksumPattern.test(value.checksum) && typeof value.reason === 'string' && value.reason, `invalid reviewed modification ${path}`)
  }
  for (const [origin, reason] of Object.entries(baseline.unlistedOrigins ?? {})) {
    assert.ok(namePattern.test(origin) && typeof reason === 'string' && reason, `invalid unlisted origin review ${origin}`)
  }
  assert.ok(Number.isFinite(Date.parse(baseline.reviewUntil)), 'invalid review date')
}

// One release image against the reviewed baseline: platform/config, distribution, exact package identities,
// every APK-owned file (only pinned official modifications differ), world and repositories.
export function inspectImage(image, baseline) {
  const platform = releasePlatforms[image?.platform]
  assert.ok(platform, `unsupported release platform ${image?.platform}`)
  const expected = baseline.platforms[image.platform].packages, reviewed = Object.keys(baseline.modifiedFiles ?? {}).sort()
  const config = image.config
  assert.equal(config?.os, 'linux', `${image.platform} image OS changed`)
  assert.equal(config.architecture, platform.architecture, `${image.platform} image architecture changed`)
  assert.ok(config.variant === undefined || config.variant === platform.variant, `${image.platform} image variant changed`)
  assert.deepEqual(config.config?.Cmd, baseline.command, `${image.platform} image command changed`)
  const entrypoint = config.config?.Entrypoint
  assert.ok(entrypoint == null || (Array.isArray(entrypoint) && entrypoint.length === 0), `${image.platform} image entrypoint changed`)
  const text = path => {
    const file = image.files?.get(path)
    assert.ok(file?.type === 'file' && file.content, `${image.platform} image missing ${path}`)
    return Buffer.from(file.content).toString('utf8')
  }
  const release = parseOsRelease(text('usr/lib/os-release'))
  assert.ok(release.ID === baseline.distribution.id && release.VERSION_ID === baseline.distribution.versionId,
    `${image.platform} distribution changed: ${release.ID} ${release.VERSION_ID}`)
  const packages = parseApkDatabase(text('lib/apk/db/installed'), platform.apkArch)
  assert.deepEqual([...packages.keys()].sort(), Object.keys(expected).sort(),
    `${image.platform} package identity differs from reviewed baseline: unexpected or missing package`)
  for (const pkg of packages.values()) {
    const want = expected[pkg.name]
    assert.ok(pkg.version === want.version && pkg.origin === want.origin && pkg.checksum === want.checksum,
      `${image.platform} package identity differs from reviewed baseline: ${pkg.name} ${pkg.version} ${pkg.checksum}`
        + ` expected ${want.version} ${want.checksum}`)
  }
  let files = 0
  const modified = []
  for (const pkg of packages.values()) for (const file of pkg.files) {
    const actual = image.files.get(file.path)
    assert.ok(actual && (actual.type === 'file' || actual.type === 'symlink'), `${image.platform} APK file missing or replaced: ${file.path}`)
    const allowed = baseline.modifiedFiles?.[file.path]
    if (allowed) {
      assert.ok(allowed.package === pkg.name && allowed.packageChecksum === file.checksum && allowed.checksum === actual.checksum,
        `${image.platform} reviewed modification no longer matches: ${file.path}`)
      modified.push(file.path)
    } else assert.equal(actual.checksum, file.checksum, `${image.platform} APK file changed after install: ${file.path}`)
    files++
  }
  assert.deepEqual(modified.sort(), reviewed, `${image.platform} reviewed modifications are stale`)
  const lines = value => value.split('\n').map(line => line.trim()).filter(line => line && !line.startsWith('#'))
  assert.deepEqual(lines(text('etc/apk/world')).sort(), [...baseline.world].sort(), `${image.platform} etc/apk/world differs from reviewed baseline`)
  assert.deepEqual(lines(text('etc/apk/repositories')), baseline.repositories, `${image.platform} etc/apk/repositories differs from reviewed baseline`)
  return {platform:image.platform, apkArch:platform.apkArch, packages, files}
}

export function evaluateOsInventory({baseline, images, secdb, comparisons, now = Date.now()}) {
  validateBaseline(baseline)
  assert.ok(Array.isArray(images) && images.length === 2, 'exactly the two release platforms required')
  assert.deepEqual(images.map(image => image?.platform).sort(), Object.keys(releasePlatforms).sort(), 'release platforms changed')
  const reviewed = Object.keys(baseline.modifiedFiles ?? {}).sort()
  const inventories = images.map(image => inspectImage(image, baseline))
  const shape = packages => [...packages.values()].map(pkg => `${pkg.name} ${pkg.version} ${pkg.origin}`).sort()
  assert.deepEqual(shape(inventories[0].packages), shape(inventories[1].packages), 'release platforms install different package versions')

  const databases = ['main', 'community'].map(name => validateSecdb(name, secdb?.[name], now))
  const fixes = mergeFixes(databases)
  const reviewUntil = Date.parse(baseline.reviewUntil)
  const origins = [...new Set(inventories.flatMap(inventory => [...inventory.packages.values()].map(pkg => pkg.origin)))].sort()
  const unknown = []
  for (const origin of origins) {
    const unlisted = Object.hasOwn(baseline.unlistedOrigins ?? {}, origin)
    if (databases.some(database => database.names.has(origin))) {
      assert.ok(!unlisted, `${origin} now has a secdb record: replace its unlisted review`)
      continue
    }
    assert.ok(unlisted, `unreviewed origin without secdb record: ${origin}`)
    assert.ok(now < reviewUntil, `unlisted origin review expired: ${origin}`)
    unknown.push(origin)
  }
  for (const origin of Object.keys(baseline.unlistedOrigins ?? {})) assert.ok(origins.includes(origin), `stale unlisted origin review: ${origin}`)

  const planned = planComparisons(inventories.map(inventory => inventory.packages), fixes)
  assert.ok(comparisons instanceof Map, 'native comparisons missing')
  assert.deepEqual([...comparisons.keys()].sort(), planned, 'native comparison set does not match the planned secfix pairs')
  for (const [pair, result] of comparisons) assert.ok(['<', '=', '>'].includes(result), `invalid native comparison ${pair}: ${result}`)
  const pending = []
  for (const {platform, packages} of inventories) for (const pkg of packages.values()) {
    for (const fix of fixes.get(pkg.origin) ?? []) {
      if (comparisons.get(`${pkg.version}|${fix.fixed}`) === '<') pending.push(`${platform} ${pkg.name} ${pkg.version} < ${fix.fixed} ${fix.ids.join(',')}`)
    }
  }
  assert.equal(pending.length, 0, `pending Alpine secfix blocks release: ${pending.join('; ')}`)
  return {status:unknown.length ? 'pass-with-unknown-origins' : 'pass', baseImage:baseline.baseImage,
    distribution:`${baseline.distribution.id} ${baseline.distribution.versionId}`, buildTimeFixes:baseline.buildTimeFixes,
    platforms:inventories.map(({platform, apkArch, packages, files}) => ({platform, apkArch, packages:packages.size, files})),
    modifiedFiles:reviewed, secdb:databases.map(({names, fixes:_, ...database}) => database), comparisons:planned.length,
    unknownOrigins:unknown, reviewUntil:new Date(reviewUntil).toISOString(),
    limitations:['not a whole-image vulnerability scan: only APK packages against the official v3.23 secdb',
      'origins without a secdb record are unknown, not clean',
      'embedded Go/SQLite C, frontend assets, Node and builder images are covered by other gates or remain open',
      'apk verifies repository signatures at build time; this gate pins exact package identity checksums']}
}

export function parseArguments(argv) {
  assert.equal(argv.length, 2, 'usage: check-os-advisories.mjs openreader-ci:TAG openreader-ci-arm64:TAG')
  const amd64 = /^openreader-ci:([A-Za-z0-9._-]{1,128})$/.exec(argv[0])
  const arm64 = /^openreader-ci-arm64:([A-Za-z0-9._-]{1,128})$/.exec(argv[1])
  assert.ok(amd64 && arm64 && amd64[1] === arm64[1], 'only the two loaded release validation images are accepted')
  return {'linux/amd64':argv[0], 'linux/arm64':argv[1]}
}

async function fetchSecdb(name) {
  const url = `${secdbRoot}${name}.json`
  const response = await fetch(url, {redirect:'error', signal:AbortSignal.timeout(30_000), headers:{accept:'application/json'}})
  assert.equal(response.status, 200, `${url} returned ${response.status}`)
  const chunks = []
  let size = 0
  for await (const chunk of response.body) {
    size += chunk.length
    assert.ok(size <= 16 << 20, `${url} exceeds 16 MiB`)
    chunks.push(chunk)
  }
  const bytes = Buffer.concat(chunks)
  return {url, bytes, sha256:sha256(bytes), lastModified:response.headers.get('last-modified'), document:JSON.parse(bytes.toString('utf8'))}
}

async function main() {
  const references = parseArguments(process.argv.slice(2))
  const root = join(dirname(fileURLToPath(import.meta.url)), '..')
  const baseline = JSON.parse(await readFile(join(root, 'scripts', 'os-apk-baseline.json'), 'utf8'))
  const output = await mkdtemp(join(tmpdir(), 'openreader-os-advisories-'))
  console.log(`OS advisory evidence: ${output}`)
  try {
    const images = []
    for (const [platform, reference] of Object.entries(references)) {
      const archive = join(output, `${platform.replace('/', '-')}.tar`)
      await runTool('docker', ['save', '-o', archive, reference], {timeout:600_000})
      const {size} = await stat(archive)
      assert.ok(size > 0 && size <= 2 ** 31, `${platform} image archive size ${size}`)
      const image = {platform, ...readImageArchive(await readFile(archive))}
      await rm(archive)
      const installed = image.files.get('lib/apk/db/installed')
      assert.ok(installed?.type === 'file', `${platform} image missing lib/apk/db/installed`)
      await writeFile(join(output, `${platform.replace('/', '-')}-installed`), installed.content)
      await writeFile(join(output, `${platform.replace('/', '-')}-config.json`), JSON.stringify(image.config, null, 2))
      images.push(image)
    }
    const secdb = {}
    for (const name of ['main', 'community']) {
      const fetched = await fetchSecdb(name)
      await writeFile(join(output, `secdb-${name}.json`), fetched.bytes)
      secdb[name] = {document:fetched.document, lastModified:fetched.lastModified, sha256:fetched.sha256, url:fetched.url}
    }
    const now = Date.now()
    const databases = images.map(image => parseApkDatabase(Buffer.from(image.files.get('lib/apk/db/installed').content).toString('utf8'),
      releasePlatforms[image.platform].apkArch))
    const pairs = planComparisons(databases, mergeFixes(['main', 'community'].map(name => validateSecdb(name, secdb[name], now))))
    const comparisons = new Map()
    if (pairs.length) {
      const script = 'while [ "$#" -ge 2 ]; do /sbin/apk --no-network version -t "$1" "$2" || exit 1; shift 2; done; [ "$#" -eq 0 ]'
      const result = await runTool('docker', ['run', '--rm', '--pull', 'never', '--network', 'none', '--platform', 'linux/amd64',
        '--entrypoint', '/bin/sh', references['linux/amd64'], '-c', script, 'sh', ...pairs.flatMap(pair => pair.split('|'))],
      {timeout:300_000, maxBytes:1 << 20})
      assert.ok(result.endsWith('\n'), 'incomplete native comparison output')
      const lines = result.slice(0, -1).split('\n')
      assert.equal(lines.length, pairs.length, 'native comparison output count mismatch')
      pairs.forEach((pair, index) => comparisons.set(pair, lines[index]))
    }
    await writeFile(join(output, 'comparisons.json'), JSON.stringify(Object.fromEntries(comparisons), null, 2))
    const summary = evaluateOsInventory({baseline, images, secdb, comparisons, now})
    await writeFile(join(output, 'summary.json'), JSON.stringify(summary, null, 2))
    console.log(JSON.stringify(summary, null, 2))
  } catch (error) {
    await writeFile(join(output, 'failure.txt'), String(error?.stack ?? error))
    throw error
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => {console.error(`OS advisory gate failed: ${error.message}`); process.exitCode = 1})
}
