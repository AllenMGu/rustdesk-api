import fs from 'node:fs'

const file = process.argv[2]
if (!file) {
  throw new Error('usage: node patch-webclient.mjs <src/utils/webclient.js>')
}

let source = fs.readFileSync(file, 'utf8')
const replacements = [
  {
    before: 'window.open(`${app.setting.rustdeskConfig.api_server}/webclient2/#/${row.id}`)',
    after: 'window.open(`${app.setting.rustdeskConfig.api_server}/webclient/#/?id=${encodeURIComponent(row.id)}`)'
  },
  {
    before: 'return `${app.setting.rustdeskConfig.api_server}/webclient2/#/?share_token=${token}`',
    after: 'return `${app.setting.rustdeskConfig.api_server}/webclient/#/?share_token=${encodeURIComponent(token)}`'
  }
]

for (const { before, after } of replacements) {
  const count = source.split(before).length - 1
  if (count !== 1) {
    throw new Error(`expected exactly one upstream Web Client v2 link, found ${count}: ${before}`)
  }
  source = source.replace(before, after)
}

fs.writeFileSync(file, source)
