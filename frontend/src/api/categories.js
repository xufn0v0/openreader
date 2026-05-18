import api from './client'

export function listCategories() {
  return api.get('/categories')
}

export function createCategory(payload) {
  return api.post('/categories', payload)
}

export function updateCategory(id, payload) {
  return api.put(`/categories/${id}`, payload)
}

export function reorderCategories(ids) {
  return api.put('/categories/reorder', { ids })
}

export function deleteCategory(id) {
  return api.delete(`/categories/${id}`)
}
