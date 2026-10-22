import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { evalEventTools } from '../../desktopexporter/internal/frontend/src/components/evals/eval-tool-events.ts'
import { aggregateUsage, toolEvidence } from './opencode-v2-evidence.mjs'
import { missingToolLogs } from './tool-logs.ts'
import { verifyToolEvent } from './opencode-v2-verify-evidence.mjs'
import { nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'
import { observerURL, validateSchedule, converge } from './opencode-v2-run-plan.mjs'
import { readJSON as read, sha256 as sha, loadRunTelemetry, awaitTelemetry } from './opencode-v2-observer.mjs'

const jsonLines = file => fs.existsSync(file) ? fs.readFileSync(file, 'utf8').split('\n').filter(Boolean).map(JSON.parse) : []
const attr = (attributes, key) => attributes?.find(attribute => attribute.key === key)?.value
const save = (file, value) => fs.writeFileSync(file, JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
const ownedEvidence = (root, file) => {
  const evidence = fs.realpathSync(file)
  assert.equal(path.dirname(evidence), path.join(root, 'runs'), 'Evidence must belong to this run')
  return evidence
}

export function verifyInvestigator(root, row, wrapper, rubric, question, nativeSpans = []) {
  const metadata = row.response.metadata
  assert.equal(metadata.runtime, 'opencode-v2')
  assert.equal(metadata.captureUnavailable ? metadata.expectedRuntimeVersion : metadata.runtimeVersion, '2.0.29')
  const evidence = ownedEvidence(root, metadata.evidence)
  const identity = read(path.join(evidence, 'identity.json'))
  const raw = read(path.join(evidence, fs.existsSync(path.join(evidence, 'provider-exception.json')) ? 'provider-exception.json' : 'response.json'))
  assert.deepEqual(raw.metadata, metadata, 'Original provider metadata must survive scheduling')
  assert.equal(raw.error, row.response.error)
  assert.equal(raw.output, row.response.output)
  const contextFile = path.join(evidence, 'context.json')
  const context = fs.existsSync(contextFile) ? read(contextFile).data : undefined
  if (!context) assert.ok(raw.error, 'A successful investigator requires captured native context')
  if (fs.existsSync(path.join(evidence, 'server-info.json'))) assert.ok(fs.existsSync(path.join(evidence, 'owned-shutdown.json')), 'Started V2 server must be stopped and waited for')
  const toolsFile = path.join(evidence, 'tools.json')
  const tools = fs.existsSync(toolsFile) ? read(toolsFile) : []
  if (!metadata.captureUnavailable) {
    const assistants = (context ?? []).filter(message => message.type === 'assistant')
    assert.deepEqual(tools, assistants.flatMap(message => message.content.filter(part => part.type === 'tool')
      .map(part => ({ messageID: message.id, sessionID: metadata.sessionID, ...part }))))
    const usage = aggregateUsage(assistants)
    assert.deepEqual(metadata.usage, usage.usage); assert.deepEqual(metadata.usageAvailability, usage.usageAvailability)
    assert.deepEqual(raw.tokenUsage, usage.tokenUsage, 'No invented provider token totals')
    assert.equal(metadata.toolCalls, tools.length)
    assert.equal(metadata.failedToolCalls, tools.filter(tool => tool.state.status === 'error').length)
    const malformed = tools.filter(tool => !Object.hasOwn(tool.state, 'input')).map(tool => ({ toolID: tool.id, field: 'state.input', reason: 'Missing required V2 tool input' }))
    assert.deepEqual(metadata.evidenceErrors, malformed)
    if (malformed.length) assert.ok(raw.error)
  } else assert.ok(raw.error, 'Capture unavailable must be an explicit execution error')
  const policy = jsonLines(path.join(evidence, 'policy.jsonl'))
  assert.equal(policy.filter(event => event.type === 'retry.veto').length, 0, 'No model retry attempts')
  const spans = wrapper.spans.filter(span => span.traceId === row.traceId)
  assert.ok(spans.length, 'Missing wrapper trace for result')
  const rootSpans = spans.filter(span => span.name === 'promptfoo.test_case')
  assert.equal(rootSpans.length, 1)
  assert.equal(Number(attr(rootSpans[0].attributes, 'promptfoo.repeat.index')?.intValue), identity.promptfoo.repeatIndex)
  const events = spans.flatMap(span => (span.events ?? []).filter(event => event.name === 'tool.execution').map(event => ({ span, event })))
  const logs = (wrapper.logs ?? []).filter(log => log.traceId === row.traceId && log.eventName === 'tool.execution')
  const received = nativeSpans.filter(span => span.traceId === row.traceId)
  const nativeEvents = received.flatMap(span => (span.events ?? []).filter(event => event.name === 'tool.execution').map(event => ({ span, event, native: true })))
  const records = [...events, ...nativeEvents, ...logs.map(event => ({ span: spans.find(span => span.spanId === event.spanId), event }))]
  assert.equal(records.length, tools.length)
  assert.equal(new Set(tools.map(tool => tool.id)).size, tools.length, 'Duplicate original tool IDs in attempt')
  for (const tool of tools) {
    assert.equal(tool.sessionID, metadata.sessionID)
    const matches = records.filter(({ event }) => attr(event.attributes, 'gen_ai.tool.call.id')?.stringValue === tool.id)
    assert.equal(matches.length, 1)
    if (matches[0].native) {
      const evidence = toolEvidence(tool)
      assert.equal(missingToolLogs([{ eventName: 'tool.execution', attributes: evidence.attributes, timestamp: evidence.timestamp, observedTimestamp: 0 }], row.traceId, [matches[0].span]).length, 0)
    } else {
      verifyToolEvent(tool, matches[0].event)
      assert.equal(matches[0].span.spanId, identity.promptfoo.traceparent.split('-')[2])
    }
  }
  if (raw.error) {
    assert.equal(row.failureReason, 2)
    assert.equal(row.gradingResult?.componentResults?.length ?? 0, 0, 'No grader after investigator execution error')
  }
  const packetFile = path.join(evidence, 'grading-packet.json')
  const packet = fs.existsSync(packetFile) ? read(packetFile) : undefined
  if (packet) {
    assert.equal(packet.question, question); assert.equal(packet.reference, rubric.reference); assert.deepEqual(packet.criteria, rubric.criteria)
    assert.deepEqual(packet.evidence.events.map(event => JSON.parse(event.raw)), tools)
  }
  const attributes = items => (items ?? []).map(item => {
    const [type, value] = Object.entries(item.value)[0]
    return { key: item.key, value: { kind: ({ stringValue: 'string', boolValue: 'bool', intValue: 'int64', doubleValue: 'double' })[type], value } }
  })
  const projected = evalEventTools([...spans, ...received].map(span => ({ spanData: { traceID: span.traceId, spanID: span.spanId, attributes: attributes(span.attributes),
    events: (span.events ?? []).map(event => ({ name: event.name, attributes: attributes(event.attributes) })) } })),
    logs.map((log, index) => ({ logRef: String(index), traceID: log.traceId, spanID: log.spanId, eventName: log.eventName, attributes: attributes(log.attributes) })))
  assert.equal(projected.length, tools.length, 'UI must recognize every uniquely identified recorded call')
  return { evidence, identity, raw, tools, packet, spans, report: { resultID: row.id, provider: row.provider, repeatIndex: identity.promptfoo.repeatIndex,
    traceID: row.traceId, sessionID: metadata.sessionID, success: row.success, score: row.score, failureReason: row.failureReason,
    error: row.error, responseError: raw.error, nativeErrors: metadata.errors, evidenceErrors: metadata.evidenceErrors,
    capturedContext: Boolean(context), captureUnavailable: metadata.captureUnavailable ?? false,
    modelRequests: policy.filter(event => event.type === 'model.context').length, tools: tools.length, failedTools: metadata.failedToolCalls,
    usage: metadata.usage, usageAvailability: metadata.usageAvailability, sourceTokenUsage: row.response.tokenUsage, sourceCost: row.response.cost,
    wrapperSpans: spans.length, wrapperToolEvents: events.length, wrapperToolLogs: logs.length, nativeToolEvents: nativeEvents.length, uiRecognizedEventTools: projected.length,
    rawWrapperUploads: [...new Set(spans.flatMap(span => wrapper.spanReceipts[span.traceId + '/' + span.spanId] ?? []))],
  } }
}

export function verifyGrader(root, row, investigator, graders) {
  const matches = graders.filter(item => item.identity.promptfoo.traceparent?.split('-')[1] === row.traceId)
  assert.ok(matches.length <= 1, 'Repeated grader invocation for result')
  const components = row.gradingResult?.componentResults ?? []
  if (investigator.raw.error) { assert.equal(matches.length, 0); return { invoked: false } }
  if (!matches.length) {
    assert.equal(row.success, false, 'Successful assessment must have a separate grader')
    assert.ok(row.failureReason === 2 && row.error || components.some(component => component.metadata?.graderError && component.reason), 'Missing grader requires retained execution failure detail')
    return { invoked: false, blocked: true, gradingResult: row.gradingResult, error: row.error }
  }
  const grader = matches[0]
  assert.ok(investigator.packet, 'Captured grading packet required for invoked graders')
  const prompt = fs.readFileSync(path.join(grader.evidence, 'prompt.txt'), 'utf8')
  const content = JSON.parse(prompt).map(message => message.content).join('\n')
  assert.ok(content.includes(JSON.stringify(investigator.packet)), 'Grader must receive the exact approved evidence packet')
  const responseFile = path.join(grader.evidence, 'response.json')
  if (!fs.existsSync(responseFile)) {
    // Approved Pi provider can throw before saving a response (for example,
    // credential preflight or spawn failure). Retain the native error row and
    // whatever evidence exists; do not fabricate a Pi response or usage.
    assert.equal(row.success, false)
    assert.ok(row.failureReason === 2 && row.error || components.some(component => component.metadata?.graderError && component.reason))
    return { invoked: true, evidence: grader.evidence, captureUnavailable: true, error: row.error, gradingResult: row.gradingResult }
  }
  const raw = read(responseFile)
  assert.equal(raw.metadata.runtime, 'pi'); assert.equal(raw.metadata.role, 'grader')
  assert.equal(raw.metadata.model, 'openai-codex/gpt-6.1-sol')
  assert.equal(raw.metadata.toolCalls, 0)
  if (raw.metadata.sessionID) assert.notEqual(raw.metadata.sessionID, row.response.metadata.sessionID)
  const events = jsonLines(path.join(grader.evidence, 'events.jsonl'))
  assert.equal(events.filter(event => event.type === 'auto_retry_start').length, 0)
  assert.equal(events.filter(event => event.type === 'tool_execution_start').length, 0)
  const messages = events.filter(event => event.type === 'message_end' && event.message.role === 'assistant')
  assert.ok(messages.length <= 1, 'Grader must be a single response, without tools/retries')
  if (!raw.error) assert.equal(messages.length, 1)
  assert.equal(components.length, 1)
  assert.equal(components[0].assertion.type, 'llm-rubric')
  if (components[0].metadata?.evidence) {
    assert.equal(ownedEvidence(root, components[0].metadata.evidence), grader.evidence)
    assert.equal(components[0].metadata.sessionID, raw.metadata.sessionID)
  }
  // Native failed/partial judgments and grader execution/JSON failures are valid
  // observed outcomes. Keep their recorded failureReason, score and explanation.
  if (raw.error) assert.equal(row.success, false)
  return { invoked: true, evidence: grader.evidence, sessionID: raw.metadata.sessionID, error: raw.error, exit: raw.metadata.exit,
    usage: messages[0]?.message.usage, gradingResult: row.gradingResult }
}

export async function importOriginal(root, endpoint, bytes, rows, request = fetch) {
  const digest = sha(bytes), receiptFile = path.join(root, 'preview-import.json')
  let receipt
  if (fs.existsSync(receiptFile)) {
    receipt = read(receiptFile)
    assert.equal(receipt.originalNativeExportSHA256, digest); assert.equal(receipt.endpoint, endpoint)
  } else {
    // Claim before sending: after an ambiguous network failure, never resend.
    save(path.join(root, 'preview-import.started.json'), { endpoint, originalNativeExportSHA256: digest })
    const response = await request(endpoint + '/api/evals/ingest?name=' + encodeURIComponent(path.basename(root) + '/promptfoo.json'), {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: bytes, signal: AbortSignal.timeout(60000),
    })
    receipt = { endpoint, status: response.status, response: await response.text(), originalNativeExportSHA256: digest, otlpResent: false }
    save(receiptFile, receipt)
  }
  assert.equal(receipt.status, 200, receipt.response)
  const reply = JSON.parse(receipt.response)
  assert.equal(reply.complete, true); assert.ok(Array.isArray(reply.ids))
  const get = async route => {
    const response = await request(endpoint + route, { signal: AbortSignal.timeout(10000) })
    assert.equal(response.status, 200); return response.json()
  }
  const experiments = await converge(async () => {
    const found = []
    for (let offset = 0; ; offset += 100) {
      const page = await get('/api/evals/experiments?limit=100&offset=' + offset)
      found.push(...page.experiments.filter(item => reply.ids.includes(item.datasetID)))
      if (!page.hasMore) break
    }
    return found
  }, found => found.reduce((sum, item) => sum + item.resultCount, 0) === rows.length)
  const imported = []
  for (const experiment of experiments) {
    let after = -1
    for (;;) {
      const page = await get('/api/evals/experiments/' + experiment.id + '/rows?limit=100&after=' + after)
      imported.push(...page.rows.map(item => JSON.parse(item.json)))
      if (!page.hasMore) break
      assert.ok(page.nextAfter > after); after = page.nextAfter
    }
  }
  assert.equal(imported.length, rows.length)
  assert.equal(new Set(imported.map(row => row.id)).size, rows.length)
  for (const row of rows) assert.deepEqual(imported.find(item => item.id === row.id), row, 'Every original source row must survive import')
  return { experiments, nativeRowsPreserved: true, resultIDs: imported.map(row => row.id), otlpResent: false }
}

export async function verifyRun(root, { importSource = false, env = process.env } = {}) {
  root = fs.realpathSync(root)
  const plan = read(path.join(root, 'run-plan.json'))
  assert.equal(plan.version, '2.0.29')
  const spec = { ...plan, models: plan.models ?? plan.config.providers.map(provider => provider.config.model) }
  const target = fs.existsSync(path.join(root, 'observer-target.json')) ? read(path.join(root, 'observer-target.json')) : { observerAPI: plan.observer }
  const endpoint = observerURL(env.OTEL_EVAL_OBSERVER_API ?? target.observerAPI)
  assert.equal(endpoint, observerURL(target.observerAPI), 'Verifier must use the recorded observer')
  const exportFile = path.join(root, 'promptfoo.json'), bytes = fs.readFileSync(exportFile), digest = sha(bytes)
  const source = JSON.parse(bytes), rows = source.results.results
  assert.ok(source.evalId && source.config)
  if (fs.existsSync(path.join(root, 'original-export-hashes.json'))) for (const [name, hash] of Object.entries(read(path.join(root, 'original-export-hashes.json')))) {
    assert.ok(['promptfoo.json', 'promptfoo-rows.jsonl'].includes(name))
    assert.equal(sha(fs.readFileSync(path.join(root, name))), hash, 'Original native export integrity')
  }
  assert.ok(fs.existsSync(path.join(root, 'owned-shutdown.json')))
  assert.equal(read(path.join(root, 'source-preservation.json')).unchanged, true)
  const snapshotFile = path.join(root, 'source-snapshot.json')
  const sourceDir = fs.existsSync(snapshotFile) ? path.join(root, 'sources') : import.meta.dirname
  if (fs.existsSync(snapshotFile)) for (const [file, hash] of Object.entries(read(snapshotFile).files))
    assert.equal(sha(fs.readFileSync(path.join(sourceDir, file))), hash, 'Source snapshot integrity')
  const identities = new Map(rows.map(row => {
    const evidence = ownedEvidence(root, row.response.metadata.evidence)
    return [evidence, read(path.join(evidence, 'identity.json'))]
  }))
  const slots = validateSchedule(rows, spec, identities)
  const telemetry = loadRunTelemetry(root)
  assert.ok(telemetry.wrapper.spans.every(span => rows.some(row => row.traceId === span.traceId)), 'Unexpected wrapper trace')
  const graders = fs.readdirSync(path.join(root, 'runs')).filter(name => name.startsWith('grader-')).map(name => {
    const evidence = path.join(root, 'runs', name)
    return { evidence, identity: read(path.join(evidence, 'identity.json')) }
  })
  const rubric = read(path.join(sourceDir, 'rubrics.json'))['checkout-failure']
  const question = read(path.join(sourceDir, 'tasks.json')).find(task => task.id === 'checkout-failure').goal
  const reports = [], nativeSessions = new Set(), nativeTools = new Set(), graderSessions = new Set()
  for (const row of rows) {
    const investigator = verifyInvestigator(root, row, telemetry.wrapper, rubric, question, [...telemetry.attempts.values()].flatMap(data => data.spans))
    assert.equal(investigator.identity.promptfoo.evaluationId, source.evalId)
    if (row.response.metadata.sessionID) {
      assert.ok(!nativeSessions.has(row.response.metadata.sessionID)); nativeSessions.add(row.response.metadata.sessionID)
    }
    for (const tool of investigator.tools) {
      const key = tool.sessionID + '/' + tool.id
      assert.ok(!nativeTools.has(key), 'Duplicate original session/tool identity'); nativeTools.add(key)
    }
    const grader = verifyGrader(root, row, investigator, graders)
    if (grader.sessionID) { assert.ok(!graderSessions.has(grader.sessionID)); graderSessions.add(grader.sessionID) }
    const native = telemetry.attempts.get(investigator.evidence) ?? telemetry.legacy
    if (spec.nativeTelemetry && !native) throw new Error('Missing native capture for scheduled attempt')
    if (native) {
      for (const resource of native.summary.resources) {
        const id = attr(resource?.attributes, 'eval.evidence.id')?.stringValue
        assert.equal(id, path.basename(investigator.evidence), 'Received native resource must identify its owned attempt')
      }
      if (!investigator.raw.error && spec.nativeTelemetry) assert.ok(native.spans.length)
    }
    reports.push({ ...investigator.report, evidence: investigator.evidence, grader,
      native: native ? nativeTelemetryBoundaries(native, investigator.identity.promptfoo.traceparent) : undefined,
      rawNativeUploads: native?.receipts?.map(receipt => path.join(investigator.evidence, 'native-otlp', receipt.name)),
    })
  }
  assert.equal(graders.length, reports.filter(report => report.grader.invoked).length, 'Unexpected orphan grader attempt')
  assert.equal(new Set(rows.map(row => row.response.metadata.evidence)).size, identities.size)
  const expectedEvidence = new Set(identities.keys())
  for (const evidence of telemetry.attempts.keys()) assert.ok(expectedEvidence.has(evidence), 'Unexpected native attempt')
  const observed = await awaitTelemetry(endpoint, telemetry, { fresh: spec.mode === 'sol-luna' })
  const reportDirectory = fs.mkdtempSync(path.join(root, 'verification-'))
  const report = { evalId: source.evalId, mode: spec.mode ?? 'one', slots, resultCount: rows.length,
    nativeOutcomes: rows.map(row => ({ id: row.id, success: row.success, score: row.score, failureReason: row.failureReason })),
    originalNativeExportSHA256: digest, observerAPI: endpoint, attempts: reports, observer: observed,
    wrapperScopes: telemetry.wrapper.summary.scopes, wrapperSpans: telemetry.wrapper.spans.length,
    wrapperToolEvents: reports.reduce((sum, item) => sum + item.wrapperToolEvents, 0),
    wrapperToolLogs: reports.reduce((sum, item) => sum + item.wrapperToolLogs, 0),
    investigators: reports.length, graders: graders.length, uploadsRetried: 0,
  }
  save(path.join(reportDirectory, 'received-data-verification.json'), report)
  let imported
  if (importSource) {
    imported = await importOriginal(root, endpoint, bytes, rows)
    save(path.join(reportDirectory, 'preview-verification.json'), imported)
  }
  assert.equal(sha(fs.readFileSync(exportFile)), digest)
  fs.writeFileSync(path.join(reportDirectory, 'handoff.md'), `# OpenCode V2 verification\n\nEval: ${source.evalId}\nResults: ${rows.length}; investigators: ${reports.length}; graders: ${graders.length}.\nSlots: ${slots.join(', ')}\nNative outcomes: ${JSON.stringify(report.nativeOutcomes)}\n\nObserver: ${endpoint}\nVerified spans: ${observed.expected.spans}; logs: ${observed.expected.logs}; events: ${observed.expected.events}; metric datapoints: ${observed.expected.datapoints}.\nWrapper spans: ${report.wrapperSpans}; wrapper tool events: ${report.wrapperToolEvents}.\nOriginal export SHA-256: ${digest}\nImport: ${imported ? 'all original rows verified; single import receipt at root' : 'not requested'}. OTLP never replayed.\n\nSee received-data-verification.json for per-attempt native/wrapper raw-upload paths, scope counts, native errors, usage availability, grader evidence, IDs and recorded failure/partial/pass outcomes.\n`, { flag: 'wx', mode: 0o600 })
  return { root, reportDirectory, resultCount: rows.length, slots, spans: observed.expected.spans, logs: observed.expected.logs, imported: Boolean(imported) }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const args = process.argv.slice(2)
  if (!args[0] || args.slice(1).some(arg => arg !== '--import')) throw new Error('Usage: opencode-v2-verify.mjs <completed-run-root> [--import]')
  console.log(JSON.stringify(await verifyRun(args[0], { importSource: args.includes('--import') })))
}
