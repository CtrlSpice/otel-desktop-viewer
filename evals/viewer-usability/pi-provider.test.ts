import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { SUITE, TEMPORARY } from './runtime.ts'
import { observerEnvironment } from './clean-environment.ts'

test('native providers × repetitions retain six fresh Pi sessions and untouched export identities', { skip: process.platform !== 'darwin', timeout: 120000 }, async () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'pi-native-repetitions-'))
  fs.chmodSync(root, 0o700)
  const blocked = path.join(root, 'private-canary.txt')
  fs.writeFileSync(blocked, 'PRIVATE_CANARY_MUST_NOT_REACH_MODEL')
  fs.writeFileSync(path.join(root, 'AGENTS.md'), 'CONTEXT_CANARY_MUST_NOT_REACH_MODEL')
  const binary = path.join(root, 'fixture-viewer')
  fs.writeFileSync(binary, '#!/bin/sh\nprintf PI_TOOL_OK\n', { mode: 0o700 })
  const requests: { messages: { role: string }[] }[] = []
  const batches: any[] = []
  const server = http.createServer(async (req, res) => {
    let body = ''
    for await (const chunk of req) body += chunk
    const request = JSON.parse(body)
    if (req.url === '/v1/traces') {
      batches.push(request)
      res.writeHead(200, { 'Content-Type': 'application/json' }).end('{}')
      return
    }
    requests.push(request)
    const turn = request.messages.filter((message: { role: string }) => message.role === 'tool').length
    const delta = turn < 2 ? { role: 'assistant', tool_calls: [{ index: 0, id: 'tool-' + turn, type: 'function', function: {
      name: turn === 0 ? 'bash' : 'read', arguments: JSON.stringify(turn === 0 ? { command: './bin/otel-desktop-viewer' } : { path: blocked }),
    } }] } : { role: 'assistant', content: 'PI_OK' }
    const chunk = (delta: unknown, finish_reason: string | null) => ({ id: 'fixture-' + requests.length, object: 'chat.completion.chunk', created: 1, model: request.model, choices: [{ index: 0, delta, finish_reason }] })
    res.writeHead(200, { 'Content-Type': 'text/event-stream' })
    res.write('data: ' + JSON.stringify(chunk(delta, null)) + '\n\n')
    res.end('data: ' + JSON.stringify(chunk({}, turn < 2 ? 'tool_calls' : 'stop')) + '\n\ndata: [DONE]\n\n')
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const address = server.address()
  assert.ok(address && typeof address === 'object')
  Object.assign(process.env, observerEnvironment({ OTEL_EVAL_OBSERVER_HTTP: `http://127.0.0.1:${address.port}` }))
  try {
    process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
    process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
    const packageName = 'promptfoo'
    const { evaluate } = await import(packageName)
    const address = server.address()
    assert.ok(address && typeof address === 'object')
    await evaluate({
      tracing: { enabled: true },
      prompts: ['{{goal}}'],
      providers: ['one', 'two'].map(model => ({ id: 'file://' + path.join(SUITE, 'pi-provider.ts'), label: model, config: {
        root, provider: 'fixture', model, role: 'investigator', binary, baseUrl: `http://127.0.0.1:${address.port}/v1`, timeoutMs: 15000,
      } })),
      tests: [{ description: 'case-one', vars: { task_id: 'case-one', goal: 'Use the viewer.' }, assert: [{ type: 'equals', value: 'PI_OK' }] }],
      outputPath: [path.join(root, 'promptfoo.json'), path.join(root, 'rows.jsonl')], writeLatestResults: true,
    }, { repeat: 3, cache: false, maxConcurrency: 1, share: false })
    const exported = JSON.parse(fs.readFileSync(path.join(root, 'promptfoo.json'), 'utf8'))
    const rows = exported.results.results
    assert.equal(exported.results.version, 3)
    assert.equal(typeof exported.evalId, 'string')
    assert.equal(exported.config.providers.length, 2)
    assert.equal(exported.config.tests.length, 1)
    assert.equal(rows.length, 6)
    assert.ok(rows.every((row: { success: boolean }) => row.success), JSON.stringify(rows))
    assert.equal(new Set(rows.map((row: any) => row.response.metadata.sessionID)).size, 6)
    const spans = batches.flatMap(batch => batch.resourceSpans.flatMap((r: any) => r.scopeSpans.flatMap((s: any) => s.spans)))
    const cases = spans.filter(span => span.name === 'promptfoo.test_case')
    assert.equal(cases.length, 6)
    const attribute = (span: any, key: string) => span.attributes.find((a: any) => a.key === key)?.value
    for (const row of rows) {
      assert.equal(row.response.metadata.promptfoo, undefined)
      const identity = JSON.parse(fs.readFileSync(path.join(row.response.metadata.evidence, 'identity.json'), 'utf8'))
      const [, traceId, spanId] = identity.promptfoo.traceparent.split('-')
      const providerSpan = spans.find(span => span.traceId === traceId && span.spanId === spanId)
      assert.ok(providerSpan, 'Pi response must identify an exported native Promptfoo provider span')
      const toolEvents = providerSpan.events.filter((event: any) => event.name === 'tool.execution')
      assert.equal(toolEvents.length, 2)
      assert.equal(attribute(toolEvents[0], 'gen_ai.tool.call.id')?.stringValue, 'tool-0')
      assert.equal(attribute(toolEvents[0], 'tool.is_error')?.boolValue, false)
      assert.equal(attribute(toolEvents[1], 'tool.is_error')?.boolValue, true)
      assert.match(attribute(toolEvents[0], 'tool.output')?.stringValue, /PI_TOOL_OK/)
      assert.equal(spans.filter(span => span.traceId === traceId && attribute(span, 'gen_ai.operation.name')?.stringValue === 'execute_tool').length, 0)
      const rootSpan = cases.find(span => span.traceId === traceId)
      assert.ok(rootSpan)
      assert.equal(providerSpan.parentSpanId, rootSpan.spanId)
      assert.equal(attribute(rootSpan, 'promptfoo.eval.id')?.stringValue, exported.evalId)
      assert.equal(Number(attribute(rootSpan, 'promptfoo.repeat.index')?.intValue), identity.promptfoo.repeatIndex)
    }
    for (const model of ['one', 'two']) {
      const group = rows.filter((row: any) => row.provider.id === `pi:fixture/${model}`)
      assert.equal(group.length, 3)
      assert.deepEqual(group.map((row: any) => JSON.parse(fs.readFileSync(path.join(row.response.metadata.evidence, 'identity.json'), 'utf8')).promptfoo.repeatIndex).sort(), [0, 1, 2])
      assert.ok(group.every((row: any) => row.repeatIndex === undefined && row.vars.task_id === 'case-one'))
    }
    assert.equal(requests.length, 18)
    assert.equal(requests.filter(request => request.messages.every(message => message.role !== 'assistant')).length, 6)
    assert.ok(!JSON.stringify(requests).includes('CANARY_MUST_NOT_REACH_MODEL'))
    for (const row of rows) {
      const events = fs.readFileSync(path.join(row.response.metadata.evidence, 'events.jsonl'), 'utf8').trim().split('\n').map(line => JSON.parse(line))
      const tools = events.filter(event => event.type === 'tool_execution_end')
      assert.equal(tools[0].isError, false)
      assert.match(JSON.stringify(tools[0].result), /PI_TOOL_OK/)
      assert.equal(tools[1].isError, true)
    }
  } finally {
    server.closeAllConnections()
    await new Promise<void>(resolve => server.close(() => resolve()))
  }
})
