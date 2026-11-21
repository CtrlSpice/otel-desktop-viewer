import test, { type TestContext } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { withBinaryTeardown } from './attempt-teardown.ts'
import { TEMPORARY, readJson } from './runtime.ts'

function fixture(t: TestContext, kind: 'attempt' | 'export-replay' = 'attempt') {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'teardown-test-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const directory = path.join(root, 'owned')
  const canonicalBinary = path.join(root, 'retained-viewer')
  const executable = path.join(directory, kind === 'attempt' ? 'workspace/bin/otel-desktop-viewer' : 'otel-desktop-viewer')
  fs.mkdirSync(path.dirname(executable), { recursive: true })
  fs.writeFileSync(canonicalBinary, 'fixture executable')
  fs.copyFileSync(canonicalBinary, executable)
  return { root, directory, canonicalBinary, executable, kind }
}

for (const outcome of ['success', 'error', 'timeout', 'cancellation']) {
  test(`harness teardown removes only the verified executable after ${outcome}`, async t => {
    const state = fixture(t)
    const answer = path.join(state.directory, 'answer.txt')
    const exported = path.join(state.directory, 'workspace/export.json')
    const failure = new Error(outcome)
    const run = withBinaryTeardown(state, async register => {
      fs.writeFileSync(answer, 'saved answer')
      fs.writeFileSync(exported, '{"resourceSpans":[]}')
      register(async () => {
        assert.ok(fs.existsSync(state.executable), 'Shutdown must precede deletion')
        fs.writeFileSync(path.join(state.directory, 'owned-shutdown.json'), '{}')
      })
      if (outcome !== 'success') throw failure
      return 'completed'
    })
    if (outcome === 'success') assert.equal(await run, 'completed')
    else await assert.rejects(run, error => error === failure)
    assert.equal(fs.existsSync(state.executable), false)
    assert.equal(fs.readFileSync(state.canonicalBinary, 'utf8'), 'fixture executable')
    assert.equal(fs.readFileSync(answer, 'utf8'), 'saved answer')
    assert.equal(fs.readFileSync(exported, 'utf8'), '{"resourceSpans":[]}')
    assert.ok(fs.existsSync(path.join(state.directory, 'owned-shutdown.json')))
    const receipt = readJson(path.join(state.directory, 'teardown.json')) as Record<string, unknown>
    assert.equal(receipt.status, 'removed')
    assert.equal(receipt.attemptError, outcome === 'success' ? undefined : String(failure))
  })
}

test('replay teardown retains proof, fixture bytes and database', async t => {
  const state = fixture(t, 'export-replay')
  for (const file of ['export-proof.json', 'fixture/trace.json', 'runtime/fixture.duckdb']) {
    const target = path.join(state.directory, file)
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.writeFileSync(target, file)
  }
  await withBinaryTeardown(state, async () => {})
  assert.equal(fs.existsSync(state.executable), false)
  for (const file of ['export-proof.json', 'fixture/trace.json', 'runtime/fixture.duckdb'])
    assert.equal(fs.readFileSync(path.join(state.directory, file), 'utf8'), file)
})

test('partial setup with no executable records that nothing needed removal', async t => {
  const state = fixture(t)
  fs.unlinkSync(state.executable)
  await assert.rejects(withBinaryTeardown(state, async () => { throw new Error('setup failed') }), /setup failed/)
  assert.equal((readJson(path.join(state.directory, 'teardown.json')) as Record<string, unknown>).status, 'not-present')
})

for (const changed of ['content', 'executable-symlink', 'parent-symlink', 'missing-canonical']) {
  test(`teardown refuses ${changed} and retains the original attempt error`, async t => {
    const state = fixture(t)
    if (changed === 'content') fs.writeFileSync(state.executable, 'modified executable')
    if (changed === 'executable-symlink') {
      fs.unlinkSync(state.executable)
      fs.symlinkSync(state.canonicalBinary, state.executable)
    }
    if (changed === 'parent-symlink') {
      const original = path.dirname(state.executable)
      const moved = path.join(state.root, 'outside-bin')
      fs.renameSync(original, moved)
      fs.symlinkSync(moved, original)
    }
    if (changed === 'missing-canonical') fs.unlinkSync(state.canonicalBinary)
    const attemptError = new Error('original attempt failed')
    await assert.rejects(withBinaryTeardown(state, async () => { throw attemptError }), error => {
      assert.ok(error instanceof AggregateError)
      assert.match(error.message, /Harness binary teardown failed/)
      assert.equal(error.errors[0], attemptError)
      return true
    })
    assert.ok(fs.lstatSync(state.executable))
    const receipt = readJson(path.join(state.directory, 'teardown.json')) as Record<string, unknown>
    assert.equal(receipt.status, 'failed')
    assert.equal(receipt.attemptError, String(attemptError))
  })
}

test('failed process shutdown retains the executable and stops teardown', async t => {
  const state = fixture(t)
  const shutdownError = new Error('owned process still running')
  await assert.rejects(withBinaryTeardown(state, async register => {
    register(async () => { throw shutdownError })
  }), error => {
    assert.ok(error instanceof AggregateError)
    assert.equal(error.errors[0], shutdownError)
    return true
  })
  assert.ok(fs.existsSync(state.executable))
  assert.equal((readJson(path.join(state.directory, 'teardown.json')) as Record<string, unknown>).status, 'failed')
})

test('unwritable deletion receipt prevents deletion and keeps the teardown error identifiable', async t => {
  const state = fixture(t)
  fs.mkdirSync(path.join(state.directory, 'teardown-started.json'))
  fs.mkdirSync(path.join(state.directory, 'teardown.json'))
  await assert.rejects(withBinaryTeardown(state, async () => {}), /Harness binary teardown failed/)
  assert.ok(fs.existsSync(state.executable))
})
