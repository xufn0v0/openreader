import api from './client'

export function listUsers() {
  return api.get('/admin/users')
}

export function updateUser(id, payload) {
  return api.put(`/admin/users/${id}`, payload)
}

export function cleanupInactiveUsers() {
  return api.post('/admin/cleanup-inactive')
}
