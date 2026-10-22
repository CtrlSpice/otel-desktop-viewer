import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { query } from './prepare.ts'
import { evalEventTools } from '../../desktopexporter/internal/frontend/src/components/evals/eval-tool-events.ts'
import { aggregateUsage } from './opencode-v2-evidence.mjs'
import { verifyToolEvent } from './opencode-v2-verify-evidence.mjs'
import { readNativeTelemetry, nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'

const args = process.argv.slice(2)
if (!args[0] || args.slice(1).some(arg => arg !== '--import')) throw new Error('Usage: opencode-v2-verify.mjs <completed-run-root> [--import]')
const root = fs.realpathSync(args[0])
if (!path.basename(root).startsWith('opencode-v2-native-')) throw new Error('Select an actual V2 native run')
const read = file => JSON.parse(fs.readFileSync(file, 'utf8'))
const sha = bytes => createHash('sha256').update(bytes).digest('hex')
const save = (name, value) => fs.writeFileSync(path.join(root, name), JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
const exportFile = path.join(root, 'promptfoo.json')
const bytes = fs.readFileSync(exportFile)
const before = sha(bytes)
const source = JSON.parse(bytes)
assert.ok(fs.existsSync(path.join(root, 'owned-shutdown.json')), 'Fixture must be stopped and waited for')
assert.equal(read(path.join(root, 'source-preservation.json')).unchanged, true)
assert.ok(source.evalId && source.config)
assert.equal(source.results.results.length, 1)
const [row] = source.results.results
const metadata = row.response.metadata
assert.equal(metadata.runtime, 'opencode-v2')
assert.equal(metadata.runtimeVersion, '2.0.29')
assert.equal(row.vars.task_id, 'checkout-failure')
const evidence = fs.realpathSync(metadata.evidence)
assert.equal(path.dirname(evidence), path.join(root, 'runs'))
const context = read(path.join(evidence, 'context.json')).data
const tools = read(path.join(evidence, 'tools.json'))
assert.equal(new Set(tools.map(tool => tool.id)).size, tools.length)
assert.ok(tools.every(tool => tool.sessionID === metadata.sessionID))
assert.equal(metadata.toolCalls, tools.length)
assert.ok(fs.existsSync(path.join(evidence, 'owned-shutdown.json')))
const policy = fs.readFileSync(path.join(evidence, 'policy.jsonl'), 'utf8').trim().split('\n').map(JSON.parse)
assert.equal(policy.filter(event => event.type === 'retry.veto').length, 0, 'Real pilot must have no retry attempts')
const assistants = context.filter(message => message.type === 'assistant')
assert.deepEqual(tools, assistants.flatMap(message => message.content.filter(part => part.type === 'tool')
  .map(part => ({ messageID: message.id, sessionID: metadata.sessionID, ...part }))), 'Tool records must match native context')
const { usage, usageAvailability, tokenUsage } = aggregateUsage(assistants)
assert.deepEqual(usage, metadata.usage)
assert.deepEqual(usageAvailability, metadata.usageAvailability)
const rawResponse = read(path.join(evidence, 'response.json'))
assert.deepEqual(rawResponse.tokenUsage, tokenUsage, 'Provider usage must retain field availability before Promptfoo defaults')
const evidenceErrors = tools.filter(tool => !Object.hasOwn(tool.state, 'input')).map(tool => ({
  toolID: tool.id, field: 'state.input', reason: 'Missing required V2 tool input',
}))
assert.deepEqual(metadata.evidenceErrors, evidenceErrors)
if (evidenceErrors.length) assert.ok(row.response.error, 'Malformed required input must fail the attempt')
let graderUsage, graderSessionID
const components = row.gradingResult?.componentResults ?? []
if (!row.response.error) {
  assert.equal(row.failureReason, 0)
  assert.equal(components.length, 1)
  const component = components[0]
  assert.equal(component.assertion.type, 'llm-rubric')
  const grader = component.metadata
  const graderEvidence = fs.realpathSync(grader.evidence)
  assert.equal(path.dirname(graderEvidence), path.join(root, 'runs'))
  assert.equal(grader.runtime, 'pi')
  assert.equal(grader.model, 'openai-codex/gpt-6.1-sol')
  assert.equal(grader.toolCalls, 0)
  assert.notEqual(grader.sessionID, metadata.sessionID)
  graderSessionID = grader.sessionID
  const events = fs.readFileSync(path.join(graderEvidence, 'events.jsonl'), 'utf8').trim().split('\n').map(JSON.parse)
  const graderMessage = events.filter(event => event.type === 'message_end' && event.message.role === 'assistant')
  assert.equal(graderMessage.length, 1)
  graderUsage = graderMessage[0].message.usage
  const supplied = JSON.parse(grader.renderedAssertionValue.split('\n').at(-1))
  const rubric = read(path.join(import.meta.dirname, 'rubrics.json'))['checkout-failure']
  assert.equal(supplied.reference, rubric.reference)
  assert.deepEqual(supplied.criteria, rubric.criteria)
  assert.equal(supplied.evidence.events.length, tools.length)
} else {
  assert.equal(row.failureReason, 2)
  assert.equal(components.length, 0)
}

const receipts = read(path.join(root, 'otlp-summary.json')).receipts
assert.ok(receipts.length)
for (const receipt of receipts) {
  assert.equal(receipt.status, 200, 'Do not reingest a partially delivered run')
  const partial = JSON.parse(receipt.reply).partialSuccess ?? {}
  assert.ok(!partial.errorMessage && !Number(partial.rejectedSpans ?? 0))
  assert.equal(receipt.contentType, 'application/json')
  assert.equal(receipt.contentEncoding, undefined)
}
const spans = receipts.flatMap(receipt => read(path.join(root, 'otlp', receipt.name)).resourceSpans
  .flatMap(resource => resource.scopeSpans.flatMap(scope => scope.spans)))
assert.equal(new Set(spans.map(span => span.traceId + '/' + span.spanId)).size, spans.length)
assert.ok(spans.every(span => span.traceId === row.traceId))
const toolEvents = spans.flatMap(span => (span.events ?? []).filter(event => event.name === 'tool.execution').map(event => ({ span, event })))
assert.equal(toolEvents.length, tools.length)
const attr = (attributes, key) => attributes.find(attribute => attribute.key === key)?.value
const identity = read(path.join(evidence, 'identity.json'))
for (const tool of tools) {
  const match = toolEvents.filter(({ event }) => attr(event.attributes, 'gen_ai.tool.call.id')?.stringValue === tool.id)
  assert.equal(match.length, 1)
  verifyToolEvent(tool, match[0].event)
  assert.equal(identity.promptfoo.traceparent.split('-')[2], match[0].span.spanId)
}
// Exercise the current UI projection against actual received OTLP data, without
// editing UI code, changing the export, or assigning a Pi type to V2 events.
const attributes = source => (source ?? []).map(attribute => {
  const [type, value] = Object.entries(attribute.value)[0]
  return { key: attribute.key, value: { kind: ({ stringValue: 'string', boolValue: 'bool', intValue: 'int64', doubleValue: 'double' })[type], value } }
})
const projected = evalEventTools(spans.map(span => ({ spanData: { traceID: span.traceId, spanID: span.spanId,
  attributes: attributes(span.attributes), events: (span.events ?? []).map(event => ({ name: event.name, attributes: attributes(event.attributes) })) } })))
const trace = row.traceId
assert.match(trace, /^[a-f0-9]{32}$/)
let native, nativeSummary
if (fs.existsSync(path.join(root, 'native-otlp/summary.json'))) {
  const nativeReceipts = read(path.join(root, 'native-otlp/summary.json')).receipts
  for (const receipt of nativeReceipts) {
    assert.equal(receipt.forwarded, true)
    assert.equal(receipt.status, 200)
    assert.ok(!Object.values(JSON.parse(receipt.reply).partialSuccess ?? {}).some(value => value && value !== '0'))
  }
  native = readNativeTelemetry(root, nativeReceipts)
  nativeSummary = nativeTelemetryBoundaries(native, identity.promptfoo.traceparent)
  assert.equal(nativeSummary.duplicateSpanRecords, 0, 'Report duplicate native deliveries; never silently deduplicate')
  const traceIDs = [...new Set(native.spans.map(span => span.traceId))]
  for (const id of traceIDs) assert.match(id, /^[a-f0-9]{32}$/)
  if (traceIDs.length) {
    // Aggregate in DuckDB so its query row cap cannot truncate large traces.
    const key = "replace(trace_id::VARCHAR,'-','') || '/' || span_id_wire(span_id)"
    const stored = await query('http://127.0.0.1:56008', `SELECT count(*),sha256(string_agg(${key}, ',' ORDER BY ${key})) FROM spans WHERE trace_id IN (${traceIDs.map(id => "'" + id + "'").join(',')})`)
    const expected = [...native.spans, ...spans.filter(span => traceIDs.includes(span.traceId))].map(span => span.traceId + '/' + span.spanId).sort()
    assert.deepEqual(stored.rows, [[expected.length, sha(expected.join(','))]], 'Stored native and wrapper span identities must match received bytes')
  }
  if (native.logs.length) {
    const times = native.logs.map(log => String(log.timeUnixNano))
    for (const time of times) assert.match(time, /^\d+$/)
    const stored = await query('http://127.0.0.1:56008', `SELECT count(*) FROM logs WHERE service_name='opencode' AND timestamp IN (${times.join(',')})`)
    assert.equal(stored.rows[0][0], native.logs.length, 'Stored native log count must match received records')
  }
}
const storedSpans = await query('http://127.0.0.1:56008', `SELECT span_id_wire(span_id) AS span_id, span_id_wire(parent_span_id) AS parent_span_id, name, status_code FROM spans WHERE trace_id='${trace}' ORDER BY start_time`)
assert.equal(storedSpans.rows.length, spans.length + (native?.spans.filter(span => span.traceId === trace).length ?? 0))
const storedEvents = await query('http://127.0.0.1:56008', `SELECT name,count(*) AS count FROM events WHERE trace_id='${trace}' GROUP BY name ORDER BY name`)
assert.equal(storedEvents.rows.find(([name]) => name === 'tool.execution')?.[1] ?? 0, tools.length)
const report = {
  evalId: source.evalId, resultID: row.id, traceID: row.traceId, investigatorSessionID: metadata.sessionID, graderSessionID,
  success: row.success, score: row.score, failureReason: row.failureReason,
  investigators: 1, graders: components.length, modelRequests: policy.filter(event => event.type === 'model.context').length,
  toolCalls: tools.length, failedToolCalls: metadata.failedToolCalls,
  nonzeroShellExits: tools.filter(tool => tool.state.metadata?.exit).length,
  usage, usageAvailability, graderUsage, originalNativeExportSHA256: before,
  otlpBatches: receipts.length, wrapperSpans: spans.length, wrapperToolEvents: toolEvents.length, nativeTelemetry: nativeSummary, storedSpans, storedEvents,
  uiRecognizedEventTools: projected.length,
  abstractionGaps: [
    ...(projected.length !== tools.length ? ['Current generic tool.execution projection does not recognize every received V2 call; inspect per-record status and attributes.'] : []),
    ...(row.response.tokenUsage?.completionDetails?.reasoning !== usage.reasoning ? ['Promptfoo completionDetails.reasoning differs from preserved V2 metadata.usage.reasoning; native export remains unchanged.'] : []),
    ...(row.response.tokenUsage?.total !== undefined ? ['Promptfoo supplied a total; V2 provider omitted it because the schema does not define additive token accounting.'] : []),
    ...(graderUsage && components[0].tokensUsed.total === 0 ? ['Approved Pi grader provider leaves native Promptfoo token totals at zero; actual received grader usage is preserved in its raw events and this report.'] : []),
  ],
}
save('received-data-verification.json', report)
if (args.includes('--import')) {
  if (fs.existsSync(path.join(root, 'preview-import.json'))) throw new Error('Import receipt already exists; do not resend')
  // Content-addressed server ingestion; send the original file bytes once.
  const response = await fetch('http://127.0.0.1:56008/api/evals/ingest?name=' + encodeURIComponent(path.basename(root) + '/promptfoo.json'), {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: bytes, signal: AbortSignal.timeout(60000),
  })
  const raw = await response.text()
  save('preview-import.json', { status: response.status, response: raw, originalNativeExportSHA256: before, otlp: 'Already accepted live; no retransmission' })
  assert.equal(response.status, 200, raw)
  const reply = JSON.parse(raw)
  assert.equal(reply.complete, true)
  const listed = await fetch('http://127.0.0.1:56008/api/evals/experiments?limit=100')
  assert.equal(listed.status, 200)
  const page = await listed.json()
  const matching = page.experiments.filter(experiment => reply.ids.includes(experiment.datasetID))
  assert.equal(matching.length, 1)
  assert.equal(matching[0].resultCount, 1)
  const selected = await fetch('http://127.0.0.1:56008/api/evals/experiments/' + matching[0].id + '/rows')
  assert.equal(selected.status, 200)
  const selectedPage = await selected.json()
  assert.equal(selectedPage.rows.length, 1)
  assert.deepEqual(JSON.parse(selectedPage.rows[0].json), row)
  save('preview-verification.json', { experiment: matching[0], nativeRowPreserved: true, rowID: row.id, otlpResent: false })
}
assert.equal(sha(fs.readFileSync(exportFile)), before, 'Native export bytes must remain unchanged')
console.log(JSON.stringify({ root, success: row.success, score: row.score, tools: tools.length, spans: spans.length, uiRecognizedEventTools: projected.length, imported: args.includes('--import'), abstractionGaps: report.abstractionGaps }))
