import axios from 'axios'
import { getToken } from '@/utils/auth'

const rdgen = axios.create({
  baseURL: '/api/admin/rdgen',
  timeout: 60000,
})

rdgen.interceptors.request.use(config => {
  const token = getToken()
  if (token) {
    config.headers['api-token'] = token
  }
  return config
})

export function createBuild(data) {
  return rdgen.post('/generator', data)
}

export function getDefaults() {
  return rdgen.get('/defaults')
}

export function getBuildStatus(params) {
  return rdgen.get('/check_for_file', { params: { ...params, format: 'json' } })
}

export function getArtifacts() {
  return rdgen.get('/artifacts', { params: { format: 'json' } })
}

export function downloadArtifact(params) {
  return rdgen.get('/download', { params, responseType: 'blob' })
}

export function deleteArtifactBuild(params) {
  return rdgen.delete('/delete_artifact_build', { params })
}
