import api from './client'

export function listExploreSources() {
  return api.get('/explore/sources')
}

export function exploreBooks(sourceId, params = {}) {
  return api.get(`/explore/${sourceId}`, { params })
}
