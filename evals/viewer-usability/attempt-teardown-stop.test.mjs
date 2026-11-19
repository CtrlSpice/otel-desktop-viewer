import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import childProcess from 'node:child_process'
import net from 'node:net'
import { syncBuiltinESMExports } from 'node:module'
import { afterEach, onFailure } from './attempt-teardown-stop.mjs'
import { withBinaryTeardown } from './attempt-teardown.ts'
import { TEMPORARY, SUITE } from './runtime.ts'

test('teardown stop hook distinguishes cleanup failures from answer outcomes and investigator deadlines', () => {
  const errors = []
  onFailure(error => errors.push(error))
  try {
    for (const result of [
      { success: true },
      { success: false, failureReason: 1, error: 'Wrong answer' },
      { response: { error: 'No answer: investigator reached its time limit', metadata: { caseOutcome: 'investigator-timeout' } } },
    ]) afterEach({ result })
    assert.equal(errors.length, 0)
    for (const result of [
      { error: 'AggregateError: Harness binary teardown failed; see receipt' },
      { response: { error: 'Harness binary teardown failed' } },
      { gradingResult: { componentResults: [{ reason: 'Grader failed: Harness binary teardown failed' }] } },
    ]) afterEach({ result })
    assert.equal(errors.length, 3)
  } finally { onFailure(undefined) }
})

test('native Promptfoo stops scheduling after an actual harness teardown failure', async t => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'teardown-scheduling-test-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const cancellation = new AbortController()
  const canonicalBinary = path.join(root, 'viewer')
  fs.writeFileSync(canonicalBinary, 'canonical')
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
  const { evaluate } = await import('promptfoo')
  let attempts = 0
  onFailure(error => cancellation.abort(error))
  try {
    await evaluate({
      prompts: ['fixture'],
      providers: [{
        id: () => 'teardown-fixture',
        async callApi() {
          attempts++
          const directory = fs.mkdtempSync(path.join(root, 'attempt-'))
          fs.mkdirSync(path.join(directory, 'workspace/bin'), { recursive: true })
          fs.writeFileSync(path.join(directory, 'workspace/bin/otel-desktop-viewer'), 'changed evidence')
          return withBinaryTeardown({ directory, canonicalBinary, kind: 'attempt' }, async () => {
            fs.writeFileSync(path.join(directory, 'answer.txt'), 'answer before cleanup failure')
            return { output: 'fixture answer' }
          })
        },
      }],
      tests: [0, 1, 2].map(index => ({ description: `case-${index}` })),
      extensions: ['file://' + path.join(SUITE, 'attempt-teardown-stop.mjs') + ':beforeAll',
        'file://' + path.join(SUITE, 'attempt-teardown-stop.mjs') + ':afterEach'],
      outputPath: path.join(root, 'promptfoo.json'), writeLatestResults: true,
    }, { maxConcurrency: 1, cache: false, share: false, abortSignal: cancellation.signal })
    assert.equal(attempts, 1, 'No later test may start after teardown failure')
    assert.equal(cancellation.signal.aborted, true)
    const output = JSON.parse(fs.readFileSync(path.join(root, 'promptfoo.json')))
    assert.equal(output.results.results.length, 1)
    assert.match(output.results.results[0].error, /Harness binary teardown failed/)
    const attempt = fs.readdirSync(root).find(name => name.startsWith('attempt-'))
    assert.equal(fs.readFileSync(path.join(root, attempt, 'answer.txt'), 'utf8'), 'answer before cleanup failure')
    assert.equal(fs.readFileSync(path.join(root, attempt, 'workspace/bin/otel-desktop-viewer'), 'utf8'), 'changed evidence')
  } finally { onFailure(undefined) }
})

for (const runtime of ['pi', 'opencode-v2']) test(`${runtime} direct launch is rejected before any provider attempt`, async t => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'direct-launch-rejection-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
  const { evaluate } = await import('promptfoo')
  let attempts = 0, externalCalls = 0
  const forbidden = () => { externalCalls++; throw new Error('External access forbidden in direct-launch test') }
  for (const name of ['spawn', 'spawnSync', 'exec', 'execSync', 'execFile', 'execFileSync', 'fork'])
    t.mock.method(childProcess, name, forbidden)
  t.mock.method(net.Socket.prototype, 'connect', forbidden)
  t.mock.method(net.Server.prototype, 'listen', forbidden)
  t.mock.method(globalThis, 'fetch', forbidden)
  syncBuiltinESMExports()
  onFailure(undefined)
  try {
    await assert.rejects(evaluate({
      prompts: ['offline fixture'],
      providers: [{ id: () => `${runtime}:offline-fixture`, callApi: async () => {
        attempts++
        return { output: 'must not run' }
      } }],
      tests: [{ description: 'rejected direct launch' }],
      extensions: ['file://' + path.join(SUITE, 'attempt-teardown-stop.mjs') + ':beforeAll',
        'file://' + path.join(SUITE, 'attempt-teardown-stop.mjs') + ':afterEach'],
    }, { maxConcurrency: 1, cache: false, share: false, writeToDatabase: false }),
    error => {
      const message = runtime === 'pi'
        ? 'Use npm run evaluate --prefix evals/viewer-usability so cleanup failures stop the batch.'
        : 'Use the OpenCode evaluation launcher so cleanup failures stop the batch.'
      assert.equal(error.cause?.message, 'Transform failed ([file transform]): ' + message)
      return true
    })
    assert.equal(attempts, 0)
    assert.equal(externalCalls, 0)
  } finally {
    t.mock.restoreAll()
    syncBuiltinESMExports()
  }
})
