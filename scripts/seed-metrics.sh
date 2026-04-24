#!/usr/bin/env bash
#
# seed-metrics.sh — Send sample metrics to an OTLP HTTP receiver.
#
# Sends Gauge, Sum, Histogram, and ExponentialHistogram metrics across multiple
# services with realistic names, units, multiple datapoints over time, and exemplars.
#
# Usage:
#   ./scripts/seed-metrics.sh                                # defaults to localhost:4318
#   OTLP_ENDPOINT=http://host:4318 ./scripts/seed-metrics.sh
#
# Requires: bash, curl, uuidgen.

set -euo pipefail

ENDPOINT="${OTLP_ENDPOINT:-http://localhost:4318}"
URL="${ENDPOINT}/v1/metrics"

now_s=$(date +%s)
now_ns=$(( now_s * 1000000000 ))
sec_ns=$(( 1000000000 ))
min_ns=$(( 60 * sec_ns ))

uuid_hex32() {
  uuidgen | tr -d '\n' | tr '[:upper:]' '[:lower:]' | tr -d '-'
}
uuid_hex16() {
  local u; u=$(uuid_hex32); echo "${u:0:16}"
}

post_metrics_json() {
  local tmpfile="$1"
  curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${URL}" \
    -H 'Content-Type: application/json' \
    --data-binary "@${tmpfile}"
}

# ── Gauge metric: single numeric value at each timestamp ──
# args: service name unit description [datapoints_json]
send_gauge() {
  local service="$1" name="$2" unit="$3" desc="$4" dps="$5"

  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceMetrics": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } },
        { "key": "deployment.environment", "value": { "stringValue": "production" } }
      ]
    },
    "scopeMetrics": [{
      "scope": { "name": "${service}.meter", "version": "0.1.0" },
      "metrics": [{
        "name": "${name}",
        "description": "${desc}",
        "unit": "${unit}",
        "gauge": {
          "dataPoints": [${dps}]
        }
      }]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_metrics_json "$tmpfile")
  printf '  %-42s Gauge     %s\n' "${service}/${name}" "$http_code"
}

# ── Sum metric: monotonic counter ──
send_sum() {
  local service="$1" name="$2" unit="$3" desc="$4" dps="$5"
  local monotonic="${6:-true}"
  local temporality="${7:-2}"

  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceMetrics": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } },
        { "key": "deployment.environment", "value": { "stringValue": "production" } }
      ]
    },
    "scopeMetrics": [{
      "scope": { "name": "${service}.meter", "version": "0.1.0" },
      "metrics": [{
        "name": "${name}",
        "description": "${desc}",
        "unit": "${unit}",
        "sum": {
          "dataPoints": [${dps}],
          "aggregationTemporality": ${temporality},
          "isMonotonic": ${monotonic}
        }
      }]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_metrics_json "$tmpfile")
  printf '  %-42s Sum       %s\n' "${service}/${name}" "$http_code"
}

# ── Histogram metric ──
send_histogram() {
  local service="$1" name="$2" unit="$3" desc="$4" dps="$5"

  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceMetrics": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } },
        { "key": "deployment.environment", "value": { "stringValue": "production" } }
      ]
    },
    "scopeMetrics": [{
      "scope": { "name": "${service}.meter", "version": "0.1.0" },
      "metrics": [{
        "name": "${name}",
        "description": "${desc}",
        "unit": "${unit}",
        "histogram": {
          "dataPoints": [${dps}],
          "aggregationTemporality": 2
        }
      }]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_metrics_json "$tmpfile")
  printf '  %-42s Histogram %s\n' "${service}/${name}" "$http_code"
}

# ── Exponential Histogram metric ──
send_exp_histogram() {
  local service="$1" name="$2" unit="$3" desc="$4" dps="$5"

  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceMetrics": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } },
        { "key": "deployment.environment", "value": { "stringValue": "production" } }
      ]
    },
    "scopeMetrics": [{
      "scope": { "name": "${service}.meter", "version": "0.1.0" },
      "metrics": [{
        "name": "${name}",
        "description": "${desc}",
        "unit": "${unit}",
        "exponentialHistogram": {
          "dataPoints": [${dps}],
          "aggregationTemporality": 2
        }
      }]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_metrics_json "$tmpfile")
  printf '  %-42s ExpHist   %s\n' "${service}/${name}" "$http_code"
}

