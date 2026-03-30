#!/usr/bin/env bash
#
# seed-traces.sh — Send a batch of sample traces to an OTLP HTTP receiver.
#
# Usage:
#   ./scripts/seed-traces.sh                                # defaults to localhost:4318
#   OTLP_ENDPOINT=http://host:4318 ./scripts/seed-traces.sh
#
# Requires: bash, curl.

set -euo pipefail

ENDPOINT="${OTLP_ENDPOINT:-http://localhost:4318}"
URL="${ENDPOINT}/v1/traces"

now_s=$(date +%s)
now_ns=$(( now_s * 1000000000 ))
hour_ns=$(( 3600 * 1000000000 ))

trace_id() { printf '%032x' "$1"; }
span_id()  { printf '%016x' "$1"; }

send_trace() {
  local idx="$1" service="$2" root_name="$3"
  local status_code="${4:-1}" child_count="${5:-0}" hours_ago="${6:-0}" dur_ms="${7:-120}"

  local tid; tid=$(trace_id "$idx")
  local root_sid; root_sid=$(span_id "$(( idx * 100 ))")
  local start_ns=$(( now_ns - hours_ago * hour_ns ))
  local end_ns=$(( start_ns + dur_ms * 1000000 ))
  local http_status=$(( status_code == 2 ? 500 : 200 ))

  # Build child spans
  local children=""
  for (( c=1; c<=child_count; c++ )); do
    local csid; csid=$(span_id "$(( idx * 100 + c ))")
    local cstart=$(( start_ns + c * 5000000 ))
    local cend=$(( cstart + 10000000 ))
    local cstatus=$(( status_code == 2 && c == 1 ? 2 : 1 ))
    [ -n "$children" ] && children="${children},"
    children="${children}{
      \"traceId\": \"${tid}\",
      \"spanId\": \"${csid}\",
      \"parentSpanId\": \"${root_sid}\",
      \"name\": \"${root_name}/child-${c}\",
      \"kind\": 3,
      \"startTimeUnixNano\": \"${cstart}\",
      \"endTimeUnixNano\": \"${cend}\",
      \"status\": { \"code\": ${cstatus} },
      \"attributes\": [
        { \"key\": \"child.index\", \"value\": { \"intValue\": \"${c}\" } }
      ]
    }"
  done

  # Exception event for error traces
  local events="[]"
  if (( status_code == 2 )); then
    events="[{
      \"timeUnixNano\": \"${start_ns}\",
      \"name\": \"exception\",
      \"attributes\": [
        { \"key\": \"exception.type\",    \"value\": { \"stringValue\": \"RuntimeError\" } },
        { \"key\": \"exception.message\", \"value\": { \"stringValue\": \"something went wrong in ${root_name}\" } }
      ]
    }]"
  fi

  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceSpans": [{
    "resource": {
      "attributes": [
        { "key": "service.name",    "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } }
      ]
    },
    "scopeSpans": [{
      "scope": { "name": "${service}.tracer", "version": "0.1.0" },
      "spans": [
        {
          "traceId": "${tid}",
          "spanId": "${root_sid}",
          "name": "${root_name}",
          "kind": 2,
          "startTimeUnixNano": "${start_ns}",
          "endTimeUnixNano": "${end_ns}",
          "status": { "code": ${status_code} },
          "attributes": [
            { "key": "http.method",      "value": { "stringValue": "GET" } },
            { "key": "http.target",      "value": { "stringValue": "/${root_name}" } },
            { "key": "http.status_code", "value": { "intValue": "${http_status}" } }
          ],
          "events": ${events}
        }${children:+,${children}}
      ]
    }]
  }]
}
JSON

  local http_code
  http_code=$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${URL}" \
    -H 'Content-Type: application/json' \
    --data-binary "@${tmpfile}")

  local label="${service}/${root_name}"
  (( status_code == 2 )) && label="${label} [ERROR]"
  printf '  %-50s %s  (spans: %d, %dh ago)\n' "$label" "$http_code" "$(( child_count + 1 ))" "$hours_ago"
}

echo "Sending 12 sample traces to ${URL} …"
echo

#          idx  service            root_name              status children hours_ago dur_ms
send_trace  1  "api-gateway"      "GET /users"               1    3        1        85
send_trace  2  "api-gateway"      "POST /orders"             1    4        3       210
send_trace  3  "api-gateway"      "GET /products"            2    2        6       340
send_trace  4  "billing-service"  "charge"                   1    1        8       150
send_trace  5  "billing-service"  "refund"                   2    0       12        45
send_trace  6  "user-service"     "authenticate"             1    2       16       110
send_trace  7  "user-service"     "register"                 1    3       20       275
send_trace  8  "catalog-service"  "search"                   1    5       24       190
send_trace  9  "catalog-service"  "get-product-detail"       1    1       30        60
send_trace 10  "order-service"    "create-order"             2    4       36       420
send_trace 11  "order-service"    "list-orders"              1    2       40        95
send_trace 12  "notification"     "send-email"               1    0       44        30

echo
echo "Done."
