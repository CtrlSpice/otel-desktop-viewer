# Rosé Pine themes and chart colours

## Ownership

- `rose-pine.json`: approved Dawn, Moon, and Pine interface colours and metadata.
- `series-palette.json`: individually selected series colours, shared across themes.
- `index.ts`: shared catalogue, palette assembly, and saved-theme resolution.
- `tokens.ts` / `plugin.ts`: generate DaisyUI, chart, and code-view CSS during
  the normal Tailwind build. No separate generation step or generated source file.
- `utils/chart-palette.ts`: stable series allocation and theme-aware density ramps.

The three-way theme toggle, charts, and generated CSS use this catalogue. Components use
semantic CSS variables. Papaya remains `#ff8000` for internal errors.

## Series colours

The 22-series cap is supported by 22 distinct colours, selected from the existing
Rosé Pine chart palette and exact published [Madoromi](https://madoromi.moonwit.ch/)
accents by samhain ([source](https://github.com/the-moonwitch/madoromi), MPL-2.0).
The shared set removes the five-step family ramps, very pale variations, and
peach/salmon near-neighbours. Blue, pink, green, purple, and yellow are interleaved.
Themes can share colours: a theme switch does not change series identity.

Trace identities allow golden/lemon but exclude the error-like pink/mauve family,
with lavender/bluebell additions giving 20 colours. Normal spans retain
distinct service/operation colours; errors use Love. Exact Love/error colours,
primary annotation colours, and papaya are not categorical series colours.

Metric-type badges use foam/info for Gauge, pine/secondary for Sum, rose
for Histogram, and gold/warning for ExpHist. Unspecified temporality uses error.
Sum and Gauge share the same series sequence, with
stable assignments for their lines/bars, legends, and sparklines. Histogram
series selectors are neutral because the distribution combines their data.

Heatmaps and histogram density use a separate surface-to-Love ramp for the active
theme, with identical count-legend swatches. Percentiles and genuine status
indicators retain success/warning/error colours.
