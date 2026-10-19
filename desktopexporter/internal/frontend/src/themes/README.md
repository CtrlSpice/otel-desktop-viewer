# Madoromi experiment

Seasonal day/night colours from [Madoromi](https://madoromi.moonwit.ch/)
by samhain, [source](https://github.com/the-moonwitch/madoromi), MPL-2.0.
The upstream colours are measured from Pilot Iroshizuku ink scans.

Use semantic names in components. `madoromi.css` maps these roles to existing
DaisyUI tokens so the experiment also covers existing controls.

| Role                | CSS token                               | Upstream source                                     |
| ------------------- | --------------------------------------- | --------------------------------------------------- |
| Background          | `--surface-canvas`                      | kami by day; chitsu by night, per viewer preference |
| Secondary panels    | `--surface-panel`                       | hyōshi                                              |
| Outer chrome        | `--surface-chrome`                      | chitsu by day; kami by night                        |
| Hover / selection   | `--surface-hover`, `--surface-selected` | diluted nijimi                                      |
| Borders             | `--border-default`, `--border-strong`   | nijimi 1 / kasure 0                                 |
| Main text           | `--text-primary`                        | sumi                                                |
| Secondary text      | `--text-secondary`                      | usuzumi 1                                           |
| Muted text          | `--text-muted`                          | kasure 2                                            |
| Disabled text       | `--text-disabled`                       | kasure 1                                            |
| Accent / focus      | `--action-accent`, `--action-focus`     | fuji                                                |
| Links / information | `--action-link`, `--status-info`        | sora                                                |
| Success             | `--status-success`                      | wakaba                                              |
| Warning             | `--status-warning`                      | yamabuki, exact published value                     |
| Telemetry error     | `--status-error`                        | beni                                                |
| Internal error      | `--status-internal-error`               | existing papaya, #ff8000                            |

## Graph colours

`chart-series.json` names ten source colours: **chart.series.01–10**.
The slots are stable when series visibility changes. The brush selects a season;
the adjacent day/night button selects its mode. Both choices persist separately.
Metrics and traces use the same pool; error spans keep the error token.

All graph swatches are exact published accents. Line charts start with the
selected season, then use other seasons of the same day/night mode to fill 24
distinct slots. The existing 22-series visibility cap is unchanged.
Histograms and heatmaps use only the selected season's metric-type accent:
peony for Histogram and kerria gold for ExponentialHistogram. Their density ramp
runs from the selected season's canvas to that accent.
The source is `palettes/madoromi.json` in the upstream repo.
The metric-type adapter starts Sum on indigo, Gauge on sky, Histogram on peony,
and ExponentialHistogram on kerria gold. These are identity colours, not statuses.
Error and persimmon accents are excluded; papaya is never a series colour.
No graph uses error red or papaya as an identity or density colour.
