import fs from 'node:fs'

const routerPath = process.argv[2]
let source = fs.readFileSync(routerPath, 'utf8')

const marker = `      {
        path: '/serverCmd',
        name: 'ServerCmd',`

if (!source.includes(marker)) {
  throw new Error('Unable to find the RustDesk API Web system-menu insertion point')
}

const routes = [
  {
    name: 'LdapSettings',
    source: `      {
        path: '/ldapSettings',
        name: 'LdapSettings',
        meta: { title: 'LDAP / Active Directory', icon: 'Connection' },
        component: () => import('@/views/ldap/index.vue'),
      },
`,
  },
  {
    name: 'ClientGenerator',
    source: `      {
        path: '/clientGenerator',
        name: 'ClientGenerator',
        meta: { title: '客户端生成器', icon: 'Tools' },
        component: () => import('@/views/rdgen/index.vue'),
      },
`,
  },
]

for (const route of routes) {
  if (!source.includes(`name: '${route.name}'`)) {
    source = source.replace(marker, route.source + marker)
  }
}

fs.writeFileSync(routerPath, source)
