import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const icons = require('../../../desktopexporter/internal/frontend/node_modules/@hugeicons/core-free-icons')

function icon(parts: [string, Record<string, string>][]) {
  const attributes = (values: Record<string, string>) => Object.entries(values)
    .filter(([name]) => name !== 'key')
    .map(([name, value]) => `${name.replace(/[A-Z]/g, letter => '-' + letter.toLowerCase())}="${value.replaceAll('&', '&amp;').replaceAll('"', '&quot;')}"`)
    .join(' ')
  return `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">${parts.map(([tag, values]) => `<${tag} ${attributes(values)}/>`).join('')}</svg>`
}

export const caret = icon(icons.ChevronDownIcon)
export const warningIcon = icon(icons.Alert02Icon)
export const meanIcon = icon(icons.ChartAverageIcon)
export const timeIcon = icon(icons.Clock01Icon)
export const p95Icon = icon(icons.ChartHistogramIcon)
export const costIcon = icon(icons.Coins01Icon)

export function viewerStyles() {
  const directory = path.resolve(import.meta.dirname, '../../../desktopexporter/internal/server/static/assets')
  const file = fs.readdirSync(directory).find(file => /^index-.*\.css$/.test(file))
  if (!file) throw new Error('Viewer component stylesheet is missing')
  // Fonts are embedded separately from the viewer's existing font package.
  return fs.readFileSync(path.join(directory, file), 'utf8').replace(/@font-face\s*\{[^}]*\}/g, '')
}
