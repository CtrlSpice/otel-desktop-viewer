import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { tmpdir } from 'node:os'
import { test } from 'node:test'
import { execFileSync } from 'node:child_process'
import { runMode, runSettings, assertFreshRoot, evaluationConfig, validateSchedule, converge } from './opencode-v2-run-plan.mjs'
import { aggregateUsage } from './opencode-v2-evidence.mjs'
import { verifyInvestigator, verifyGrader, importOriginal } from './opencode-v2-verify.mjs'
import { telemetryExpectation } from './opencode-v2-observer.mjs'
import Provider from './opencode-v2-provider.mjs'

const temporary = t => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(tmpdir(), 'opencode/opencode-v2-offline-')))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  return root
}
const write = (root, name, value) => fs.writeFileSync(path.join(root, name), JSON.stringify(value))
const traceID = n => n.toString(16).padStart(32, '0')
const spanID = n => n.toString(16).padStart(16, '0')
const rubric = { reference: 'fixture reference', criteria: { pass: 'fixture criterion' } }
const question = 'fixture question'

function sixRows(root) {
  fs.mkdirSync(path.join(root, 'runs'))
  const rows = [], identities = new Map(), graders = [], wrapper = { spans: [], spanReceipts: {} }
  const outcomes = ['pass', 'partial', 'investigator-error', 'fail', 'grader-error', 'pass']
  for (let index = 0; index < 6; index++) {
    const model = index < 3 ? 'gpt-6.1-sol' : 'gpt-6-luna', repeatIndex = index % 3, outcome = outcomes[index]
    const evidence = path.join(root, 'runs/investigator-opencode-v2-' + index)
    fs.mkdirSync(evidence)
    const trace = traceID(index + 1), parent = spanID(index + 101)
    const identity = { provider: 'opencode-v2:openai/' + model, promptfoo: { repeatIndex, evaluationId: 'eval_fixture', traceparent: `00-${trace}-${parent}-01` } }
    identities.set(evidence, identity); write(evidence, 'identity.json', identity)
    const assistants = outcome === 'investigator-error' ? [] : [{ type: 'assistant', id: 'message-' + index, content: [], tokens: { input: 0, output: 0, reasoning: 0, cache: { read: 0, write: 0 } } }]
    const usage = aggregateUsage(assistants)
    const metadata = { runtime: 'opencode-v2', runtimeVersion: '2.0.29', model: 'openai/' + model, repeatIndex, evidence,
      sessionID: 'ses_' + index, usage: usage.usage, usageAvailability: usage.usageAvailability, toolCalls: 0, failedToolCalls: 0, evidenceErrors: [] }
    const response = { metadata, ...(usage.tokenUsage ? { tokenUsage: usage.tokenUsage } : {}),
      ...(outcome === 'investigator-error' ? { error: 'fixture 401 intact' } : { output: 'fixture answer' }) }
    write(evidence, 'response.json', response); write(evidence, 'tools.json', [])
    if (outcome !== 'investigator-error') write(evidence, 'context.json', { data: assistants })
    const packet = { question, ...rubric, evidence: { events: [] } }
    const score = outcome === 'pass' ? 1 : outcome === 'partial' ? 0.5 : 0
    const row = { id: 'result-' + index, traceId: trace, provider: { id: identity.provider, label: 'OpenCode V2 ' + model },
      vars: { task_id: 'checkout-failure' }, response, success: outcome === 'pass', score,
      failureReason: outcome.includes('error') ? 2 : outcome === 'pass' ? 0 : 1,
      gradingResult: outcome === 'investigator-error' ? null : { componentResults: [{ assertion: { type: 'llm-rubric' }, pass: outcome === 'pass', score, reason: 'fixture judgment' }] } }
    if (outcome !== 'investigator-error') {
      write(evidence, 'grading-packet.json', packet)
      const graderEvidence = path.join(root, 'runs/grader-' + index); fs.mkdirSync(graderEvidence)
      const graderIdentity = { promptfoo: { traceparent: `00-${trace}-${spanID(index + 201)}-01` } }
      write(graderEvidence, 'identity.json', graderIdentity)
      const graderRaw = { metadata: { runtime: 'pi', role: 'grader', model: 'openai-codex/gpt-6.1-sol', sessionID: 'grader_session_' + index, toolCalls: 0, exit: outcome === 'grader-error' ? 1 : 0 },
        ...(outcome === 'grader-error' ? { error: 'fixture grader error intact' } : { output: JSON.stringify({ pass: row.success, score, reason: 'fixture judgment' }) }) }
      write(graderEvidence, 'response.json', graderRaw)
      write(graderEvidence, 'prompt.txt', [{ role: 'user', content: 'Instructions\n' + JSON.stringify(packet) }])
      fs.writeFileSync(path.join(graderEvidence, 'events.jsonl'), outcome === 'grader-error' ? '' : JSON.stringify({ type: 'message_end', message: { role: 'assistant', usage: { input: 0, output: 0 } } }) + '\n')
      graders.push({ evidence: graderEvidence, identity: graderIdentity })
    }
    wrapper.spans.push({ traceId: trace, spanId: spanID(index + 301), name: 'promptfoo.test_case', attributes: [{ key: 'promptfoo.repeat.index', value: { intValue: repeatIndex } }], events: [] },
      { traceId: trace, spanId: parent, name: row.provider.label, attributes: [], events: [] })
    rows.push(row)
  }
  return { rows, identities, graders, wrapper }
}

