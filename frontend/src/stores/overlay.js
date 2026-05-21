import { defineStore } from 'pinia'

export const useOverlayStore = defineStore('overlay', {
  state: () => ({
    bookInfoVisible: false,
    bookInfoBook: null,
    bookInfoOptions: {},
    bookManageVisible: false,
    bookGroupVisible: false,
    bookGroupMode: 'manage',
    importBookVisible: false,
    bookmarkVisible: false,
    bookmarkBook: null,
    searchBookContentVisible: false,
    searchBook: null,
    rssVisible: false,
    webdavVisible: false,
    userManageVisible: false,
    replaceRulesVisible: false,
  }),
  actions: {
    openBookInfo(book, options = {}) {
      this.bookInfoBook = book
      this.bookInfoOptions = options
      this.bookInfoVisible = true
    },
    closeBookInfo() {
      this.bookInfoVisible = false
    },
    openBookManage() {
      this.bookManageVisible = true
    },
    openBookGroup(mode = 'manage', book = null, options = {}) {
      if (book) {
        this.bookInfoBook = book
        this.bookInfoOptions = options
      }
      this.bookGroupMode = mode
      this.bookGroupVisible = true
    },
    openImportBook() {
      this.importBookVisible = true
    },
    openBookmark(book) {
      this.bookmarkBook = book
      this.bookmarkVisible = true
    },
    openSearchBookContent(book) {
      this.searchBook = book
      this.searchBookContentVisible = true
    },
    openReplaceRules() {
      this.replaceRulesVisible = true
    },
    openRSS() {
      this.rssVisible = true
    },
    openUserManage() {
      this.userManageVisible = true
    },
    openLocalStore(router) {
      router?.push?.({ name: 'local-store' })
    },
    openWebDAV() {
      this.webdavVisible = true
    },
  },
})
