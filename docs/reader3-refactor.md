# Reader3-Inspired Refactor Plan

This project uses `hectorqin/reader` as an architecture reference, but keeps an independent Go + Vue 3 implementation.

## Mapping

- Reader3 `storage/data/<user>/<book>_<author>/` -> OpenReader `library/data/<user>/<book>_<author>/`
- Reader3 `Book`/`BookChapter` metadata -> OpenReader SQLite `books`/`chapters`
- Reader3 chapter cache -> OpenReader `cache/`
- Reader3 reading state -> OpenReader `reading_progress` and `bookmarks`
- Reader3 Web UI reader controls -> OpenReader Vue 3 reader settings and side toolbars

## Phase 1: Local Books

- Keep SQLite for indexes, categories, progress, bookmarks and chapter lookup.
- Keep original imported files outside the database in the portable `library/` tree.
- Write `bookSource.json` and `chapters.json` next to the original file.
- Preserve parsed TXT chapter `start`/`end` offsets in the generated chapter archive.
- Keep chapter body cache in `cache/` for fast reader-page loading.

## Phase 2: Reader Core

- Extract reader pagination, progress calculation and settings into composables.
- Align desktop reader layout with fixed-width reading paper plus side controls.
- Keep mobile mode full-screen and gesture-oriented.
- Add font family, font size, line height, theme and page width settings as first-class reader preferences.

## Phase 3: Sources And Sync

- Add book-source import/export compatible with common Reader-style JSON fields.
- Separate source parsing from bookshelf persistence.
- Add refresh/update jobs for remote books.
- Keep Docker persistence simple: mount `data/`, `cache/` and `library/`.
