import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { TEMPORARY } from './runtime.ts'
import { prepare } from './prepare.ts'
import { stageDataset, datasetSource } from './stage-dataset.ts'
import { ownedViewerStop } from './owned-viewer-stop.ts'

// Runs only with an explicitly selected, freshly built viewer. Model responses
// come from loopback; the only credential is a nonfunctional fixture string.
test('native tools propagate into viewer RPCs and retain success/error trace evidence', {
  skip: !process.env.OTEL_EVAL_VIEWER_BINARY, timeout: 90000,
}, async () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-native-telemetry-'))
  fs.chmodSync(root, 0o700)
  const observerRoot = path.join(root, 'observer'), run = path.join(root, 'run')
  for (const directory of [observerRoot, run]) {
    fs.mkdirSync(directory, { mode: 0o700 })
    fs.copyFileSync(process.env.OTEL_EVAL_VIEWER_BINARY!, path.join(directory, 'otel-desktop-viewer'))
    stageDataset(datasetSource(), path.join(directory, 'fixture'))
  }
  const save = (file: string, value: unknown) => fs.writeFileSync(path.join(root, file), JSON.stringify(value, null, 2), { flag: 'wx', mode: 0o600 })
  const previous = { ...process.env }
  let observer: Awaited<ReturnType<typeof prepare>> | undefined
  let viewer: Awaited<ReturnType<typeof prepare>> | undefined
  const requests: unknown[] = []
  let toolsReturned = 0
  const model = http.createServer(async (req, res) => {
    let raw = ''; for await (const chunk of req) raw += chunk
    const body = JSON.parse(raw)
    requests.push({ path: req.url, body })
    assert.equal(req.headers.traceparent, undefined, 'Model endpoint must not receive owned-server context')
    const tool = body.tools?.some((t: { name: string }) => t.name === 'bash') && toolsReturned < 2
    const sql = toolsReturned === 0 ? 'SELECT 1 AS value' : 'SELECT * FROM missing_telemetry_test_table'
    const callID = 'call_fixture_' + toolsReturned
    if (tool) toolsReturned++
    const item = tool
      ? { id: 'fc_' + callID, type: 'function_call', call_id: callID, name: 'bash', arguments: JSON.stringify({
        command: './bin/otel-desktop-viewer query "' + sql + '" --endpoint ' + viewer!.endpoint + ' --json', description: 'Check viewer query' }), status: 'completed' }
      : { id: 'msg_fixture', type: 'message', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: 'Fixture finished.', annotations: [] }] }
    const response = { id: 'resp_' + requests.length, object: 'response', created_at: Math.floor(Date.now() / 1000), model: 'gpt-6-luna', status: 'completed', output: [item],
      usage: { input_tokens: 20, output_tokens: 10, total_tokens: 30, input_tokens_details: { cached_tokens: 0 }, output_tokens_details: { reasoning_tokens: 0 } } }
    if (!body.stream) { res.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify(response)); return }
    res.writeHead(200, { 'Content-Type': 'text/event-stream' })
    let sequence = 0
    const emit = (type: string, data: object) => res.write('event: ' + type + '\ndata: ' + JSON.stringify({ type, sequence_number: sequence++, ...data }) + '\n\n')
    emit('response.created', { response: { ...response, status: 'in_progress', output: [] } })
    emit('response.output_item.added', { output_index: 0, item: { ...item, status: 'in_progress', ...(tool ? { arguments: '' } : { content: [] }) } })
    if (tool) {
      emit('response.function_call_arguments.delta', { item_id: item.id, output_index: 0, delta: item.arguments })
      emit('response.function_call_arguments.done', { item_id: item.id, output_index: 0, arguments: item.arguments })
    } else {
      const text = 'Fixture finished.'
      const part = { type: 'output_text', text, annotations: [] }
      emit('response.content_part.added', { item_id: item.id, output_index: 0, content_index: 0, part: { ...part, text: '' } })
      emit('response.output_text.delta', { item_id: item.id, output_index: 0, content_index: 0, delta: text })
      emit('response.output_text.done', { item_id: item.id, output_index: 0, content_index: 0, text })
      emit('response.content_part.done', { item_id: item.id, output_index: 0, content_index: 0, part })
    }
    emit('response.output_item.done', { output_index: 0, item })
    emit('response.completed', { response })
    res.end()
  })
  model.listen(0, '127.0.0.1'); await once(model, 'listening')
  const { port } = model.address() as { port: number }
  const { NativeAttempt, afterAll } = await import('./native-lifecycle.ts')
  try {
    observer = await prepare(observerRoot)
    const command = JSON.parse(fs.readFileSync(path.join(observerRoot, 'runtime/process.json'), 'utf8')).command as string[]
    Object.assign(process.env, { OTEL_EVAL_RUN_DIR: run,
      OTEL_EVAL_OBSERVER_HTTP: 'http://127.0.0.1:' + command[command.indexOf('--http') + 1],
      PROMPTFOO_CONFIG_DIR: path.join(run, 'promptfoo-state'), PROMPTFOO_DISABLE_TELEMETRY: '1' })
    viewer = await prepare(run, 'http://127.0.0.1:' + command[command.indexOf('--grpc') + 1])
    const authFile = path.join(root, 'auth.json')
    save('auth.json', { openai: { type: 'api', key: 'local-fixture-only' } })
    const attempt = await NativeAttempt.create('openai/gpt-6-luna', 'investigator', 'telemetry-fixture', {
      authFile, opencodeBinary: process.env.OTEL_EVAL_OPENCODE_BINARY ?? '/opt/homebrew/bin/opencode',
      permission: { '*': 'allow', task: 'deny' },
    }, false, 30000)
    const configFile = path.join(attempt.isolated.env.OPENCODE_CONFIG_DIR, 'opencode.json')
    const config = JSON.parse(fs.readFileSync(configFile, 'utf8'))
    config.provider.openai.options = { baseURL: 'http://127.0.0.1:' + port + '/v1', apiKey: 'local-fixture-only' }
    fs.writeFileSync(configFile, JSON.stringify(config))
    const { runNativeEvaluation } = await import('./native-evaluate.ts')
    const suite = { prompts: ['Run the scripted queries and report completion.'], providers: [attempt],
      tests: [{ vars: { eval_model: 'openai/gpt-6-luna', task_id: 'telemetry-fixture' }, assert: [{ type: 'equals', value: 'Fixture finished.' }] }] }
    // The runtime accepts native providers; its inferred default config type is narrower.
    const exit = await runNativeEvaluation(suite as unknown as Parameters<typeof runNativeEvaluation>[0], 1)
    assert.equal(exit, 0)
    const result = JSON.parse(fs.readFileSync(path.join(run, 'pilot.json'), 'utf8')).results.results[0]
    assert.equal(result.success, true, JSON.stringify(result.response))
    assert.equal(result.response.metadata.toolCalls, 2)
    assert.equal(result.response.metadata.failedToolCalls, 1)
    const receipt = JSON.parse(fs.readFileSync(path.join(attempt.evidence, 'telemetry.json'), 'utf8'))
    assert.equal(receipt.drained, true)
    assert.equal(receipt.incompleteOperations, 0)
    assert.equal(receipt.failure, null)
    // Graceful viewer shutdown flushes its native exporter before inspection.
    await ownedViewerStop(run, viewer)(); viewer = undefined
    const cli = async (...args: string[]) => JSON.parse((await promisify(execFile)(path.join(observerRoot, 'otel-desktop-viewer'), [...args, '--endpoint', observer!.endpoint, '--json'], { maxBuffer: 16 * 1024 * 1024 })).stdout)
    const sql = `SELECT service_name, name, span_id_wire(span_id), span_id_wire(parent_span_id), status_code FROM spans WHERE trace_id_wire(trace_id)='${receipt.traceID}' ORDER BY start_time`
    let stored
    for (let retry = 0; retry < 50; retry++) {
      stored = await cli('query', sql)
      const rows = stored.rows as [string, string, string, string | null, number][]
      const ids = new Set(rows.map(row => row[2]))
      if (rows.some(row => !row[3]) && rows.filter(row => row[1] === 'viewer.rpc query').length === 2 &&
          rows.every(row => !row[3] || ids.has(row[3]))) break
      await new Promise(resolve => setTimeout(resolve, 100))
    }
    save('trace-query.json', stored)
    const rows = stored.rows as [string, string, string, string | null, number][]
    const ids = new Set(rows.map(row => row[2]))
    assert.equal(rows.filter(row => !row[3]).length, 1)
    for (const row of rows) if (row[3]) assert(ids.has(row[3]), 'Missing parent: ' + JSON.stringify(row))
    assert.deepEqual([...new Set(rows.map(row => row[0]))].sort(), ['opencode', 'otel-desktop-viewer', 'viewer-eval'])
    const tools = rows.filter(row => row[1] === 'tool bash')
    const rpcs = rows.filter(row => row[1] === 'viewer.rpc query')
    assert.equal(tools.length, 2); assert.equal(rpcs.length, 2)
    assert.equal(rows.filter(row => row[1].startsWith('agent.turn')).length, 3)
    assert.equal(tools.filter(row => row[4] === 2).length, 1)
    assert.equal(rpcs.filter(row => row[4] === 2).length, 1)
    for (const rpc of rpcs) assert(tools.some(tool => tool[2] === rpc[3] && tool[4] === rpc[4]))
    for (const tool of tools) {
      const detail = await cli('span', receipt.traceID, tool[2])
      save('tool-' + tool[2] + '.json', detail)
      assert(detail.span.links.length > 0)
      const attrs = JSON.stringify(detail.span.attributes)
      assert(attrs.includes('gen_ai.tool.call.arguments'))
      assert(attrs.includes('gen_ai.tool.call.result'))
      for (const link of detail.span.links) {
        const linked = await cli('span', link.traceID, link.spanID)
        assert.equal(linked.status, 'found', 'Native detail link did not arrive')
      }
    }
    assert.throws(() => process.kill(Number(fs.readFileSync(path.join(attempt.evidence, 'process.pid'), 'utf8')), 0))
    save('completion.json', { receipt, modelRequests: requests.length, inference: 'local deterministic responses only' })
    console.log('Native telemetry evidence: ' + root)
  } finally {
    console.log('Retained telemetry check: ' + root)
    save('model-requests.json', requests)
    try { await afterAll() } finally {
      try { if (viewer) await ownedViewerStop(run, viewer)() } finally {
        if (observer) await ownedViewerStop(observerRoot, observer)()
        model.closeAllConnections()
        await new Promise<void>(resolve => model.close(() => resolve()))
        for (const key of Object.keys(process.env)) if (!(key in previous)) delete process.env[key]
        Object.assign(process.env, previous)
      }
    }
  }
})
