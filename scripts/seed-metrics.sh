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
# ============================================================================

echo "Histogram metrics …"

# HTTP request duration — realistic latency distribution
ex_trace3=$(uuid_hex32)
ex_sid3=$(uuid_hex16)
send_histogram "api-gateway" "http.server.request.duration" "s" \
  "Duration of inbound HTTP requests" \
  "{
    \"startTimeUnixNano\": \"$(( now_ns - 10 * min_ns ))\",
    \"timeUnixNano\": \"${now_ns}\",
    \"count\": \"1847\",
    \"sum\": 924.35,
    \"min\": 0.002,
    \"max\": 4.8,
    \"bucketCounts\": [\"120\", \"340\", \"520\", \"410\", \"280\", \"102\", \"45\", \"18\", \"8\", \"3\", \"1\"],
    \"explicitBounds\": [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0],
    \"attributes\": [
      { \"key\": \"http.method\", \"value\": { \"stringValue\": \"GET\" } },
      { \"key\": \"http.route\", \"value\": { \"stringValue\": \"/api/v2/orders\" } }
    ],
    \"exemplars\": [{
      \"timeUnixNano\": \"$(( now_ns - 3 * min_ns ))\",
      \"asDouble\": 0.342,
      \"traceId\": \"${ex_trace3}\",
      \"spanId\": \"${ex_sid3}\",
      \"filteredAttributes\": [
        { \"key\": \"http.status_code\", \"value\": { \"intValue\": \"200\" } }
      ]
    }]
  }"

# DB query duration
send_histogram "order-service" "db.client.query.duration" "ms" \
  "Duration of database queries" \
  "{
    \"startTimeUnixNano\": \"$(( now_ns - 15 * min_ns ))\",
    \"timeUnixNano\": \"${now_ns}\",
    \"count\": \"3240\",
    \"sum\": 48600.0,
    \"min\": 0.5,
    \"max\": 850.0,
    \"bucketCounts\": [\"890\", \"1200\", \"620\", \"310\", \"120\", \"55\", \"28\", \"12\", \"4\", \"1\"],
    \"explicitBounds\": [1.0, 5.0, 10.0, 25.0, 50.0, 100.0, 250.0, 500.0, 1000.0],
    \"attributes\": [
      { \"key\": \"db.system\", \"value\": { \"stringValue\": \"postgresql\" } },
      { \"key\": \"db.operation\", \"value\": { \"stringValue\": \"SELECT\" } }
    ]
  }"

# Response body size distribution
send_histogram "catalog-service" "http.server.response.body.size" "By" \
  "Size of HTTP response bodies" \
  "{
    \"startTimeUnixNano\": \"$(( now_ns - 20 * min_ns ))\",
    \"timeUnixNano\": \"${now_ns}\",
    \"count\": \"2100\",
    \"sum\": 157286400.0,
    \"min\": 128,
    \"max\": 524288,
    \"bucketCounts\": [\"200\", \"450\", \"680\", \"420\", \"210\", \"90\", \"35\", \"12\", \"3\"],
    \"explicitBounds\": [256, 1024, 4096, 16384, 65536, 131072, 262144, 524288],
    \"attributes\": [
      { \"key\": \"http.route\", \"value\": { \"stringValue\": \"/api/v2/products\" } }
    ]
  }"

echo

# ============================================================================
# Exponential Histogram metrics — high-resolution distributions
# ============================================================================

echo "Exponential histogram metrics …"

send_exp_histogram "payment-service" "payment.processing.duration" "ms" \
  "Time to process a payment transaction end-to-end" \
  "{
    \"startTimeUnixNano\": \"$(( now_ns - 10 * min_ns ))\",
    \"timeUnixNano\": \"${now_ns}\",
    \"count\": \"580\",
    \"sum\": 145000.0,
    \"min\": 45.0,
    \"max\": 2800.0,
    \"scale\": 3,
    \"zeroCount\": \"0\",
    \"positive\": {
      \"offset\": 12,
      \"bucketCounts\": [\"28\", \"65\", \"142\", \"178\", \"89\", \"42\", \"21\", \"10\", \"4\", \"1\"]
    },
    \"negative\": {
      \"offset\": 0,
      \"bucketCounts\": []
    },
    \"attributes\": [
      { \"key\": \"payment.provider\", \"value\": { \"stringValue\": \"stripe\" } },
      { \"key\": \"payment.method\", \"value\": { \"stringValue\": \"card\" } }
    ]
  }"

send_exp_histogram "user-service" "auth.token.validation.duration" "us" \
  "Time to validate authentication tokens" \
  "{
    \"startTimeUnixNano\": \"$(( now_ns - 5 * min_ns ))\",
    \"timeUnixNano\": \"${now_ns}\",
    \"count\": \"12400\",
    \"sum\": 3720000.0,
    \"min\": 80.0,
    \"max\": 15000.0,
    \"scale\": 2,
    \"zeroCount\": \"0\",
    \"positive\": {
      \"offset\": 6,
      \"bucketCounts\": [\"1200\", \"3800\", \"4200\", \"2100\", \"720\", \"280\", \"70\", \"22\", \"6\", \"2\"]
    },
    \"negative\": {
      \"offset\": 0,
      \"bucketCounts\": []
    },
    \"attributes\": [
      { \"key\": \"auth.method\", \"value\": { \"stringValue\": \"jwt\" } }
    ]
  }"

echo
echo "Done. Sent 17 metric payloads across 6 services."
