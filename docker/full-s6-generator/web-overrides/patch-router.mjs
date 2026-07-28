import fs from 'node:fs'

const routerPath = process.argv[2]
const source = fs.readFileSync(routerPath, 'utf8')

if (source.includes("name: 'ClientGenerator'")) {
  process.exit(0)
}

const marker = `      {
        path: '/serverCmd',
        name: 'ServerCmd',`

if (!source.includes(marker)) {
  throw new Error('Unable to find the RustDesk API Web system-menu insertion point')
}

const generatorRoute = `      {
        path: '/clientGenerator',
        name: 'ClientGenerator',
        meta: { title: '客户端生成器', icon: 'Tools' },
        component: () => import('@/views/rdgen/index.vue'),
      },
`

fs.writeFileSync(routerPath, source.replace(marker, generatorRoute + marker))
