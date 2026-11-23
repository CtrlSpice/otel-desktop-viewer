import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { promisify } from 'node:util'
import { stopRun } from './opencode-v2-execution-stop.mjs'

const exec = promisify(execFile)
const read = file => JSON.parse(fs.readFileSync(file))
let active

export function liveResultHooks(config, suite = import.meta.dirname) {
  const file = 'file://' + path.join(suite, 'opencode-v2-live-results.mjs')
  // Promptfoo groups grading when the worker pool is one. Every case runs on
  // its serial queue; the concurrent queue remains empty.
  config.evaluateOptions.maxConcurrency = 2
  config.tests = config.tests.map(test => ({ ...test, options: { ...test.options, runSerially: true } }))
  config.extensions = [...config.extensions ?? [], file + ':beforeEach', file + ':afterEach']
}

export function beginLiveResults(root, endpoint) {
  assert.ok(!active)
  const dir = path.join(root, 'published-results')
  fs.mkdirSync(dir, { mode: 0o700 })
  active = { dir, endpoint, count: 0 }
}

export function afterEach({ result }) {
  try {
    assert.ok(active && !active.pending)
    assert.ok(result.evaluationId && result.traceId)
    active.pending = { run: result.evaluationId, trace: result.traceId }
  } catch (error) { stopRun(error); throw error }
}

async function publish(exported) {
  const { dir, endpoint, pending } = active
  assert.equal(exported.evalId, pending.run)
  const rows = exported.results.results.filter(r => r.traceId === pending.trace)
  assert.equal(rows.length, 1, 'Expected one persisted native grade for this trace')
  const row = rows[0]
  assert.match(row.id, /^[0-9a-f-]{36}$/)
  const bytes = JSON.stringify(row)
  const prefix = path.join(dir, row.id)
  fs.writeFileSync(prefix + '.json', bytes, { flag: 'wx', mode: 0o600 })
  const reply = await fetch(`${endpoint}/api/evals/runs/${encodeURIComponent(pending.run)}/results/${row.id}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: bytes,
    redirect: 'error', signal: AbortSignal.timeout(90000),
  })
  assert.equal(reply.status, 200, 'Viewer publication failed: HTTP ' + reply.status)
  const receipt = await reply.json()
  assert.ok(receipt.complete && receipt.resultID === row.id)
  fs.writeFileSync(prefix + '.receipt.json', JSON.stringify(receipt), { flag: 'wx', mode: 0o600 })
  active.count++
  active.pending = undefined
}

export async function beforeEach() {
  if (!active?.pending) return
  try {
    // afterEach runs before Promptfoo assigns the persistent row ID. Export its
    // own persisted representation here, before admitting the next investigator.
    const snapshot = path.join(active.dir, 'latest-export.json')
    const cli = path.join(import.meta.dirname, 'node_modules/promptfoo/dist/src/entrypoint.js')
    await exec(process.execPath, [cli, 'export', 'eval', active.pending.run, '-o', snapshot])
    await publish(read(snapshot))
  } catch (error) { stopRun(error); throw error }
}

export async function finishLiveResults(exported) {
  try {
    if (active.pending) await publish(exported)
    assert.equal(active.count, exported.results.results.length)
    for (const row of exported.results.results)
      assert.deepEqual(read(path.join(active.dir, row.id + '.json')), row, 'Published row differs from final native export')
  } finally { active = undefined }
}
