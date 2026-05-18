import api from './client'

export function searchBooks(payload) {
  return api.post('/search', payload)
}
