import fs from 'node:fs'

const [utilsFile, shareComponentFile] = process.argv.slice(2)
if (!utilsFile || !shareComponentFile) {
  throw new Error(
    'usage: node patch-webclient.mjs <src/utils/webclient.js> <shareByWebClient.vue>'
  )
}

let utilsSource = fs.readFileSync(utilsFile, 'utf8')

const linkFunctionPattern = /export const toWebClientLink = \(row\) => \{[\s\S]*?\n\}/g
const linkFunctions = utilsSource.match(linkFunctionPattern) || []
if (linkFunctions.length !== 1) {
  throw new Error(`expected exactly one toWebClientLink function, found ${linkFunctions.length}`)
}
utilsSource = utilsSource.replace(
  linkFunctionPattern,
  `export const toWebClientLink = (row) => {
  window.open(\`${'${app.setting.rustdeskConfig.api_server}'}/webclient/#/?id=${'${encodeURIComponent(row.id)}'}\`)
}`
)

const shareFunctionPattern = /export function get[A-Za-z0-9_]*ShareUrl \(token\) \{\n\s*return [^\n]+\n\}/g
const shareFunctions = utilsSource.match(shareFunctionPattern) || []
if (shareFunctions.length !== 1) {
  throw new Error(`expected exactly one Web Client share URL function, found ${shareFunctions.length}`)
}
utilsSource = utilsSource.replace(
  shareFunctionPattern,
  `export function getShareUrl (token) {
  return \`${'${app.setting.rustdeskConfig.api_server}'}/webclient/#/?share_token=${'${encodeURIComponent(token)}'}\`
}`
)

let componentSource = fs.readFileSync(shareComponentFile, 'utf8')
const shareSymbolPattern = /get[A-Za-z0-9_]*ShareUrl/g
const shareSymbols = componentSource.match(shareSymbolPattern) || []
if (shareSymbols.length !== 2 || new Set(shareSymbols).size !== 1) {
  throw new Error(`expected one imported share URL symbol used once, found ${shareSymbols.length}`)
}
componentSource = componentSource.replace(shareSymbolPattern, 'getShareUrl')

fs.writeFileSync(utilsFile, utilsSource)
fs.writeFileSync(shareComponentFile, componentSource)
