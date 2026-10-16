import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { AttemptTelemetry, withOpenCodeContext } from './eval-telemetry.ts'
import { observerEnvironment } from './clean-environment.ts'

test('observer setup requires an explicit endpoint and rejects embedded credentials', () => {
  assert.deepEqual(observerEnvironment({}), {})
  assert.throws(() => observerEnvironment({ OTEL_EVAL_OBSERVER_HTTP: 'http://user:secret@localhost:4318' }))
  assert.equal(observerEnvironment({ OTEL_EVAL_OBSERVER_HTTP: 'http://localhost:4318/' }).PROMPTFOO_OTEL_ENDPOINT, 'http://localhost:4318/v1/traces')
})

test('native hook identities build connected turns/tools and scope HTTP context to the owned server', async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'otel-eval-context-test-'))
  fs.chmodSync(root, 0o700)
  const batches: any[] = []
  const probes: { url: string; parent?: string }[] = []
  const server = http.createServer(async (req, res) => {
    let body = ''
    for await (const chunk of req) body += chunk
    if (req.url === '/v1/traces') batches.push(JSON.parse(body))
    else probes.push({ url: req.url!, parent: req.headers.traceparent as string | undefined })
    res.writeHead(200, { 'Content-Type': 'application/json' }).end('{}')
  })
  server.listen(0, '127.0.0.1'); await once(server, 'listening')
  const address = server.address() as { port: number }
  const endpoint = `http://127.0.0.1:${address.port}`
  Object.assign(process.env, observerEnvironment({ OTEL_EVAL_OBSERVER_HTTP: endpoint }), {
    PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo'), PROMPTFOO_DISABLE_TELEMETRY: '1',
  })
  let identity: ReturnType<AttemptTelemetry['identity']> | undefined
  let toolParent = ''
  const packageName = 'promptfoo'
  const { evaluate } = await import(packageName)
  try {
    const evaluation = await evaluate({ writeLatestResults: true, tracing: { enabled: true }, prompts: ['fixture'], tests: [{ assert: [{ type: 'equals', value: 'finished' }] }],
      providers: [{ id: () => 'trace-fixture', async callApi(_prompt: string, callContext: { traceparent: string }) {
        const evidence = path.join(root, 'attempt'); fs.mkdirSync(evidence)
        const attempt = await new AttemptTelemetry({ root, evidence, model: 'fixture/model', role: 'investigator', task: 'request-count', parent: callContext.traceparent, redact: text => text.replaceAll('SECRET', '[REDACTED]') }).listen()
        const hook = async (event: unknown) => {
          const res = await fetch(attempt.endpoint, { method: 'POST', headers: { Authorization: 'Bearer ' + attempt.token }, body: JSON.stringify(event) })
          assert.equal(res.status, 200)
          return await res.json() as Record<string, string>
        }
        try {
          assert.equal((await fetch(attempt.endpoint, { method: 'POST', body: '{}' })).status, 403)
          await hook({ type: 'ready' })
          await hook({ type: 'event', event: { type: 'message.updated', properties: { info: { id: 'm1', sessionID: 's1', role: 'assistant', time: { created: Date.now() } } } } })
          const native = { traceId: '11111111111111111111111111111111', spanId: '2222222222222222', traceFlags: 1 }
          await hook({ type: 'tool.before', tool: 'bash', sessionID: 's1', callID: 'c1', messageID: 'm1', args: { command: 'SECRET' }, native })
          // A same-named tool in another session must not reuse its context.
          await hook({ type: 'tool.before', tool: 'bash', sessionID: 's2', callID: 'c1', args: {} })
          const carrier = await hook({ type: 'shell.env', sessionID: 's1', callID: 'c1', messageID: 'm1' })
          toolParent = carrier.traceparent
          const other = await hook({ type: 'shell.env', sessionID: 's2', callID: 'c1' })
          assert.notEqual(carrier.traceparent, other.traceparent)
          await withOpenCodeContext(endpoint, attempt.context, async () => {
            await fetch(endpoint + '/owned')
            // localhost is a different origin even when it reaches the same server.
            await fetch(`http://localhost:${address.port}/unowned`)
          })
          await fetch(endpoint + '/outside')
          await hook({ type: 'tool.after', tool: 'bash', sessionID: 's1', callID: 'c1', result: { output: 'failed SECRET', metadata: { exit: 1 } } })
          await hook({ type: 'tool.after', tool: 'bash', sessionID: 's2', callID: 'c1', result: { output: 'ok', metadata: { exit: 0 } } })
          await hook({ type: 'event', event: { type: 'message.updated', properties: { info: { id: 'm1', sessionID: 's1', role: 'assistant', time: { created: Date.now() - 100, completed: Date.now() } } } } })
          identity = attempt.identity()
          await hook({ type: 'drain' })
        } finally { await attempt.finish({ output: 'finished' }) }
        const interruptedEvidence = path.join(root, 'interrupted'); fs.mkdirSync(interruptedEvidence)
        const interrupted = await new AttemptTelemetry({ root, evidence: interruptedEvidence, model: 'fixture/model', role: 'investigator', task: 'cancelled', parent: callContext.traceparent, redact: text => text }).listen()
        interrupted.accept({ type: 'ready' })
        interrupted.accept({ type: 'tool.before', sessionID: 'stopped', callID: 'pending', tool: 'bash' })
        interrupted.accept({ type: 'drain' })
        await interrupted.finish({ error: 'User cancelled' })
        const receipt = JSON.parse(fs.readFileSync(path.join(interruptedEvidence, 'telemetry.json'), 'utf8'))
        assert.equal(receipt.incompleteOperations, 1)
        assert.equal(receipt.failure, 'User cancelled')
        assert.equal(interrupted.failure, 'Native operations did not report completion')
        return { output: 'finished' }
      } }],
    }, { cache: false, share: false, maxConcurrency: 1 })
    const summary = await evaluation.toEvaluateSummary()
    assert.equal(summary.results[0].success, true, JSON.stringify(summary.results[0].response))
    const spans = batches.flatMap(batch => batch.resourceSpans.flatMap((r: any) => r.scopeSpans.flatMap((s: any) => s.spans)))
    const attemptSpan = spans.find(span => span.spanId === identity!.spanID)
    assert(attemptSpan)
    const turn = spans.find(span => span.name === 'agent.turn 1')
    assert.equal(turn.parentSpanId, identity!.spanID)
    const tool = spans.find(span => span.spanId === toolParent.split('-')[2])
    assert.equal(tool.parentSpanId, turn.spanId)
    assert.equal(tool.traceId, identity!.traceID)
    assert.equal(tool.status.code, 2)
    const recordedTool = identity!.tools.find(entry => entry.spanID === tool.spanId)!
    assert.equal(BigInt(Date.parse(recordedTool.startedAt!)) * 1_000_000n, BigInt(tool.startTimeUnixNano))
    assert.equal(tool.links[0].traceId, '11111111111111111111111111111111')
    assert(!JSON.stringify(tool).includes('SECRET'))
    assert.match(probes.find(p => p.url === '/owned')!.parent!, /^00-/)
    assert.equal(probes.find(p => p.url === '/unowned')!.parent, undefined)
    assert.equal(probes.find(p => p.url === '/outside')!.parent, undefined)
    assert(spans.some(span => span.spanId === attemptSpan.parentSpanId))
    const cancelled = spans.find(span => span.name === 'investigator cancelled')
    assert.equal(cancelled.status.code, 2)
    assert.equal(spans.find(span => span.parentSpanId === cancelled.spanId).status.code, 2)
  } finally { await new Promise<void>(resolve => server.close(() => resolve())) }
})
