// Madoromi Mix by samhain (the-moonwitch), MPL-2.0.
// Pinned revision: f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695.
// See third_party/madoromi/LICENSE and mix.ts for the unmodified selection rules.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
import { readFile, writeFile } from 'node:fs/promises'
import ts from 'typescript'
import { format } from 'prettier'

const upstream = new URL('../../../../third_party/madoromi/', import.meta.url)
async function moduleURL(file, dependencies = {}) {
  const source = await readFile(new URL(file, upstream), 'utf8')
  let javascript = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022,
    },
  }).outputText
  for (const [specifier, url] of Object.entries(dependencies)) {
    javascript = javascript.replaceAll(
      JSON.stringify(specifier),
      JSON.stringify(url)
    )
  }
  return `data:text/javascript;base64,${Buffer.from(javascript).toString('base64')}`
}

const color = await moduleURL('color.ts')
const palette = await moduleURL('palette.ts', { './color': color })
const { harmonyInks } = await import(
  await moduleURL('mix.ts', { './palette': palette })
)
const { inks } = JSON.parse(
  await readFile(new URL('ink-curves.json', upstream), 'utf8')
)
const catalogue = JSON.parse(
  await readFile(
    new URL('../src/themes/graph-inks.json', import.meta.url),
    'utf8'
  )
)
const views = new Map(
  Object.entries(inks).map(([id, lab]) => [id, { curve: { lab } }])
)
const modes = ['analogous', 'complementary', 'triadic', 'tetradic']
const result = Object.fromEntries(
  Object.keys(catalogue.inks).map(id => [
    id,
    Object.fromEntries(
      modes.map(mode => [
        mode,
        harmonyInks(views, [id], mode).filter(ink =>
          Object.hasOwn(catalogue.inks, ink)
        ),
      ])
    ),
  ])
)
const output = await format(
  JSON.stringify(
    {
      source: 'f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695',
      inks: result,
    },
    null,
    2
  ),
  { parser: 'json' }
)
const target = new URL(
  '../src/themes/service-ink-harmonies.json',
  import.meta.url
)
if (process.argv.includes('--check')) {
  if ((await readFile(target, 'utf8')) !== output)
    throw new Error('Service ink harmonies are stale')
} else {
  await writeFile(target, output)
}
