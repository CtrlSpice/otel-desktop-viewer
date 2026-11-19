import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'
import ClaudeProvider, { claudeArguments, claudeEvidence, cancelClaudeAttempts } from './claude-provider.ts'

test('Claude tool counts deduplicate streamed message snapshots and match error results by call ID', () => {
  const message = { type: 'assistant', message: { model: 'claude-opus-5-5', content: [{ type: 'tool_use', id: 'one', name: 'Bash' }] } }
  const events = [message, message,
    { type: 'assistant', message: { model: '<synthetic>', content: [{ type: 'text', text: 'Not logged in' }] } },
    { type: 'assistant', message: { model: 'claude-opus-5-5', content: [{ type: 'tool_use', id: 'two', name: 'Read' }] } },
    { type: 'user', message: { content: [{ type: 'tool_result', tool_use_id: 'one', is_error: true }, { type: 'tool_result', tool_use_id: 'two' }] } },
    { type: 'user', message: { content: [{ type: 'tool_result', tool_use_id: 'one', is_error: true }] } },
  ]
  assert.deepEqual(claudeEvidence(events), { toolCalls: 2, failedToolCalls: 1, actualModels: ['claude-opus-5-5'] })
})

for (const outcome of ['success', 'error', 'timeout', 'cancellation']) {
  test(`Claude investigator teardown retains evidence after ${outcome}`, { timeout: 15000 }, async t => {
    const root = fs.mkdtempSync(path.join(tmpdir(), 'opencode/claude-teardown-offline-'))
    t.after(() => fs.rmSync(root, { recursive: true, force: true }))
    const cli = path.join(root, 'claude-fixture')
    const binary = path.join(root, 'viewer')
    const marker = path.join(root, 'started')
    fs.writeFileSync(binary, 'fixture viewer')
    const events = [
      { type: 'system', subtype: 'init', model: 'claude-opus-5-5', session_id: 'fixture-session' },
      { type: 'result', result: 'fixture answer', session_id: 'fixture-session' },
    ]
    fs.writeFileSync(cli, `#!${process.execPath}
if (process.argv[2] === '--version') console.log('2.1.307 (Claude Code)');
else {
  const fs = require('node:fs');
  fs.writeFileSync('handoff.json', '{"resourceSpans":[]}');
  fs.writeFileSync(${JSON.stringify(marker)}, 'ready');
  console.log(${JSON.stringify(JSON.stringify(events[0]))});
  ${outcome === 'success' ? `console.log(${JSON.stringify(JSON.stringify(events[1]))});` :
    outcome === 'error' ? 'process.exitCode = 1;' : 'setInterval(() => {}, 1000);'}
}
`, { mode: 0o700 })
    const provider = new ClaudeProvider({ config: { root, model: 'claude-opus-5-5', cliBinary: cli,
      binary, telemetryEndpoint: 'http://127.0.0.1:1', timeoutMs: outcome === 'timeout' ? 500 : 10000 } })
    const pending = provider.callApi('fixture prompt', { vars: { task_id: 'trace-export' } })
    if (outcome === 'cancellation') {
      const deadline = Date.now() + 5000
      while (!fs.existsSync(marker) && Date.now() < deadline) await delay(10)
      assert.ok(fs.existsSync(marker), 'Fixture must start before cancellation')
      await cancelClaudeAttempts()
    }
    const response = await pending
    if (outcome === 'success') assert.equal(response.output, 'fixture answer')
    else assert.match(response.error!, outcome === 'timeout' ? /timeout/ : /execution failed/)
    assert.equal(fs.existsSync(path.join(response.metadata.workspace, 'bin/otel-desktop-viewer')), false)
    assert.equal(fs.readFileSync(path.join(response.metadata.workspace, 'handoff.json'), 'utf8'), '{"resourceSpans":[]}')
    assert.equal(fs.readFileSync(binary, 'utf8'), 'fixture viewer')
    const evidence = response.metadata.evidence
    assert.deepEqual(JSON.parse(fs.readFileSync(path.join(evidence, 'response.json'), 'utf8')), response)
    assert.equal(JSON.parse(fs.readFileSync(path.join(evidence, 'teardown.json'), 'utf8')).status, 'removed')
    const shutdown = JSON.parse(fs.readFileSync(path.join(evidence, 'owned-shutdown.json'), 'utf8'))
    assert.throws(() => process.kill(shutdown.pid, 0), /ESRCH/)
  })
}

