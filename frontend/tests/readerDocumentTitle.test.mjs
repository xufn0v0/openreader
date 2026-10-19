import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import { effectScope, nextTick, ref } from 'vue'

// Contract: docs/compat/browser-document-title-fixed-baseline-second-audit-p0-contract.md R1-R5, 8.1(2).

const bookA = { id: 1, title: '书A' }
const bookB = { id: 2, title: '书B' }
const rowsA = [
  { id: 11, bookId: 1, index: 0, title: '第一章' },
  { id: 12, bookId: 1, index: 1, title: '第二章' },
  { id: 13, bookId: 1, index: 2, title: '' },
  { id: 14, bookId: 1, index: 3, title: '   ' },
  { id: 15, bookId: 1, index: 4, title: '  第五章  ' },
]
const rowsB = [
  { id: 21, bookId: 2, index: 0, title: '第一章' },
  { id: 22, bookId: 2, index: 1, title: '番外' },
]
// Temporary remote sessions are never persisted: the API serializes book.id and row.bookId as 0.
const remoteBook = { id: 0, title: '远程临时书' }
const remoteRows = [
  { id: 0, bookId: 0, index: 0, title: '第一章' },
  { id: 0, bookId: 0, index: 1, title: '第二章' },
]

async function loadReaderDocumentTitle() {
  try {
    return await import('../src/composables/useReaderDocumentTitle.js')
  } catch (error) {
    assert.fail(`reader document title composable must exist at src/composables/useReaderDocumentTitle.js (${error.code || error.message})`)
  }
}

async function shellTitleFromIndexHTML() {
  const html = await readFile(new URL('../index.html', import.meta.url), 'utf8')
  const match = html.match(/<title>([^<]*)<\/title>/i)
  assert.ok(match, 'frontend/index.html must declare a <title>')
  return match[1].trim()
}

async function settle() {
  await nextTick()
  await nextTick()
}

async function mountReaderTitle({
  book = null,
  chapters = [],
  currentIndex = 0,
  committedChapter = null,
  shellTitle,
} = {}) {
  const { useReaderDocumentTitle } = await loadReaderDocumentTitle()
  assert.equal(typeof useReaderDocumentTitle, 'function', 'useReaderDocumentTitle must be exported')
  const reader = {
    book: ref(book),
    chapters: ref(chapters),
    currentIndex: ref(currentIndex),
    // The committed chapter only changes after content arrives; the tab title must not wait for it.
    chapter: ref(committedChapter),
    titles: [],
    scope: effectScope(),
  }
  reader.scope.run(() => useReaderDocumentTitle({
    book: reader.book,
    chapters: reader.chapters,
    currentIndex: reader.currentIndex,
    chapter: reader.chapter,
    setTitle: title => reader.titles.push(title),
    ...(shellTitle === undefined ? {} : { shellTitle }),
  }))
  await settle()
  return reader
}

function lastTitle(reader) {
  return reader.titles.at(-1)
}

function assertWellFormed(titles) {
  for (const title of titles) {
    assert.equal(typeof title, 'string', `title ${JSON.stringify(title)} must be a string`)
    assert.notEqual(title.trim(), '', 'an empty tab title must never be written')
    assert.doesNotMatch(title, /^(undefined|null)(\s-\s|$)/, `title ${JSON.stringify(title)} leaks a missing book name`)
    assert.doesNotMatch(title, /^\s*-/, `title ${JSON.stringify(title)} starts with the separator`)
  }
}

