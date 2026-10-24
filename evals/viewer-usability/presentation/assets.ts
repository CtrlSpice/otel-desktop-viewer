import fs from 'node:fs'
import { stripTypeScriptTypes } from 'node:module'
import { viewerFonts } from './fonts.ts'
import { viewerStyles } from './viewer-components.ts'

export interface Asset {
  type: string
  body: string | Buffer
}

/** Only these public assets are written or served; no run directory is exposed. */
export function presentationAssets(): Map<string, Asset> {
  const source = (name: string) =>
    fs.readFileSync(new URL(name, import.meta.url), 'utf8')
  const script = (name: string) => ({
    type: 'text/javascript; charset=utf-8',
    body: stripTypeScriptTypes(source(name)),
  })
  const fonts = viewerFonts()
  const assets = new Map<string, Asset>([
    ['viewer.css', { type: 'text/css; charset=utf-8', body: viewerStyles() }],
    [
      'results.css',
      { type: 'text/css; charset=utf-8', body: source('results.css') },
    ],
    [
      'fonts.css',
      {
        type: 'text/css; charset=utf-8',
        body: fonts.map(font => font.css).join('\n\n') + '\n',
      },
    ],
    ['theme.js', script('theme.ts')],
    ['interactions.js', script('interactions.ts')],
  ])
  for (const font of fonts)
    assets.set(font.name, { type: 'font/woff2', body: font.bytes })
  return assets
}
