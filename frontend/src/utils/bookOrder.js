function toTime(value) {
  const time = new Date(value || 0).getTime()
  return Number.isFinite(time) ? time : 0
}

function progressFor(book, progressByBook) {
  return progressByBook?.[book?.id] || book?.progress || null
}

export function compareByShelfOrderWithProgress(progressByBook) {
  return (a, b) => {
    const aProgressAt = toTime(progressFor(a, progressByBook)?.updatedAt)
    const bProgressAt = toTime(progressFor(b, progressByBook)?.updatedAt)
    if (aProgressAt !== bProgressAt) return bProgressAt - aProgressAt

    const aShelfAt = Math.max(toTime(a?.updatedAt), toTime(a?.createdAt))
    const bShelfAt = Math.max(toTime(b?.updatedAt), toTime(b?.createdAt))
    if (aShelfAt !== bShelfAt) return bShelfAt - aShelfAt
    return Number(b?.id || 0) - Number(a?.id || 0)
  }
}

export function compareByShelfOrder(a, b) {
  return compareByShelfOrderWithProgress()(a, b)
}

export function shelfOrderTime(book, progressByBook) {
  const progressAt = toTime(progressFor(book, progressByBook)?.updatedAt)
  const shelfAt = Math.max(toTime(book?.updatedAt), toTime(book?.createdAt))
  return Math.max(progressAt, shelfAt)
}

export function sortByShelfOrder(books, progressByBook) {
  const list = Array.isArray(books) ? books : []
  return [...list].sort(compareByShelfOrderWithProgress(progressByBook))
}

export function compareRecentBook(a, b, progressByBook) {
  const aTime = shelfOrderTime(a, progressByBook)
  const bTime = shelfOrderTime(b, progressByBook)
  if (aTime !== bTime) return bTime - aTime
  return Number(b?.id || 0) - Number(a?.id || 0)
}
