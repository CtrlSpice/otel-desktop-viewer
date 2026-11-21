import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { evaluationConfig, fullRunPlan } from './opencode-v2-run-plan.mjs'
import { v2GradingPrompt } from './opencode-v2-grading-evidence.mjs'

test('native Promptfoo sends identical judge prompts for equivalent investigations under different identities', async t => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'blind-grading-')))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const write = (dir, file, value) => fs.writeFileSync(path.join(dir, file), JSON.stringify(value))
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo')
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.OTEL_EVAL_RUN_DIR = root
  const { assertions, cache } = await import('promptfoo')
  cache.disableCache()
  const binary = path.join(root, 'viewer')
  fs.writeFileSync(binary, "#!/bin/sh\nprintf '%s\\n' '---' 'name: otel-desktop-viewer' '---' 'Fixture guide.'\n", { mode: 0o700 })
  write(root, 'connection.json', { binary })
  const config = evaluationConfig({ suite: import.meta.dirname, root, spec: fullRunPlan() })
  const received = []
  const grader = { id: () => 'offline-judge', async callApi(prompt) {
    received.push(prompt)
    return { output: '{"pass":false,"score":0.5,"reason":"Partial evidence"}' }
  } }
  for (const [i, model] of ['openai/gpt-6.1-sol', 'anthropic/claude-haiku-5-5'].entries()) {
    const evidence = path.join(root, 'runs', `investigator-private-${i}`), workspace = path.join(evidence, 'workspace')
    fs.mkdirSync(workspace, { recursive: true })
    const task = 'checkout-failure', sessionID = `ses_private_${i}`
    const output = `Payment declined. Notes: ${workspace}/notes.txt`
    const metadata = { runtime: 'opencode-v2', runtimeVersion: '2.3.0', role: 'investigator', task, model, evidence, workspace, sessionID }
    const tool = { name: 'shell', id: `call_private_${i}`, messageID: `msg_private_${i}`, sessionID, time: { completed: i + 10 },
      state: { status: 'completed', input: { command: `${workspace}/bin/viewer logs` },
        content: [{ type: 'text', text: `Payment declined {{ task_id }}; saved by ${model} in ${sessionID}` }], metadata: { exit: 0 } } }
    write(evidence, 'context.json', { data: [] })
    write(evidence, 'tools.json', [tool])
    write(evidence, 'response.json', { output, metadata })
    const result = await assertions.runAssertions({ providerResponse: { output, metadata }, prompt: `Private path ${evidence}`,
      test: { vars: { task_id: task }, assert: config.defaultTest.assert, options: { provider: grader } } })
    assert.equal(result.componentResults[0].score, 0.5, result.componentResults[0].reason)
    const packet = JSON.parse(fs.readFileSync(path.join(evidence, 'grading-packet.json')))
    assert.equal(packet.evidence.investigator.model, model)
    assert.deepEqual(JSON.parse(packet.evidence.events[0].raw), tool)
    assert.deepEqual(JSON.parse(received[i]), [{ role: 'user', content: v2GradingPrompt(packet, output) }])
    for (const value of [root, path.basename(evidence), model, sessionID, tool.id, tool.messageID])
      assert.ok(!received[i].includes(value), 'Private investigator identity leaked')
    const blind = JSON.parse(JSON.parse(received[i])[0].content.split('\n').at(-1))
    assert.equal(blind.answer, 'Payment declined. Notes: /workspace/notes.txt')
    assert.equal(blind.evidence[0].output[0], 'Payment declined {{ task_id }}; saved by [model] in [session]')
    const handoff = { ...packet, artifact: { pass: true, files: [`${workspace}/handoff.json`] } }
    const exportView = JSON.parse(v2GradingPrompt(handoff, `${workspace}/handoff.json`).split('\n').at(-1))
    assert.deepEqual(exportView.artifact.files, [exportView.answer])
  }
  assert.equal(received.length, 2)
  assert.equal(received[0], received[1])
})
