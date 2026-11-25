import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import os from 'node:os'
import test from 'node:test'
import { prepare } from './prepare.ts'
import { withBinaryTeardown } from './attempt-teardown.ts'
import { datasetSource, stageDataset } from './stage-dataset.ts'
import { beginLiveResults, checkLiveResultSupport, afterEach, finishLiveResults, verifyLiveReadback } from './opencode-v2-live-results.mjs'

test('malformed readback fields fail with fixed diagnostics before they are used', async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(),'opencode/readback-types-'))
  const dir = path.join(root,'published-results')
  fs.mkdirSync(dir)
  const run = 'fixture', id = '11111111-1111-4111-8111-111111111111'
  const row = {id,evaluationId:run,provider:{id:'offline'}}, json = JSON.stringify(row)
  fs.writeFileSync(path.join(root,'promptfoo.json'),JSON.stringify({evalId:run,results:{results:[row]}}))
  fs.writeFileSync(path.join(dir,id+'.json'),json)
  const receipt = {complete:true, resultID:id, datasetID:'a'.repeat(64), ordinal:0,
    retainedSHA256:createHash('sha256').update(json).digest('hex')}
  const receiptPath = path.join(dir,id+'.receipt.json')
  const item = {id:'b'.repeat(64),datasetID:receipt.datasetID,resultCount:1}
  const page = {experiments:[item],hasMore:false}
  const entry = {ordinal:0,json}
  const detail = {contextJSON:JSON.stringify({evalId:run}), rows:[entry],hasMore:false,nextAfter:0}
  const marker = 'private-fixture-value'
  let replies
  t.mock.method(globalThis,'fetch',async () => {
    assert.ok(replies.length > 0,'Unexpected pagination request')
    return Response.json(replies.shift())
  })
  const verify = () => verifyLiveReadback(root,'http://fixture.invalid')
  for (const [saved, responses, message] of [
    [{...receipt,complete:marker}, [], 'Invalid publication receipt'],
    [{...receipt,retainedSHA256:[receipt.retainedSHA256]}, [], 'Invalid publication receipt'],
    [{...receipt,ordinal:'0'}, [], 'Invalid publication receipt'],
    [{...receipt,datasetID:[receipt.datasetID]}, [], 'Invalid publication receipt'],
    [receipt, [{...page,experiments:marker}], 'Invalid experiment page'],
    [receipt, [{...page,hasMore:marker}], 'Invalid experiment page'],
    [receipt, [{...page,experiments:[{...item,resultCount:marker}]}], 'Invalid experiment summary'],
    [receipt, [{...page,experiments:[{...item,resultCount:2}]}], 'Unexpected retained result count'],
    [receipt, [{...page,experiments:[{...item,id:marker}]}], 'Invalid experiment summary'],
    [receipt, [page,{...detail,hasMore:marker}], 'Invalid retained result page'],
    [receipt, [page,{...detail,hasMore:true,nextAfter:marker}], 'Invalid retained result page'],
    [receipt, [page,{...detail,hasMore:true,nextAfter:1}], 'Invalid retained result cursor'],
    [receipt, [page,{...detail,contextJSON:[detail.contextJSON]}], 'Invalid retained result page'],
    [receipt, [page,{...detail,rows:[{...entry,json:{text:marker}}]}], 'Invalid retained result row'],
    [receipt, [page,{...detail,rows:[{...entry,ordinal:marker}]}], 'Invalid retained result row'],
  ]) {
    fs.writeFileSync(receiptPath,JSON.stringify(saved))
    replies = responses.slice()
    await assert.rejects(verify, error => error.message === message &&
      !error.stack.includes(marker) && !JSON.stringify(error).includes(marker))
    assert.equal(replies.length,0)
  }
})

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
      let corrupt = 'integer'
      t.mock.method(globalThis,'fetch',async (url,options) => {
        const r = await fetchReal(url,options)
        if (!String(url).includes('/rows?')) return r
        if (corrupt === 'response') return new Response('synthetic-readback-secret')
        const page = await r.json(), text = page.rows[0].json
        if (corrupt === 'context') return Response.json({...page,contextJSON:'synthetic-readback-secret'})
        assert.ok(!text.includes('synthetic-readback-secret') && text.includes('"password":"****"'))
        assert.ok(text.includes('9007199254740993'))
        page.rows[0].json = text.replace('9007199254740993','9007199254740992')
        return Response.json(page)
      })
      await assert.rejects(verifyLiveReadback(root,endpoint),error =>
        error.message === 'Retained result digest mismatch' && !JSON.stringify(error).includes('synthetic-readback-secret'))
      for (corrupt of ['response','context'])
        await assert.rejects(verifyLiveReadback(root,endpoint),error =>
          error.message === 'Invalid JSON in evaluation readback' && !error.stack.includes('synthetic-readback-secret'))
      t.diagnostic(`Real-observer readback evidence: ${root}`)
    })
  })
