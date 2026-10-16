import fs from 'node:fs'
import path from 'node:path'

// Reuse the viewer's installed fonts without adding a dependency or network request.
export function viewerFonts() {
  const directory = path.resolve(
    import.meta.dirname,
    '../../../desktopexporter/internal/frontend/node_modules/@fontsource/atkinson-hyperlegible-next/files'
  )
  return [400, 500, 600, 700].map(weight => {
    const name = `atkinson-hyperlegible-next-latin-${weight}-normal.woff2`
    const bytes = fs.readFileSync(path.join(directory, name))
    const css = `@font-face {
  font-family: "Atkinson Hyperlegible Next";
  font-style: normal;
  font-weight: ${weight};
  font-display: swap;
  src: url("./${name}") format("woff2");
}`
    return { name, bytes, css }
  })
}
