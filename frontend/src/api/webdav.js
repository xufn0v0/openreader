import axios from 'axios'

export function listWebDAV(path = '') {
  return axios.get(webdavURL(path), { responseType: 'text' })
}

export function uploadWebDAV({ path = '', file }) {
  return axios.put(webdavURL(joinPath(path, file.name)), file, {
    headers: { 'Content-Type': file.type || 'application/octet-stream' },
  })
}

export function createWebDAVDirectory({ path = '', name }) {
  return axios({ method: 'MKCOL', url: webdavURL(joinPath(path, name)) })
}

export function renameWebDAV({ path, newPath }) {
  return axios({
    method: 'MOVE',
    url: webdavURL(path),
    headers: { Destination: webdavURL(newPath) },
  })
}

export function deleteWebDAV(path) {
  return axios.delete(webdavURL(path))
}

export function downloadWebDAV(path) {
  return axios.get(webdavURL(path), { responseType: 'blob' })
}

function webdavURL(path) {
  const clean = String(path || '').replace(/^\/+/, '')
  return `/webdav/${clean.split('/').map(encodeURIComponent).join('/')}`
}

function joinPath(base, name) {
  return [base, name].filter(Boolean).join('/')
}
