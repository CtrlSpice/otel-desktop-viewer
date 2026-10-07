# Shared OTLP datasets

These are ordinary OTLP/HTTP JSON request bodies. Development commands, tests and
evals can load the same files. IDs, exact integer values and correlations are
defined in the data rather than regenerated at runtime.

| Dataset | Contents | Stored records after one load |
| --- | --- | --- |
| `demo/` | Captured synthetic dev data: multi-service traces, errors, deep/orphan traces, scalar and histogram Metric series, logs | 101 spans, 23 logs, 9,901 datapoints |
| `checkout/` | Small checkout failure with correlated logs and a Metric exemplar | 6 spans, 3 logs, 7 datapoints |
| `usability-pilot/` | Original six-task evaluation fixture, with typed values, a 31-value inventory and child-time boundaries | 33 spans, 7 logs, 2 datapoints |

`demo/manifest.json` lists the requests and capture time bounds. The demo was
exported through the viewer's SQL CLI from its stored OTLP reconstruction. It
preserves retained data, not original HTTP request bytes or rejected duplicate
spans. `checkout/README.md` describes the smaller scenario and curl commands.

```sh
make populate-traces populate-logs populate-metrics
make populate-traces populate-logs populate-metrics OTLP_DATASET=testdata/otlp/checkout
```

- `OTLP_ENDPOINT` selects the receiver (default `http://localhost:4318`).
- Files are sent unchanged; timestamps are fixed. Select All or the recorded
  window. No timestamp rewriting or randomized IDs occurs during loading.
- Load once into a fresh store when asserting counts. Re-sending logs/datapoints
  appends records; duplicate span identities may be rejected.
- Keep JSON indented with two spaces. Preserve exact decimal-string integers,
  timestamp presence and negative zero; avoid floating-point conversion when
  editing or formatting received values.
- Keep eval prompts and answer keys in the eval suite, not in telemetry attributes.

Run `go test ./internal/testdataset` to validate OTLP decoding, fixture formatting,
record counts and checkout correlations.