test('Claude grader has no tools, while investigator permissions stay unchanged', () => {
  const grader = claudeArguments('claude-opus-5-5', 'grader', 'saved evidence')
  assert.equal(grader[grader.indexOf('--tools') + 1], '')
  assert.ok(!grader.includes('--allowedTools'))
  assert.equal(grader.at(-1), '-p')
  assert.ok(!grader.includes('saved evidence'))
  for (const flag of ['--safe-mode', '--restricted', '--strict-mcp-config', '--no-chrome', '--no-session-persistence'])
    assert.ok(grader.includes(flag))
  const investigator = claudeArguments('claude-opus-5-5', 'investigator', 'investigate')
  assert.equal(investigator[investigator.indexOf('--tools') + 1], 'Bash,Read,Edit,Write')
  assert.ok(investigator.includes('Bash(./bin/otel-desktop-viewer *)'))
})

for (const scenario of ['success', 'wrong-model', 'tool-call', 'large-input']) test(`Claude grader retains ${scenario} subprocess evidence`, async t => {
  const root = fs.mkdtempSync(path.join(tmpdir(), 'opencode/claude-grader-offline-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const cli = path.join(root, 'claude-fixture')
  const model = scenario === 'wrong-model' ? 'claude-sonnet-5-5' : 'claude-opus-5-5'
  const events = [
    { type: 'system', subtype: 'init', model, session_id: 'fixture-session' },
    { type: 'assistant', message: { model, content: scenario === 'tool-call' ? [{ type: 'tool_use', id: 'unexpected', name: 'Read' }] : [{ type: 'text', text: 'fixture grade' }] } },
    { type: 'result', result: '{"pass":true,"score":1,"reason":"recorded answer"}', session_id: 'fixture-session' },
  ]
  fs.writeFileSync(cli, `#!/bin/sh\nif [ "$1" = "--version" ]; then\n  echo '2.1.326 (Claude Code)'\nelse\n  cat > received-prompt.txt\n  cat <<'EVENTS'\n${events.map(event => JSON.stringify(event)).join('\n')}\nEVENTS\nfi\n`, { mode: 0o700 })
  const source = { evaluationId: 'eval-source', resultID: 'source-row', repeatIndex: 2,
    traceparent: '00-' + '1'.repeat(32) + '-' + '2'.repeat(16) + '-01' }
  const provider = new ClaudeProvider({ config: { root, role: 'grader', model: 'claude-opus-5-5', cliBinary: cli,
    source, telemetryEndpoint: 'http://127.0.0.1:1', timeoutMs: 5000 } })
  const prompt = scenario === 'large-input' ? 'Recorded Unicode ☃\n' + 'x'.repeat(1_100_000) + '\n{{ unchanged }} {% unchanged %}' : 'saved evidence'
  const response = await provider.callApi(prompt, { vars: { task_id: 'checkout-failure' } })
  assert.equal(response.metadata.role, 'grader')
  assert.equal(response.metadata.runtimeVersion, '2.1.326')
  assert.deepEqual(response.metadata.source, source)
  assert.equal(response.metadata.inputTransport, 'stdin')
  assert.ok(!fs.existsSync(path.join(response.metadata.workspace, 'bin')))
  assert.equal(fs.readFileSync(path.join(response.metadata.workspace, 'received-prompt.txt'), 'utf8'), prompt)
  assert.equal(fs.readFileSync(path.join(response.metadata.evidence, 'prompt.txt'), 'utf8'), prompt)
  const launch = JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'launch.json'), 'utf8'))
  assert.ok(!launch.args.includes(prompt))
  assert.equal(launch.inputTransport, 'stdin')
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'response.json'), 'utf8')), response)
  if (scenario === 'success' || scenario === 'large-input') {
    assert.equal(response.error, undefined)
    assert.equal(response.metadata.toolCalls, 0)
    assert.ok(response.output)
  } else {
    assert.match(response.error!, scenario === 'wrong-model' ? /model mismatch/ : /grader used tools/)
  }
})
