# Rosé Pine chart accents

The interface uses its original Dawn, Moon, and Pine themes. Madoromi contributes
only five extra series colours per mode, stored in `chart-accents.json`.

These are exact published [Madoromi](https://madoromi.moonwit.ch/) values by
samhain ([source](https://github.com/the-moonwitch/madoromi), MPL-2.0).

| Role       | Dawn      | Moon / Pine | Madoromi source                             |
| ---------- | --------- | ----------- | ------------------------------------------- |
| Green      | `#618c32` | `#7bd6b6`   | Winter Day / Spring Night |
| Blue       | `#526ccb` | `#829eeb`   | Summer Day / Winter Night |
| Pink       | `#b25d9d` | `#d18bb7`   | Spring Day / Winter Night |
| Teal / sky | `#028e7f` | `#5ea3d4`   | Winter Day teal / Autumn Night sky blue     |
| Yellow     | `#e3940b` | `#ffc849`   | Winter Day yamabuki / Spring Night yamabuki |

## Fit with Rosé Pine

Selection used OKLCH lightness/chroma and comparison with Rosé Pine's existing
accents. Excluding yellow, Dawn additions have L 0.56–0.60 and C 0.10–0.15;
dark-mode additions have L 0.69–0.81 and C 0.10–0.12. They extend the subdued
green, blue, and pink families. The yellow adds a clear golden/butter tone alongside
Rosé Pine's existing gold; olive and mustard yellows are excluded.
The pink is cooler than Rosé Pine's warm rose and error red. These are categorical
colours, not status indicators; existing success/warning/error roles are retained.

The accents follow the first four original series colours, before subsequent
lightness variants. Metric palettes contain 25 distinct colours; traces use the
four cool additions for 24 distinct colours. Both
retain stable prefixes as visibility changes. Metrics reserve iris for
annotations; traces exclude the gold/orange family. Exact telemetry-error colours
and papaya `#ff8000` are never added as series colours.

Heatmap density is separate: Rosé Pine's surface-to-love ramp is used in all
three themes, including Dawn's own love shade. Heatmaps may use love; categorical
series do not. Count legends use the same scale as their cells and bars.
