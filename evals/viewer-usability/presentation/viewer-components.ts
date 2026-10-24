import fs from 'node:fs'
import path from 'node:path'

export function viewerStyles() {
  const directory = path.resolve(import.meta.dirname, '../../../desktopexporter/internal/server/static/assets')
  const file = fs.readdirSync(directory).find(file => /^index-.*\.css$/.test(file))
  if (!file) throw new Error('Viewer component stylesheet is missing')
  // Font files are copied separately from the viewer's existing font package.
  return fs.readFileSync(path.join(directory, file), 'utf8').replace(/@font-face\s*\{[^}]*\}/g, '')
}
