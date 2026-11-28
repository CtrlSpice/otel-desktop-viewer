// Generates the viewer's Madoromi adapter from the pinned MPL-2.0 sources.
// Madoromi by samhain (the-moonwitch): https://github.com/the-moonwitch/madoromi
// Pinned revision: f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises'

type Vec3 = [number, number, number]

interface ColorModule {
  oklabToHex: (lab: Vec3) => string
  oklabToOklch: (lab: Vec3) => Vec3
  toSrgbGamut: (lab: Vec3) => Vec3
}

interface Swatch {
  hex: string
  oklch: Vec3
}

interface Flavour {
  season: keyof typeof seasons
  dark: boolean
  colours: Record<string, Swatch>
}

interface PaletteFile {
  flavours: Record<string, Flavour>
  accents: Array<{ key: string }>
  seasons: Array<{ grey: string }>
}

interface InkCurvesFile {
  inks: Record<string, number[]>
  order: string[]
}

interface InkNamesFile {
  english: Record<string, string>
}

interface InkRampsFile {
  ramps: Record<string, Vec3[]>
}

interface ThemeColors {
  canvas: string
  panel: string
  chrome: string
  text: string
  subtle: string
  muted: string
  primary: string
  secondary: string
  accent: string
  sakura: string
  warning: string
  error: string
  info: string
  success: string
  highlightLow: string
  highlightMedium: string
  highlightHigh: string
}

interface ThemeDefinition {
  name: string
  colors: ThemeColors
  syntax: Record<string, string>
}

interface Adjustment {
  theme: string
  role: string
  original: string
  adjusted: string
  minContrast: number
}

interface InterfaceInkSource {
  ink: string
  at: number
  strength: number
  day: number
  night: number
}

interface InterfaceModeColors {
  primary: string
  secondary: string
  accent: string
  info: string
  success: string
  warning: string
  error: string
}

interface InterfaceModePalette {
  colors: InterfaceModeColors
  syntax: Record<string, string>
}

interface BrightSwatch {
  ink: string
  id: string
  lab: Vec3
}

async function readJSON<T>(url: URL): Promise<T> {
  const parsed: T = JSON.parse(await readFile(url, 'utf8'))
  if (!(parsed instanceof Object)) {
    throw new Error(`Expected JSON object in ${url.pathname}`)
  }
  return parsed
}

const frontend = new URL('../', import.meta.url)
const upstream = new URL('../../../third_party/madoromi/', frontend)
const palette = await readJSON<PaletteFile>(new URL('palette.json', upstream))
const { inks, order: inkOrder } = await readJSON<InkCurvesFile>(
  new URL('ink-curves.json', upstream)
)
const { english: inkNames } = await readJSON<InkNamesFile>(
  new URL('ink-names.json', upstream)
)
const { ramps: officialRamps } = await readJSON<InkRampsFile>(
  new URL('ink-ramps.json', upstream)
)
const { oklabToHex, oklabToOklch, toSrgbGamut }: ColorModule = await import(
  new URL('color.ts', upstream).href
)

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
} as const

type RoleKey = keyof typeof roles

const seasons = {
  haru: 'Spring',
  natsu: 'Summer',
  aki: 'Autumn',
  fuyu: 'Winter',
} as const

const surfaceKeys = [
  'canvas',
  'panel',
  'chrome',
  'highlightLow',
  'highlightMedium',
] as const

const readableRoles = [
  'text',
  'primary',
  'secondary',
  'accent',
  'info',
  'success',
] as const satisfies readonly RoleKey[]

function at(
  swatch: Pick<Swatch, 'oklch'>,
  L = swatch.oklch[0],
  strength = 1
): string {
  const [, C, h] = swatch.oklch
  const angle = (h * Math.PI) / 180
  return oklabToHex([
    L,
    C * strength * Math.cos(angle),
    C * strength * Math.sin(angle),
  ])
}

