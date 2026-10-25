# Chart Palettes

Charts use [Madoromi](https://madoromi.moonwit.ch/) inks.

- The top-bar **Ink · Season · Day/Night** controls default to To-ro (warm
  lantern), Autumn and the system's Day/Night preference. Valid saved choices
  take priority. The eight interface themes use 20% of source background chroma;
  categorical graph colours remain fixed.
- Services and metric series use seven official shades from each of 32 inks:
  224 categorical colours, omitting the palest eighth shade. The colour dot beside a
  Home service name opens its override picker; Gauge/Sum series in Details use an
  edit icon. Hovering or focusing a series header emphasizes its visible chart line
  without changing selection, visibility or saved colours.
  Service colours are saved by service-name text; series colours by `metricRef`
  and `seriesRef`. Reset restores the saved automatic assignment. Other owners'
  reserved colours are disabled; crowded scopes offer curve sliders.
- Automatic service, metric-series and single-service trace span-name colours
  follow the selected interface ink's pinned
  [Madoromi Mix](https://madoromi.moonwit.ch/#palettes/mix/) relationships:
  analogous families first, an early complementary accent, then broader
  harmonies and the full catalogue. Families interleave before repeating shades.
  Switching inks repaints automatic colours; switching back restores that ink's
  saved assignments. Manual overrides stay fixed. Existing assignments seed the
  initially selected ink, and visited inks retain their saved reset slots.
  Services, each metric and span names have separate ownership scopes.
  The theme preview uses the same ranking for sample bars without allocating
  sample services or changing saved assignments.
- Histogram and heatmap counts share one app-wide ink, defaulting to To-ro.
  Adaptive ramps contain 1–12 bands; Day reverses Night's exact shades for 2–12
  bands, while a single band uses its mode's high-count endpoint. Seasons retain
  the selected ink. Selecting the top-bar ink updates the count ramp and
  legend; the chart picker can then choose a different count ink. Reloading
  restores both saved ink choices independently. The chart count-ink control
  leaves categorical colours unchanged; the interface ink updates automatic
  service, metric-series and single-service span-name assignments.
- Error markers use the seasonal theme's original Beni colour, including on
  otherwise overridden service bars. Internal viewer faults use Papaya.

The generated palette data, source revision, role mappings and picker behaviour
are documented in the [theme reference](../desktopexporter/internal/frontend/src/themes/README.md).
Madoromi's source and full MPL-2.0 licence are distributed with the viewer.
