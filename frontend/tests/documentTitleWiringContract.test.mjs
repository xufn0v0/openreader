import assert from 'node:assert/strict'
import { readdir, readFile } from 'node:fs/promises'
import { join, relative, sep } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

// Contract: docs/compat/browser-document-title-fixed-baseline-second-audit-p0-contract.md 7.4, 8.1(3) (Reader part).

const sourceRoot = fileURLToPath(new URL('../src/', import.meta.url))

async function sourceFiles(directory = sourceRoot) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await sourceFiles(path))
    else if (/\.(?:js|vue)$/.test(entry.name)) files.push(path)
  }
  return files
}

test('Reader derives its tab title from the current book, catalog and chapter index', async () => {
  const reader = await readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
  assert.ok(
    /import \{ useReaderDocumentTitle \} from '\.\.\/composables\/useReaderDocumentTitle'/.test(reader),
    'Reader.vue must import useReaderDocumentTitle',
  )
  assert.ok(
    /useReaderDocumentTitle\(\{\s*book,\s*chapters,\s*currentIndex,?\s*\}\)/.test(reader),
    'Reader.vue must call useReaderDocumentTitle with exactly book, chapters and currentIndex',
  )
})

test('only utils/documentTitle.js assigns document.title', async () => {
  const writers = []
  for (const file of await sourceFiles()) {
    const source = await readFile(file, 'utf8')
    if (/document\s*\.\s*title\s*=(?!=)|document\s*\[\s*['"`]title['"`]\s*\]\s*=(?!=)/.test(source)) {
      writers.push(relative(sourceRoot, file).split(sep).join('/'))
    }
  }
  assert.deepEqual(writers, ['utils/documentTitle.js'])
})
