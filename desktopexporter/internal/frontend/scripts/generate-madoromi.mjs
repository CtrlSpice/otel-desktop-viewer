// Generates the viewer's Madoromi adapter from the pinned MPL-2.0 sources.
// Madoromi by samhain (the-moonwitch): https://github.com/the-moonwitch/madoromi
// Pinned revision: f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import ts from 'typescript'

const frontend = new URL('../', import.meta.url)
const upstream = new URL('../../../third_party/madoromi/', frontend)
const palette = JSON.parse(
  await readFile(new URL('palette.json', upstream), 'utf8')
)
const { inks, order: inkOrder } = JSON.parse(
  await readFile(new URL('ink-curves.json', upstream), 'utf8')
)
const { english: inkNames } = JSON.parse(
  await readFile(new URL('ink-names.json', upstream), 'utf8')
)
const { ramps: officialRamps } = JSON.parse(
  await readFile(new URL('ink-ramps.json', upstream), 'utf8')
)
async function upstreamModule(file, dependencies = {}) {
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
const colorModule = await upstreamModule('color.ts')
const { oklabToHex, oklabToOklch } = await import(colorModule)

const TINT = 0.2
const TEXT_CONTRAST = 4.5
const roles = {
  text: 'sumi',
  subtle: 'usuzumi1',
  muted: 'kasure1',
  primary: 'fuji',
  secondary: 'ruri',
  accent: 'tokiwa',
  sakura: 'sakura',
  warning: 'yamabuki',
  error: 'beni',
  info: 'sora',
  success: 'wakaba',
}
const seasons = {
  haru: 'Spring',
  natsu: 'Summer',
  aki: 'Autumn',
  fuyu: 'Winter',
}

function at(swatch, L = swatch.oklch[0], strength = 1) {
  const [, C, h] = swatch.oklch
  const angle = (h * Math.PI) / 180
  return oklabToHex([
    L,
    C * strength * Math.cos(angle),
    C * strength * Math.sin(angle),
  ])
}
// Read the pinned ink curve at the requested lightness, as upstream colourAt does.
function inkLabAt(ink, L) {
  const lab = inks[ink]
  const n = lab.length / 3
  if (L < lab[0] || L > lab[(n - 1) * 3]) {
    throw new RangeError(`${ink} does not cover lightness ${L}`)
  }
  let i = 0
  while (i < n - 2 && lab[(i + 1) * 3] < L) i++
  const f = Math.min(
    1,
    Math.max(0, (L - lab[i * 3]) / (lab[(i + 1) * 3] - lab[i * 3] || 1))
  )
  return [
    L,
    lab[i * 3 + 1] + (lab[(i + 1) * 3 + 1] - lab[i * 3 + 1]) * f,
    lab[i * 3 + 2] + (lab[(i + 1) * 3 + 2] - lab[i * 3 + 2]) * f,
  ]
}
function inkAt(ink, L) {
  return oklabToHex(inkLabAt(ink, L))
}
function luminance(hex) {
  const rgb = hex.match(/[\da-f]{2}/gi).map(c => parseInt(c, 16) / 255)
  return rgb
    .map(c => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
    .reduce((sum, c, i) => sum + c * [0.2126, 0.7152, 0.0722][i], 0)
}
function contrast(a, b) {
  const [lo, hi] = [luminance(a), luminance(b)].sort((x, y) => x - y)
  return (hi + 0.05) / (lo + 0.05)
}
function readable(swatch, backgrounds, dark) {
  let hex = swatch.hex
  let L = swatch.oklch[0]
  while (backgrounds.some(bg => contrast(hex, bg) < TEXT_CONTRAST)) {
    L += dark ? 0.001 : -0.001
    if (L <= 0 || L >= 1) throw new Error('No readable ink shade')
    hex = at(swatch, L)
  }
  return hex
}
function selection(fg, bg) {
  return (
    '#' +
    [1, 3, 5]
      .map(i =>
        Math.round(
          parseInt(fg.slice(i, i + 2), 16) * 0.45 +
            parseInt(bg.slice(i, i + 2), 16) * 0.55
        )
          .toString(16)
          .padStart(2, '0')
      )
      .join('')
  )
}

const adjustments = []
const definitions = Object.fromEntries(
  Object.entries(palette.flavours).map(([id, f]) => {
    const c = f.colours
    const tint = key => at(c[key], c[key].oklch[0], TINT)
    const colors = {
      canvas: tint('kami'),
      panel: tint('hyoshi'),
      chrome: tint('chitsu'),
      ...Object.fromEntries(
        Object.entries(roles).map(([key, sourceKey]) => [key, c[sourceKey].hex])
      ),
      highlightLow: tint('nijimi0'),
      highlightMedium: selection(tint('nijimi2'), tint('hyoshi')),
      highlightHigh: tint('nijimi1'),
    }
    const backgrounds = [
      'canvas',
      'panel',
      'chrome',
      'highlightLow',
      'highlightMedium',
    ].map(key => colors[key])
    for (const role of [
      'text',
      'primary',
      'secondary',
      'accent',
      'info',
      'success',
    ]) {
      const original = colors[role]
      const swatch = c[roles[role]]
      colors[role] = readable(swatch, backgrounds, f.dark)
      if (colors[role] !== original)
        adjustments.push({
          theme: id,
          role,
          original,
          adjusted: colors[role],
          minContrast: Math.min(
            ...backgrounds.map(bg => contrast(colors[role], bg))
          ),
        })
    }
    const syntax = Object.fromEntries(
      [
        ...palette.accents.map(({ key }) => key),
        'sumi',
        'kasure2',
        'usuzumi0',
      ].map(key => [key, readable(c[key], backgrounds, f.dark)])
    )
    // Editor outlines and bracket fills are marks, not text foregrounds.
    syntax.kasure0 = c.kasure0.hex
    syntax.nijimi2 = tint('nijimi2')
    return [
      `madoromi-${id}`,
      {
        name: `${seasons[f.season]} ${f.dark ? 'Night' : 'Day'}`,
        colors,
        syntax,
      },
    ]
  })
)
const json = JSON.stringify(definitions, null, 2) + '\n'
await writeFile(new URL('src/themes/madoromi.json', frontend), json)

// Full measured curves, equally spaced in OKLab lightness including endpoints.
// Store each adaptive size independently; shorter ramps are never prefixes.
// Keep the graph catalogue colourful: omit the seasonal neutral families and
// the remaining black, muted-green and brown-dominant ramps.
const neutralInks = new Set([
  ...palette.seasons.map(season => season.grey),
  'take-sumi',
  'hoteison',
  'syun-gyo',
  'tsukushi',
])
const countInks = inkOrder.filter(id => !neutralInks.has(id))
// Six complementary roles inspired by Rose Pine's balance, using Madoromi inks.
// Sample each source once to retain its hue in both modes, then soften chroma.
// Pine is deep blue (Tsuki-yo); Foam is mint green (Shin-ryoku).
const interfaceInks = {
  iris: { ink: 'jurojin', at: 0.8, strength: 0.9, day: 0.51, night: 0.79 },
  pine: { ink: 'tsuki-yo', at: 0.63, strength: 0.85, day: 0.47, night: 0.7 },
  foam: { ink: 'shin-ryoku', at: 0.67, strength: 0.65, day: 0.51, night: 0.81 },
  gold: { ink: 'to-ro', at: 0.81, strength: 0.7, day: 0.53, night: 0.81 },
  rose: { ink: 'benzaiten', at: 0.84, strength: 0.6, day: 0.53, night: 0.84 },
  love: { ink: 'momiji', at: 0.7, strength: 0.7, day: 0.5, night: 0.7 },
}
const syntaxRoles = {
  sakura: 'rose',
  nadeshiko: 'rose',
  botan: 'love',
  fuji: 'iris',
  kikyo: 'iris',
  beni: 'love',
  akane: 'rose',
  kaki: 'gold',
  yamabuki: 'gold',
  wakaba: 'foam',
  tokiwa: 'foam',
  sora: 'pine',
  ruri: 'pine',
  ai: 'rose',
}
// Check each mode against every seasonal surface so accents stay exactly the
// same between seasons, not just similar. Only Day/Night changes their strength.
const interfaceModes = Object.fromEntries(
  ['day', 'night'].map(mode => {
    const dark = mode === 'night'
    const backgrounds = Object.entries(definitions)
      .filter(([id]) => id.endsWith(dark ? '-yoru' : '-hiru'))
      .flatMap(([, theme]) =>
        ['canvas', 'panel', 'chrome', 'highlightLow', 'highlightMedium'].map(
          key => theme.colors[key]
        )
      )
    const accents = Object.fromEntries(
      Object.entries(interfaceInks).map(([role, source]) => {
        const [, C, h] = oklabToOklch(inkLabAt(source.ink, source.at))
        const swatch = { oklch: [source[mode], C * source.strength, h] }
        return [
          role,
          readable({ ...swatch, hex: at(swatch) }, backgrounds, dark),
        ]
      })
    )
    const syntax = Object.fromEntries(
      Object.entries(syntaxRoles).map(([slot, role]) => [slot, accents[role]])
    )
    return [
      mode,
      {
        colors: {
          primary: syntax.fuji,
          secondary: syntax.nadeshiko,
          accent: syntax.tokiwa,
          info: syntax.sora,
          success: syntax.wakaba,
          warning: syntax.yamabuki,
          error: syntax.beni,
        },
        syntax,
      },
    ]
  })
)
const interfacePalette = Object.fromEntries(
  Object.keys(definitions).map(id => [
    id,
    interfaceModes[id.endsWith('-yoru') ? 'night' : 'day'],
  ])
)
const interfaceJson = JSON.stringify(interfacePalette, null, 2) + '\n'
await writeFile(
  new URL('src/themes/interface-palette.json', frontend),
  interfaceJson
)
const countRamps = Object.fromEntries(
  countInks.map(id => {
    const lab = inks[id]
    const lo = lab[0]
    const hi = lab.at(-3)
    return [
      id,
      Object.fromEntries(
        Array.from({ length: 12 }, (_, index) => {
          const steps = index + 1
          const night = Array.from({ length: steps }, (_, i) =>
            inkAt(id, i === steps - 1 ? hi : lo + ((hi - lo) * i) / (steps - 1))
          )
          return [
            steps,
            {
              night,
              day: steps === 1 ? [inkAt(id, lo)] : [...night].reverse(),
            },
          ]
        })
      ),
    ]
  })
)
const countJson = JSON.stringify(countRamps, null, 2) + '\n'
const countNamesJson =
  JSON.stringify(
    Object.fromEntries(countInks.map(id => [id, inkNames[id]])),
    null,
    2
  ) + '\n'
await writeFile(new URL('src/themes/count-palettes.json', frontend), countJson)
await writeFile(
  new URL('src/themes/count-palette-names.json', frontend),
  countNamesJson
)

// Categorical colours use the official swatches, not resampled count bands.
// A slot walks the ink order at one shade before proceeding to the next shade.
const graphInks = Object.fromEntries(
  countInks.map(id => [id, officialRamps[id].map(oklabToHex)])
)
const graphSlots = Array.from({ length: 7 }, (_, shade) =>
  countInks.map(id => `${id}:${shade + 1}`)
).flat()
const graphJson =
  JSON.stringify(
    {
      version: 'madoromi-inks-v1',
      inks: graphInks,
      slots: graphSlots,
    },
    null,
    2
  ) + '\n'
await writeFile(new URL('src/themes/graph-inks.json', frontend), graphJson)
// Slider positions follow each ink's original curve between official shades
// 1 and 7. Keep the palest eighth shade outside the categorical picker.
const curveSteps = 1000
const graphCurveJson =
  JSON.stringify({
    steps: curveSteps,
    inks: Object.fromEntries(
      countInks.map(id => {
        const lo = officialRamps[id][0][0]
        const hi = officialRamps[id][6][0]
        const positions = officialRamps[id]
          .slice(0, 7)
          .map(lab => Math.round(((lab[0] - lo) / (hi - lo)) * curveSteps))
        const colors = Array.from({ length: curveSteps + 1 }, (_, position) =>
          inkAt(
            id,
            position === curveSteps
              ? hi
              : lo + ((hi - lo) * position) / curveSteps
          )
        )
        // The seven existing choices retain their exact source values at their
        // nearest slider positions; moving a thumb never quantizes a saved preset.
        positions.forEach((position, shade) => {
          colors[position] = graphInks[id][shade]
        })
        return [
          id,
          { positions, colors: colors.map(color => color.slice(1)).join('') },
        ]
      })
    ),
  }) + '\n'
await writeFile(
  new URL('src/themes/graph-ink-curves.json', frontend),
  graphCurveJson
)
// Ship the adapted source, its inputs, attribution and complete upstream licence.
const publicSource = new URL('public/licenses/madoromi/', frontend)
await mkdir(publicSource, { recursive: true })
await writeFile(new URL('themes.json', publicSource), json)
await writeFile(new URL('count-palettes.json', publicSource), countJson)
await writeFile(
  new URL('count-palette-names.json', publicSource),
  countNamesJson
)
await writeFile(new URL('graph-inks.json', publicSource), graphJson)
await writeFile(new URL('graph-ink-curves.json', publicSource), graphCurveJson)
await writeFile(new URL('interface-palette.json', publicSource), interfaceJson)
for (const file of [
  'LICENSE',
  'NOTICE',
  'palette.json',
  'ink-curves.json',
  'ink-names.json',
  'ink-ramps.json',
  'color.ts',
  'palette.ts',
  'mix.ts',
  'README.md',
]) {
  await writeFile(
    new URL(file.endsWith('.ts') ? `${file}.txt` : file, publicSource),
    await readFile(new URL(file, upstream))
  )
}
await writeFile(
  new URL('generate-madoromi.mjs', publicSource),
  await readFile(new URL(import.meta.url))
)
console.log(
  JSON.stringify(
    {
      variants: Object.keys(definitions).length,
      backgroundTint: TINT,
      adjustments,
    },
    null,
    2
  )
)