// Read the pinned ink curve at the requested lightness, as upstream colourAt does.
function inkLabAt(ink: string, L: number): Vec3 {
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

function inkAt(ink: string, L: number): string {
  return oklabToHex(inkLabAt(ink, L))
}

function luminance(hex: string): number {
  const channels = hex.match(/[\da-f]{2}/gi)
  if (!channels || channels.length !== 3) {
    throw new Error(`Invalid hex color: ${hex}`)
  }
  const rgb = channels.map(c => parseInt(c, 16) / 255)
  const weights = [0.2126, 0.7152, 0.0722]
  return rgb
    .map(c => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
    .reduce((sum, c, i) => sum + c * weights[i], 0)
}

function contrast(a: string, b: string): number {
  const [lo, hi] = [luminance(a), luminance(b)].sort((x, y) => x - y)
  return (hi + 0.05) / (lo + 0.05)
}

function readable(
  swatch: Swatch,
  backgrounds: string[],
  dark: boolean
): string {
  let hex = swatch.hex
  let L = swatch.oklch[0]
  while (backgrounds.some(bg => contrast(hex, bg) < TEXT_CONTRAST)) {
    L += dark ? 0.001 : -0.001
    if (L <= 0 || L >= 1) throw new Error('No readable ink shade')
    hex = at(swatch, L)
  }
  return hex
}

function selection(fg: string, bg: string): string {
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

const adjustments: Adjustment[] = []
const definitions: Record<string, ThemeDefinition> = Object.fromEntries(
  Object.entries(palette.flavours).map(([id, f]) => {
    const c = f.colours
    const tint = (key: string): string => at(c[key], c[key].oklch[0], TINT)
    const colors: ThemeColors = {
      canvas: tint('kami'),
      panel: tint('hyoshi'),
      chrome: tint('chitsu'),
      text: c[roles.text].hex,
      subtle: c[roles.subtle].hex,
      muted: c[roles.muted].hex,
      primary: c[roles.primary].hex,
      secondary: c[roles.secondary].hex,
      accent: c[roles.accent].hex,
      sakura: c[roles.sakura].hex,
      warning: c[roles.warning].hex,
      error: c[roles.error].hex,
      info: c[roles.info].hex,
      success: c[roles.success].hex,
      highlightLow: tint('nijimi0'),
      highlightMedium: selection(tint('nijimi2'), tint('hyoshi')),
      highlightHigh: tint('nijimi1'),
    }
    const backgrounds = surfaceKeys.map(key => colors[key])
    for (const role of readableRoles) {
      const original = colors[role]
      const swatch = c[roles[role]]
      colors[role] = readable(swatch, backgrounds, f.dark)
      if (colors[role] !== original) {
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
    }
    const syntax: Record<string, string> = Object.fromEntries(
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
  iris: {
    ink: 'murasaki-shikibu',
    at: 0.8,
    strength: 0.65,
    day: 0.51,
    night: 0.79,
  },
  pine: { ink: 'tsuki-yo', at: 0.63, strength: 0.85, day: 0.47, night: 0.7 },
  foam: { ink: 'shin-ryoku', at: 0.67, strength: 0.65, day: 0.51, night: 0.81 },
  gold: { ink: 'to-ro', at: 0.81, strength: 0.7, day: 0.53, night: 0.81 },
  rose: { ink: 'fuyu-gaki', at: 0.84, strength: 0.5, day: 0.53, night: 0.84 },
  love: { ink: 'momiji', at: 0.7, strength: 0.7, day: 0.5, night: 0.7 },
} satisfies Record<string, InterfaceInkSource>

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
} satisfies Record<string, keyof typeof interfaceInks>

// Check each mode against every seasonal surface so accents stay exactly the
// same between seasons, not just similar. Only Day/Night changes their strength.
const interfaceModes: Record<'day' | 'night', InterfaceModePalette> = {
  day: buildInterfaceMode('day'),
  night: buildInterfaceMode('night'),
}

function buildInterfaceMode(mode: 'day' | 'night'): InterfaceModePalette {
  const dark = mode === 'night'
  const suffix = dark ? '-yoru' : '-hiru'
  const backgrounds: string[] = []
  for (const [id, theme] of Object.entries(definitions)) {
    if (!id.endsWith(suffix)) continue
    for (const key of surfaceKeys) {
      backgrounds.push(theme.colors[key])
    }
  }
  const accents: Record<string, string> = Object.fromEntries(
    Object.entries(interfaceInks).map(([role, source]) => {
      const [, C, h] = oklabToOklch(inkLabAt(source.ink, source.at))
      const oklch: Vec3 = [source[mode], C * source.strength, h]
      const swatch = { oklch }
      return [role, readable({ ...swatch, hex: at(swatch) }, backgrounds, dark)]
    })
  )
  const syntax: Record<string, string> = Object.fromEntries(
    Object.entries(syntaxRoles).map(([slot, role]) => [slot, accents[role]])
  )
  return {
    colors: {
      primary: syntax.fuji,
      secondary: syntax.ruri,
      accent: syntax.tokiwa,
      info: syntax.sora,
      success: syntax.wakaba,
      warning: syntax.yamabuki,
      error: syntax.beni,
    },
    syntax,
  }
}

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

// Sequential counts move from Primary/Iris through Love to Rose.
// Bound lightness away from both black and paper-white. Interpolate in OKLab,
// gamut-map, then resample equal perceptual arc length for each adaptive size.
// The original ink curves supply the anchor hues/chroma, not RGB gradients.
const heatmapAnchors = [
  {
    ink: 'murasaki-shikibu',
    sourceLightness: 0.8,
    lightness: 0.48,
    strength: 0.8,
  },
  { ink: 'momiji', sourceLightness: 0.7, lightness: 0.68, strength: 0.7 },
  { ink: 'fuyu-gaki', sourceLightness: 0.84, lightness: 0.87, strength: 0.6 },
]
const heatmapStops: Vec3[] = heatmapAnchors.map(source => {
  const [, a, b] = inkLabAt(source.ink, source.sourceLightness)
  return [source.lightness, a * source.strength, b * source.strength]
})
const mixLab = (a: Vec3, b: Vec3, t: number): Vec3 => [
  a[0] + (b[0] - a[0]) * t,
  a[1] + (b[1] - a[1]) * t,
  a[2] + (b[2] - a[2]) * t,
]
const deltaE = (a: Vec3, b: Vec3): number =>
  Math.hypot(a[0] - b[0], a[1] - b[1], a[2] - b[2])
const heatmapPath: Vec3[] = heatmapStops
  .slice(1)
  .flatMap((stop, index) =>
    Array.from({ length: 256 }, (_, step) =>
      toSrgbGamut(mixLab(heatmapStops[index], stop, step / 256))
    )
  )
heatmapPath.push(toSrgbGamut(heatmapStops[heatmapStops.length - 1]))
const arc = [0]
for (let i = 1; i < heatmapPath.length; i++) {
  arc.push(arc[i - 1] + deltaE(heatmapPath[i - 1], heatmapPath[i]))
}
function heatmapAt(t: number): string {
  const target = t * arc[arc.length - 1]
  let index = 1
  while (index < arc.length - 1 && arc[index] < target) index++
  const fraction = (target - arc[index - 1]) / (arc[index] - arc[index - 1])
  return oklabToHex(
    mixLab(heatmapPath[index - 1], heatmapPath[index], fraction)
  )
}
const heatmapRamps = Object.fromEntries(
  Array.from({ length: 12 }, (_, index) => {
    const steps = index + 1
    const night = Array.from({ length: steps }, (_, i) =>
      heatmapAt(steps === 1 ? 1 : i / (steps - 1))
    )
    return [
      steps,
      { night, day: steps === 1 ? [heatmapAt(0)] : [...night].reverse() },
    ]
  })
)
const heatmapJson =
  JSON.stringify(
    {
      version: 'ink-spectrum-v2',
      anchors: heatmapAnchors,
      ramps: heatmapRamps,
    },
    null,
    2
  ) + '\n'
await writeFile(
  new URL('src/themes/heatmap-palette.json', frontend),
  heatmapJson
)

const countRamps = Object.fromEntries(
  countInks.map(id => {
    const lab = inks[id]
    const lo = lab[0]
    const hi = lab[lab.length - 3]
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
const graphInks: Record<string, string[]> = Object.fromEntries(
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

// One bright representative from every colourful ink. Shade numbers alone do
// not describe brightness consistently across different source curves.
const brightRepresentatives = {
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
} satisfies Record<string, number>
const brightSwatches: BrightSwatch[] = []
for (const ink of countInks) {
  const swatches = officialRamps[ink].slice(0, 7)
  for (let i = 0; i < swatches.length; i++) {
    const lab = swatches[i]
    const [L, a, b] = lab
    if (L >= 0.55 && L <= 0.87 && Math.hypot(a, b) >= 0.1) {
      brightSwatches.push({ ink, id: `${ink}:${i + 1}`, lab })
    }
  }
}
const brightByID = new Map(brightSwatches.map(swatch => [swatch.id, swatch]))
const vividSwatches = Object.entries(brightRepresentatives).map(
  ([ink, shade]) => {
    const swatch = brightByID.get(`${ink}:${shade}`)
    if (!swatch) throw new Error(`Missing bright representative: ${ink}`)
    return swatch
  }
)
const firstBright = brightByID.get('shin-kai:5')
if (!firstBright) throw new Error('Missing starting bright swatch: shin-kai:5')
const orderedBright: BrightSwatch[] = [firstBright]
function appendBright(pool: BrightSwatch[]): void {
  let remaining = pool.filter(swatch => !orderedBright.includes(swatch))
  while (remaining.length) {
    const separation = (swatch: BrightSwatch): number =>
      Math.min(...orderedBright.map(other => deltaE(swatch.lab, other.lab)))
    remaining.sort((a, b) => separation(b) - separation(a))
    orderedBright.push(remaining[0])
    remaining = remaining.slice(1)
  }
}
// Use all 32 families before further shades. Allocation order is independent
// of interface theme, season and mode; existing owners never move.
appendBright(vividSwatches)
appendBright(brightSwatches)
const brightSlotsJson =
  JSON.stringify(
    {
      version: 'fixed-bright-v1',
      slots: orderedBright.map(swatch => swatch.id),
    },
    null,
    2
  ) + '\n'
await writeFile(
  new URL('src/themes/bright-ink-slots.json', frontend),
  brightSlotsJson
)

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
await writeFile(new URL('bright-ink-slots.json', publicSource), brightSlotsJson)
await writeFile(new URL('graph-ink-curves.json', publicSource), graphCurveJson)
await writeFile(new URL('interface-palette.json', publicSource), interfaceJson)
await writeFile(new URL('heatmap-palette.json', publicSource), heatmapJson)
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
await rm(new URL('generate-madoromi.mjs', publicSource), { force: true })
await writeFile(
  new URL('generate-madoromi.ts.txt', publicSource),
  await readFile(new URL(import.meta.url))
)
console.log(
  JSON.stringify(
    {
      variants: Object.keys(definitions).length,
      backgroundTint: TINT,
      brightFamilies: vividSwatches.length,
      brightSwatches: brightSwatches.length,
      adjustments,
    },
    null,
    2
  )
)
