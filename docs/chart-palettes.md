# Chart Palettes

Charts use [Madoromi](https://madoromi.moonwit.ch/) inks.

- Services and metric series use seven official shades from each of 32 inks:
  224 categorical colours. Their exact colours stay the same across seasons and
  Day/Night. The colour picker identifies each ink and lets you choose a shade.
- Histogram and heatmap counts use a shared ink ramp. Day reverses Night's shades;
  the number of bands adapts to the counts. Selecting the top-bar ink updates this
  ramp, and the chart's ink picker can choose a different one.
- Error markers use the seasonal theme's Beni colour.

The generated palette data, source revision, role mappings and picker behaviour
are documented in the [theme reference](../desktopexporter/internal/frontend/src/themes/README.md).
Madoromi's source and full MPL-2.0 licence are distributed with the viewer.
