import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import PiProvider from './pi-provider.ts'
import ClaudeProvider from './claude-provider.ts'
import CrossFamilyJudge, { judgeFor } from './opencode-v2-judge.mjs'
import { evaluationConfig, fullRunPlan } from './opencode-v2-run-plan.mjs'

test('native rubric routes all twelve identities without passing the identity in the prompt', async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'cross-family-judge-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'state')
  const { assertions, cache } = await import('promptfoo')
  cache.disableCache()
  const calls = []
  for (const adapter of [PiProvider, ClaudeProvider]) t.mock.method(adapter.prototype, 'callApi', async function (prompt) {
    calls.push({ id: this.id(), prompt })
    return { output: '{"pass":true,"score":1,"reason":"fixture"}' }
  })
  const spec = fullRunPlan()
  const config = evaluationConfig({ suite: import.meta.dirname, root, spec })
  const options = config.defaultTest.options
  const assertion = { ...config.defaultTest.assert[0], value: 'Blind fixture criterion and answer' }
  for (const model of spec.models) {
    const result = await assertions.runAssertions({ provider: { id: () => `opencode-v2:${model}` },
      providerResponse: { output: 'fixture' }, test: { assert: [assertion], options }, traceId: '1'.repeat(32) })
    assert.equal(result.pass, true, result.reason)
    assert.equal(calls.at(-1).id, model.startsWith('openai/') ? 'claude-code:claude-opus-5-5' : 'pi:openai-codex/gpt-6.1-sol')
  }
  assert.equal(calls.length, 12)
  assert.equal(calls.filter(c => c.id.startsWith('claude-code:')).length, 3)
  assert.equal(new Set(calls.map(c => c.prompt)).size, 1)
  assert.throws(() => judgeFor('unrecognised'), /Unknown investigator/)
  await assert.rejects(new CrossFamilyJudge({ config: {} }).callApi('fixture', {}), /Unknown investigator/)
  assert.equal(calls.length, 12)
})
