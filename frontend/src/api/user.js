import api from './client'

export function loginUser(mode, payload) {
  return api.post(`/auth/${mode}`, payload)
}

export function getMe() {
  return api.get('/me')
}
