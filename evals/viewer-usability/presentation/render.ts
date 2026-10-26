import path from 'node:path'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import { build } from '../../../desktopexporter/internal/frontend/node_modules/vite/dist/node/index.js'
import type { Presentation } from './aggregate.ts'
import { escapeHtml } from './format.ts'

export { escapeHtml } from './format.ts'

// Compile the typed Svelte templates once per Node process. The static export
// needs no client framework, hydration payload, generated source file or server.
const frontend = path.resolve(
  import.meta.dirname,
  '../../../desktopexporter/internal/frontend'
)
const require = createRequire(path.join(frontend, 'package.json'))
const { svelte } = (await import(
  pathToFileURL(require.resolve('@sveltejs/vite-plugin-svelte')).href
)) as typeof import('@sveltejs/vite-plugin-svelte')
const compiled = await build({
  configFile: false,
  root: frontend,
  logLevel: 'error',
  plugins: [svelte({ configFile: false })],
  resolve: { dedupe: ['svelte', '@hugeicons/core-free-icons'] },
  ssr: { noExternal: true },
  build: {
    ssr: path.join(import.meta.dirname, 'components/render.ts'),
    write: false,
    minify: false,
  },
})
if (Array.isArray(compiled) || !('output' in compiled))
  throw new Error('Expected one results template bundle')
const chunks = compiled.output.filter(output => output.type === 'chunk')
if (chunks.length !== 1) throw new Error('Expected one results template module')
const templates = (await import(
  `data:text/javascript;base64,${Buffer.from(chunks[0].code).toString('base64')}`
)) as {
  renderResults: (data: Presentation) => string
}

export function render(data: Presentation, assetBase = './assets') {
  const assets = escapeHtml(assetBase)
  return `<!doctype html>
<html lang="en" data-theme="rose-pine">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta name="color-scheme" content="dark light">
    <title>Viewer investigation evals</title>
    <script src="${assets}/theme.js"></script>
    <link rel="stylesheet" href="${assets}/viewer.css">
    <link rel="stylesheet" href="${assets}/fonts.css">
    <link rel="stylesheet" href="${assets}/results.css">
    <script src="${assets}/interactions.js" defer></script>
  </head>
  <body data-metric="result">
${templates.renderResults(data)}
  </body>
</html>
`
}
