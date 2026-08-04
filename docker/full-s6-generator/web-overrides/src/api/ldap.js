import request from '@/utils/request'

export function getLdapConfig () {
  return request({
    url: '/ldap/config',
    method: 'get',
  })
}

export function saveLdapConfig (data) {
  return request({
    url: '/ldap/config',
    method: 'put',
    data,
  })
}

export function testLdapConfig (data) {
  return request({
    url: '/ldap/test',
    method: 'post',
    data,
  })
}

export function rollbackLdapConfig () {
  return request({
    url: '/ldap/rollback',
    method: 'post',
  })
}
