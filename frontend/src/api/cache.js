import api from './client'

export function getCacheStats() {
  return api.get('/cache/stats')
}

export function clearCache() {
  return api.delete('/cache')
}