test('specific six-run plan uses real catalogue IDs and ordinary repeat scheduling', () => {
  const spec = runMode(['--run-sol-luna'])
  assert.deepEqual(spec.models, ['gpt-6.1-sol', 'gpt-6-luna']); assert.equal(spec.repeat, 3); assert.equal(spec.nativeTelemetry, true)
  const config = evaluationConfig({ suite: '/suite', root: '/fresh', spec, task: { id: 'checkout-failure', goal: 'approved question' }, connection: { endpoint: 'http://127.0.0.1:61001' },
    binary: '/fresh/otel-desktop-viewer', authFile: '/private-auth', authEntry: 'openai-codex', piAuthFile: '/private-pi', observerHTTP: 'http://127.0.0.1:61002' })
  assert.deepEqual(config.evaluateOptions, { repeat: 3, maxConcurrency: 1, cache: false })
  assert.equal(config.tests.length, 1); assert.equal(config.providers.length, 2)
  assert.equal(new Set(config.providers.map(provider => provider.label)).size, 2)
  assert.ok(config.providers.every(provider => provider.config.telemetryForward === 'http://127.0.0.1:61002'))
  assert.equal(config.defaultTest.options.provider.config.model, 'gpt-6.1-sol')
  assert.equal(config.defaultTest.assert[0].threshold, 1)
  assert.ok(!JSON.stringify(config).includes('gpt-6.1-luna'))
  assert.ok(!JSON.stringify(config).includes('56010'))
  assert.equal(runMode(['--run-one']).repeat, 1)
  assert.equal(runMode(['--run-one', '--native-telemetry']).nativeTelemetry, true)
  assert.throws(() => runMode(['--run-one', '--run-sol-luna']))
})

test('new mode requires explicit observer/binary and preserves main-owned root files', t => {
  const root = temporary(t), spec = runMode(['--run-sol-luna'])
  fs.mkdirSync(path.join(root, 'observer')); fs.writeFileSync(path.join(root, 'otel-desktop-viewer'), 'main-owned binary fixture')
  const connection = { endpoint: 'http://127.0.0.1:61001', otlpHTTP: 'http://127.0.0.1:61002', database: path.join(root, 'observer/fresh.duckdb'), pid: 123 }
  write(path.join(root, 'observer'), 'connection.json', connection)
  const env = { OTEL_EVAL_RUN_DIR: root, OTEL_EVAL_OBSERVER_API: connection.endpoint, OTEL_EVAL_OBSERVER_HTTP: connection.otlpHTTP, OTEL_EVAL_VIEWER_BINARY: path.join(root, 'otel-desktop-viewer') }
  assertFreshRoot(root); assert.equal(runSettings(spec, env, root).observerAPI, connection.endpoint)
  for (const key of Object.keys(env)) { const missing = { ...env }; delete missing[key]; assert.throws(() => runSettings(spec, missing, root), /Required/) }
  assert.throws(() => runSettings(spec, { ...env, OTEL_EVAL_OBSERVER_HTTP: 'http://127.0.0.1:56010' }, root), /differs/)
  assert.equal(fs.readFileSync(env.OTEL_EVAL_VIEWER_BINARY, 'utf8'), 'main-owned binary fixture')
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(root, 'observer/connection.json'))), connection)
  write(root, 'run-started.json', {}); assert.throws(() => assertFreshRoot(root), /reuse/)
})

test('six rows accept mixed pass/partial/fail/investigator and grader errors, preserving repeat identities', t => {
  const root = temporary(t), data = sixRows(root), spec = runMode(['--run-sol-luna'])
  const before = JSON.stringify(data.rows)
  assert.equal(validateSchedule(data.rows, spec, data.identities).length, 6)
  const results = data.rows.map(row => {
    const investigator = verifyInvestigator(root, row, data.wrapper, rubric, question)
    return verifyGrader(root, row, investigator, data.graders)
  })
  assert.equal(results.filter(result => result.invoked).length, 5)
  assert.equal(results[2].invoked, false)
  assert.equal(results[4].error, 'fixture grader error intact')
  assert.equal(JSON.stringify(data.rows), before)
  const bad = structuredClone(data.rows); bad[5].response.metadata.repeatIndex = 0
  assert.throws(() => validateSchedule(bad, spec, data.identities))
  const duplicate = [...data.rows.slice(0, 5), data.rows[0]]
  assert.throws(() => validateSchedule(duplicate, spec, data.identities), /Distinct/)
})