# Helper: build a gauge/sum datapoint JSON
# args: minutes_ago value [attributes_json] [exemplar_trace_id]
dp_gauge() {
  local mins_ago="$1" value="$2"
  local attrs="${3:-}"
  local ex_tid="${4:-}"
  local ts=$(( now_ns - mins_ago * min_ns ))
  local start_ts=$(( ts - 60 * sec_ns ))
  local attr_json="[]"
  [ -n "$attrs" ] && attr_json="[${attrs}]"
  local exemplars=""
  if [ -n "$ex_tid" ]; then
    local ex_sid; ex_sid=$(uuid_hex16)
    exemplars=", \"exemplars\": [{
      \"timeUnixNano\": \"${ts}\",
      \"asDouble\": ${value},
      \"traceId\": \"${ex_tid}\",
      \"spanId\": \"${ex_sid}\",
      \"filteredAttributes\": []
    }]"
  fi
  cat <<DP
{
  "startTimeUnixNano": "${start_ts}",
  "timeUnixNano": "${ts}",
  "asDouble": ${value},
  "attributes": ${attr_json}${exemplars}
}
DP
}

echo "Sending sample metrics to ${URL} …"
echo

# ============================================================================
# Gauge metrics — snapshots of current values
# ============================================================================

echo "Gauge metrics …"

# CPU usage across time (8 datapoints spanning ~30 min)
send_gauge "api-gateway" "system.cpu.utilization" "1" \
  "CPU utilization as a fraction of total capacity" \
  "$(dp_gauge 28 0.42),
   $(dp_gauge 24 0.51),
   $(dp_gauge 20 0.68),
   $(dp_gauge 16 0.73),
   $(dp_gauge 12 0.65),
   $(dp_gauge 8  0.58),
   $(dp_gauge 4  0.71),
   $(dp_gauge 0  0.62)"

# Memory usage
send_gauge "api-gateway" "process.runtime.jvm.memory.usage" "By" \
  "JVM heap memory currently in use" \
  "$(dp_gauge 25 524288000),
   $(dp_gauge 20 612745216),
   $(dp_gauge 15 498073600),
   $(dp_gauge 10 701890560),
   $(dp_gauge 5  658505728),
   $(dp_gauge 0  589824000)"

# Active connections
send_gauge "order-service" "db.client.connections.usage" "{connections}" \
  "Number of active database connections" \
  "$(dp_gauge 20 12 '{"key":"db.name","value":{"stringValue":"orders_primary"}}'),
   $(dp_gauge 15 18 '{"key":"db.name","value":{"stringValue":"orders_primary"}}'),
   $(dp_gauge 10 24 '{"key":"db.name","value":{"stringValue":"orders_primary"}}'),
   $(dp_gauge 5  31 '{"key":"db.name","value":{"stringValue":"orders_primary"}}'),
   $(dp_gauge 0  22 '{"key":"db.name","value":{"stringValue":"orders_primary"}}')"

# Queue depth
send_gauge "notification-service" "messaging.queue.depth" "{messages}" \
  "Number of messages waiting in the notification queue" \
  "$(dp_gauge 20 45),
   $(dp_gauge 15 120),
   $(dp_gauge 10 89),
   $(dp_gauge 5  210),
   $(dp_gauge 0  67)"

# Thread count
send_gauge "payment-service" "process.runtime.jvm.threads.count" "{threads}" \
  "Current number of JVM threads" \
  "$(dp_gauge 15 42),
   $(dp_gauge 10 48),
   $(dp_gauge 5  51),
   $(dp_gauge 0  46)"

echo

# ============================================================================
# Sum metrics — monotonic counters
# ============================================================================

echo "Sum metrics …"

