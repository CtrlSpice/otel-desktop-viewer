# Madoromi themes

Source: https://github.com/the-moonwitch/madoromi

Pinned revision: `f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695`.

Madoromi is by [samhain / the-moonwitch](https://github.com/the-moonwitch).
Its code and themes are licensed under the Mozilla Public License 2.0; the
complete licence is in `LICENSE`. `palette.json` and `color.ts` are unmodified
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

The viewer's adapted theme data remains covered by MPL-2.0. The project's
Apache-2.0 licence is unchanged.

## Viewer adaptation

`desktopexporter/internal/frontend/scripts/generate-madoromi.mjs` generates
`src/themes/madoromi.json`, `src/themes/count-palettes.json`,
`src/themes/count-palette-names.json`, `src/themes/graph-inks.json` and
`src/themes/graph-ink-curves.json` and `src/themes/accent-inks.json` from these files.
Run it with Node from the frontend directory after installing its existing dependencies:

```sh
node scripts/generate-madoromi.mjs
```

All eight seasonal day/night variants use 20% of the upstream chroma for the
three paper colours and nijimi 0–2, preserving their OKLCH lightness and hue.
Day canvas/panel/chrome use kami/hyoshi/chitsu. Night reverses the outer two.
Hover uses nijimi 0; selection uses nijimi 2 at 45% over the panel, blended in
sRGB; stronger highlights use nijimi 1.

Body text uses sumi, labels use usuzumi 1, primary uses the fuji slot, metadata uses
ruri, accent uses tokiwa, info uses sora and success uses wakaba. Text and accent
foregrounds move in 0.001 OKLCH lightness steps only as needed to reach 4.5:1
against all three surfaces, hover and selection. Subtle usuzumi 1, Muted kasure 1,
sakura and yamabuki warning fills retain their original colours without contrast
adjustments. Error uses each variant's original Beni colour from the pinned
upstream palette.

The global Accent ink picker runs the unmodified upstream analogous mixer for
each of the 32 seed inks. Ajisai is the default; its family adds Ama-iro and
Murasaki-shikibu. The mixed slots supply Primary, Secondary, Accent and the
fourteen syntax accents. Seasonal paper, text, highlights and semantic status
colours retain their existing roles. Each mixed foreground receives the same
readability adjustment against the viewer's five seasonal surfaces. The choice
is independent of graph ownership and count palettes.
The Accent ink picker previews the named seed's seven official graph swatches,
not colours from its neighbouring inks or mixed syntax roles.

The generated syntax palette follows the upstream code guide's role slots and
contains all fourteen named slots plus Sumi, Kasure 2 and Usuzumi 0. Slot names
refer to the selected analogous mix rather than fixed rainbow hues. Each syntax foreground
uses the same shared readability correction. The upstream guide assigns Wakaba
to strings and Kaki to literals; the viewer uses Sumi for their value text and
JSON/query quote marks for calmer, cohesive reading. Keywords use Fuji, keys Ruri,
operators Sora and escapes Botan. Syntax Beni is the
native built-in colour; invalid syntax uses the semantic Beni Error colour.
Editor bracket outlines use original Kasure 0 and fills use tinted Nijimi 2.

Histogram count palettes offer 32 colourful inks. The picker excludes the four
seasonal neutral sources (Kiri-same, Fukagawa-nezu, Yama-guri and Fuyu-syogun),
plus Take-sumi, Hoteison, Syun-gyo and Tsukushi: the black, muted-green and
brown-dominant ramps. All 40 curves remain in the source distribution.
Each adaptive
size from 2 to 12 samples the full measured lightness interval at equally spaced
OKLab L values, including both endpoints, interpolating a/b at each L and using
the pinned `oklabToHex` conversion. Night orders shades dark to light; Day uses
the exact same shades reversed. A one-step ramp uses the high-count endpoint
for its mode (lightest at Night, darkest by Day). Seasons do not change the ink.

Categorical graphs and their per-service/per-series override picker use the
first seven official swatches from each of those 32 inks. Their 224 colours keep
the original hex values across themes. The palest eighth swatch is omitted;
shades also used by the UI are permitted. The searchable seven-dot picker keeps
the ink's original name and English name, and visibly links to Madoromi.

Taken shades are disabled within the owner scope, including hidden owners and
their saved reset colours. With fewer than 32 unreserved presets, per-ink
sliders sample 1,001 positions between official shades 1 and 7. The seven preset
positions retain exact source colours; other positions interpolate the original
curve at equally spaced OKLab lightness values. Sliders skip occupied hex colours
and save the ink/position identity without changing automatic assignments.

The frontend distributes the full licence, these inputs, the generator and the
adapted JSON at `/licenses/madoromi/`. Those files are the source form of the
theme adaptation. The unmodified conversion, palette builder and mixer sources
are served as `color.ts.txt`, `palette.ts.txt` and `mix.ts.txt` for download; their
repository filenames use the `.ts` extension.
