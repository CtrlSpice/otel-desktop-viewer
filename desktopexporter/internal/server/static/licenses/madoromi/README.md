# Madoromi themes

Source: https://github.com/the-moonwitch/madoromi

Project: https://madoromi.moonwit.ch/

Pinned revision:
[`f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695`](https://github.com/the-moonwitch/madoromi/tree/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695).

Madoromi is by [samhain / the-moonwitch](https://github.com/the-moonwitch).
Its code and themes are licensed under the Mozilla Public License 2.0; the
complete licence is in `LICENSE` and the attribution is in `NOTICE`.
The pinned upstream [README](https://github.com/the-moonwitch/madoromi/blob/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695/README.md#licence)
records the author and licence. Iroshizuku is a trademark of Pilot Corporation;
Madoromi is not affiliated with Pilot.

## Pinned source inputs

`palette.json` and `color.ts` are unmodified
files from that revision (`palettes/madoromi.json` and `src/core/color.ts`).
`mix.ts` and `palette.ts` are the unmodified `src/core/` mixer and palette builder
from the same revision.
`ink-curves.json` extracts all 40 unmodified 96-point OKLab ink curves from
`public/precomputed/summary.json` at the same revision, together with its ink
`order`. The picker preserves that full-ramp colour-similarity order after
omitting the excluded ink families.
`ink-names.json` extracts the English names from `LINEUP` in `src/inks.ts` at
that revision, preserving upstream wording. The picker places each English name
in parentheses after the original ink name.
`ink-ramps.json` extracts each ink's eight official `ramp` swatches from the
same pinned summary. These preserve the source's curve-specific sampling.

The source inputs, extracted names and measurements, adapted JSON and
Madoromi-derived generator remain covered by MPL-2.0. The surrounding project's
Apache-2.0 licence does not replace those terms.

## Viewer adaptation

`desktopexporter/internal/frontend/scripts/generate-madoromi.mjs` generates
`src/themes/madoromi.json`, `src/themes/count-palettes.json`,
`src/themes/count-palette-names.json`, `src/themes/graph-inks.json`,
`src/themes/graph-ink-curves.json`, `src/themes/interface-palette.json` and
`src/themes/heatmap-palette.json` from these files.
Run it with Node from the frontend directory after installing its existing dependencies:

```sh
node scripts/generate-madoromi.mjs
```

All eight variants (Spring, Summer, Autumn and Winter, each in Day and Night)
use 20% of the upstream chroma for the three paper colours and nijimi 0–2,
preserving their OKLCH lightness and hue.
Canvas/panel/chrome use kami/hyoshi/chitsu in both Day and Night, following the
pinned upstream [paper guide](https://github.com/the-moonwitch/madoromi/blob/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695/palettes/style-guide.md#paper-紙).
Hover uses nijimi 0; selection uses nijimi 2 at 45% over the panel, blended in
sRGB; stronger highlights use nijimi 1.

Body text uses sumi and labels use usuzumi 1. Text and accent
foregrounds move in 0.001 OKLCH lightness steps only as needed to reach 4.5:1
against all three surfaces, hover and selection. Subtle usuzumi 1, Muted kasure 1,
and the Sakura attribution heart retain their original colours without contrast
adjustments.

The interface uses a six-role arrangement inspired by Rosé Pine, with colours
derived from Madoromi's measured curves: Murasaki-shikibu for soft violet Iris,
Tsuki-yo for blue Pine, Shin-ryoku for mint-green Foam, To-ro for Gold, Fuyu-gaki for warm Rose
and Momiji for berry Love. Each source has one fixed sample hue and softened
chroma, with separate Day/Night lightness. The generator checks all four seasons'
five surfaces together, keeping identical accents across seasons within a mode.
These are viewer adaptations, not upstream mix output or Rosé Pine hex colours.
Primary uses Iris, metadata uses Rose, information uses Pine, success and accents
use Foam, warnings use Gold and errors use Love. Seasonal paper, neutral text and
highlights retain their existing roles. Appearance
controls select only season and Day/Night. Graph colours are independent of those
controls, and the main pane's footer retains visible Madoromi attribution.

The generated syntax palette follows the upstream code guide's role slots and
contains all fourteen named slots plus Sumi, Kasure 2 and Usuzumi 0. Slot names
map to the six shared ink roles. Each syntax foreground
uses the same shared readability correction. The upstream guide assigns Wakaba
to strings and Kaki to literals; the viewer uses Sumi for their value text and
JSON/query quote marks for calmer, cohesive reading. Keywords use Fuji, keys Ruri,
operators Sora and escapes Botan. These resolve to Iris, blue Pine, blue Pine
and Love respectively. Functions and annotations use Rose; types use Gold.
Built-ins and invalid syntax share Love.
Editor bracket outlines use original Kasure 0 and fills use tinted Nijimi 2.

Histogram and heatmap counts use one sequential violet → blue → green → gold
spectrum. Murasaki-shikibu, Tsuki-yo, Shin-ryoku and To-ro supply hue/chroma
anchors at OKLab lightness 0.48, 0.61, 0.74 and 0.87. The generator interpolates
in OKLab, maps into sRGB gamut and samples equal perceptual arc-length intervals
for each adaptive size from 2–12. Night orders shades dark to light; Day uses
the exact same shades reversed. A one-step ramp uses its mode's high-count
endpoint. Both perceived lightness and grayscale luminance remain ordered.
Seasons do not change the ramp.

The source catalogue
retains 32 colourful inks, excluding the four seasonal neutral sources
(Kiri-same, Fukagawa-nezu, Yama-guri and Fuyu-syogun),
plus Take-sumi, Hoteison, Syun-gyo and Tsukushi: the black, muted-green and
brown-dominant ramps. All 40 curves remain in the source distribution.
The source-only single-ink ramps sample the full measured lightness interval at equally spaced
OKLab L values, including both endpoints, interpolating a/b at each L and using
the pinned `oklabToHex` conversion. Night orders shades dark to light; Day uses
the exact same shades reversed. A one-step ramp uses the high-count endpoint
for its mode (lightest at Night, darkest by Day). Seasons do not change the ink.

The categorical catalogue contains the first seven official swatches from each
of those 32 inks. Their 224 colours keep the original hex values across themes.
Automatic graphs use all 118 swatches with OKLab lightness 0.55–0.87 and chroma
at least 0.10, visiting all 32 families before further shades. The palest eighth
swatch is omitted. This ordering is a viewer adaptation over unchanged source
colours. Home attribute-value bars also use official shades 3–5 of these inks,
selected by a stable typed-value hash.

There are no graph swatch or count-ink pickers. Existing saved overrides remain
readable, including curve positions sampled between official shades 1 and 7.
Automatic assignment preserves identity ownership and skips saved manual colours.

## Distributed licence and source

The frontend distributes the following files under `/licenses/madoromi/`,
alongside its executable assets. Start with `/licenses/madoromi/README.md`;
the directory URL itself is not a file index.

- `LICENSE`: complete, unmodified upstream MPL-2.0 licence.
- `NOTICE`, `README.md`: author attribution, pinned provenance and adaptation
  details.
- `palette.json`: unmodified upstream palette with eight seasonal variants.
- `ink-curves.json`, `ink-names.json`, `ink-ramps.json`: all 40 pinned ink curves,
  original-name keys, English names and official swatches.
- `color.ts.txt`, `palette.ts.txt`, `mix.ts.txt`: unmodified upstream conversion,
  palette builder and mixer sources.
- `generate-madoromi.mjs`: viewer adaptation generator.
- `themes.json`: generated `src/themes/madoromi.json`.
- `interface-palette.json`: the fixed interface palette across eight themes.
- `heatmap-palette.json`: ink-derived count spectrum anchors and adaptive ramps.
- `count-palettes.json`, `count-palette-names.json`: generated adaptive count
  ramps of sizes 1–12 and pinned English names.
- `graph-inks.json`, `graph-ink-curves.json`: official graph swatches, 224
  assignment slots and sampled slider curves.

These files provide the source form of the theme adaptation. The generator
copies the notices and source inputs from `third_party/madoromi/` into
`desktopexporter/internal/frontend/public/licenses/madoromi/`, together with its
own source and the adapted JSON. Vite copies those public files into the frontend
build; `make build-ts` copies that build into `desktopexporter/internal/server/static/`
for embedding and serving by the Go executable.

To use the distributed sources in a checkout, restore the `.ts.txt` downloads
to their original `.ts` names under `third_party/madoromi/`. Put the pinned input
JSON, `LICENSE`, `NOTICE` and `README.md` there, and place `generate-madoromi.mjs`
in the frontend's `scripts/` directory. Run the command above with the frontend
dependencies installed.