# HTTP request count (with exemplar linking to a trace)
ex_trace1=$(uuid_hex32)
ex_trace2=$(uuid_hex32)
send_sum "api-gateway" "http.server.request.count" "{requests}" \
  "Total number of HTTP requests received" \
  "$(dp_gauge 20 14200 '{"key":"http.method","value":{"stringValue":"GET"}}'),
   $(dp_gauge 15 18450 '{"key":"http.method","value":{"stringValue":"GET"}}'),
   $(dp_gauge 10 23100 '{"key":"http.method","value":{"stringValue":"GET"}}' "$ex_trace1"),
   $(dp_gauge 5  28700 '{"key":"http.method","value":{"stringValue":"GET"}}'),
   $(dp_gauge 0  34520 '{"key":"http.method","value":{"stringValue":"GET"}}' "$ex_trace2")"

# Error count
send_sum "payment-service" "http.server.error.count" "{errors}" \
  "Total number of 5xx responses" \
  "$(dp_gauge 20 12),
   $(dp_gauge 15 15),
   $(dp_gauge 10 23),
   $(dp_gauge 5  31),
   $(dp_gauge 0  38)"

# Bytes sent
send_sum "catalog-service" "http.server.response.body.size" "By" \
  "Total bytes sent in HTTP responses" \
  "$(dp_gauge 20 104857600),
   $(dp_gauge 15 157286400),
   $(dp_gauge 10 209715200),
   $(dp_gauge 5  262144000),
   $(dp_gauge 0  335544320)"

# Database queries
send_sum "order-service" "db.client.queries.count" "{queries}" \
  "Total SQL queries executed" \
  "$(dp_gauge 20 8420),
   $(dp_gauge 15 12890),
   $(dp_gauge 10 17340),
   $(dp_gauge 5  22100),
   $(dp_gauge 0  27650)"

# Cache hits
send_sum "user-service" "cache.hit.count" "{hits}" \
  "Total cache lookup hits" \
  "$(dp_gauge 15 5200),
   $(dp_gauge 10 7800),
   $(dp_gauge 5  10400),
   $(dp_gauge 0  13100)" \
  "true" "2"

# Cache misses
send_sum "user-service" "cache.miss.count" "{misses}" \
  "Total cache lookup misses" \
  "$(dp_gauge 15 820),
   $(dp_gauge 10 1100),
   $(dp_gauge 5  1350),
   $(dp_gauge 0  1640)" \
  "true" "2"

echo

# ============================================================================
# Histogram metrics — value distributions with explicit bucket bounds
#
# For heatmap visualization we need many timestamps across a time range,
# with multiple streams (attribute sets) per metric. Each call to
# send_histogram sends one metric with all its datapoints.
# ============================================================================

echo "Histogram metrics …"

# Helper: build one histogram datapoint JSON fragment.
# args: timestamp_ns start_timestamp_ns bucket_counts_json attributes_json
#       count sum min max
dp_hist() {
  local ts="$1" start_ts="$2" bcounts="$3" attrs="$4"
  local count="$5" sum="$6" min_v="$7" max_v="$8"
  cat <<DP
{
  "startTimeUnixNano": "${start_ts}",
  "timeUnixNano": "${ts}",
  "count": "${count}",
  "sum": ${sum},
  "min": ${min_v},
  "max": ${max_v},
  "bucketCounts": ${bcounts},
  "explicitBounds": [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0],
  "attributes": ${attrs}
}
DP
}

