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
const hashID = value => typeof value === 'string' && /^[0-9a-f]{64}$/.test(value)
const resultID = value => typeof value === 'string' && /^[0-9a-f-]{36}$/.test(value)
const ordinal = value => Number.isSafeInteger(value) && value >= 0
function checkReceipt(receipt, id) {
  assert.ok(receipt?.complete === true && typeof receipt.resultID === 'string' && receipt.resultID === id &&
    hashID(receipt.datasetID) && ordinal(receipt.ordinal) && hashID(receipt.retainedSHA256), 'Invalid publication receipt')
}
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
  const put = spec?.paths?.['/api/evals/runs/{runID}/results/{resultID}']?.put
  const required = put?.responses?.['200']?.content?.['application/json']?.schema?.required
  assert.ok(put?.operationId === 'putEvalResult' && Array.isArray(required) && required.includes('retainedSHA256'),
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
  assert.ok(typeof exported?.evalId === 'string' && exported.evalId === pending.run,'Unexpected exported run')
  const rows = exported.results.results.filter(r => r.traceId === pending.trace)
  assert.equal(rows.length, 1, 'Expected one persisted native grade for this trace')
  const row = rows[0]
  assert.ok(resultID(row.id),'Invalid saved result identity')
  const bytes = JSON.stringify(row)
  const prefix = path.join(dir, row.id)
  fs.writeFileSync(prefix + '.json', bytes, { flag: 'wx', mode: 0o600 })
  const reply = await fetch(`${endpoint}/api/evals/runs/${encodeURIComponent(pending.run)}/results/${row.id}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: bytes,
    redirect: 'error', signal: bounded(active.signal),
  })
  assert.equal(reply.status, 200, 'Viewer publication failed: HTTP ' + reply.status)
  const receipt = parseJSON(await reply.text())
  checkReceipt(receipt, row.id)
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
    active.signal?.throwIfAborted()
    const exported = readExport(path.join(active.dir,'../promptfoo.json'))
    if (active.pending) await publish(exported)
    assert.equal(active.count, exported.results.results.length)
    for (const row of exported.results.results)
      assert.ok(fs.readFileSync(path.join(active.dir,row.id+'.json'),'utf8') === JSON.stringify(row),'Published row differs from final native export')
  } finally { active = undefined }
}

export async function verifyLiveReadback(root,endpoint,signal) {
  const source = readExport(path.join(root,'promptfoo.json'))
  assert.ok(typeof source?.evalId === 'string' && Array.isArray(source.results?.results),'Invalid saved evaluation')
  const groups = new Map(), providers = new Map(), seen = new Set()
  const get = async route => {
    const res = await fetch(endpoint+route,{redirect:'error',signal:bounded(signal)})
    assert.ok(res.status === 200,'Could not read retained evaluation')
    return parseJSON(await res.text())
  }
  for (const row of source.results.results) {
    assert.ok(row?.evaluationId === source.evalId && resultID(row.id) &&
      typeof row.provider?.id === 'string','Invalid saved result identity')
    const prefix = path.join(root,'published-results',row.id)
    assert.ok(fs.readFileSync(prefix+'.json','utf8')===JSON.stringify(row),'Publication differs from final native result')
    const r = read(prefix+'.receipt.json')
    checkReceipt(r, row.id)
    if (!groups.has(r.datasetID)) groups.set(r.datasetID,new Map())
    const group = groups.get(r.datasetID)
    assert.ok(!group.has(r.ordinal),'Duplicate publication ordinal'); group.set(r.ordinal,{id:row.id,provider:row.provider.id,digest:r.retainedSHA256})
    if (!providers.has(row.provider.id)) providers.set(row.provider.id,r.datasetID)
    assert.ok(providers.get(row.provider.id)===r.datasetID,'Published model split across datasets')
  }
  assert.ok(providers.size === groups.size,'Unexpected publication dataset count')
  for (let offset=0;;offset+=100) {
    const page = await get('/api/evals/experiments?limit=100&offset='+offset)
    assert.ok(Array.isArray(page?.experiments) && typeof page.hasMore === 'boolean' &&
      (!page.hasMore || page.experiments.length > 0),'Invalid experiment page')
    for (const item of page.experiments) {
      assert.ok(hashID(item?.datasetID) && hashID(item.id) && ordinal(item.resultCount),'Invalid experiment summary')
      if (!groups.has(item.datasetID)) continue
      const group = groups.get(item.datasetID)
      assert.ok(item.resultCount === group.size,'Unexpected retained result count')
      for (let after=-1;;) {
        const detail = await get(`/api/evals/experiments/${item.id}/rows?limit=100&after=${after}`)
        assert.ok(typeof detail?.contextJSON === 'string' && Array.isArray(detail.rows) &&
          typeof detail.hasMore === 'boolean' && Number.isSafeInteger(detail.nextAfter) && detail.nextAfter >= -1,'Invalid retained result page')
        const context = parseJSON(detail.contextJSON)
        assert.ok(typeof context?.evalId === 'string' && context.evalId===source.evalId,'Unexpected retained run')
        let last = after
        for (const entry of detail.rows) {
          assert.ok(ordinal(entry?.ordinal) && typeof entry.json === 'string','Invalid retained result row')
          assert.ok(entry.ordinal > last,'Invalid retained result order')
          last = entry.ordinal
          const expected = group.get(entry.ordinal)
          // Hash the exact row JSON string, not a parsed and reserialized number.
          assert.ok(expected && !seen.has(expected.id) && sha(entry.json)===expected.digest,'Retained result digest mismatch')
          const row = parseJSON(entry.json)
          assert.ok(typeof row?.id === 'string' && typeof row.provider?.id === 'string' &&
            row.id===expected.id && row.provider.id===expected.provider,'Unexpected retained result identity')
          seen.add(expected.id)
        }
        assert.ok(detail.nextAfter === last && (!detail.hasMore || last > after),'Invalid retained result cursor')
        if (!detail.hasMore) break
        after=last
      }
    }
    if (!page.hasMore) break
  }
  assert.ok(seen.size === source.results.results.length,'Missing retained result')
  return {publication:'per-result',resultIDs:[...seen],otlpResent:false}
}
