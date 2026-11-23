import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { promisify } from 'node:util'
import { stopRun } from './opencode-v2-execution-stop.mjs'

const exec = promisify(execFile)
function parseJSON(text,reviver) {
  try { return JSON.parse(text,reviver) } catch { throw Error('Invalid JSON in evaluation readback') }
}
const read = file => parseJSON(fs.readFileSync(file,'utf8'))
let active
const sha = bytes => createHash('sha256').update(bytes).digest('hex')
// Keep numeric tokens when extracting native rows from a saved JSON export.
const readExport = file => parseJSON(fs.readFileSync(file, 'utf8'), (_k, value, ctx) =>
  typeof value === 'number' ? JSON.rawJSON(ctx.source) : value)

export function liveResultHooks(config, suite = import.meta.dirname) {
  const file = 'file://' + path.join(suite, 'opencode-v2-live-results.mjs')
  // Promptfoo groups grading when the worker pool is one. Every case runs on
  // its serial queue; the concurrent queue remains empty.
  config.evaluateOptions.maxConcurrency = 2
  config.tests = config.tests.map(test => ({ ...test, options: { ...test.options, runSerially: true } }))
  config.extensions = [...config.extensions ?? [], file + ':beforeEach', file + ':afterEach']
}

export async function checkLiveResultSupport(endpoint, signal) {
  const res = await fetch(endpoint + '/openapi.json', {redirect:'error', signal:bounded(signal)})
  assert.equal(res.status, 200, 'Could not inspect observer capabilities')
  const spec = parseJSON(await res.text())
  const put = spec.paths?.['/api/evals/runs/{runID}/results/{resultID}']?.put
  assert.ok(put?.operationId === 'putEvalResult' &&
    put.responses?.['200']?.content?.['application/json']?.schema?.required?.includes('retainedSHA256'),
    'Observer does not support verified result publication')
}

function bounded(signal) {
  return AbortSignal.any([AbortSignal.timeout(90000), ...(signal ? [signal] : [])])
}

export function beginLiveResults(root, endpoint, signal) {
  assert.ok(!active)
  const dir = path.join(root, 'published-results')
  fs.mkdirSync(dir, { mode: 0o700 })
  active = { dir, endpoint, signal, count: 0 }
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
    redirect: 'error', signal: bounded(active.signal),
  })
  assert.equal(reply.status, 200, 'Viewer publication failed: HTTP ' + reply.status)
  const receipt = parseJSON(await reply.text())
  assert.ok(receipt.complete && receipt.resultID === row.id)
  assert.ok(/^[0-9a-f]{64}$/.test(receipt.retainedSHA256),'Missing redacted result digest')
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
    await exec(process.execPath, [cli, 'export', 'eval', active.pending.run, '-o', snapshot], { signal: bounded(active.signal), killSignal: 'SIGKILL' })
    await publish(readExport(snapshot))
  } catch (error) { stopRun(error); throw error }
}

export async function finishLiveResults() {
  try {
    const exported = readExport(path.join(active.dir,'../promptfoo.json'))
    if (active.pending) await publish(exported)
    assert.equal(active.count, exported.results.results.length)
    for (const row of exported.results.results)
      assert.ok(fs.readFileSync(path.join(active.dir,row.id+'.json'),'utf8') === JSON.stringify(row),'Published row differs from final native export')
  } finally { active = undefined }
}

export async function verifyLiveReadback(root,endpoint,signal) {
  const source = readExport(path.join(root,'promptfoo.json'))
  const groups = new Map(), providers = new Map(), seen = new Set()
  const get = async route => {
    const res = await fetch(endpoint+route,{redirect:'error',signal:bounded(signal)})
    assert.equal(res.status,200)
    return parseJSON(await res.text())
  }
  for (const row of source.results.results) {
    assert.ok(row.evaluationId === source.evalId && /^[0-9a-f-]{36}$/.test(row.id),'Invalid saved result identity')
    const prefix = path.join(root,'published-results',row.id)
    assert.ok(fs.readFileSync(prefix+'.json','utf8')===JSON.stringify(row),'Publication differs from final native result')
    const r = read(prefix+'.receipt.json')
    assert.ok(r.complete && r.resultID===row.id && /^[0-9a-f]{64}$/.test(r.retainedSHA256),'Invalid publication receipt')
    if (!groups.has(r.datasetID)) groups.set(r.datasetID,new Map())
    const group = groups.get(r.datasetID)
    assert.ok(!group.has(r.ordinal)); group.set(r.ordinal,{id:row.id,provider:row.provider.id,digest:r.retainedSHA256})
    if (!providers.has(row.provider.id)) providers.set(row.provider.id,r.datasetID)
    assert.ok(providers.get(row.provider.id)===r.datasetID,'Published model split across datasets')
  }
  assert.equal(providers.size, groups.size)
  for (let offset=0;;offset+=100) {
    const page = await get('/api/evals/experiments?limit=100&offset='+offset)
    for (const item of page.experiments) if (groups.has(item.datasetID)) {
      const group = groups.get(item.datasetID)
      assert.equal(item.resultCount,group.size)
      for (let after=-1;;) {
        const detail = await get(`/api/evals/experiments/${item.id}/rows?limit=100&after=${after}`)
        assert.ok(parseJSON(detail.contextJSON).evalId===source.evalId,'Unexpected retained run')
        for (const entry of detail.rows) {
          const expected = group.get(entry.ordinal)
          // Hash the exact row JSON string, not a parsed and reserialized number.
          assert.ok(expected && !seen.has(expected.id) && sha(entry.json)===expected.digest,'Retained result digest mismatch')
          const row = parseJSON(entry.json)
          assert.ok(row.id===expected.id && row.provider?.id===expected.provider,'Unexpected retained result identity')
          seen.add(expected.id)
        }
        if (!detail.hasMore) break
        assert.ok(detail.nextAfter>after); after=detail.nextAfter
      }
    }
    if (!page.hasMore) break
  }
  assert.equal(seen.size,source.results.results.length)
  return {publication:'per-result',resultIDs:[...seen],otlpResent:false}
}
