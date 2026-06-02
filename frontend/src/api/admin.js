import api from './client'

export function listUsers() {
  return api.get('/admin/users')
}

export function createUser(payload) {
  return api.post('/admin/users', payload)
}

export function updateUser(id, payload) {
  return api.put(`/admin/users/${id}`, payload)
}

export function resetUserPassword(id, payload) {
  return api.put(`/admin/users/${id}/password`, payload)
}

export function deleteUsers(ids) {
  return api.post('/admin/users/batch-delete', { ids })
}

export function cleanupInactiveUsers() {
  return api.post('/admin/cleanup-inactive')
}
