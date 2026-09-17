import { mkdir, readFile, writeFile } from 'node:fs/promises'

const root = new URL('../..', import.meta.url)
const palettePath = new URL(
  'desktopexporter/internal/frontend/src/utils/chart-palettes.json',
  root
)
const output = new URL('docs/images/chart-palettes/', root)
const palettes = JSON.parse(await readFile(palettePath, 'utf8'))

const themes = [
  ['rose-pine', 'Rose Pine', '#1f1d2e', '#e0def4'],
  ['rose-pine-moon', 'Rose Pine Moon', '#2a273f', '#e0def4'],
  ['rose-pine-dawn', 'Rose Pine Dawn', '#faf4ed', '#575279'],
]
const families = [
  ['pine', 'Pine'],
  ['foam', 'Foam'],
  ['gold', 'Gold'],
  ['rose', 'Rose'],
  ['iris', 'Iris'],
  ['love', 'Mulberry'],
]

function svgFor(theme, label, background, text) {
  const rows = families
    .map(([key, family], row) => {
      const swatches = palettes[theme][key]
        .map(
          (hex, column) => `<g transform="translate(${180 + column * 132} ${88 + row * 88})">
  <rect width="112" height="48" rx="8" fill="${hex}"/>
  <text x="56" y="70" text-anchor="middle">${hex}</text>
</g>`
        )
        .join('\n')
      return `<text x="28" y="${118 + row * 88}" class="family">${family}</text>\n${swatches}`
    })
    .join('\n')
  return `<svg xmlns="http://www.w3.org/2000/svg" width="860" height="630" viewBox="0 0 860 630" role="img" aria-labelledby="title description">
<title id="title">${label} chart palette</title>
<desc id="description">Six chart-colour families with five swatches each.</desc>
<style>
  text { fill: ${text}; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; }
  .title { font-family: ui-sans-serif, system-ui, sans-serif; font-size: 26px; font-weight: 700; }
  .family { font-family: ui-sans-serif, system-ui, sans-serif; font-size: 17px; font-weight: 650; }
</style>
<rect width="860" height="630" fill="${background}"/>
<text x="28" y="48" class="title">${label} chart palette</text>
<text x="180" y="76">1</text><text x="312" y="76">2</text><text x="444" y="76">3</text><text x="576" y="76">4</text><text x="708" y="76">5</text>
${rows}
</svg>
`
}

await mkdir(output, { recursive: true })
for (const [theme, label, background, text] of themes) {
  await writeFile(new URL(`${theme}.svg`, output), svgFor(theme, label, background, text))
}
