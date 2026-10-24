# Madoromi themes

Source: https://github.com/the-moonwitch/madoromi

Pinned revision: `f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695`.

Madoromi is by [samhain / the-moonwitch](https://github.com/the-moonwitch).
Its code and themes are licensed under the Mozilla Public License 2.0; the
complete licence is in `LICENSE`. `palette.json` and `color.ts` are unmodified
files from that revision (`palettes/madoromi.json` and `src/core/color.ts`).
`ink-curves.json` extracts all 40 unmodified 96-point OKLab ink curves from
`public/precomputed/summary.json` at the same revision, together with its ink
`order`. The picker preserves that full-ramp colour-similarity order after
omitting the excluded ink families.
`ink-names.json` extracts the English names from `LINEUP` in `src/inks.ts` at
that revision, preserving upstream wording. The picker places each English name
in parentheses after the original ink name.

The viewer's adapted theme data remains covered by MPL-2.0. The project's
Apache-2.0 licence is unchanged.

## Viewer adaptation

`desktopexporter/internal/frontend/scripts/generate-madoromi.mjs` generates
`src/themes/madoromi.json` and `src/themes/count-palettes.json` from these files.
Run it with Node from the frontend directory after installing its existing dependencies:

```sh
node scripts/generate-madoromi.mjs
```

All eight seasonal day/night variants use 20% of the upstream chroma for the
three paper colours and nijimi 0–2, preserving their OKLCH lightness and hue.
Day canvas/panel/chrome use kami/hyoshi/chitsu. Night reverses the outer two.
Hover uses nijimi 0; selection uses nijimi 2 at 55% over the panel, blended in
sRGB; stronger highlights use nijimi 1.

Body text uses sumi, labels use usuzumi 1, primary uses fuji, metadata uses ruri,
accent uses tokiwa, info uses sora and success uses wakaba. Text and accent
foregrounds move in 0.001 OKLCH lightness steps only as needed to reach 4.5:1
against all three surfaces, hover and selection. Subtle usuzumi 1, Muted kasure 1,
sakura and yamabuki warning fills retain their original colours without contrast
adjustments. Error uses the Momiji
curve in all eight variants, sampled at each variant's original beni lightness.
Interpolation between stored OKLab points follows upstream `colourAt` in
`src/core/palette.ts`, with the pinned conversion code handling the sRGB gamut.

The generated syntax palette follows the upstream code guide and contains all
fourteen named accents plus Sumi, Kasure 2 and Usuzumi 0. Each syntax foreground
uses the same shared readability correction. The upstream guide assigns Wakaba
to strings and Kaki to literals; the viewer uses Sumi for their value text and
reserves Wakaba for JSON/query quote marks. Keywords use Fuji, keys Ruri,
operators Sora and escapes Botan. Syntax Beni is the
native built-in colour; invalid syntax uses the viewer's Momiji Error colour.
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

The frontend distributes the full licence, these inputs, the generator and the
adapted JSON at `/licenses/madoromi/`. Those files are the source form of the
theme adaptation. The unmodified conversion source is served as `color.ts.txt`
for download; its repository filename is `color.ts`. Categorical graph palettes
are managed separately.
