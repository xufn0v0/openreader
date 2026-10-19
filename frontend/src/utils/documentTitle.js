// The only writer of the top-level document.title (browser tab, window switcher, history, bookmarks).
// The shell title must match the <title> in frontend/index.html; documentTitle.test.mjs checks it.
export const SHELL_DOCUMENT_TITLE = 'OpenReader'

export function formatReaderDocumentTitle(bookName, chapterName) {
  const book = bookName === undefined || bookName === null ? '' : String(bookName)
  if (!book.trim()) return ''
  const chapter = chapterName === undefined || chapterName === null ? '' : String(chapterName).trim()
  return chapter ? `${book} - ${chapter}` : book
}

export function setDocumentTitle(title) {
  if (typeof document === 'undefined' || !title) return
  document.title = title
}
