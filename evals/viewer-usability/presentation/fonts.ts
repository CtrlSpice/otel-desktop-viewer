import fs from 'node:fs'
import path from 'node:path'

// Reuse the viewer's installed fonts without adding a dependency or network request.
export function viewerFonts() {
  const directory = path.resolve(import.meta.dirname,
    '../../../desktopexporter/internal/frontend/node_modules/@fontsource/atkinson-hyperlegible-next/files')
  return [400, 500, 600, 700].map(weight => {
    const bytes = fs.readFileSync(path.join(directory,
      `atkinson-hyperlegible-next-latin-${weight}-normal.woff2`))
    return `@font-face{font-family:"Atkinson Hyperlegible Next";font-style:normal;font-weight:${weight};font-display:swap;src:url(data:font/woff2;base64,${bytes.toString('base64')}) format("woff2")}`
  }).join('\n')
}
