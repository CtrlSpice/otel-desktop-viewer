# Checkout failure

Fixed OTLP/HTTP JSON requests based on the former seed script's checkout,
payment-decline, CPU and queue scenarios. No generator or Perl is required.

- Window: **2026-10-07 07:58–08:00:02 UTC**.
- Six spans across five services; the payment provider declines the charge.
- Three correlated logs: payment error, gateway error and low-stock warning.
- Three Metrics, seven datapoints and one exemplar linked to the gateway span.
- Trace ID: `4bf92f3577b34da6a3ce929d0e0e4736`.

From the repo root, with a viewer already running:

```sh
curl --fail-with-body -H 'Content-Type: application/json' \
  --data-binary @testdata/otlp/checkout/traces.json http://localhost:4318/v1/traces
curl --fail-with-body -H 'Content-Type: application/json' \
  --data-binary @testdata/otlp/checkout/logs.json http://localhost:4318/v1/logs
curl --fail-with-body -H 'Content-Type: application/json' \
  --data-binary @testdata/otlp/checkout/metrics.json http://localhost:4318/v1/metrics

otel-desktop-viewer trace 4bf92f3577b34da6a3ce929d0e0e4736
otel-desktop-viewer attributes values error.type --service payment-service \
  --start 2026-10-07T07:58:00Z --end 2026-10-07T08:00:02Z
```

Choose the fixed window or All in the viewer. Load once into a fresh store for
repeatable counts; re-sending logs or datapoints appends more records.
