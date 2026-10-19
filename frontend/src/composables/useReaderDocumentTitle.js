import { onScopeDispose, unref, watch } from 'vue'
import {
  SHELL_DOCUMENT_TITLE,
  formatReaderDocumentTitle,
  setDocumentTitle,
} from '../utils/documentTitle.js'

// Upstream Reader titles the tab `书名 - 章节名` and switches to the target catalog title as soon as
// navigation starts, so the chapter name comes from the catalog row at currentIndex rather than the
// committed chapter. A book switch replaces book before its catalog arrives; a row is only used while it
// belongs to the current book, so the new name is never paired with the previous book's chapter.
export function useReaderDocumentTitle(options) {
  const setTitle = options.setTitle || setDocumentTitle
  const shellTitle = options.shellTitle ?? SHELL_DOCUMENT_TITLE

  watch(
    () => readerDocumentTitle(
      unref(options.book),
      unref(options.chapters),
      unref(options.currentIndex),
    ),
    (title) => {
      if (title) setTitle(title)
    },
    { immediate: true },
  )

  onScopeDispose(() => setTitle(shellTitle))
}

function readerDocumentTitle(book, chapters, currentIndex) {
  const row = Array.isArray(chapters) ? chapters[Number(currentIndex)] : null
  const chapterTitle = row && persistedId(row.bookId) === persistedId(book?.id) ? row.title : ''
  return formatReaderDocumentTitle(book?.title, chapterTitle)
}

// Shelf catalog rows carry their bookId; temporary remote sessions serialize book.id and row.bookId as 0.
function persistedId(value) {
  const id = Number(value)
  return Number.isFinite(id) && id > 0 ? id : 0
}