# HTTP request duration — 30 timestamps, 3 routes (streams), Delta temporality.
# Simulates a realistic traffic pattern: /orders has a latency spike mid-range,
# /products stays fast, /users climbs toward the tail.
hist_dps=""
for i in $(seq 0 29); do
  ts_i=$(( now_ns - (59 - i * 2) * min_ns ))
  start_i=$(( ts_i - 2 * min_ns ))

  # Stream 1: /api/v2/orders — spike around i=12..18
  if [ $i -ge 12 ] && [ $i -le 18 ]; then
    c1="[\"20\",\"40\",\"60\",\"120\",\"200\",\"180\",\"90\",\"45\",\"15\",\"5\",\"2\"]"
    n1=777; s1=388.5; mn1=0.003; mx1=4.2
  else
    c1="[\"80\",\"150\",\"200\",\"160\",\"100\",\"40\",\"15\",\"5\",\"2\",\"1\",\"0\"]"
    n1=753; s1=188.25; mn1=0.002; mx1=2.1
  fi
  a1='[{"key":"http.method","value":{"stringValue":"GET"}},{"key":"http.route","value":{"stringValue":"/api/v2/orders"}}]'
  d1=$(dp_hist "$ts_i" "$start_i" "$c1" "$a1" "$n1" "$s1" "$mn1" "$mx1")

  # Stream 2: /api/v2/products — consistently fast
  c2="[\"120\",\"220\",\"310\",\"180\",\"80\",\"25\",\"8\",\"3\",\"1\",\"0\",\"0\"]"
  a2='[{"key":"http.method","value":{"stringValue":"GET"}},{"key":"http.route","value":{"stringValue":"/api/v2/products"}}]'
  d2=$(dp_hist "$ts_i" "$start_i" "$c2" "$a2" 947 142.05 0.001 1.2)

  # Stream 3: /api/v2/users — gradually shifting right
  shift_val=$(( i / 5 ))
  case $shift_val in
    0) c3="[\"100\",\"180\",\"250\",\"150\",\"70\",\"20\",\"5\",\"2\",\"0\",\"0\",\"0\"]" ;;
    1) c3="[\"60\",\"140\",\"220\",\"190\",\"100\",\"40\",\"15\",\"5\",\"2\",\"0\",\"0\"]" ;;
    2) c3="[\"30\",\"90\",\"170\",\"200\",\"150\",\"80\",\"35\",\"12\",\"4\",\"1\",\"0\"]" ;;
    3) c3="[\"15\",\"50\",\"120\",\"180\",\"170\",\"120\",\"60\",\"25\",\"8\",\"3\",\"1\"]" ;;
    *) c3="[\"10\",\"30\",\"80\",\"150\",\"190\",\"160\",\"90\",\"40\",\"15\",\"5\",\"2\"]" ;;
  esac
  a3='[{"key":"http.method","value":{"stringValue":"GET"}},{"key":"http.route","value":{"stringValue":"/api/v2/users"}}]'
  d3=$(dp_hist "$ts_i" "$start_i" "$c3" "$a3" 772 231.6 0.002 3.5)

  [ -n "$hist_dps" ] && hist_dps="${hist_dps},"
  hist_dps="${hist_dps}${d1},${d2},${d3}"
done

send_histogram "api-gateway" "http.server.request.duration" "s" \
  "Duration of inbound HTTP requests" \
  "${hist_dps}"

# DB query duration — 30 timestamps, 2 streams (SELECT vs INSERT)
hist_dps2=""
for i in $(seq 0 29); do
  ts_i=$(( now_ns - (59 - i * 2) * min_ns ))
  start_i=$(( ts_i - 2 * min_ns ))

  c_sel="[\"200\",\"350\",\"180\",\"90\",\"40\",\"15\",\"6\",\"3\",\"1\",\"0\"]"
  a_sel='[{"key":"db.system","value":{"stringValue":"postgresql"}},{"key":"db.operation","value":{"stringValue":"SELECT"}}]'
  d_sel=$(dp_hist "$ts_i" "$start_i" "$c_sel" "$a_sel" 885 13275.0 0.5 420.0)

  c_ins="[\"50\",\"120\",\"200\",\"150\",\"80\",\"35\",\"15\",\"8\",\"3\",\"1\"]"
  a_ins='[{"key":"db.system","value":{"stringValue":"postgresql"}},{"key":"db.operation","value":{"stringValue":"INSERT"}}]'
  d_ins=$(dp_hist "$ts_i" "$start_i" "$c_ins" "$a_ins" 662 16550.0 1.2 680.0)

  [ -n "$hist_dps2" ] && hist_dps2="${hist_dps2},"
  hist_dps2="${hist_dps2}${d_sel},${d_ins}"
done

send_histogram "order-service" "db.client.query.duration" "ms" \
  "Duration of database queries" \
  "${hist_dps2}"

echo

# ============================================================================
# Exponential Histogram metrics — high-resolution distributions
#
# Same multi-timestamp, multi-stream pattern as above.
# ============================================================================

echo "Exponential histogram metrics …"

