import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import OpenCodeV2Provider, { withSharedHost } from './opencode-v2-provider.mjs'
import { hostApi } from './opencode-v2-connect.mjs'
import { TEMPORARY } from './runtime.ts'
import { afterEach as executionStop, onFailure } from './opencode-v2-execution-stop.mjs'

function fixture(t) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(TEMPORARY, 'v2-teardown-test-')))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const binary = path.join(root, 'viewer')
  fs.writeFileSync(binary, 'fixture viewer')
  const registry = path.join(root, 'registry')
  fs.mkdirSync(registry)
  return { root, binary, registry }
}

test('shared host accepts interrupt and wait after cancelling ordinary requests', { timeout: 5000 }, async t => {
  const controller = new AbortController(), received = []
  const server = http.createServer((req, res) => {
    received.push(req.url)
    if (req.url === '/api/info') controller.abort(new Error('run cancelled'))
    else res.writeHead(204).end()
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  t.after(() => { server.closeAllConnections(); server.close() })
  const api = hostApi('http://127.0.0.1:' + server.address().port, {}, controller.signal)
  await assert.rejects(api('/api/info'), /run cancelled/)
  await assert.rejects(api('/api/plugin'), /run cancelled/)
  for (const route of ['/api/session/ses_fixture/interrupt', '/api/experimental/session/ses_fixture/wait'])
    await api(route, 'POST', undefined, undefined, AbortSignal.timeout(2000))
  assert.deepEqual(received, ['/api/info', '/api/session/ses_fixture/interrupt', '/api/experimental/session/ses_fixture/wait'])
})

for (const outcome of ['success', 'error', 'timeout', 'timeout-expected-abort', 'timeout-provider-error',
  'timeout-mixed-errors', 'timeout-unknown-abort', 'cancellation', 'host-cancellation', 'nondeadline-abort', 'shutdown-failure']) {
  test(`shared V2 harness teardown handles ${outcome} without stopping the shared host`, { timeout: 10000 }, async t => {
    const { root, binary, registry } = fixture(t)
    const receiver = http.createServer((_req, res) => res.end('{}'))
    receiver.listen(0, '127.0.0.1')
    await once(receiver, 'listening')
    t.after(() => { receiver.closeAllConnections(); receiver.close() })
    const endpoint = 'http://127.0.0.1:' + receiver.address().port
    const cancellation = new AbortController()
    const hostController = new AbortController()
    let interrupted = false, drained = false
    const history = [
      { type: 'assistant', content: [{ type: 'text', text: 'fixture answer' }], finish: 'stop',
        model: { providerID: 'openai', id: 'fixture-model' } },
      { type: 'idle', outcome: 'succeeded' },
    ]
    const guide = '---\nname: otel-desktop-viewer\n---\nFixture skill.'
    if (outcome === 'success') {
      fs.writeFileSync(binary, `#!/bin/sh\n[ "$1" = skills ] || exit 1\ncat <<'EOF'\n${guide}\nEOF\n`)
      fs.chmodSync(binary, 0o700)
      history[0].content.unshift({ type: 'tool', name: 'shell', id: 'call_fixture', time: { created: 1, ran: 1, completed: 2 },
        state: { status: 'completed', input: { command: './bin/otel-desktop-viewer skills' }, content: [{ type: 'text', text: guide }] } })
    }
    const originalBinary = fs.readFileSync(binary)
    if (outcome === 'timeout-provider-error') {
      history[0].error = { type: 'provider', status: 402 }
      history[1].outcome = 'failed'
    }
    if (['timeout-expected-abort', 'timeout-mixed-errors', 'nondeadline-abort', 'timeout-unknown-abort'].includes(outcome)) {
      history[0].error = { type: 'aborted', message: outcome === 'timeout-unknown-abort' ? 'Upstream canceled' : 'Step interrupted' }
      history[1].outcome = 'interrupted'
    }
    if (outcome === 'timeout-mixed-errors') history.unshift({
      type: 'assistant', content: [], finish: 'error', model: { providerID: 'openai', id: 'fixture-model' },
      error: { type: 'provider', status: 402 },
    })
    const host = {
      root, registry, signal: hostController.signal,
      telemetry: { receipts: [], async flush() {} },
      async checkPolicy() {},
      async api(route, _method, _body, _traceparent, signal) {
        if (route === '/api/session') return { data: { id: 'ses_fixture' } }
        if (route.endsWith('/interrupt')) {
          if (outcome === 'shutdown-failure') throw new Error('fixture could not interrupt')
          interrupted = true
          return {}
        }
        if (route.endsWith('/context')) return { data: interrupted || drained ? history : [] }
        if (route.endsWith('/prompt')) {
          if (outcome === 'error' || outcome === 'shutdown-failure') throw new Error('fixture prompt failed')
          if (outcome === 'cancellation') cancellation.abort(new Error('fixture cancellation'))
          if (outcome === 'host-cancellation') hostController.abort(new Error('fixture host cancellation'))
          return {}
        }
        if (route.endsWith('/wait')) {
          if (outcome.startsWith('timeout') || outcome === 'cancellation' || outcome === 'host-cancellation') {
            if (!interrupted) {
              if (signal.aborted) throw signal.reason
              // Keep the process alive while AbortSignal.timeout's unref'd timer runs.
              const keepAlive = setInterval(() => {}, 1000)
              try {
                await new Promise((resolve, reject) => signal.addEventListener('abort', () => reject(signal.reason), { once: true }))
              } finally { clearInterval(keepAlive) }
            }
          }
          drained = true
          return {}
        }
        if (route.endsWith('/export')) return { data: history }
        throw new Error('Unexpected fixture route: ' + route)
      },
    }
    const provider = new OpenCodeV2Provider({ config: { root, binary, model: 'fixture-model', sharedHost: true,
      fixtureEndpoint: endpoint, timeoutMs: outcome.startsWith('timeout') ? 40 : 5000 } })
    const attempt = withSharedHost(host, () => provider.callApi('fixture prompt', {
      vars: { task_id: 'trace-export' }, repeatIndex: 0,
      traceparent: '00-' + '1'.repeat(32) + '-' + '2'.repeat(16) + '-01',
    }, { abortSignal: cancellation.signal }))
    if (outcome === 'shutdown-failure') await assert.rejects(attempt, /Harness binary teardown failed/)
    else {
      const response = await attempt
      assert.equal(response.metadata.skillFetched, outcome === 'success' ? 'yes' : outcome.startsWith('timeout') || ['error','nondeadline-abort'].includes(outcome) ? 'no' : 'unknown')
      assert.equal(JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'response.json'))).metadata.skillFetched, response.metadata.skillFetched)
      if (outcome === 'success') assert.equal(response.output, 'fixture answer')
      else assert.ok(response.error)
      const ordinaryTimeout = ['timeout', 'timeout-expected-abort'].includes(outcome)
      if (ordinaryTimeout) {
        assert.equal(response.metadata.caseOutcome, 'investigator-timeout')
        assert.equal(response.error, 'No answer: investigator reached its time limit')
        assert.equal(response.output, undefined)
      } else assert.equal(response.metadata.caseOutcome, undefined)
      if (outcome.startsWith('timeout') || outcome === 'nondeadline-abort') {
        assert.deepEqual(response.metadata.errors.map(({ messageID, ...error }) => error),
          history.filter(item => item.error).map(item => item.error), 'Keep every raw context error')
      }
      if (['timeout-provider-error', 'timeout-mixed-errors'].includes(outcome))
        assert.match(response.error, /"type":"provider","status":402/)
      {
        let stopped = false
        onFailure(() => { stopped = true })
        try {
          executionStop({ result: { failureReason: response.error ? 2 : 0, response } })
          assert.equal(stopped, false, 'Saved outcomes do not cancel other scheduled cases')
        } finally { onFailure(undefined) }
      }
      assert.ok(drained, 'The session must finish before deletion')
      assert.deepEqual(fs.readdirSync(registry), [])
    }
    const evidence = path.join(root, 'runs', fs.readdirSync(path.join(root, 'runs'))[0])
    assert.ok(fs.existsSync(path.join(evidence, 'response.json')))
    assert.equal(fs.existsSync(path.join(evidence, 'workspace/bin/otel-desktop-viewer')), outcome === 'shutdown-failure')
    assert.equal(JSON.parse(fs.readFileSync(path.join(evidence, 'teardown.json'))).status, outcome === 'shutdown-failure' ? 'failed' : 'removed')
    assert.deepEqual(fs.readFileSync(binary), originalBinary)
    assert.equal(hostController.signal.aborted, outcome === 'host-cancellation')
  })
}

