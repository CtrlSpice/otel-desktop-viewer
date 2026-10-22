import test from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'
import { once } from 'node:events'
import { ROOT_CONTEXT, trace } from '@opentelemetry/api'
import { exportToolLogs, missingToolLogs, type ToolLog } from './tool-logs.ts'

const span = { traceId: 'a'.repeat(32), spanId: 'b'.repeat(16), traceFlags: 0 }
const invocation = trace.setSpanContext(ROOT_CONTEXT, span)
const record: ToolLog = { eventName: 'tool.execution', timestamp: 1700000000000, observedTimestamp: 1700000001234,
  attributes: { 'gen_ai.tool.call.id': 'call-1', 'gen_ai.tool.name': 'shell', 'tool.input': '{"command":"false"}',
    'tool.output': '{"exit":1}', 'tool.is_error': true, 'opencode.event.timestamp.source': 'tool.time.completed' } }

test('complete genuine span-event evidence suppresses enrichment; partial, conflicting or uncorrelated records do not', () => {
  const event = { name: record.eventName, attributes: Object.entries(record.attributes).map(([key, value]) => ({ key,
    value: typeof value === 'boolean' ? { boolValue: value } : { stringValue: String(value) } })) }
  const native = { ...span, events: [event] }
  const original = JSON.stringify(native)
  assert.deepEqual(missingToolLogs([record], span.traceId, [native]), [])
  assert.equal(JSON.stringify(native), original)
  assert.deepEqual(missingToolLogs([record], span.traceId, [{ ...native, traceId: 'c'.repeat(32) }]), [record])
  assert.deepEqual(missingToolLogs([record], span.traceId, [{ ...native, events: [{ ...event, attributes: event.attributes.filter(attribute => attribute.key !== 'tool.output') }] }]), [record])
  assert.deepEqual(missingToolLogs([{ ...record, attributes: { ...record.attributes, 'tool.is_error': false } }], span.traceId, [native]).length, 1)
  assert.deepEqual(missingToolLogs([record], span.traceId, [{ ...native, events: [event, event] }]), [record])
})

test('SDK exports named logs with native correlation, exact evidence and both timestamps, even for unsampled spans', async () => {
  const batches: any[] = []
  const server = http.createServer(async (request, response) => {
    assert.equal(request.url, '/v1/logs')
    const chunks: Buffer[] = []
    for await (const chunk of request) chunks.push(chunk)
    batches.push(JSON.parse(Buffer.concat(chunks).toString()))
    response.writeHead(200, { 'Content-Type': 'application/json' }).end('{}')
  })
  server.listen(0, '127.0.0.1'); await once(server, 'listening')
  const address = server.address(); assert.ok(address && typeof address === 'object')
  try {
    await exportToolLogs([record], invocation, 'OpenCode', 'fixture', `http://127.0.0.1:${address.port}`)
    assert.equal(batches.length, 1, 'Flush completes before returning')
    const resource = batches[0].resourceLogs[0]
    assert.equal(resource.scopeLogs[0].scope.name, 'viewer.eval.tool-evidence')
    const logs = resource.scopeLogs[0].logRecords
    assert.equal(logs.length, 1)
    const log = logs[0]
    assert.equal(log.eventName, 'tool.execution')
    assert.equal(log.traceId, span.traceId)
    assert.equal(log.spanId, span.spanId)
    assert.equal(log.flags ?? 0, 0)
    assert.equal(log.timeUnixNano, '1700000000000000000')
    assert.equal(log.observedTimeUnixNano, '1700000001234000000')
    assert.deepEqual(log.body, { stringValue: 'shell' })
    const attrs = Object.fromEntries(log.attributes.map((item: any) => [item.key, item.value]))
    assert.deepEqual(attrs['tool.is_error'], { boolValue: true })
    assert.deepEqual(attrs['tool.input'], { stringValue: record.attributes['tool.input'] })
    assert.deepEqual(attrs['tool.output'], { stringValue: record.attributes['tool.output'] })
    assert.deepEqual(attrs['eval.evidence.source'], { stringValue: 'OpenCode' })
    assert.equal(log.severityNumber ?? 0, 0, 'A tool error must not invent log severity')
    assert.equal(batches[0].resourceSpans, undefined)
  } finally { await new Promise<void>(resolve => server.close(() => resolve())) }
})

test('telemetry-free runs stay optional; configured logging refuses an invented span association', async () => {
  await exportToolLogs([record], ROOT_CONTEXT, 'Pi', 'fixture', '')
  await assert.rejects(exportToolLogs([record], ROOT_CONTEXT, 'Pi', 'fixture', 'http://127.0.0.1:1'), /recorded invocation span/)
})

test('rejected log exports are reported to the caller, not silently treated as captured evidence', async () => {
  const server = http.createServer((_, response) => response.writeHead(400).end('rejected'))
  server.listen(0, '127.0.0.1'); await once(server, 'listening')
  const address = server.address(); assert.ok(address && typeof address === 'object')
  try {
    await assert.rejects(exportToolLogs([record], invocation, 'Pi', 'fixture', `http://127.0.0.1:${address.port}`), /log export failed/)
  } finally { await new Promise<void>(resolve => server.close(() => resolve())) }
})
