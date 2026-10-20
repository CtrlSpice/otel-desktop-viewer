# Heatmap density preview

The same multicolour pattern at four densities, including an empty hole and a
detached island. Each size has an explicit histogram and an exponential histogram
with matching finite-bucket boundaries and counts. Exponential buckets use scale
4 and positive offset 0; their identities are not changed for display.

```sh
make populate-metrics OTLP_DATASET=testdata/otlp/heatmap-density
```

Select **All** in the time filter and service `heatmap-density`. Metric names start
with `preview.density.hist.` or `preview.density.exhist.`.

| Suffix | Time columns | Finite positive buckets |
| --- | ---: | ---: |
| `01_roomy` | 32 | 16 |
| `02_medium` | 80 | 32 |
| `03_dense` | 112 | 64 |
| `04_very_dense` | 160 | 96 |

Resize the chart or Details pane to compare both sides of the 12px threshold.
When either cell dimension falls below 12px, adjacent colours touch and only
the occupied area's perimeter is rounded. Dots switch from every boundary to
every second boundary on both axes. Empty buckets remain empty; edge dots and
interior dots all use 75% opacity.

The names describe relative density, not a forced rendering mode. Chart size
determines the mode. Each metric has one series; one load adds 768 datapoints
across eight metrics. Loading again appends datapoints.