test('non-shared V2 preflight errors still remove the owned executable copy', async t => {
  const { root, binary } = fixture(t)
  const cli = path.join(root, 'opencode-fixture')
  fs.writeFileSync(cli, '#!/bin/sh\necho wrong-version\n', { mode: 0o700 })
  const provider = new OpenCodeV2Provider({ config: { root, binary, model: 'fixture-model', opencodeBinary: cli } })
  const response = await provider.callApi('fixture prompt')
  assert.equal(response.metadata.skillFetched, 'unknown')
  assert.match(response.error, /Only OpenCode/)
  assert.equal(fs.existsSync(path.join(response.metadata.workspace, 'bin/otel-desktop-viewer')), false)
  assert.equal(JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'teardown.json'))).status, 'removed')
  assert.equal(fs.readFileSync(binary, 'utf8'), 'fixture viewer')
})

for (const fault of ['session-start', 'prompt-response', 'wait-response', 'prompt-response-and-context', 'disk-full', 'cleanup', 'unavailable-fixture', 'missing-policy', 'context-read', 'export-read']) {
  test(`full benchmark scheduling after ${fault}`, async t => {
    const { root, binary, registry } = fixture(t)
    const controller = new AbortController()
    onFailure(error => controller.abort(error))
    t.after(() => onFailure(undefined))
    const receiver = http.createServer((_req, res) => res.writeHead(fault === 'unavailable-fixture' ? 503 : 200).end('{}'))
    receiver.listen(0, '127.0.0.1')
    await once(receiver, 'listening')
    t.after(() => { receiver.closeAllConnections(); receiver.close() })
    let attempted = 0, admitted = false, interrupted = false, prompts = 0
    const host = { root, registry, signal: controller.signal, telemetry: { receipts: [], async flush() {} },
      async checkPolicy() { if (fault === 'missing-policy') throw Error('isolation unavailable') },
      async api(route) {
        if (route === '/api/session') {
          if (fault === 'session-start' && attempted === 1) throw Error('One session refused')
          return { data: { id: 'ses_' + attempted } }
        }
        if (admitted && ['context-read', 'prompt-response-and-context'].includes(fault) && route.endsWith('/context')) throw Error('lost context response')
        if (admitted && fault === 'export-read' && route.endsWith('/export')) throw Error('lost session export')
        if (route.endsWith('/context')) return { data: admitted ? [
          { type: 'assistant', model: { providerID: 'openai', id: 'fixture' }, content: [{ type: 'text', text: 'answer' }], finish: 'stop' },
          { type: 'idle', outcome: 'succeeded' },
        ] : [] }
        if (route.endsWith('/prompt')) {
          admitted = true
          prompts++
          if (attempted === 1 && fault.startsWith('prompt-response')) throw Object.assign(Error('lost admission response'), {syscall: 'read', code: 'ECONNRESET'})
        }
        if (route.endsWith('/interrupt')) interrupted = true
        if (route.endsWith('/wait') && attempted === 1 && !interrupted && fault === 'wait-response') throw Object.assign(Error('lost wait response'), {syscall: 'read', code: 'ECONNRESET'})
        if (route.endsWith('/wait') && fault === 'cleanup') throw Error('cannot drain owned session')
        return {}
      } }
    if (fault === 'disk-full') {
      const write = fs.writeFileSync
      t.mock.method(fs, 'writeFileSync', (file, ...args) => {
        if (path.basename(file) === 'prompt.txt') throw Object.assign(Error('ENOSPC writing prompt'), { code: 'ENOSPC' })
        return write(file, ...args)
      })
    }
    const provider = new OpenCodeV2Provider({ config: { root, binary, model: 'fixture', sharedHost: true,
      fixtureEndpoint: `http://127.0.0.1:${receiver.address().port}`, timeoutMs: 5000 } })
    process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
    process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
    const { evaluate } = await import('promptfoo')
    const rowsFile = path.join(root, 'result.json')
    await withSharedHost(host, () => evaluate({
      prompts: ['{{task_id}}'], tests: ['one', 'two'].map(task_id => ({ vars: { task_id } })),
      providers: [{ id: () => provider.id(), callApi(prompt, context) {
        attempted++
        admitted = false
        interrupted = false
        return provider.callApi(prompt, { ...context, traceparent: `00-${'1'.repeat(32)}-${'2'.repeat(16)}-01` })
      } }], defaultTest: { assert: [{ type: 'llm-rubric', value: 'fixture' }],
        options: { provider: { id: () => 'offline-judge', callApi: async () => ({ output: '{"pass":true,"score":1,"reason":"fixture"}' }) } } },
      extensions: ['file://' + path.join(import.meta.dirname, 'opencode-v2-execution-stop.mjs') + ':afterEach'],
      outputPath: rowsFile,
    }, { maxConcurrency: 1, cache: false, share: false, abortSignal: controller.signal }))
    const recoverable = ['session-start','prompt-response','wait-response'].includes(fault)
    assert.equal(attempted, recoverable ? 2 : 1)
    assert.equal(controller.signal.aborted, !recoverable)
    const receipts = fs.readdirSync(path.join(root, 'runs')).map(name =>
      JSON.parse(fs.readFileSync(path.join(root, 'runs', name, 'teardown.json'))).status)
    assert.ok(receipts.every(status => status === (fault === 'cleanup' ? 'failed' : 'removed')))
    if (recoverable) {
      const rows = JSON.parse(fs.readFileSync(rowsFile)).results.results
      assert.equal(rows.length, 2)
      const first = rows.find(row => row.vars.task_id === 'one').response
      assert.equal(first.error, { 'session-start': 'One session refused', 'wait-response': 'lost wait response', 'prompt-response': 'lost admission response' }[fault])
      if (fault !== 'session-start') {
        assert.equal(first.output, 'answer')
        assert.equal(fs.readFileSync(path.join(first.metadata.evidence, 'answer.txt'), 'utf8'), 'answer')
        assert.ok(fs.existsSync(path.join(first.metadata.evidence, 'session-export.json')))
        assert.equal(prompts, 2, 'Exactly one submission per case')
      }
      assert.equal(rows.find(row => row.vars.task_id === 'two').success, true)
    } else assert.match(controller.signal.reason.message, {
      'disk-full': /ENOSPC/, cleanup: /teardown failed/, 'unavailable-fixture': /503/, 'missing-policy': /isolation unavailable/,
      'context-read': /Cannot capture investigator evidence: lost context response/, 'export-read': /Cannot capture investigator evidence: lost session export/,
      'prompt-response-and-context': /Cannot capture investigator evidence: lost context response/,
    }[fault])
    if (fault === 'export-read') {
      const dir = fs.readdirSync(path.join(root, 'runs'))[0]
      const saved = JSON.parse(fs.readFileSync(path.join(root, 'runs', dir, 'context.json')))
      assert.equal(saved.data[0].content[0].text, 'answer')
    }
  })
}
