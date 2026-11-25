import assert from 'node:assert/strict'
import { once } from 'node:events'
import fs from 'node:fs'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { beginLiveResults, finishLiveResults, liveResultHooks } from './opencode-v2-live-results.mjs'
import { onFailure } from './opencode-v2-execution-stop.mjs'

// Promptfoo binds its DB connection on import; the CLI export must use the
// same private database for every evaluation in this test process.
process.env.PROMPTFOO_CONFIG_DIR = fs.mkdtempSync(path.join(os.tmpdir(),'opencode/live-promptfoo-state-'))
process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'

test('each persisted native grade is published before the next investigation, including an ungraded result', async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'opencode/live-grades-'))
  const calls = [], rows = []
  const server = http.createServer(async (req, res) => {
    let text = ''; for await (const data of req) text += data
    const row = JSON.parse(text)
    assert.equal(req.method, 'PUT')
    assert.equal(req.url, `/api/evals/runs/${encodeURIComponent(row.evaluationId)}/results/${row.id}`)
    calls.push('publish ' + row.vars.n); rows.push(row)
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify({ complete: true, resultID: row.id, datasetID: '1'.repeat(64), ordinal: rows.length - 1, retainedSHA256:'0'.repeat(64) }))
  })
  server.listen(0, '127.0.0.1'); await once(server, 'listening')
  t.after(() => { server.closeAllConnections(); server.close() })
  const output = path.join(root, 'promptfoo.json')
  const config = {
    providers: [{ id: () => 'offline-test', callApi: async n => {
      calls.push('investigate ' + n); return { output: n }
    } }], prompts: ['{{n}}'], tracing: { enabled: true }, writeLatestResults: true,
    defaultTest: { assert: [{ type: 'llm-rubric', value: 'check' }], options: { provider: {
      id: () => 'offline-grader', callApi: async () => {
        calls.push('grade'); return rows.length === 1 ? { error: 'synthetic grader error' } : { output: '{"pass":true,"score":1,"reason":"ok"}' }
      },
    } } }, tests: ['one', 'two', 'three'].map(n => ({ vars: { n } })), extensions: [], outputPath: output,
    evaluateOptions: { repeat: 1, cache: false },
  }
  liveResultHooks(config)
  beginLiveResults(root, `http://127.0.0.1:${server.address().port}`)
  const { evaluate } = await import('promptfoo')
  await evaluate(config, {...config.evaluateOptions, share:false})
  await finishLiveResults()
  assert.deepEqual(calls,['investigate one','grade','publish one','investigate two','grade','publish two','investigate three','grade','publish three'])
  assert.equal(rows[0].success,true)
  assert.equal(rows[1].gradingResult.componentResults[0].metadata.graderError,true)
})

test('a publication failure stops the next investigation and leaves the saved result', async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'opencode/live-publish-error-'))
  const attempts = []
  const controller = new AbortController()
  onFailure(error => controller.abort(error))
  t.after(() => onFailure(undefined))
  const server = http.createServer((req,res) => { req.resume(); res.writeHead(502).end() })
  server.listen(0,'127.0.0.1'); await once(server,'listening')
  t.after(() => { server.closeAllConnections(); server.close() })
  const config = { providers: [{ id: () => 'offline', callApi: async n => { attempts.push(n); return {output:n} } }],
    prompts:['{{n}}'], tests:['one','two'].map(n => ({vars:{n},assert:[{type:'equals',value:n}]})),
    tracing:{enabled:true},writeLatestResults:true,evaluateOptions:{cache:false} }
  liveResultHooks(config)
  beginLiveResults(root,`http://127.0.0.1:${server.address().port}`,controller.signal)
  const {evaluate} = await import('promptfoo')
  try { await evaluate(config,{...config.evaluateOptions,share:false,abortSignal:controller.signal}) }
  catch (error) { assert.ok(controller.signal.aborted, String(error)) }
  assert.match(controller.signal.reason.message,/HTTP 502/)
  await assert.rejects(finishLiveResults(), error => error === controller.signal.reason)
  assert.deepEqual(attempts,['one'])
  const dir = path.join(root,'published-results'), files = fs.readdirSync(dir)
  const saved = files.filter(f=>/^[a-f0-9-]{36}\.json$/.test(f))
  assert.equal(saved.length,1)
  assert.equal(JSON.parse(fs.readFileSync(path.join(dir,saved[0]))).success,true)
  assert.ok(!files.some(f=>f.endsWith('.receipt.json')))
})
