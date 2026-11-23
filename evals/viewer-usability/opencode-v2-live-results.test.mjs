import assert from 'node:assert/strict'
import { once } from 'node:events'
import fs from 'node:fs'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { beginLiveResults, finishLiveResults, liveResultHooks } from './opencode-v2-live-results.mjs'

test('each persisted native grade is published before the next investigation, including an ungraded result', async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'opencode/live-grades-'))
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'state')
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  const calls = [], rows = []
  const server = http.createServer(async (req, res) => {
    let text = ''; for await (const data of req) text += data
    const row = JSON.parse(text)
    assert.equal(req.method, 'PUT')
    assert.equal(req.url, `/api/evals/runs/${encodeURIComponent(row.evaluationId)}/results/${row.id}`)
    calls.push('publish ' + row.vars.n); rows.push(row)
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify({ complete: true, resultID: row.id, datasetID: 'fixture', ordinal: rows.length - 1 }))
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
  const exported = JSON.parse(fs.readFileSync(output))
  await finishLiveResults(exported)
  assert.deepEqual(calls,['investigate one','grade','publish one','investigate two','grade','publish two','investigate three','grade','publish three'])
  assert.equal(rows[0].success,true)
  assert.equal(rows[1].gradingResult.componentResults[0].metadata.graderError,true)
})
