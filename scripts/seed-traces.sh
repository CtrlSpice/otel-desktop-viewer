#!/usr/bin/env bash
#
# seed-traces.sh — Send sample traces to an OTLP HTTP receiver (events, links, deep trees, orphans).
#
# Usage:
#   ./scripts/seed-traces.sh                                # defaults to localhost:4318
#   OTLP_ENDPOINT=http://host:4318 ./scripts/seed-traces.sh
#
# Requires: bash, curl, uuidgen (trace ids = uuid hex 32 chars; span ids = first 16 hex chars).

set -euo pipefail

ENDPOINT="${OTLP_ENDPOINT:-http://localhost:4318}"
URL="${ENDPOINT}/v1/traces"

now_s=$(date +%s)
now_ns=$(( now_s * 1000000000 ))
hour_ns=$(( 3600 * 1000000000 ))
ms_ns=$(( 1000000 ))

# 32 lowercase hex chars (OTLP trace id)
uuid_trace_id() {
  uuidgen | tr -d '\n' | tr '[:upper:]' '[:lower:]' | tr -d '-'
}

# 16 lowercase hex chars (OTLP span id): truncate a fresh uuid
uuid_span_id() {
  local u
  u=$(uuid_trace_id)
  echo "${u:0:16}"
}

rnd_trace_id() { uuid_trace_id; }
rnd_span_id()  { uuid_span_id; }

post_trace_json() {
  local tmpfile="$1"
  curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${URL}" \
    -H 'Content-Type: application/json' \
    --data-binary "@${tmpfile}"
}

send_trace() {
  local service="$1" root_name="$2"
  local status_code="${3:-1}" child_count="${4:-0}" hours_ago="${5:-0}" dur_ms="${6:-120}"

  local tid; tid=$(rnd_trace_id)
  local root_sid; root_sid=$(rnd_span_id)
  local link_tid link_sid c2_link_tid c2_link_sid
  link_tid=$(rnd_trace_id)
  link_sid=$(rnd_span_id)
  local start_ns=$(( now_ns - hours_ago * hour_ns ))
  local end_ns=$(( start_ns + dur_ms * 1000000 ))
  local http_status=$(( status_code == 2 ? 500 : 200 ))

  # Build child spans (events + optional link on child 2)
  local children=""
  for (( c=1; c<=child_count; c++ )); do
    local csid; csid=$(rnd_span_id)
    local cstart=$(( start_ns + c * 5000000 ))
    local cend=$(( cstart + 10000000 ))
    local cstatus=$(( status_code == 2 && c == 1 ? 2 : 1 ))
    local cextra=""
    cextra=", \"events\": [{
      \"timeUnixNano\": \"${cstart}\",
      \"name\": \"child.checkpoint\",
      \"attributes\": [
        { \"key\": \"child.index\", \"value\": { \"intValue\": \"${c}\" } },
        { \"key\": \"checkpoint\", \"value\": { \"stringValue\": \"after_dispatch\" } }
      ]
    }]"
    if (( c == 2 )); then
      c2_link_tid=$(rnd_trace_id)
      c2_link_sid=$(rnd_span_id)
      cextra="${cextra}, \"links\": [{
        \"traceId\": \"${c2_link_tid}\",
        \"spanId\": \"${c2_link_sid}\",
        \"attributes\": [
          { \"key\": \"peer.operation\", \"value\": { \"stringValue\": \"synthetic_async_consumer\" } }
        ]
      }]"
    fi
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
      ]${cextra}
    }"
  done

  # Root span events + link to another trace (need not exist in store)
  local events="[]"
  local links
  links="[{
    \"traceId\": \"${link_tid}\",
    \"spanId\": \"${link_sid}\",
    \"attributes\": [
      { \"key\": \"link.relationship\", \"value\": { \"stringValue\": \"follows_from\" } },
      { \"key\": \"messaging.system\", \"value\": { \"stringValue\": \"kafka\" } }
    ]
  }]"
  if (( status_code == 2 )); then
    events="[{
      \"timeUnixNano\": \"${start_ns}\",
      \"name\": \"exception\",
      \"attributes\": [
        { \"key\": \"exception.type\",    \"value\": { \"stringValue\": \"RuntimeError\" } },
        { \"key\": \"exception.message\", \"value\": { \"stringValue\": \"something went wrong in ${root_name}\" } }
      ]
    },{
      \"timeUnixNano\": \"$(( start_ns + 5 * ms_ns ))\",
      \"name\": \"http.response.flush\",
      \"attributes\": [
        { \"key\": \"http.status_code\", \"value\": { \"intValue\": \"${http_status}\" } }
      ]
    }]"
  else
    events="[{
      \"timeUnixNano\": \"${start_ns}\",
      \"name\": \"request.received\",
      \"attributes\": [
        { \"key\": \"net.peer.ip\", \"value\": { \"stringValue\": \"10.0.1.2\" } }
      ]
    },{
      \"timeUnixNano\": \"$(( start_ns + 3 * ms_ns ))\",
      \"name\": \"auth.verified\",
      \"attributes\": [
        { \"key\": \"auth.scheme\", \"value\": { \"stringValue\": \"bearer\" } }
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
          "events": ${events},
          "links": ${links}
        }${children:+,${children}}
      ]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_trace_json "$tmpfile")

  local label="${service}/${root_name}"
  (( status_code == 2 )) && label="${label} [ERROR]"
  printf '  %-50s %s  (spans: %d, %dh ago)\n' "$label" "$http_code" "$(( child_count + 1 ))" "$hours_ago"
}