# Helper: build one exp-histogram datapoint JSON fragment.
# args: timestamp_ns start_timestamp_ns scale pos_offset pos_counts_json
#       zero_count attributes_json count sum min max
dp_exphist() {
  local ts="$1" start_ts="$2" scale="$3" pos_off="$4" pos_counts="$5"
  local zero_count="$6" attrs="$7"
  local count="$8" sum="$9" min_v="${10}" max_v="${11}"
  cat <<DP
{
  "startTimeUnixNano": "${start_ts}",
  "timeUnixNano": "${ts}",
  "count": "${count}",
  "sum": ${sum},
  "min": ${min_v},
  "max": ${max_v},
  "scale": ${scale},
  "zeroCount": "${zero_count}",
  "positive": {
    "offset": ${pos_off},
    "bucketCounts": ${pos_counts}
  },
  "negative": {
    "offset": 0,
    "bucketCounts": []
  },
  "attributes": ${attrs}
}
DP
}

# Payment processing duration — 30 timestamps, 3 providers
exphist_dps=""
for i in $(seq 0 29); do
  ts_i=$(( now_ns - (59 - i * 2) * min_ns ))
  start_i=$(( ts_i - 2 * min_ns ))

  # Stream 1: stripe/card — normal distribution around bucket 15-16
  p1='["12","28","65","142","178","89","42","21","10","4","1"]'
  a1='[{"key":"payment.provider","value":{"stringValue":"stripe"}},{"key":"payment.method","value":{"stringValue":"card"}}]'
  d1=$(dp_exphist "$ts_i" "$start_i" 3 12 "$p1" 0 "$a1" 592 148000.0 45.0 2800.0)

  # Stream 2: stripe/wallet — faster, tighter distribution
  p2='["30","95","180","120","45","15","5","2"]'
  a2='[{"key":"payment.provider","value":{"stringValue":"stripe"}},{"key":"payment.method","value":{"stringValue":"wallet"}}]'
  d2=$(dp_exphist "$ts_i" "$start_i" 3 10 "$p2" 0 "$a2" 492 49200.0 22.0 900.0)

  # Stream 3: paypal/card — slower, wider tail; spike around i=15..22
  if [ $i -ge 15 ] && [ $i -le 22 ]; then
    p3='["5","15","35","80","120","95","60","35","20","12","6","3","1"]'
    n3=487; s3=195000.0; mn3=80.0; mx3=5200.0
  else
    p3='["8","22","55","110","140","80","40","18","8","3","1"]'
    n3=485; s3=145500.0; mn3=60.0; mx3=3500.0
  fi
  a3='[{"key":"payment.provider","value":{"stringValue":"paypal"}},{"key":"payment.method","value":{"stringValue":"card"}}]'
  d3=$(dp_exphist "$ts_i" "$start_i" 3 13 "$p3" 0 "$a3" "$n3" "$s3" "$mn3" "$mx3")

  [ -n "$exphist_dps" ] && exphist_dps="${exphist_dps},"
  exphist_dps="${exphist_dps}${d1},${d2},${d3}"
done

send_exp_histogram "payment-service" "payment.processing.duration" "ms" \
  "Time to process a payment transaction end-to-end" \
  "${exphist_dps}"

# Auth token validation — 30 timestamps, 2 methods
exphist_dps2=""
for i in $(seq 0 29); do
  ts_i=$(( now_ns - (59 - i * 2) * min_ns ))
  start_i=$(( ts_i - 2 * min_ns ))

  p_jwt='["400","1200","2100","1800","600","180","50","12","4"]'
  a_jwt='[{"key":"auth.method","value":{"stringValue":"jwt"}}]'
  d_jwt=$(dp_exphist "$ts_i" "$start_i" 2 6 "$p_jwt" 0 "$a_jwt" 6346 1903800.0 80.0 12000.0)

  p_api='["200","650","1100","800","300","90","25","8","2"]'
  a_api='[{"key":"auth.method","value":{"stringValue":"api_key"}}]'
  d_api=$(dp_exphist "$ts_i" "$start_i" 2 5 "$p_api" 0 "$a_api" 3175 793750.0 50.0 8000.0)

  [ -n "$exphist_dps2" ] && exphist_dps2="${exphist_dps2},"
  exphist_dps2="${exphist_dps2}${d_jwt},${d_api}"
done

send_exp_histogram "user-service" "auth.token.validation.duration" "us" \
  "Time to validate authentication tokens" \
  "${exphist_dps2}"

echo
echo "Done. Sent metric payloads across 6 services (histograms enriched with 30 timestamps x 2-3 streams each)."
