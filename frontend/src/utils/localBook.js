export function isLocalBook(book) {
  if (!book) return false
  if (Number(book.sourceId || 0) === 0) return true
  if (String(book.url || book.bookUrl || '').startsWith('local://')) return true
  return Boolean(
    book.originalFile ||
    book.libraryPath ||
    book.tocFile ||
    book.sourceFile ||
    book.localPath ||
    book.filePath ||
    book.fileName,
  )
}

export function localBookSearchText(book, extra = []) {
  return normalizeLocalBookSearch([
    book?.title,
    book?.name,
    book?.bookName,
    book?.author,
    book?.lastChapter,
    book?.latestChapter,
    book?.latestChapterTitle,
    book?.durChapterTitle,
    book?.originalFile,
    book?.libraryPath,
    book?.tocFile,
    book?.sourceFile,
    book?.localPath,
    book?.filePath,
    book?.fileName,
    book?.url,
    book?.bookUrl,
    ...extra,
  ].filter(Boolean).join(' '))
}

export function normalizeLocalBookSearch(value) {
  return String(value || '')
    .toLowerCase()
    .replace(/[\s·•._\-—–:：，,。.!！?？()[\]【】《》"'“”‘’/\\]+/g, '')
}
