# Chart Palettes

Charts use [Madoromi](https://madoromi.moonwit.ch/) inks.

- The top-bar **Season · Day/Night** controls default to Autumn and the system's
  Day/Night preference. Saved choices take priority. The eight interface themes
  use 20% of source background chroma and one shared six-role accent palette.
  Appearance changes do not reassign graph colours.
- The categorical catalogue has seven official shades from each of 32 inks:
  224 colours, omitting the palest eighth shade. New automatic assignments hash
  identities into 118 bright swatches, avoiding occupied slots where possible.
  Existing assignments and manual overrides retain their colours.
- Services, each metric's native series identities and single-service trace span
  names have separate saved ownership scopes. Filtering, sorting and visibility
  changes do not reassign their colours. Hovering or focusing a series header
  emphasizes its visible line independently of inspection and visibility.
- The colour dot beside a Home service name or a Gauge/Sum series opens the shared
  picker. It offers a colour field, hue strip, Hex input, seven ink-shade dots and
  Reset. Reset restores that owner's saved automatic assignment.
- Histogram and heatmap counts share a direct Murasaki-shikibu (Primary/Iris) →
  Fuyu-gaki (Rose) ramp. Adaptive ramps contain 1–12 bands; Day reverses Night's
  exact shades for 2–12 bands, while a singleton uses its mode's high-count endpoint. Seasons
  do not change the ramp.
- Percentile guides use Shin-ryoku shade 6 for p50, Daikokuten shade 2 for p95 and
  Benzaiten shade 2 for p99: green, amber-yellow and red. These chart swatches stay
  fixed across Day/Night and seasons; their labels use neutral text.
- Error markers retain the interface Error role, including on otherwise
  overridden service bars. Internal viewer faults use Papaya.

The generated palette data, source revision, role mappings and picker behaviour
are documented in the [theme reference](../desktopexporter/internal/frontend/src/themes/README.md).
Madoromi's source and full MPL-2.0 licence are distributed with the viewer.
