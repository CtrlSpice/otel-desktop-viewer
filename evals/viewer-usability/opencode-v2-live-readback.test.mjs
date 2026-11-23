import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import os from 'node:os'
import test from 'node:test'
import { prepare } from './prepare.ts'
import { withBinaryTeardown } from './attempt-teardown.ts'
import { datasetSource, stageDataset } from './stage-dataset.ts'
import { beginLiveResults, checkLiveResultSupport, afterEach, finishLiveResults, verifyLiveReadback } from './opencode-v2-live-results.mjs'

test('real observer verifies the redacted grade, including large integers, without revealing the source on mismatch',
  { skip: !process.env.OTEL_EVAL_VIEWER_BINARY }, async t => {
    const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(),'opencode/live-redaction-')))
    const canonicalBinary = process.env.OTEL_EVAL_VIEWER_BINARY
    fs.copyFileSync(canonicalBinary,path.join(root,'otel-desktop-viewer'),fs.constants.COPYFILE_EXCL)
    stageDataset(datasetSource(),path.join(root,'fixture'))
    await withBinaryTeardown({directory:root,canonicalBinary,kind:'export-replay'},async register => {
      const {endpoint} = await prepare(root,undefined,undefined,register)
      await checkLiveResultSupport(endpoint)
      const run = 'offline-readback', id = '11111111-1111-4111-8111-111111111111', trace = '1'.repeat(32)
      const row = { id,evaluationId:run,traceId:trace,provider:{id:'offline',label:'Offline'},
        response:{output:'synthetic answer',metadata:{password:'synthetic-readback-secret',sequence:JSON.rawJSON('9007199254740993')}},
        gradingResult:{pass:true,score:1,reason:'fixture'},success:true,score:1 }
      fs.writeFileSync(path.join(root,'promptfoo.json'),JSON.stringify({evalId:run,config:{},results:{version:3,results:[row]}}))
      beginLiveResults(root,endpoint)
      afterEach({result:row})
      await finishLiveResults()
      assert.deepEqual((await verifyLiveReadback(root,endpoint)).resultIDs,[id])
      const fetchReal = globalThis.fetch
      t.mock.method(globalThis,'fetch',async (url,options) => {
        const r = await fetchReal(url,options)
        if (!String(url).includes('/rows?')) return r
        const page = await r.json(), text = page.rows[0].json
        assert.ok(!text.includes('synthetic-readback-secret') && text.includes('"password":"****"'))
        assert.ok(text.includes('9007199254740993'))
        page.rows[0].json = text.replace('9007199254740993','9007199254740992')
        return Response.json(page)
      })
      await assert.rejects(verifyLiveReadback(root,endpoint),error =>
        error.message === 'Retained result digest mismatch' && !JSON.stringify(error).includes('synthetic-readback-secret'))
      t.diagnostic(`Real-observer readback evidence: ${root}`)
    })
  })