# One trace, depth 5 below root (server → client → DB → nested DB → …), multi-service names for the waterfall.
send_deep_hierarchy_trace() {
  local service="deep-stack-service"
  local tid shell_root l1_http l2_db l3_db l2_cache l1_worker l2_queue l3_handler l4_down
  tid=$(rnd_trace_id)
  shell_root=$(rnd_span_id)
  l1_http=$(rnd_span_id)
  l2_db=$(rnd_span_id)
  l3_db=$(rnd_span_id)
  l2_cache=$(rnd_span_id)
  l1_worker=$(rnd_span_id)
  l2_queue=$(rnd_span_id)
  l3_handler=$(rnd_span_id)
  l4_down=$(rnd_span_id)
  local deep_link_tid deep_link_span grpc_link_tid grpc_link_span
  deep_link_tid=$(rnd_trace_id)
  deep_link_span=$(rnd_span_id)
  grpc_link_tid=$(rnd_trace_id)
  grpc_link_span=$(rnd_span_id)

  local t0=$now_ns
  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceSpans": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "1.0.0" } }
      ]
    },
    "scopeSpans": [{
      "scope": { "name": "${service}.tracer", "version": "0.1.0" },
      "spans": [
        {
          "traceId": "${tid}",
          "spanId": "${shell_root}",
          "name": "shell/checkout-flow",
          "kind": 2,
          "startTimeUnixNano": "${t0}",
          "endTimeUnixNano": "$(( t0 + 800 * ms_ns ))",
          "status": { "code": 1 },
          "attributes": [
            { "key": "span.depth", "value": { "intValue": "0" } }
          ],
          "events": [
            {
              "timeUnixNano": "${t0}",
              "name": "checkout.session.start",
              "attributes": [
                { "key": "cart.item_count", "value": { "intValue": "3" } }
              ]
            }
          ],
          "links": [
            {
              "traceId": "${deep_link_tid}",
              "spanId": "${deep_link_span}",
              "attributes": [
                { "key": "note", "value": { "stringValue": "synthetic upstream marketing click" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${l1_http}",
          "parentSpanId": "${shell_root}",
          "name": "HTTP GET /checkout",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 10 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 600 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "$(( t0 + 12 * ms_ns ))",
              "name": "net.dns.lookup",
              "attributes": [
                { "key": "net.host.name", "value": { "stringValue": "api.internal" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${l2_db}",
          "parentSpanId": "${l1_http}",
          "name": "db/checkout_snapshot",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 30 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 500 * ms_ns ))",
          "status": { "code": 1 }
        },
        {
          "traceId": "${tid}",
          "spanId": "${l3_db}",
          "parentSpanId": "${l2_db}",
          "name": "db/line_items_subquery",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 50 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 450 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "$(( t0 + 55 * ms_ns ))",
              "name": "db.stmt.complete",
              "attributes": [
                { "key": "db.rows", "value": { "intValue": "1842" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${l2_cache}",
          "parentSpanId": "${l1_http}",
          "name": "cache/session_lookup",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 400 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 580 * ms_ns ))",
          "status": { "code": 1 }
        },
        {
          "traceId": "${tid}",
          "spanId": "${l1_worker}",
          "parentSpanId": "${shell_root}",
          "name": "worker/dispatch_payment",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 20 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 750 * ms_ns ))",
          "status": { "code": 1 }
        },
        {
          "traceId": "${tid}",
          "spanId": "${l2_queue}",
          "parentSpanId": "${l1_worker}",
          "name": "queue/consume_payment_job",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 100 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 700 * ms_ns ))",
          "status": { "code": 1 }
        },
        {
          "traceId": "${tid}",
          "spanId": "${l3_handler}",
          "parentSpanId": "${l2_queue}",
          "name": "handler/process_payment",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 150 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 650 * ms_ns ))",
          "status": { "code": 1 }
        },
        {
          "traceId": "${tid}",
          "spanId": "${l4_down}",
          "parentSpanId": "${l3_handler}",
          "name": "grpc/ChargeCard",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 200 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 620 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "$(( t0 + 220 * ms_ns ))",
              "name": "rpc.metadata.received",
              "attributes": [
                { "key": "rpc.grpc.status_code", "value": { "intValue": "0" } }
              ]
            }
          ],
          "links": [
            {
              "traceId": "${grpc_link_tid}",
              "spanId": "${grpc_link_span}",
              "attributes": [
                { "key": "rpc.system", "value": { "stringValue": "grpc" } }
              ]
            }
          ]
        }
      ]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_trace_json "$tmpfile")
  printf '  %-50s %s  (spans: %d, deep tree)\n' "${service}/checkout-flow (depth 5)" "$http_code" 9
}

