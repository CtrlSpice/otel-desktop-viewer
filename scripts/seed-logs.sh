#!/usr/bin/env bash
#
# seed-logs.sh — Send sample log records to an OTLP HTTP receiver.
#
# Usage:
#   ./scripts/seed-logs.sh                                # defaults to localhost:4318
#   OTLP_ENDPOINT=http://host:4318 ./scripts/seed-logs.sh
#
# Requires: bash, curl, uuidgen.

set -euo pipefail

ENDPOINT="${OTLP_ENDPOINT:-http://localhost:4318}"
URL="${ENDPOINT}/v1/logs"

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

post_logs_json() {
  local tmpfile="$1"
  curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${URL}" \
    -H 'Content-Type: application/json' \
    --data-binary "@${tmpfile}"
}

# send_log service severity_text severity_number body [minutes_ago] [trace_id] [span_id] [event_name] [body_type] [extra_attrs_json]
send_log() {
  local service="$1"
  local sev_text="$2"
  local sev_num="$3"
  local body="$4"
  local mins_ago="${5:-0}"
  local trace_id="${6:-}"
  local span_id="${7:-}"
  local event_name="${8:-}"
  local body_type="${9:-string}"
  local extra_attrs="${10:-}"

  local ts=$(( now_ns - mins_ago * min_ns ))
  local observed_ts=$(( ts + 500000 ))

  local trace_field=""
  local span_field=""
  [ -n "$trace_id" ] && trace_field="\"traceId\": \"${trace_id}\","
  [ -n "$span_id" ]  && span_field="\"spanId\": \"${span_id}\","

  local attrs="[]"
  if [ -n "$extra_attrs" ]; then
    attrs="[${extra_attrs}]"
  fi

  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceLogs": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } },
        { "key": "deployment.environment", "value": { "stringValue": "production" } }
      ]
    },
    "scopeLogs": [{
      "scope": { "name": "${service}.logger", "version": "0.1.0" },
      "logRecords": [{
        "timeUnixNano": "${ts}",
        "observedTimeUnixNano": "${observed_ts}",
        ${trace_field}
        ${span_field}
        "severityNumber": ${sev_num},
        "severityText": "${sev_text}",
        "body": { "stringValue": $(printf '%s' "$body" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))') },
        "attributes": ${attrs}
      }]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_logs_json "$tmpfile")
  printf '  %-14s %-8s %s  (%d min ago)\n' "$service" "$sev_text" "$http_code" "$mins_ago"
}

# send_log_batch posts multiple log records in a single request
send_log_batch() {
  local service="$1"
  shift
  local records="$*"

  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceLogs": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } },
        { "key": "deployment.environment", "value": { "stringValue": "production" } }
      ]
    },
    "scopeLogs": [{
      "scope": { "name": "${service}.logger", "version": "0.1.0" },
      "logRecords": [${records}]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_logs_json "$tmpfile")
  printf '  %-14s batch    %s  (%d records)\n' "$service" "$http_code" "$(echo "$records" | tr -cd '{' | wc -c | tr -d ' ')"
}

echo "Sending sample logs to ${URL} …"
echo

# --- Simple single-record logs across services and severities ---

# TRACE-level (1-4)
send_log "api-gateway" "TRACE" 1 \
  "Entering middleware chain for request /api/v2/orders" 30

# DEBUG-level (5-8)
send_log "user-service" "DEBUG" 5 \
  "Cache miss for user profile uid=a8f2c, falling back to DB" 28

send_log "catalog-service" "DEBUG" 7 \
  "Resolved 42 product images from CDN in 38ms" 25

# INFO-level (9-12)
send_log "api-gateway" "INFO" 9 \
  "Request completed: POST /api/v2/orders → 201 Created (1.2s)" 20 \
  "$(uuid_hex32)" "$(uuid_hex16)"

send_log "order-service" "INFO" 10 \
  "Order ORD-20260411-7829 created successfully with 3 line items" 18

send_log "notification-service" "INFO" 9 \
  "Confirmation email queued for delivery to user@example.com" 15

send_log "shipping-service" "INFO" 10 \
  "Pickup scheduled with carrier FedEx, tracking: 7948302841" 12

# WARN-level (13-16)
send_log "inventory-service" "WARN" 13 \
  "Stock level below threshold: SKU-4412 has 3 remaining (threshold: 10)" 10 \
  "" "" "" "string" \
  '{ "key": "sku", "value": { "stringValue": "SKU-4412" } }, { "key": "remaining", "value": { "intValue": "3" } }, { "key": "threshold", "value": { "intValue": "10" } }'

send_log "payment-service" "WARN" 14 \
  "Payment gateway response time degraded: p99=2.3s (SLA: 1s)" 8

send_log "api-gateway" "WARN" 13 \
  "Rate limit approaching for client app-id=mobile-ios: 890/1000 requests in window" 5

# ERROR-level (17-20)
send_log "payment-service" "ERROR" 17 \
  "Payment declined: card ending 4242, reason: insufficient_funds" 4 \
  "$(uuid_hex32)" "$(uuid_hex16)" "" "string" \
  '{ "key": "error.type", "value": { "stringValue": "PaymentDeclinedError" } }, { "key": "card.last4", "value": { "stringValue": "4242" } }'

send_log "order-service" "ERROR" 17 \
  "Failed to persist order: deadlock detected on orders table, retrying (attempt 2/3)" 3

send_log "user-service" "ERROR" 18 \
  "Authentication failed: JWT signature verification error for token issued by idp.example.com" 2

# FATAL-level (21-24)
send_log "inventory-service" "FATAL" 21 \
  "Database connection pool exhausted: 0/50 connections available, all queries blocked" 1

# --- JSON body log ---
send_log "api-gateway" "INFO" 9 \
  '{"method":"POST","path":"/api/v2/orders","status":201,"duration_ms":1247,"request_id":"req-f8a2b","user_id":"usr-9c4e1","items":3,"total_cents":15499}' 22 \
  "$(uuid_hex32)" "$(uuid_hex16)" "" "json" \
  '{ "key": "http.method", "value": { "stringValue": "POST" } }, { "key": "http.route", "value": { "stringValue": "/api/v2/orders" } }'

# --- Log with event name ---
send_log "catalog-service" "INFO" 10 \
  "Product price recalculated after discount rules applied" 14 \
  "" "" "price.recalculated"

# --- A few more to give some volume ---
send_log "api-gateway" "INFO" 9 \
  "Health check passed: all upstream dependencies responding" 45

send_log "user-service" "INFO" 9 \
  "Session renewed for user usr-9c4e1, new expiry in 30m" 40

send_log "catalog-service" "WARN" 13 \
  "Product description exceeds recommended length: SKU-8827 (4200 chars, limit 3000)" 35

send_log "order-service" "DEBUG" 6 \
  "Computing order totals: subtotal=12499, tax=2340, shipping=660" 17

send_log "payment-service" "INFO" 10 \
  "Refund processed: txn-88f2a, amount=5000 cents, reason=customer_request" 7

send_log "notification-service" "ERROR" 17 \
  "SMTP connection timeout after 30s: smtp.mailer.example.com:587" 6

send_log "shipping-service" "DEBUG" 5 \
  "Carrier rate query: FedEx=12.50, UPS=14.20, USPS=8.90 — selected USPS" 11

echo
echo "Done. Sent $(( 23 )) log records."
