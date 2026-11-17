# Heatmap playground

The synthetic distributions used to design and profile the blob heatmaps.
These fixed OTLP payloads preserve the samples shown in the preview.

```sh
make populate-metrics OTLP_DATASET=testdata/otlp/heatmap-playground
```

Select **All** in the time filter and look for service `heatmap-playground`.
Each distribution has `preview.hist.*` and `preview.exhist.*` metrics made
from the same observations, with three route series: `/checkout`, `/catalog`,
and `/search`. Both types use delta temporality and milliseconds.

| Distribution | Time slots | Shape |
| --- | ---: | --- |
| `moving_peak` | 48 | Tight cluster moving across bucket boundaries |
| `two_peaks` | 96 | Fast and slow populations exchanging dominance |
| `long_tail` | 128 | Dense core, changing heavy tail, rare outliers |
| `bursts_and_gaps` | 96 | Traffic bursts and slowdowns; 13 missing slots |
| `sparse_spikes` | 160 | Sparse traffic, exact zeros, isolated latency spikes |
| `busy_mixture` | 128 | Three moving populations with changing spread |

Slots are ten seconds apart. The files contain 12 metrics, 36 series, and
3,858 datapoints. Rendering may merge time slots according to the viewer's
query resolution. Missing time slots remain absent from the source data.

The explicit histogram has 25 bounds; exponential histograms use scale 2
and a 0.25 ms zero threshold. Count, sum, minimum, and maximum agree between
each paired datapoint. Bucket counts sum to the recorded count.

Load once into a fresh store for reproducible counts. Reloading appends data.
