// Madoromi-derived swatches by samhain (the-moonwitch), MPL-2.0.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
import { readFile, writeFile } from 'node:fs/promises'
import { format } from 'prettier'

const readJSON = async url => JSON.parse(await readFile(url, 'utf8'))
const graph = await readJSON(
  new URL('../src/themes/graph-inks.json', import.meta.url)
)
const { ramps } = await readJSON(
  new URL('../../../../third_party/madoromi/ink-ramps.json', import.meta.url)
)

// One bright representative from every colourful ink. Shade numbers alone do
// not describe brightness consistently across different source curves.
const representatives = {
  'shimbashi-iro': 3,
  ebisu: 2,
  'ama-iro': 3,
  'kon-peki': 4,
  ajisai: 4,
  jurojin: 4,
  'murasaki-shikibu': 4,
  'asa-gao': 5,
  'tsuyu-kusa': 5,
  'shin-kai': 5,
  'tsuki-yo': 5,
  rikka: 6,
  'ku-jaku': 6,
  'sui-gyoku': 6,
  'shin-ryoku': 7,
  'ina-ho': 6,
  'edo-murasaki': 4,
  'yama-budo': 5,
  tsutsuji: 4,
  bishamonten: 3,
  'hana-ikada': 3,
  kosumosu: 2,
  momiji: 3,
  benzaiten: 2,
  'fuyu-gaki': 2,
  'yu-yake': 2,
  'to-ro': 3,
  daikokuten: 3,
  'hotaru-bi': 4,
  'chiku-rin': 6,
  fukurokuju: 5,
  'syo-ro': 5,
}
const bright = Object.keys(graph.inks)
  .flatMap(ink =>
    ramps[ink]
      .slice(0, 7)
      .map((lab, i) => ({ ink, id: `${ink}:${i + 1}`, lab }))
  )
  .filter(
    ({ lab: [L, a, b] }) => L >= 0.55 && L <= 0.87 && Math.hypot(a, b) >= 0.1
  )
const byID = new Map(bright.map(swatch => [swatch.id, swatch]))
const vivid = Object.entries(representatives).map(([ink, shade]) => {
  const swatch = byID.get(`${ink}:${shade}`)
  if (!swatch) throw new Error(`Missing bright representative: ${ink}`)
  return swatch
})
const distance = (a, b) => Math.hypot(...a.lab.map((v, i) => v - b.lab[i]))
const ordered = [byID.get('shin-kai:5')]
function append(pool) {
  let remaining = pool.filter(swatch => !ordered.includes(swatch))
  while (remaining.length) {
    const separation = swatch =>
      Math.min(...ordered.map(other => distance(swatch, other)))
    remaining.sort((a, b) => separation(b) - separation(a))
    ordered.push(remaining[0])
    remaining = remaining.slice(1)
  }
}
// Use all 32 families before further shades. Allocation order is independent
// of interface theme, season and mode; existing owners never move.
append(vivid)
append(bright)
const output = await format(
  JSON.stringify({
    version: 'fixed-bright-v1',
    source: graph.source,
    slots: ordered.map(swatch => swatch.id),
  }),
  { parser: 'json' }
)
const target = new URL('../src/themes/bright-ink-slots.json', import.meta.url)
if (process.argv.includes('--check')) {
  if ((await readFile(target, 'utf8')) !== output)
    throw new Error('Bright ink ranking is stale')
} else {
  await writeFile(target, output)
}
console.log(
  `One ranking; ${vivid.length} ink families; ${bright.length} bright automatic swatches`
)