# Root plus spans whose parentSpanId never appears in this batch (orphans in the UI).
send_orphan_spans_trace() {
  local service="orphan-lab"
  local tid root o1 o2 o3 missing_a missing_b root_link_tid root_link_sid o2_link_tid o2_link_sid o3_link_tid o3_link_sid
  tid=$(rnd_trace_id)
  root=$(rnd_span_id)
  o1=$(rnd_span_id)
  o2=$(rnd_span_id)
  o3=$(rnd_span_id)
  missing_a=$(rnd_span_id)
  missing_b=$(rnd_span_id)
  root_link_tid=$(rnd_trace_id)
  root_link_sid=$(rnd_span_id)
  o2_link_tid=$(rnd_trace_id)
  o2_link_sid=$(rnd_span_id)
  o3_link_tid=$(rnd_trace_id)
  o3_link_sid=$(rnd_span_id)

  local t0=$now_ns
  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceSpans": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "0.0.1" } }
      ]
    },
    "scopeSpans": [{
      "scope": { "name": "${service}.tracer", "version": "0.1.0" },
      "spans": [
        {
          "traceId": "${tid}",
          "spanId": "${root}",
          "name": "ingest/partial-batch",
          "kind": 2,
          "startTimeUnixNano": "${t0}",
          "endTimeUnixNano": "$(( t0 + 400 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "${t0}",
              "name": "batch.parser.start",
              "attributes": [
                { "key": "batch.bytes", "value": { "intValue": "4096" } }
              ]
            }
          ],
          "links": [
            {
              "traceId": "${root_link_tid}",
              "spanId": "${root_link_sid}",
              "attributes": [
                { "key": "link.preset", "value": { "stringValue": "prior_ingest_attempt" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${o1}",
          "parentSpanId": "${missing_a}",
          "name": "orphan/missing_parent_A",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 20 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 200 * ms_ns ))",
          "status": { "code": 1 },
          "attributes": [
            { "key": "note", "value": { "stringValue": "parentSpanId not in this export" } }
          ],
          "events": [
            {
              "timeUnixNano": "$(( t0 + 25 * ms_ns ))",
              "name": "orphan.span.attached",
              "attributes": [
                { "key": "synthetic", "value": { "stringValue": "true" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${o2}",
          "parentSpanId": "${missing_b}",
          "name": "orphan/missing_parent_B",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 40 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 220 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "$(( t0 + 45 * ms_ns ))",
              "name": "work.unit.done",
              "attributes": []
            }
          ],
          "links": [
            {
              "traceId": "${o2_link_tid}",
              "spanId": "${o2_link_sid}",
              "attributes": [
                { "key": "note", "value": { "stringValue": "orphan still links outward" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${o3}",
          "parentSpanId": "${missing_a}",
          "name": "orphan/sibling_same_missing_parent",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 60 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 300 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "$(( t0 + 70 * ms_ns ))",
              "name": "dedupe.checkpoint",
              "attributes": [
                { "key": "shard", "value": { "intValue": "7" } }
              ]
            }
          ],
          "links": [
            {
              "traceId": "${o3_link_tid}",
              "spanId": "${o3_link_sid}",
              "attributes": []
            }
          ]
        }
      ]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_trace_json "$tmpfile")
  printf '  %-50s %s  (spans: %d, orphans + events/links)\n' "${service}/partial-batch" "$http_code" 4
}

# No true root: parent id is absent from export, but orphan head has its own children (subtree under missing parent).
send_orphan_subtree_trace() {
  local service="orphan-subtree-lab"
  local tid ghost head ca cb head_link_tid head_link_sid cb_link_tid cb_link_sid
  tid=$(rnd_trace_id)
  ghost=$(rnd_span_id)
  head=$(rnd_span_id)
  ca=$(rnd_span_id)
  cb=$(rnd_span_id)
  head_link_tid=$(rnd_trace_id)
  head_link_sid=$(rnd_span_id)
  cb_link_tid=$(rnd_trace_id)
  cb_link_sid=$(rnd_span_id)

  local t0=$now_ns
  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  cat > "$tmpfile" <<JSON
{
  "resourceSpans": [{
    "resource": {
      "attributes": [
        { "key": "service.name", "value": { "stringValue": "${service}" } },
        { "key": "service.version", "value": { "stringValue": "0.0.1" } }
      ]
    },
    "scopeSpans": [{
      "scope": { "name": "${service}.tracer", "version": "0.1.0" },
      "spans": [
        {
          "traceId": "${tid}",
          "spanId": "${head}",
          "parentSpanId": "${ghost}",
          "name": "orphan/subtree_head",
          "kind": 3,
          "startTimeUnixNano": "${t0}",
          "endTimeUnixNano": "$(( t0 + 350 * ms_ns ))",
          "status": { "code": 1 },
          "attributes": [
            { "key": "edge", "value": { "stringValue": "parent_span_id_not_in_batch" } }
          ],
          "events": [
            {
              "timeUnixNano": "$(( t0 + 5 * ms_ns ))",
              "name": "subtree.head.bootstrap",
              "attributes": [
                { "key": "children.expected", "value": { "intValue": "2" } }
              ]
            }
          ],
          "links": [
            {
              "traceId": "${head_link_tid}",
              "spanId": "${head_link_sid}",
              "attributes": [
                { "key": "causal", "value": { "stringValue": "scheduled_by_external" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${ca}",
          "parentSpanId": "${head}",
          "name": "orphan/subtree_head/child_a",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 30 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 200 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "$(( t0 + 40 * ms_ns ))",
              "name": "child_a.phase1",
              "attributes": []
            },
            {
              "timeUnixNano": "$(( t0 + 120 * ms_ns ))",
              "name": "child_a.phase2",
              "attributes": [
                { "key": "rows", "value": { "intValue": "12" } }
              ]
            }
          ]
        },
        {
          "traceId": "${tid}",
          "spanId": "${cb}",
          "parentSpanId": "${head}",
          "name": "orphan/subtree_head/child_b",
          "kind": 3,
          "startTimeUnixNano": "$(( t0 + 50 * ms_ns ))",
          "endTimeUnixNano": "$(( t0 + 280 * ms_ns ))",
          "status": { "code": 1 },
          "events": [
            {
              "timeUnixNano": "$(( t0 + 90 * ms_ns ))",
              "name": "child_b.retry",
              "attributes": [
                { "key": "attempt", "value": { "intValue": "2" } }
              ]
            }
          ],
          "links": [
            {
              "traceId": "${cb_link_tid}",
              "spanId": "${cb_link_sid}",
              "attributes": [
                { "key": "downstream", "value": { "stringValue": "synthetic" } }
              ]
            }
          ]
        }
      ]
    }]
  }]
}
JSON

  local http_code
  http_code=$(post_trace_json "$tmpfile")
  printf '  %-50s %s  (spans: %d, orphan w/ children)\n' "${service}/subtree-under-missing-parent" "$http_code" 3
}

echo "Sending sample traces to ${URL} …"
echo

#          service            root_name              status children hours_ago dur_ms
send_trace "api-gateway"      "GET /users"               1    3        1        85
send_trace "api-gateway"      "POST /orders"             1    4        3       210
send_trace "api-gateway"      "GET /products"            2    2        6       340
send_trace "billing-service"  "charge"                   1    1        8       150
send_trace "billing-service"  "refund"                   2    0       12        45
send_trace "user-service"     "authenticate"             1    2       16       110
send_trace "user-service"     "register"                 1    3       20       275
send_trace "catalog-service"  "search"                   1    5       24       190
send_trace "catalog-service"  "get-product-detail"       1    1       30        60
send_trace "order-service"    "create-order"             2    4       36       420
send_trace "order-service"    "list-orders"              1    2       40        95
send_trace "notification"     "send-email"               1    0       44        30

echo
echo "Deep hierarchy + orphan scenarios …"
echo

send_deep_hierarchy_trace
send_orphan_spans_trace
send_orphan_subtree_trace

echo
echo "Done."
