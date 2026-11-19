import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import OpenCodeV2Provider, { withSharedHost } from './opencode-v2-provider.mjs'
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

for (const outcome of ['success', 'error', 'timeout', 'timeout-provider-error', 'cancellation', 'shutdown-failure']) {
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
    if (outcome === 'timeout-provider-error') {
      history[0].error = { type: 'provider', status: 402 }
      history[1].outcome = 'failed'
    }
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
          return {}
        }
        if (route.endsWith('/wait')) {
          if (outcome.startsWith('timeout') || outcome === 'cancellation') {
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
      if (outcome === 'success') assert.equal(response.output, 'fixture answer')
      else assert.ok(response.error)
      if (outcome === 'timeout') assert.equal(response.metadata.caseOutcome, 'investigator-timeout')
      if (outcome === 'timeout-provider-error') {
        assert.equal(response.metadata.caseOutcome, undefined)
        assert.match(response.error, /"type":"provider","status":402/)
        assert.equal(response.metadata.errors[0].status, 402)
        let stopped = false
        onFailure(() => { stopped = true })
        try {
          executionStop({ result: { failureReason: 2, response } })
          assert.equal(stopped, true)
          stopped = false
          executionStop({ result: { failureReason: 2, response: { ...response,
            error: 'No answer: investigator reached its time limit',
            metadata: { ...response.metadata, caseOutcome: 'investigator-timeout' } } } })
          assert.equal(stopped, true)
        } finally { onFailure(undefined) }
      }
      assert.ok(drained, 'The session must finish before deletion')
      assert.deepEqual(fs.readdirSync(registry), [])
    }
    const evidence = path.join(root, 'runs', fs.readdirSync(path.join(root, 'runs'))[0])
    assert.ok(fs.existsSync(path.join(evidence, 'response.json')))
    assert.equal(fs.existsSync(path.join(evidence, 'workspace/bin/otel-desktop-viewer')), outcome === 'shutdown-failure')
    assert.equal(JSON.parse(fs.readFileSync(path.join(evidence, 'teardown.json'))).status, outcome === 'shutdown-failure' ? 'failed' : 'removed')
    assert.equal(fs.readFileSync(binary, 'utf8'), 'fixture viewer')
    assert.equal(hostController.signal.aborted, false)
  })
}

test('non-shared V2 preflight errors still remove the owned executable copy', async t => {
  const { root, binary } = fixture(t)
  const cli = path.join(root, 'opencode-fixture')
  fs.writeFileSync(cli, '#!/bin/sh\necho wrong-version\n', { mode: 0o700 })
  const provider = new OpenCodeV2Provider({ config: { root, binary, model: 'fixture-model', opencodeBinary: cli } })
  const response = await provider.callApi('fixture prompt')
  assert.match(response.error, /Only OpenCode/)
  assert.equal(fs.existsSync(path.join(response.metadata.workspace, 'bin/otel-desktop-viewer')), false)
  assert.equal(JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'teardown.json'))).status, 'removed')
  assert.equal(fs.readFileSync(binary, 'utf8'), 'fixture viewer')
})
