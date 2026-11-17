# Gauge gaps

`demo.gauge.gaps` contains 653 samples from three hosts, every five seconds from
2026-11-07 12:00 through 12:20 UTC:

| Host | Readings |
| --- | --- |
| `continuous` | Uninterrupted low-valued comparison line |
| `long-outage` | No samples from 12:05 up to 12:08; real zeros from 12:11 through 12:12 |
| `short-outages` | No samples during 12:02–12:02:30, 12:10–12:10:20, and 12:16–12:18 (endpoints exclusive) |

```sh
make populate-metrics OTLP_DATASET=testdata/otlp/gauge-gaps
```

In Metrics, open `demo.gauge.gaps`, select **All** time and **Raw**. The three
lines show each host's outages. Sparklines simplify those lines into rough shape
previews for choosing a series; short outages may disappear. The zero interval
remains connected. Hover over a main-chart gap to see that it has no reading;
use keyboard navigation to inspect the real samples at either side.

Gap detection follows the chart's M4 buckets, so changing the time window can
change which short gaps qualify. Timestamps are fixed. Load once to avoid
appending duplicate samples. Regenerate the JSON with `python3
testdata/otlp/gauge-gaps/generate.py`.