test('offline preflight exceptions retain separate slot evidence and do not prevent following calls', async t => {
  const root = temporary(t)
  class OfflineFailure extends Provider { async callAttempt() { throw new Error('offline preflight failure') } }
  const provider = new OfflineFailure({ config: { root, model: 'gpt-6-luna' } })
  const results = []
  for (let repeatIndex = 0; repeatIndex < 3; repeatIndex++) results.push(await provider.callApi('offline', { repeatIndex, traceparent: `00-${traceID(repeatIndex + 1)}-${spanID(1)}-01`, vars: { task_id: 'checkout-failure' } }))
  assert.equal(new Set(results.map(result => result.metadata.evidence)).size, 3)
  for (const [index, result] of results.entries()) {
    assert.equal(result.error, 'offline preflight failure'); assert.equal(result.metadata.repeatIndex, index)
    assert.equal(result.metadata.captureUnavailable, true)
    assert.equal(result.metadata.runtimeVersion, undefined, 'Do not claim a started runtime on preflight failure')
    assert.deepEqual(JSON.parse(fs.readFileSync(path.join(result.metadata.evidence, 'response.json'))), result)
  }
})

test('grader preflight exceptions remain verifiable without invented response or usage', t => {
  const root = temporary(t), data = sixRows(root), row = data.rows[4]
  fs.unlinkSync(path.join(data.graders[3].evidence, 'response.json'))
  row.error = 'Approved grader preflight failed'
  row.gradingResult = null
  const investigator = verifyInvestigator(root, row, data.wrapper, rubric, question)
  const grader = verifyGrader(root, row, investigator, data.graders)
  assert.equal(grader.invoked, true); assert.equal(grader.captureUnavailable, true)
  assert.equal(grader.usage, undefined)
  row.failureReason = 1; row.error = 'Ordinary failed criterion'
  assert.throws(() => verifyGrader(root, row, investigator, data.graders))
})

test('collector convergence is bounded and repeats only observations', async () => {
  let now = 0, reads = 0
  const result = await converge(async () => ++reads, value => value === 3, { timeoutMs: 10, intervalMs: 2, now: () => now, sleep: async ms => { now += ms } })
  assert.equal(result, 3); assert.equal(reads, 3)
  now = 0; reads = 0
  await assert.rejects(converge(async () => { reads++; return 0 }, value => value === 6,
    { timeoutMs: 5, intervalMs: 2, now: () => now, sleep: async ms => { now += ms } }), /deadline/)
  assert.ok(reads <= 4)
})

test('large telemetry expectations retain all native IDs, events and metric datapoints', () => {
  const spans = Array.from({ length: 8001 }, (_, index) => ({ traceId: traceID(1), spanId: spanID(index + 1), events: index === 0 ? [{ name: 'exception' }] : [] }))
  const result = telemetryExpectation({ spans, logs: [{ timeUnixNano: '1792660000000000001' }], metrics: [{ gauge: { dataPoints: [{ asInt: '0' }] } }] })
  assert.equal(result.spans, 8001); assert.equal(result.events, 1); assert.equal(result.datapoints, 1)
  assert.throws(() => telemetryExpectation({ spans: [spans[0], spans[0]], logs: [], metrics: [] }), /Duplicate/)
})

test('import sends original six-row bytes once and verifies both model groups including failures', async t => {
  const root = temporary(t), { rows } = sixRows(root), bytes = Buffer.from(JSON.stringify({ results: { results: rows } }) + '\n  ')
  let posts = 0
  const request = async (url, options) => {
    if (options?.method === 'POST') {
      posts++; assert.deepEqual(options.body, bytes)
      return { status: 200, text: async () => JSON.stringify({ complete: true, ids: ['dataset'] }) }
    }
    const value = url.includes('/rows?') ? { rows: (url.includes('/sol/') ? rows.slice(0, 3) : rows.slice(3)).map(row => ({ json: JSON.stringify(row) })), hasMore: false }
      : { experiments: [{ id: 'sol', datasetID: 'dataset', resultCount: 3 }, { id: 'luna', datasetID: 'dataset', resultCount: 3 }], hasMore: false }
    return { status: 200, json: async () => value }
  }
  const result = await importOriginal(root, 'http://127.0.0.1:61001', bytes, rows, request)
  assert.equal(result.resultIDs.length, 6)
  await importOriginal(root, 'http://127.0.0.1:61001', bytes, rows, request)
  assert.equal(posts, 1, 'Second verification uses receipt, not a second upload')
})

