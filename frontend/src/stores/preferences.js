import { defineStore } from 'pinia'
import api from '../api/client'

const PREFERENCE_KEYS = ['shelf', 'search']
const SHELF_LAYOUT_VERSION = 2
const DEFAULT_SHELF = { view: 'grid', layoutVersion: SHELF_LAYOUT_VERSION }
const DEFAULT_SEARCH = { searchType: 'all', group: '', sourceId: '', concurrent: 60 }
const concurrentOptions = [8, 16, 32, 60]
const syncTimers = new Map()

export const usePreferencesStore = defineStore('preferences', {
  state: () => ({
    shelf: readLocalShelfPreference(),
    search: readLocalSearchPreference(),
    syncBaseUpdatedAt: {},
    syncing: {},
    syncError: {},
  }),
  persist: true,
  actions: {
    setShelfView(view) {
      this.shelf = { ...this.shelf, layoutVersion: SHELF_LAYOUT_VERSION, view: view === 'list' ? 'list' : 'grid' }
      this.schedulePreferenceSync('shelf')
    },
    setSearchConfig(config = {}) {
      this.search = sanitizeSearchPreference({ ...this.search, ...config })
      this.schedulePreferenceSync('search')
    },
    applyPreference(key, value, updatedAt = '') {
      if (key === 'shelf') this.shelf = sanitizeShelfPreference(value)
      if (key === 'search') this.search = sanitizeSearchPreference(value)
      if (updatedAt) this.syncBaseUpdatedAt[key] = updatedAt
      this.syncError[key] = ''
    },
    async loadPreferences() {
      await Promise.all(PREFERENCE_KEYS.map(key => this.loadPreference(key)))
    },
    async loadPreference(key) {
      if (!PREFERENCE_KEYS.includes(key) || !hasAuthToken()) return null
      try {
        const { data } = await api.get(`/settings/${key}`)
        if (data?.value && typeof data.value === 'object') {
          this.applyPreference(key, data.value, data.updatedAt || '')
          return data.value
        }
        await this.savePreference(key)
        return preferencePayload(this, key)
      } catch (err) {
        this.syncError[key] = readErrorMessage(err)
        return null
      }
    },
    schedulePreferenceSync(key) {
      if (!PREFERENCE_KEYS.includes(key) || !hasAuthToken()) return
      this.syncError[key] = ''
      if (syncTimers.has(key)) clearTimeout(syncTimers.get(key))
      syncTimers.set(key, setTimeout(() => {
        syncTimers.delete(key)
        this.savePreference(key).catch(() => {})
      }, 700))
    },
    async savePreference(key) {
      if (!PREFERENCE_KEYS.includes(key) || !hasAuthToken()) return null
      if (syncTimers.has(key)) {
        clearTimeout(syncTimers.get(key))
        syncTimers.delete(key)
      }
      this.syncing[key] = true
      this.syncError[key] = ''
      try {
        const { data, headers } = await api.put(`/settings/${key}`, {
          value: preferencePayload(this, key),
          baseUpdatedAt: this.syncBaseUpdatedAt[key] || '',
        })
        if (data?.value && headers?.['x-openreader-setting-conflict']) {
          this.applyPreference(key, data.value, data.updatedAt || '')
          return data.value
        }
        if (data?.updatedAt) this.syncBaseUpdatedAt[key] = data.updatedAt
        return data?.value || preferencePayload(this, key)
      } catch (err) {
        this.syncError[key] = readErrorMessage(err)
        return null
      } finally {
        this.syncing[key] = false
      }
    },
  },
})

function preferencePayload(state, key) {
  if (key === 'shelf') return sanitizeShelfPreference(state.shelf)
  if (key === 'search') return sanitizeSearchPreference(state.search)
  return {}
}

function sanitizeShelfPreference(value = {}) {
  const migrated = Number(value.layoutVersion || 0) < SHELF_LAYOUT_VERSION
  return {
    ...DEFAULT_SHELF,
    view: !migrated && value.view === 'list' ? 'list' : 'grid',
  }
}

function sanitizeSearchPreference(value = {}) {
  const searchType = ['all', 'group', 'single'].includes(value.searchType) ? value.searchType : DEFAULT_SEARCH.searchType
  return {
    ...DEFAULT_SEARCH,
    searchType,
    group: typeof value.group === 'string' ? value.group : '',
    sourceId: value.sourceId === undefined || value.sourceId === null ? '' : value.sourceId,
    concurrent: concurrentOptions.includes(Number(value.concurrent)) ? Number(value.concurrent) : DEFAULT_SEARCH.concurrent,
  }
}

function readLocalShelfPreference() {
  try {
    return sanitizeShelfPreference({ view: window.localStorage?.getItem('openreader_shelf_view') })
  } catch {
    return DEFAULT_SHELF
  }
}

function readLocalSearchPreference() {
  try {
    return sanitizeSearchPreference(JSON.parse(window.localStorage?.getItem('openreader_sidebar_search') || '{}'))
  } catch {
    return DEFAULT_SEARCH
  }
}

function hasAuthToken() {
  return typeof localStorage !== 'undefined' && Boolean(localStorage.getItem('openreader_token'))
}

function readErrorMessage(err) {
  return err?.response?.data?.error || err?.message || '同步失败'
}
