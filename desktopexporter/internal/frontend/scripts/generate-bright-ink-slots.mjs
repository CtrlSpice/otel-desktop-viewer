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
const harmonies = await readJSON(
  new URL('../src/themes/service-ink-harmonies.json', import.meta.url)
)
const { ramps } = await readJSON(
  new URL('../../../../third_party/madoromi/ink-ramps.json', import.meta.url)
)

// Vivid starting colours. Shade numbers alone do not describe brightness
// consistently across different inks.
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
const all = Object.keys(graph.inks).flatMap(ink =>
  ramps[ink].slice(0, 7).map((lab, i) => ({ ink, id: `${ink}:${i + 1}`, lab }))
)
// Three usable shades of the actual named ink: some depth, a vivid middle,
// and a lighter colour where the source offers one. Avoid near-black/paper
// extremes. These same swatches supply the theme control's three dots.
const shades = {
  'shimbashi-iro': [2, 3, 4],
  ebisu: [2, 3, 5],
  'ama-iro': [2, 3, 5],
  'kon-peki': [3, 4, 6],
  ajisai: [2, 4, 6],
  jurojin: [3, 4, 6],
  'murasaki-shikibu': [3, 4, 6],
  'asa-gao': [4, 5, 7],
  'tsuyu-kusa': [4, 5, 7],
  'shin-kai': [4, 5, 7],
  'tsuki-yo': [4, 5, 7],
  rikka: [3, 6, 7],
  'ku-jaku': [5, 6, 7],
  'sui-gyoku': [5, 6, 7],
  'shin-ryoku': [5, 6, 7],
  'ina-ho': [5, 6, 7],
  'edo-murasaki': [3, 4, 5],
  'yama-budo': [4, 5, 7],
  tsutsuji: [3, 4, 6],
  bishamonten: [2, 3, 6],
  'hana-ikada': [2, 3, 6],
  kosumosu: [2, 4, 6],
  momiji: [2, 3, 6],
  benzaiten: [2, 4, 6],
  'fuyu-gaki': [2, 3, 5],
  'yu-yake': [2, 4, 6],
  'to-ro': [2, 3, 5],
  daikokuten: [2, 3, 4],
  'hotaru-bi': [2, 3, 4],
  'chiku-rin': [4, 5, 6],
  fukurokuju: [3, 4, 5],
  'syo-ro': [4, 5, 6],
}
const previews = Object.fromEntries(
  Object.entries(shades).map(([ink, values]) => [
    ink,
    values.map(shade => `${ink}:${shade}`),
  ])
)
const previewIDs = new Set(Object.values(previews).flat())
const bright = all.filter(
  ({ id, lab: [L, a, b] }) =>
    previewIDs.has(id) || (L >= 0.6 && L <= 0.87 && Math.hypot(a, b) >= 0.1)
)
const byID = new Map(bright.map(swatch => [swatch.id, swatch]))
const representative = ink => {
  const swatch = byID.get(`${ink}:${representatives[ink]}`)
  if (!swatch) throw new Error(`Missing bright representative: ${ink}`)
  return swatch
}
const distance = (a, b) => Math.hypot(...a.lab.map((v, i) => v - b.lab[i]))

function ranking(seed) {
  const family = harmonies.inks[seed]
  const related = previews[seed].map(id => byID.get(id))
  if (
    related.length !== 3 ||
    related.some(s => !s) ||
    !related.includes(representative(seed))
  )
    throw new Error(`Invalid preview shades: ${seed}`)
  const contrasts = family.triadic
    .filter(ink => ink !== seed)
    .map(representative)
    .sort(
      (a, b) =>
        distance(b, representative(seed)) - distance(a, representative(seed))
    )
    .slice(0, 1)
  if (contrasts.length !== 1 || contrasts.some(s => related.includes(s)))
    throw new Error(`Invalid triadic companions: ${seed}`)

  // Composition and order are separate: seed first, then visually separated
  // neighbours. The 3:1 composition is not a fixed on/on/on/off sequence.
  const ordered = [representative(seed)]
  const append = (pool, contrastSet = new Set()) => {
    let remaining = pool.filter(s => !ordered.includes(s))
    while (remaining.length) {
      const last = ordered.at(-1)
      let candidates = remaining.filter(
        s => !contrastSet.has(last) || !contrastSet.has(s)
      )
      if (!candidates.length) candidates = remaining
      candidates.sort((a, b) => distance(b, last) - distance(a, last))
      const next = candidates[0]
      ordered.push(next)
      remaining = remaining.filter(s => s !== next)
    }
  }
  append([...related, ...contrasts], new Set(contrasts))
  // Larger scopes use other bright shades of these families, then the wider
  // bright catalogue. Exhaustion must never introduce a dark/pale shade.
  const familyInks = new Set([...family.analogous, ...family.triadic])
  append(bright.filter(s => familyInks.has(s.ink)))
  append(Object.keys(graph.inks).map(representative))
  append(bright)
  if (
    ordered.length !== bright.length ||
    new Set(ordered).size !== bright.length
  )
    throw new Error(`Incomplete bright ranking: ${seed}`)
  return ordered.map(s => s.id)
}

const output = await format(
  JSON.stringify({
    version: 'shades-3-1-v1',
    source: harmonies.source,
    representatives,
    previews,
    inks: Object.fromEntries(
      Object.keys(graph.inks).map(ink => [ink, ranking(ink)])
    ),
  }),
  { parser: 'json' }
)
const target = new URL('../src/themes/bright-ink-slots.json', import.meta.url)
if (process.argv.includes('--check')) {
  if ((await readFile(target, 'utf8')) !== output)
    throw new Error('Bright ink rankings are stale')
} else {
  await writeFile(target, output)
}
console.log(
  `${Object.keys(graph.inks).length} ink rankings; ${bright.length} bright automatic swatches`
)