test('ambiguous import failure cannot trigger an upload retry', async t => {
  const root = temporary(t); let uploads = 0
  const request = async () => { uploads++; throw new Error('connection ended after upload') }
  await assert.rejects(importOriginal(root, 'http://127.0.0.1:61001', Buffer.from('{}'), [], request), /connection ended/)
  await assert.rejects(importOriginal(root, 'http://127.0.0.1:61001', Buffer.from('{}'), [], request), /EEXIST/)
  assert.equal(uploads, 1)
})

test('actual Promptfoo schedules six uncached slots through provider errors and persists native repeat traces', t => {
  const root = temporary(t), providerFile = path.join(root, 'offline-provider.mjs')
  fs.writeFileSync(providerFile, `
    import fs from 'node:fs'
    import path from 'node:path'
    export default class OfflineProvider {
      constructor({config, label}) { this.config = config; this.label = label }
      id() { return 'opencode-v2:openai/' + this.config.model }
      async callApi(prompt, context) {
        const evidence = fs.mkdtempSync(path.join(this.config.root, 'attempt-'))
        const identity = {provider: this.id(), promptfoo: {repeatIndex: context.repeatIndex, evaluationId: context.evaluationId, traceparent: context.traceparent}}
        fs.writeFileSync(path.join(evidence, 'identity.json'), JSON.stringify(identity))
        const metadata = {model: 'openai/' + this.config.model, evidence, repeatIndex: context.repeatIndex}
        return context.repeatIndex === 1 ? {error: 'offline scheduled failure', metadata} : {output: 'OK', metadata}
      }
    }
  `)
  const script = `
    import fs from 'node:fs'
    import path from 'node:path'
    import { nativeTelemetryReceiver } from './opencode-v2-telemetry.mjs'
    const root = ${JSON.stringify(root)}
    const receiver = await nativeTelemetryReceiver(root, undefined, {directoryName: 'otlp'})
    process.env.PROMPTFOO_OTEL_ENABLED = 'true'
    process.env.PROMPTFOO_OTEL_ENDPOINT = receiver.endpoint + '/v1/traces'
    process.env.PROMPTFOO_OTEL_LOCAL_EXPORT = 'false'
    const { evaluate } = await import('promptfoo')
    try {
      await evaluate({prompts: ['offline only'], tracing: {enabled: true},
        providers: ['gpt-6.1-sol', 'gpt-6-luna'].map(model => ({id: ${JSON.stringify('file://' + providerFile)}, label: 'OpenCode V2 ' + model, config: {root, model}})),
        tests: [{vars: {task_id: 'checkout-failure'}, assert: [{type: 'javascript', value: 'output === "OK"'}]}],
        outputPath: path.join(root, 'promptfoo.json'),
      }, {repeat: 3, maxConcurrency: 1, cache: false, share: false})
    } finally { await receiver.close() }
  `
  execFileSync(process.execPath, ['--input-type=module', '-e', script], { cwd: import.meta.dirname, timeout: 30000,
    env: { ...process.env, PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'), PROMPTFOO_DISABLE_TELEMETRY: '1', LOG_LEVEL: 'error' }, stdio: 'pipe' })
  const rows = JSON.parse(fs.readFileSync(path.join(root, 'promptfoo.json'))).results.results
  const identities = new Map(rows.map(row => [row.response.metadata.evidence, JSON.parse(fs.readFileSync(path.join(row.response.metadata.evidence, 'identity.json')))]))
  assert.equal(validateSchedule(rows, runMode(['--run-sol-luna']), identities).length, 6)
  assert.equal(rows.filter(row => row.failureReason === 2).length, 2)
  assert.ok(rows.every(row => !Object.hasOwn(row.vars, '__repeatIndex')), 'Use captured native repeatIndex, not removed runtime vars')
  const receipts = JSON.parse(fs.readFileSync(path.join(root, 'otlp/summary.json'))).receipts
  const spans = receipts.flatMap(receipt => JSON.parse(fs.readFileSync(path.join(root, 'otlp', receipt.name))).resourceSpans.flatMap(resource => resource.scopeSpans.flatMap(scope => scope.spans)))
  for (const row of rows) {
    const roots = spans.filter(span => span.traceId === row.traceId && span.name === 'promptfoo.test_case')
    assert.equal(roots.length, 1)
    assert.equal(Number(roots[0].attributes.find(attribute => attribute.key === 'promptfoo.repeat.index').value.intValue), row.response.metadata.repeatIndex)
  }
})
