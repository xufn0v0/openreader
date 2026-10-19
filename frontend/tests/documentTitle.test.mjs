import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

// Contract: docs/compat/browser-document-title-fixed-baseline-second-audit-p0-contract.md 7.1, 7.4, 8.1(1).

async function loadDocumentTitle() {
  try {
    return await import('../src/utils/documentTitle.js')
  } catch (error) {
    assert.fail(`reader document title helper must exist at src/utils/documentTitle.js (${error.code || error.message})`)
  }
}

async function shellTitleFromIndexHTML() {
  const html = await readFile(new URL('../index.html', import.meta.url), 'utf8')
  const match = html.match(/<title>([^<]*)<\/title>/i)
  assert.ok(match, 'frontend/index.html must declare a <title>')
  return match[1].trim()
}

test('reader tab title is the book name, a spaced ASCII hyphen, and the raw catalog title', async () => {
  const { formatReaderDocumentTitle } = await loadDocumentTitle()
  assert.equal(typeof formatReaderDocumentTitle, 'function', 'formatReaderDocumentTitle must be exported')
  assert.equal(formatReaderDocumentTitle('斗破苍穹', '第一章 陨落的天才'), '斗破苍穹 - 第一章 陨落的天才')
  assert.equal(formatReaderDocumentTitle('斗破苍穹', '  第一章 陨落的天才  '), '斗破苍穹 - 第一章 陨落的天才')
})

test('reader tab title is only the book name when the chapter title is empty or blank', async () => {
  const { formatReaderDocumentTitle } = await loadDocumentTitle()
  for (const chapterTitle of ['', null, undefined, '   ', '\t\n']) {
    assert.equal(
      formatReaderDocumentTitle('斗破苍穹', chapterTitle),
      '斗破苍穹',
      `chapter title ${JSON.stringify(chapterTitle)} must leave only the book name`,
    )
  }
})

test('reader tab title adds no chapter ordinal or product suffix', async () => {
  const { formatReaderDocumentTitle } = await loadDocumentTitle()
  assert.equal(formatReaderDocumentTitle('书', '第二章'), '书 - 第二章')
  assert.equal(formatReaderDocumentTitle('书', '第 2 章'), '书 - 第 2 章')
})

test('an unknown book name yields an empty value so the reader does not write', async () => {
  const { formatReaderDocumentTitle } = await loadDocumentTitle()
  for (const bookName of ['', null, undefined, '   ']) {
    assert.equal(
      formatReaderDocumentTitle(bookName, '第一章'),
      '',
      `book name ${JSON.stringify(bookName)} must not produce a tab title`,
    )
  }
})

test('the shell title constant is the frontend/index.html <title>', async () => {
  const { SHELL_DOCUMENT_TITLE } = await loadDocumentTitle()
  assert.equal(SHELL_DOCUMENT_TITLE, await shellTitleFromIndexHTML())
})

test('the single title writer ignores empty titles and is inert without a document', async () => {
  const { setDocumentTitle } = await loadDocumentTitle()
  assert.equal(typeof setDocumentTitle, 'function', 'setDocumentTitle must be exported')
  assert.equal(typeof globalThis.document, 'undefined')
  setDocumentTitle('书 - 第一章')

  globalThis.document = { title: '壳' }
  try {
    setDocumentTitle('书 - 第一章')
    assert.equal(globalThis.document.title, '书 - 第一章')
    setDocumentTitle('')
    assert.equal(globalThis.document.title, '书 - 第一章')
  } finally {
    delete globalThis.document
  }
})
