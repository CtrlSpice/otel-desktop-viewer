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
echo "Large multi-service trace …"
echo

# Realistic e-commerce order flow: ~40 spans across 7 services with varied depths,
# parallel branches, an error subtree, and realistic timing.
send_large_multiservice_trace() {
  local tid; tid=$(rnd_trace_id)

  # -- span ids --
  local gw_root; gw_root=$(rnd_span_id)
  local gw_auth; gw_auth=$(rnd_span_id)
  local gw_rate; gw_rate=$(rnd_span_id)
  local gw_route; gw_route=$(rnd_span_id)

  local usr_validate; usr_validate=$(rnd_span_id)
  local usr_profile; usr_profile=$(rnd_span_id)
  local usr_db_read; usr_db_read=$(rnd_span_id)
  local usr_cache_check; usr_cache_check=$(rnd_span_id)
  local usr_perms; usr_perms=$(rnd_span_id)

  local cat_list; cat_list=$(rnd_span_id)
  local cat_db; cat_db=$(rnd_span_id)
  local cat_cache; cat_cache=$(rnd_span_id)
  local cat_enrich; cat_enrich=$(rnd_span_id)
  local cat_img; cat_img=$(rnd_span_id)
  local cat_price; cat_price=$(rnd_span_id)

  local inv_reserve; inv_reserve=$(rnd_span_id)
  local inv_lock; inv_lock=$(rnd_span_id)
  local inv_db_write; inv_db_write=$(rnd_span_id)
  local inv_db_read; inv_db_read=$(rnd_span_id)
  local inv_confirm; inv_confirm=$(rnd_span_id)

  local ord_create; ord_create=$(rnd_span_id)
  local ord_validate; ord_validate=$(rnd_span_id)
  local ord_db_insert; ord_db_insert=$(rnd_span_id)
  local ord_line1; ord_line1=$(rnd_span_id)
  local ord_line2; ord_line2=$(rnd_span_id)
  local ord_line3; ord_line3=$(rnd_span_id)
  local ord_total; ord_total=$(rnd_span_id)

  local pay_charge; pay_charge=$(rnd_span_id)
  local pay_fraud; pay_fraud=$(rnd_span_id)
  local pay_fraud_ml; pay_fraud_ml=$(rnd_span_id)
  local pay_gateway; pay_gateway=$(rnd_span_id)
  local pay_ledger; pay_ledger=$(rnd_span_id)
  local pay_receipt; pay_receipt=$(rnd_span_id)

  local notif_email; notif_email=$(rnd_span_id)
  local notif_render; notif_render=$(rnd_span_id)
  local notif_smtp; notif_smtp=$(rnd_span_id)
  local notif_push; notif_push=$(rnd_span_id)
  local notif_webhook; notif_webhook=$(rnd_span_id)

  local ship_schedule; ship_schedule=$(rnd_span_id)
  local ship_carrier; ship_carrier=$(rnd_span_id)
  local ship_label; ship_label=$(rnd_span_id)

  local t0=$now_ns
  local tmpfile; tmpfile=$(mktemp)
  trap "rm -f '$tmpfile'" RETURN

  # Helper: span JSON fragment
  # args: service, spanId, parentSpanId, name, kind, startOffsetMs, endOffsetMs, statusCode [, extraJson]
  span_json() {
    local svc="$1" sid="$2" psid="$3" name="$4" kind="$5"
    local s_off="$6" e_off="$7" status="$8"
    local extra="${9:-}"
    local s_ns=$(( t0 + s_off * ms_ns ))
    local e_ns=$(( t0 + e_off * ms_ns ))
    local parent=""
    [ -n "$psid" ] && parent="\"parentSpanId\": \"${psid}\","
    cat <<SPAN
    {
      "traceId": "${tid}",
      "spanId": "${sid}",
      ${parent}
      "name": "${name}",
      "kind": ${kind},
      "startTimeUnixNano": "${s_ns}",
      "endTimeUnixNano": "${e_ns}",
      "status": { "code": ${status} },
      "attributes": [
        { "key": "service.layer", "value": { "stringValue": "${svc}" } }
      ]${extra:+,${extra}}
    }
SPAN
  }

  cat > "$tmpfile" <<JSON
{
  "resourceSpans": [
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "api-gateway" } },
          { "key": "service.version", "value": { "stringValue": "2.4.1" } },
          { "key": "deployment.environment", "value": { "stringValue": "production" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "api-gateway.tracer" }, "spans": [
$(span_json "api-gateway" "$gw_root"  "" "POST /api/v2/orders" 2 0 1200 1 "\"events\": [{\"timeUnixNano\":\"${t0}\",\"name\":\"request.received\",\"attributes\":[{\"key\":\"http.method\",\"value\":{\"stringValue\":\"POST\"}},{\"key\":\"http.url\",\"value\":{\"stringValue\":\"/api/v2/orders\"}}]}]"),
$(span_json "api-gateway" "$gw_auth"  "$gw_root" "middleware/authenticate" 1 2 18 1),
$(span_json "api-gateway" "$gw_rate"  "$gw_root" "middleware/rate-limit" 1 18 22 1),
$(span_json "api-gateway" "$gw_route" "$gw_root" "router/dispatch" 1 22 1180 1)
      ]}]
    },
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "user-service" } },
          { "key": "service.version", "value": { "stringValue": "1.8.0" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "user-service.tracer" }, "spans": [
$(span_json "user-service" "$usr_validate"    "$gw_auth"     "user/validate-token" 1 3 16 1),
$(span_json "user-service" "$usr_profile"     "$usr_validate" "user/load-profile" 1 5 14 1),
$(span_json "user-service" "$usr_db_read"     "$usr_profile"  "db/SELECT users" 3 6 10 1),
$(span_json "user-service" "$usr_cache_check" "$usr_profile"  "cache/profile-lookup" 3 6 8 1),
$(span_json "user-service" "$usr_perms"       "$usr_validate" "user/check-permissions" 1 14 16 1)
      ]}]
    },
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "catalog-service" } },
          { "key": "service.version", "value": { "stringValue": "3.1.2" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "catalog-service.tracer" }, "spans": [
$(span_json "catalog-service" "$cat_list"   "$gw_route" "catalog/resolve-items" 1 25 280 1),
$(span_json "catalog-service" "$cat_db"     "$cat_list" "db/SELECT products" 3 28 120 1),
$(span_json "catalog-service" "$cat_cache"  "$cat_list" "cache/product-details" 3 28 45 1),
$(span_json "catalog-service" "$cat_enrich" "$cat_list" "catalog/enrich-metadata" 1 125 270 1),
$(span_json "catalog-service" "$cat_img"    "$cat_enrich" "cdn/resolve-image-urls" 3 130 200 1),
$(span_json "catalog-service" "$cat_price"  "$cat_enrich" "pricing/compute-discounts" 1 135 260 1)
      ]}]
    },
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "inventory-service" } },
          { "key": "service.version", "value": { "stringValue": "1.2.0" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "inventory-service.tracer" }, "spans": [
$(span_json "inventory-service" "$inv_reserve"  "$gw_route"    "inventory/reserve-stock" 1 285 500 1),
$(span_json "inventory-service" "$inv_lock"     "$inv_reserve" "db/advisory-lock" 3 288 310 1),
$(span_json "inventory-service" "$inv_db_write" "$inv_reserve" "db/UPDATE stock" 3 312 420 1),
$(span_json "inventory-service" "$inv_db_read"  "$inv_reserve" "db/SELECT remaining" 3 422 460 1),
$(span_json "inventory-service" "$inv_confirm"  "$inv_reserve" "inventory/confirm-hold" 1 462 495 1)
      ]}]
    },
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "order-service" } },
          { "key": "service.version", "value": { "stringValue": "2.0.3" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "order-service.tracer" }, "spans": [
$(span_json "order-service" "$ord_create"    "$gw_route"     "order/create" 1 505 780 1),
$(span_json "order-service" "$ord_validate"  "$ord_create"   "order/validate-request" 1 508 530 1),
$(span_json "order-service" "$ord_db_insert" "$ord_create"   "db/INSERT orders" 3 532 600 1),
$(span_json "order-service" "$ord_line1"     "$ord_db_insert" "db/INSERT line_items[0]" 3 535 555 1),
$(span_json "order-service" "$ord_line2"     "$ord_db_insert" "db/INSERT line_items[1]" 3 556 575 1),
$(span_json "order-service" "$ord_line3"     "$ord_db_insert" "db/INSERT line_items[2]" 3 576 595 1),
$(span_json "order-service" "$ord_total"     "$ord_create"   "order/compute-totals" 1 602 770 1)
      ]}]
    },
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "payment-service" } },
          { "key": "service.version", "value": { "stringValue": "4.0.1" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "payment-service.tracer" }, "spans": [
$(span_json "payment-service" "$pay_charge"   "$gw_route"    "payment/charge" 1 785 1050 1),
$(span_json "payment-service" "$pay_fraud"    "$pay_charge"  "payment/fraud-check" 1 788 880 1),
$(span_json "payment-service" "$pay_fraud_ml" "$pay_fraud"   "ml/score-transaction" 3 790 870 1),
$(span_json "payment-service" "$pay_gateway"  "$pay_charge"  "stripe/create-charge" 3 882 1000 2 "\"events\": [{\"timeUnixNano\":\"$(( t0 + 950 * ms_ns ))\",\"name\":\"exception\",\"attributes\":[{\"key\":\"exception.type\",\"value\":{\"stringValue\":\"PaymentDeclinedError\"}},{\"key\":\"exception.message\",\"value\":{\"stringValue\":\"Card declined: insufficient funds\"}}]}]"),
$(span_json "payment-service" "$pay_ledger"   "$pay_charge"  "db/INSERT ledger_entry" 3 1002 1030 1),
$(span_json "payment-service" "$pay_receipt"  "$pay_charge"  "payment/generate-receipt" 1 1032 1048 1)
      ]}]
    },
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "notification-service" } },
          { "key": "service.version", "value": { "stringValue": "1.5.0" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "notification-service.tracer" }, "spans": [
$(span_json "notification-service" "$notif_email"   "$gw_route"     "notify/send-confirmation" 1 1055 1170 1),
$(span_json "notification-service" "$notif_render"  "$notif_email"  "template/render-email" 1 1058 1090 1),
$(span_json "notification-service" "$notif_smtp"    "$notif_email"  "smtp/deliver" 3 1092 1140 1),
$(span_json "notification-service" "$notif_push"    "$notif_email"  "push/send-mobile" 3 1092 1130 1),
$(span_json "notification-service" "$notif_webhook" "$notif_email"  "webhook/post-order-event" 3 1095 1165 1)
      ]}]
    },
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "shipping-service" } },
          { "key": "service.version", "value": { "stringValue": "1.0.4" } }
        ]
      },
      "scopeSpans": [{ "scope": { "name": "shipping-service.tracer" }, "spans": [
$(span_json "shipping-service" "$ship_schedule" "$gw_route"      "shipping/schedule-pickup" 1 1055 1175 1),
$(span_json "shipping-service" "$ship_carrier"  "$ship_schedule" "carrier/query-rates" 3 1060 1120 1),
$(span_json "shipping-service" "$ship_label"    "$ship_schedule" "label/generate-pdf" 1 1122 1170 1)
      ]}]
    }
  ]
}
JSON

  local http_code
  http_code=$(post_trace_json "$tmpfile")
  printf '  %-50s %s  (spans: %d, multi-service order flow)\n' "multi-service/POST /api/v2/orders" "$http_code" 40
}

send_large_multiservice_trace

echo
echo "Done."
