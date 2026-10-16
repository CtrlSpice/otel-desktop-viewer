import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { readLiveData } from './live-data.ts'

const model = 'synthetic/investigator', grader = 'synthetic/grader'
const secret = 'PRIVATE_SENTINEL_DO_NOT_EMBED'
function fixture(t: test.TestContext) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'otel-live-data-'))
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }))
  const write = (file: string, value: unknown) => {
    const target = path.join(directory, file)
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.writeFileSync(target, JSON.stringify(value))
    return target
  }
  const plan = { models: [model], tasks: 1, repetitions: 3, sourceSHA256: 'frozen', synthetic: true }
  const manifest = write('plan.json', plan)
  write('batch/run-a/run-manifest.json', plan)
  write('batch/run-a/launch.json', {})
  const selection = { manifest, runs: [path.join(directory, 'batch')] }
  const report = (index: number, score: number) => ({
    taskID: 'checkout-failure', score, status: score === 1 ? 'pass' : score === 0.5 ? 'partial' : 'fail',
    metadata: { model: grader, elapsedSeconds: 1, sessionIDs: [`grader-${index}`], prompt: secret },
    investigator: { model, elapsedSeconds: index, toolCalls: 2, failedToolCalls: 0, sessionIDs: [`investigator-${index}`], evidence: path.join(directory, `batch/run-a/runs/investigator-${index}`), private: secret },
    criteria: [{ explanation: secret }],
  })
  return { directory, write, plan, selection, report }
}

test('live grades become results once, and the final pilot replaces reconstruction without duplication', t => {
  const f = fixture(t)
  f.write('batch/run-a/runs/investigator-1/metrics.json', f.report(1, 1).investigator)
  assert.equal(readLiveData(f.selection).data.attempts, 0, 'An ungraded investigator is not a completed result')
  const grade = f.report(1, 0.5)
  f.write('batch/run-a/runs/grader-1/grading.json', grade)
  const live = readLiveData(f.selection)
  assert.equal(live.state, 'running')
  assert.equal(live.data.attempts, 1)
  assert.equal(live.data.models[0].result.partial, 1)
  assert.equal(live.data.grader.sessions, 1)
  assert.doesNotMatch(JSON.stringify(live), new RegExp(secret))
  f.write('batch/run-a/pilot.json', { results: { results: [{ provider: { label: model }, vars: { task_id: grade.taskID }, response: { metadata: grade.investigator }, success: false, gradingResult: { score: 0.5 } }] } })
  assert.equal(readLiveData(f.selection).data.attempts, 1)
  assert.equal(readLiveData({ ...f.selection, runs: [...f.selection.runs, path.join(f.directory, 'batch/run-a')] }).data.attempts, 1)
})

test('execution errors stay unscored and a later active continuation takes precedence over the historical stopped state', t => {
  const f = fixture(t)
  const investigator = f.report(1, 1).investigator
  f.write('batch/run-a/runs/investigator-1/metrics.json', investigator)
  f.write('batch/run-a/execution-error.json', { model, task: 'checkout-failure', evidence: investigator.evidence })
  const stopped = readLiveData(f.selection)
  assert.equal(stopped.state, 'stopped')
  assert.equal(stopped.data.models[0].result.error, 1)
  assert.equal(stopped.data.models[0].result.scoredAttempts, 0)
  f.write('batch/run-b/run-manifest.json', f.plan)
  f.write('batch/run-b/launch.json', {})
  assert.equal(readLiveData(f.selection).state, 'running')
})

test('grader failures do not duplicate their investigator; recoveries remain separate', t => {
  const f = fixture(t)
  const grade = { ...f.report(1, 1), status: 'execution-error', score: undefined }
  f.write('batch/run-a/runs/grader-1/grading.json', grade)
  f.write('batch/run-a/runs/investigator-1/metrics.json', grade.investigator)
  f.write('batch/run-a/execution-error.json', { role: 'grader', model, task: grade.taskID, evidence: grade.investigator.evidence })
  f.write('batch/recovery/run-manifest.json', { ...f.plan, repetitions: 1, retryOf: grade.investigator.evidence })
  f.write('batch/recovery/runs/grader-2/grading.json', f.report(2, 1))
  const result = readLiveData(f.selection)
  assert.equal(result.data.attempts, 1)
  assert.equal(result.data.grader.executionErrors, 1)
  assert.equal(result.data.grader.sessions, 1)
  assert.equal(result.recoveries.length, 1)
  assert.equal(result.recoveries[0].models[0].result.displayPass, 1)
})

test('complete requires the full plan; duplicate attempts, incompatible sources and malformed records are rejected', t => {
  const f = fixture(t)
  for (let i = 1; i <= 3; i++) f.write(`batch/run-a/runs/grader-${i}/grading.json`, f.report(i, 1))
  f.write('batch/run-a/run-completion.json', { exit: 0 })
  assert.equal(readLiveData(f.selection).state, 'complete')
  f.write('batch/run-a/runs/grader-4/grading.json', f.report(3, 1))
  assert.throws(() => readLiveData(f.selection), /Duplicate/)
  fs.rmSync(path.join(f.directory, 'batch/run-a/runs/grader-4'), { recursive: true })
  f.write('batch/run-a/run-manifest.json', { ...f.plan, sourceSHA256: 'other' })
  assert.throws(() => readLiveData(f.selection), /different source/)
  f.write('batch/run-a/run-manifest.json', f.plan)
  fs.writeFileSync(path.join(f.directory, 'batch/run-a/runs/grader-3/grading.json'), '{')
  assert.throws(() => readLiveData(f.selection), SyntaxError)
})

test('error markers cannot read evidence outside the selected run', t => {
  const f = fixture(t)
  const outside = path.join(f.directory, 'outside')
  f.write('outside/metrics.json', f.report(1, 1).investigator)
  f.write('batch/run-a/execution-error.json', { evidence: outside })
  assert.throws(() => readLiveData(f.selection), /belong to the selected run/)
})
