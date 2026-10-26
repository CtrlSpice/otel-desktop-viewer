import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { tmpdir } from 'node:os'
import { once } from 'node:events'
import { gzipSync } from 'node:zlib'
import { test } from 'node:test'
import { nativeTelemetryReceiver, readNativeTelemetry, nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'

test('native relay preserves exact OTLP bytes, encoding, scopes and duplicate records', async () => {
  const root = fs.mkdtempSync(path.join(tmpdir(), 'opencode/opencode-v2-telemetry-test-'))
  const received = []
  const observer = http.createServer(async (request, response) => {
    const chunks = []; for await (const chunk of request) chunks.push(chunk)
    received.push({ route: request.url, encoding: request.headers['content-encoding'], bytes: Buffer.concat(chunks) })
    response.writeHead(200, { 'Content-Type': 'application/json' }).end('{"partialSuccess":{}}')
  })
  let receiver
  try {
    observer.listen(0, '127.0.0.1'); await once(observer, 'listening')
    receiver = await nativeTelemetryReceiver(root, 'http://127.0.0.1:' + observer.address().port)
    const span = { traceId: '1'.repeat(32), spanId: '2'.repeat(16), name: 'Tool.execute', attributes: [], status: { code: 2 } }
    const scope = { name: 'opencode', version: '2.0.29' }
    const traceBytes = Buffer.from(JSON.stringify({ resourceSpans: [{ scopeSpans: [{ scope, spans: [span, span] }] }] }) + '\n ')
    const logBytes = gzipSync(Buffer.from('{"resourceLogs":[{"scopeLogs":[{"scope":{"name":"opencode"},"logRecords":[{"body":{"stringValue":"fixture"}}]}]}]}'))
    // This metric is a test fixture for pass-through, not a claim about V2 support.
    const metricBytes = Buffer.from('{"resourceMetrics":[{"scopeMetrics":[{"scope":{"name":"fixture"},"metrics":[{"name":"fixture.gauge","gauge":{"dataPoints":[]}}]}]}]}')
    const batches = [['/v1/traces', traceBytes], ['/v1/logs', logBytes, 'gzip'], ['/v1/metrics', metricBytes]]
    for (const [route, body, encoding] of batches) {
      const response = await fetch(receiver.endpoint + route, { method: 'POST', body, headers: { 'Content-Type': 'application/json', ...(encoding ? { 'Content-Encoding': encoding } : {}) } })
      assert.equal(response.status, 200); await response.text()
    }
    await receiver.close(); receiver = undefined
    assert.deepEqual(received, batches.map(([route, bytes, encoding]) => ({ route, bytes, encoding })))
    const { receipts } = JSON.parse(fs.readFileSync(path.join(root, 'native-otlp/summary.json')))
    receipts.forEach((receipt, index) => {
      assert.equal(receipt.origin, 'owned-opencode-v2'); assert.equal(receipt.forwarded, true)
      assert.deepEqual(fs.readFileSync(path.join(root, 'native-otlp', receipt.name)), batches[index][1])
    })
    const data = readNativeTelemetry(root, receipts)
    assert.equal(data.spans.length, 2); assert.equal(data.logs.length, 1); assert.equal(data.metrics.length, 1)
    const summary = nativeTelemetryBoundaries(data, '00-' + '1'.repeat(32) + '-' + '3'.repeat(16) + '-01')
    assert.equal(summary.duplicateSpanRecords, 1, 'Duplicates must be reported, not silently removed')
    assert.equal(summary.nativeToolExecuteSpans, 2)
    assert.equal(summary.nativeToolSpansWithCallIDs, 0)
    assert.equal(summary.wrapperSpans, 0)
    assert.deepEqual(summary.scopes[0].scope, scope)
    fs.appendFileSync(path.join(root, 'native-otlp', receipts[0].name), ' ')
    assert.throws(() => readNativeTelemetry(root, receipts), /hash mismatch/)
  } finally {
    if (receiver) await receiver.close()
    await new Promise(resolve => observer.close(resolve))
    fs.rmSync(root, { recursive: true, force: true })
  }
})

test('native propagation report distinguishes HTTP parents from session/tool work', () => {
  const traceID = 'a'.repeat(32), parentID = 'b'.repeat(16)
  const spans = [
    { traceId: traceID, spanId: 'c'.repeat(16), parentSpanId: parentID, name: 'http.server POST' },
    { traceId: 'd'.repeat(32), spanId: 'e'.repeat(16), name: 'Tool.execute' },
  ]
  const report = nativeTelemetryBoundaries({ spans, summary: { spans: 2, logs: 0, metricRecords: 0 } }, `00-${traceID}-${parentID}-01`)
  assert.equal(report.propagation.matchingSpans, 1)
  assert.equal(report.propagation.directChildren, 1)
  assert.equal(report.propagation.sessionOrToolWork, 0)
  assert.equal(report.nativeToolSpansWithCallIDs, 0)
  assert.equal(report.nativeGenAIAttributeSpans, 0)
  assert.equal(report.nativeUsageAttributeSpans, 0)
})
