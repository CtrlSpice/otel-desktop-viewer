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
  'shimbashi-iro': [1, 3, 4],
  ebisu: [1, 2, 4],
  'ama-iro': [2, 3, 5],
  'kon-peki': [3, 4, 6],
  ajisai: [2, 4, 6],
  jurojin: [2, 4, 6],
  'murasaki-shikibu': [3, 4, 6],
  'asa-gao': [4, 5, 7],
  'tsuyu-kusa': [3, 5, 7],
  'shin-kai': [3, 5, 7],
  'tsuki-yo': [4, 5, 7],
  rikka: [3, 6, 7],
  'ku-jaku': [4, 6, 7],
  'sui-gyoku': [5, 6, 7],
  'shin-ryoku': [5, 6, 7],
  'ina-ho': [5, 6, 7],
  'edo-murasaki': [3, 4, 5],
  'yama-budo': [3, 5, 7],
  tsutsuji: [1, 4, 7],
  bishamonten: [2, 3, 6],
  'hana-ikada': [1, 3, 5],
  kosumosu: [2, 4, 6],
  momiji: [1, 3, 6],
  benzaiten: [2, 4, 6],
  'fuyu-gaki': [1, 2, 5],
  'yu-yake': [1, 2, 6],
  'to-ro': [1, 3, 6],
  daikokuten: [1, 3, 4],
  'hotaru-bi': [1, 3, 4],
  'chiku-rin': [2, 4, 6],
  fukurokuju: [3, 4, 5],
  'syo-ro': [3, 5, 6],
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
const hue = swatch => Math.atan2(swatch.lab[2], swatch.lab[1])
const chroma = swatch => Math.hypot(swatch.lab[1], swatch.lab[2])
const hueOffset = (a, b) =>
  Math.atan2(Math.sin(hue(a) - hue(b)), Math.cos(hue(a) - hue(b)))
const hueDistance = (a, b) => Math.abs(hueOffset(a, b))

function companions(seed) {
  const anchor = representative(seed)
  const chosen = previews[seed].map(id => byID.get(id))
  const sides = []
  for (const direction of [-1, 1]) {
    // Wider neighbours on either side of the seed supply different hues, not
    // almost-identical inks. Keep their strength moderate and avoid opposites.
    const candidates = bright.filter(s => {
      const angle = (hueOffset(s, anchor) * direction * 180) / Math.PI
      return (
        s.ink !== seed &&
        !sides.some(side => side.ink === s.ink) &&
        angle >= 45 &&
        angle <= 100 &&
        s.lab[0] >= 0.62 &&
        s.lab[0] <= 0.83 &&
        Math.abs(s.lab[0] - anchor.lab[0]) <= 0.18 &&
        chroma(s) <= Math.max(0.16, chroma(anchor)) &&
        chosen.every(other => distance(s, other) >= 0.1)
      )
    })
    const score = s =>
      Math.abs(hueOffset(s, anchor) / Math.PI - (direction * 70) / 180) +
      2 * Math.abs(s.lab[0] - Math.max(0.7, anchor.lab[0])) +
      Math.abs(chroma(s) - Math.min(chroma(anchor), 0.16))
    candidates.sort((a, b) => score(a) - score(b))
    const next = candidates[0]
    if (!next) throw new Error(`Missing distinct companion: ${seed}`)
    sides.push(next)
    chosen.push(next)
  }
  return sides
}

function ranking(seed) {
  const related = previews[seed].map(id => byID.get(id))
  if (
    related.length !== 3 ||
    related.some(s => !s) ||
    !related.includes(representative(seed))
  )
    throw new Error(`Invalid preview shades: ${seed}`)
  const sideColors = companions(seed)

  // Composition and order are separate: seed first, then visually separated
  // neighbours. The 3:2 mix is a composition, not a repeating sequence.
  const ordered = [representative(seed)]
  const append = (pool, contrastSet = new Set()) => {
    let remaining = pool.filter(s => !ordered.includes(s))
    while (remaining.length) {
      const last = ordered.at(-1)
      let candidates = remaining.filter(
        s => !contrastSet.has(last) || !contrastSet.has(s)
      )
      const sideCount = remaining.filter(s => contrastSet.has(s)).length
      if (!contrastSet.has(last) && remaining.length - sideCount < sideCount)
        candidates = candidates.filter(s => contrastSet.has(s))
      if (!candidates.length) candidates = remaining
      const differentInk = candidates.filter(s => s.ink !== last.ink)
      if (differentInk.length) candidates = differentInk
      candidates.sort((a, b) => distance(b, last) - distance(a, last))
      const next = candidates[0]
      ordered.push(next)
      remaining = remaining.filter(s => s !== next)
    }
  }
  append([...related, ...sideColors], new Set(sideColors))
  // Larger scopes favour a visibly new colour over another close shade of an
  // already-used family. A hue penalty keeps nearby options preferable when
  // they provide similar separation. Exhaustion still uses only this pool.
  let remaining = bright.filter(s => !ordered.includes(s))
  const novelty = s =>
    Math.min(...ordered.map(other => distance(s, other))) /
    (1 + hueDistance(s, representative(seed)) / Math.PI)
  while (remaining.length) {
    remaining.sort((a, b) => novelty(b) - novelty(a))
    ordered.push(remaining[0])
    remaining = remaining.slice(1)
  }
  if (
    ordered.length !== bright.length ||
    new Set(ordered).size !== bright.length
  )
    throw new Error(`Incomplete bright ranking: ${seed}`)
  return ordered.map(s => s.id)
}

const output = await format(
  JSON.stringify({
    version: 'varied-3-2-v1',
    source: harmonies.source,
    representatives,
    previews,
    companions: Object.fromEntries(
      Object.keys(graph.inks).map(ink => [ink, companions(ink).map(s => s.id)])
    ),
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
