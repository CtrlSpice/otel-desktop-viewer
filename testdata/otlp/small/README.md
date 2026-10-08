# Small shared workload

Fixed, synthetic shop telemetry for development, integration checks and agent
investigations. These are ordinary OTLP JSON requests; loading needs no generator.

- 32 checkout requests across two regions and two observation periods.
- A payment decline, a comparable successful checkout, a slow provider call and
  unrelated payment failures, with associated logs.
- Optional log correlations, 32 distinct customers and exact 64-bit order values.
- Gauge, cumulative Sum, delta Histogram and cumulative ExponentialHistogram data.
- Every request uses supported OTLP; malformed-input tests are separate.

Select All or the manifest's fixed window. Use a fresh viewer when checking counts.

```sh
make populate-traces populate-logs populate-metrics OTLP_DATASET=testdata/otlp/small
```

This is the initial shared size profile. Medium and large will be sized after
measuring this workload; they have not been generated yet.