test('shows the book name until the catalog row is known, then book and chapter', async () => {
  const reader = await mountReaderTitle({ book: bookA })
  assert.equal(lastTitle(reader), '书A')

  reader.chapters.value = rowsA
  await settle()
  assert.equal(lastTitle(reader), '书A - 第一章')
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('switches to the target catalog title as soon as currentIndex moves', async () => {
  const reader = await mountReaderTitle({
    book: bookA,
    chapters: rowsA,
    committedChapter: rowsA[0],
  })
  assert.equal(lastTitle(reader), '书A - 第一章')

  // Load start (and a later content failure) moves currentIndex while the committed chapter stays on 第一章.
  reader.currentIndex.value = 1
  await settle()
  assert.equal(lastTitle(reader), '书A - 第二章')
  assert.equal(reader.chapter.value.title, '第一章')
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('treats empty, blank and missing catalog titles as no chapter name', async () => {
  const reader = await mountReaderTitle({ book: bookA, chapters: rowsA })
  const expected = [
    [2, '书A'],
    [4, '书A - 第五章'],
    [3, '书A'],
    [1, '书A - 第二章'],
    [9, '书A'],
  ]
  for (const [index, title] of expected) {
    reader.currentIndex.value = index
    await settle()
    assert.equal(lastTitle(reader), title, `catalog index ${index}`)
  }
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('never pairs a new book name with the previous book catalog during a shelf switch', async () => {
  const reader = await mountReaderTitle({ book: bookA, chapters: rowsA, currentIndex: 1 })
  assert.equal(lastTitle(reader), '书A - 第二章')
  const switchStart = reader.titles.length

  // useReaderBookLoad replaces book with the shelf row before the first await; the catalog follows later,
  // and route sync may already load the target index against the old catalog.
  reader.book.value = bookB
  await settle()
  reader.currentIndex.value = 0
  await settle()
  const held = reader.titles.slice(switchStart)
  assert.deepEqual(
    held.filter(title => title !== '书B'),
    [],
    `while the catalog still belongs to 书A only 书B (or no write) is allowed, got ${JSON.stringify(held)}`,
  )

  reader.chapters.value = rowsB
  reader.currentIndex.value = 0
  await settle()
  assert.equal(lastTitle(reader), '书B - 第一章')
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('a book switch updates the tab even when both current chapters share a title', async () => {
  const reader = await mountReaderTitle({ book: bookA, chapters: rowsA, currentIndex: 0 })
  assert.equal(lastTitle(reader), '书A - 第一章')
  const switchStart = reader.titles.length

  reader.book.value = bookB
  await settle()
  assert.equal(
    reader.titles.slice(switchStart).includes('书B - 第一章'),
    false,
    '书B must not borrow 书A 第一章 before its own catalog arrives',
  )

  reader.chapters.value = rowsB
  await settle()
  assert.equal(lastTitle(reader), '书B - 第一章')
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('remote session rows pair only with the remote session book', async () => {
  const reader = await mountReaderTitle({ book: remoteBook, chapters: remoteRows })
  assert.equal(lastTitle(reader), '远程临时书 - 第一章')
  reader.currentIndex.value = 1
  await settle()
  assert.equal(lastTitle(reader), '远程临时书 - 第二章')
  const switchStart = reader.titles.length

  // A shelf book opened from the temporary reader replaces book before its catalog replaces the session rows.
  reader.book.value = bookA
  await settle()
  assert.deepEqual(
    reader.titles.slice(switchStart).filter(title => title !== '书A'),
    [],
    'a shelf book must not borrow remote session chapter titles',
  )

  reader.chapters.value = rowsA
  reader.currentIndex.value = 0
  await settle()
  assert.equal(lastTitle(reader), '书A - 第一章')
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('follows book renames without a chapter change', async () => {
  const reader = await mountReaderTitle({ book: { ...bookA }, chapters: rowsA })
  assert.equal(lastTitle(reader), '书A - 第一章')

  reader.book.value = { ...bookA, title: '书A（修订）' }
  await settle()
  assert.equal(lastTitle(reader), '书A（修订） - 第一章')

  reader.book.value.title = '书A（再修订）'
  await settle()
  assert.equal(lastTitle(reader), '书A（再修订） - 第一章')
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('does not write while the book name is unknown', async () => {
  const reader = await mountReaderTitle({ chapters: rowsA, currentIndex: 1 })
  assert.deepEqual(reader.titles, [], 'no book: the current tab title is kept')

  reader.book.value = { id: 1, title: '' }
  await settle()
  assert.deepEqual(reader.titles, [], 'an empty book name is unknown')

  reader.book.value = bookA
  await settle()
  assert.equal(lastTitle(reader), '书A - 第二章')
  const written = reader.titles.length

  reader.book.value = null
  reader.currentIndex.value = 0
  await settle()
  assert.equal(reader.titles.length, written, 'losing the book keeps the current tab title')
  assertWellFormed(reader.titles)
  reader.scope.stop()
})

test('restores the index.html shell title when the Reader scope is disposed', async () => {
  const shellTitle = await shellTitleFromIndexHTML()
  const reader = await mountReaderTitle({ book: bookA, chapters: rowsA })
  assert.equal(lastTitle(reader), '书A - 第一章')
  reader.scope.stop()
  assert.equal(lastTitle(reader), shellTitle)

  const unknown = await mountReaderTitle()
  unknown.scope.stop()
  assert.deepEqual(unknown.titles, [shellTitle], 'a Reader that never knew its book still restores the shell')

  const custom = await mountReaderTitle({ book: bookA, chapters: rowsA, shellTitle: '壳标题' })
  custom.scope.stop()
  assert.equal(lastTitle(custom), '壳标题')
})
