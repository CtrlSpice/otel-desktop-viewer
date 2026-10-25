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

// The fixed vivid representatives approved in the all-ink preview. Shade
// numbers alone do not describe brightness consistently across different inks.
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
const bright = all.filter(
  ({ lab: [L, a, b] }) => L >= 0.6 && L <= 0.87 && Math.hypot(a, b) >= 0.1
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
  const related = family.analogous.map(representative).slice(0, 3)
  const relatedPool = bright.filter(s => family.analogous.includes(s.ink))
  while (related.length < 3) {
    const candidates = relatedPool.filter(s => !related.includes(s))
    candidates.sort(
      (a, b) =>
        Math.min(...related.map(s => distance(b, s))) -
        Math.min(...related.map(s => distance(a, s)))
    )
    if (!candidates.length) throw new Error(`Missing related shade: ${seed}`)
    related.push(candidates[0])
  }
  const contrasts = family.triadic
    .filter(ink => ink !== seed)
    .map(representative)
  if (contrasts.length !== 2 || contrasts.some(s => related.includes(s)))
    throw new Error(`Invalid triadic companions: ${seed}`)

  // Composition and order are separate: seed first, then visually separated
  // neighbours, keeping the two contrasting colours apart when possible.
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
    version: 'bright-3-2-v1',
    source: harmonies.source,
    representatives,
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
