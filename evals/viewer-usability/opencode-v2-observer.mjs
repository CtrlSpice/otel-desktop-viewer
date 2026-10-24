import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { query } from './prepare.ts'
import { converge } from './opencode-v2-run-plan.mjs'
import { readNativeTelemetry } from './opencode-v2-telemetry.mjs'

export const sha256 = value => createHash('sha256').update(value).digest('hex')
export const readJSON = file => JSON.parse(fs.readFileSync(file, 'utf8'))
export function acceptedReceipts(receipts) {
  for (const receipt of receipts) {
    assert.equal(receipt.status, 200, 'OTLP rejected or undelivered; never replay uploads')
    assert.ok(!Object.values(JSON.parse(receipt.reply).partialSuccess ?? {}).some(value => value && value !== '0'), 'Partial OTLP rejection')
  }
}

export function loadRunTelemetry(root) {
  const wrapperReceipts = readJSON(path.join(root, 'otlp-summary.json')).receipts
  acceptedReceipts(wrapperReceipts)
  const wrapper = readNativeTelemetry(root, wrapperReceipts, 'otlp')
  const attempts = new Map()
  const runs = path.join(root, 'runs')
  for (const name of fs.existsSync(runs) ? fs.readdirSync(runs) : []) {
    const evidence = path.join(runs, name)
    if (!name.startsWith('investigator-opencode-v2-')) continue
    assert.ok(!fs.existsSync(path.join(evidence, 'native-telemetry-error.json')), 'Native capture error; inspect raw bytes')
    const file = path.join(evidence, 'native-otlp/summary.json')
    if (!fs.existsSync(file)) continue
    const receipts = readJSON(file).receipts
    acceptedReceipts(receipts)
    assert.ok(receipts.every(receipt => receipt.forwarded), 'Native uploads must have been forwarded live')
    attempts.set(evidence, { ...readNativeTelemetry(evidence, receipts), receipts })
  }
  // Preserve verification of the original single-run capture layout.
  let legacy
  if (fs.existsSync(path.join(root, 'native-otlp/summary.json'))) {
    const receipts = readJSON(path.join(root, 'native-otlp/summary.json')).receipts
    acceptedReceipts(receipts); legacy = readNativeTelemetry(root, receipts)
  }
  const native = [...attempts.values(), ...(legacy ? [legacy] : [])]
  return { wrapper, wrapperReceipts, attempts, legacy,
    spans: [...wrapper.spans, ...native.flatMap(data => data.spans)],
    logs: [...wrapper.logs, ...native.flatMap(data => data.logs)],
    metrics: [...wrapper.metrics, ...native.flatMap(data => data.metrics)],
  }
}

export function telemetryExpectation(data) {
  const spanKeys = data.spans.map(span => span.traceId + '/' + span.spanId).sort()
  assert.equal(new Set(spanKeys).size, spanKeys.length, 'Duplicate received span IDs; do not silently deduplicate')
  const traceIDs = [...new Set(data.spans.map(span => span.traceId))]
  for (const id of traceIDs) assert.match(id, /^[a-f0-9]{32}$/)
  const logTimes = data.logs.map(log => String(log.timeUnixNano))
  for (const time of logTimes) assert.match(time, /^\d+$/)
  const events = data.spans.reduce((sum, span) => sum + (span.events?.length ?? 0), 0)
  const datapoints = data.metrics.reduce((sum, metric) => sum + ['sum', 'gauge', 'histogram', 'exponentialHistogram', 'summary']
    .reduce((count, key) => count + (metric[key]?.dataPoints?.length ?? 0), 0), 0)
  return { spans: spanKeys.length, spanIdentitySHA256: sha256(spanKeys.join(',')), logs: data.logs.length, events, datapoints, traceIDs, logTimes }
}

export async function observerTotals(endpoint) {
  const result = await query(endpoint, 'SELECT (SELECT count(*) FROM spans), (SELECT count(*) FROM logs), (SELECT count(*) FROM metric_datapoints)')
  assert.equal(result.truncated, false)
  return result.rows[0]
}

export async function awaitTelemetry(endpoint, data, { fresh = false, timeoutMs = 60000 } = {}) {
  const expected = telemetryExpectation(data)
  const ids = expected.traceIDs.map(id => "'" + id + "'").join(',') || 'NULL'
  const key = "replace(trace_id::VARCHAR,'-','') || '/' || span_id_wire(span_id)"
  const check = async () => {
    const [spans, logs, events, totals] = await Promise.all([
      query(endpoint, `SELECT count(*),sha256(coalesce(string_agg(${key}, ',' ORDER BY ${key}),'')) FROM spans WHERE trace_id IN (${ids})`),
      expected.logTimes.length ? query(endpoint, `SELECT count(*) FROM logs WHERE timestamp IN (${expected.logTimes.join(',')})`) : { rows: [[0]] },
      query(endpoint, `SELECT count(*) FROM events WHERE trace_id IN (${ids})`),
      fresh ? observerTotals(endpoint) : undefined,
    ])
    return { spans: spans.rows[0][0], spanIdentitySHA256: spans.rows[0][1], logs: logs.rows[0][0], events: events.rows[0][0],
      ...(fresh ? { totals } : {}) }
  }
  const observed = await converge(check, value => value.spans === expected.spans && value.spanIdentitySHA256 === expected.spanIdentitySHA256 &&
    value.logs === expected.logs && value.events === expected.events && (!fresh || JSON.stringify(value.totals) === JSON.stringify([expected.spans, expected.logs, expected.datapoints])), { timeoutMs })
  return { expected, observed, uploadsRetried: 0 }
}
