import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import assert from 'node:assert/strict'
import { prepare } from './prepare.ts'
import { TEMPORARY, readJson, record } from './runtime.ts'

const stub = `#!${process.execPath}
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const args = process.argv.slice(2);
const settings = JSON.parse(fs.readFileSync(path.join(__dirname,'settings.json')));
const port = flag => Number(args[args.indexOf(flag)+1]);
const rpc = http.createServer(async (req,res) => {
  let body = ''; for await (const chunk of req) body += chunk;
  const request = JSON.parse(body);
  const result = request.method === 'getStats' ? {rejections:settings.rejections || []} :
    {columns:[{name:'count'}],rows:request.params.sql === 'SELECT 1' ? [[1]] : [settings.counts || [0,0,0]],truncated:false};
  res.end(JSON.stringify({jsonrpc:'2.0',id:1,result}));
});
const receiver = http.createServer(async (req,res) => {
  const chunks = []; for await (const chunk of req) chunks.push(chunk);
  fs.writeFileSync(path.join(process.cwd(),'received.bin'),Buffer.concat(chunks));
  res.end(JSON.stringify(settings.receipt || {}));
});
rpc.listen(port('--browser-port'),'127.0.0.1');
receiver.listen(port('--http'),'127.0.0.1');
process.on('SIGTERM',()=>{rpc.close();receiver.close();rpc.closeAllConnections();receiver.closeAllConnections();});
`

function setup(settings: unknown) {
  const root = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-typescript-prepare-test-')
  )
  fs.mkdirSync(path.join(root, 'fixture'))
  fs.writeFileSync(path.join(root, 'otel-desktop-viewer'), stub, {
    mode: 0o700,
  })
  fs.writeFileSync(path.join(root, 'settings.json'), JSON.stringify(settings))
  fs.writeFileSync(
    path.join(root, 'fixture/manifest.json'),
    JSON.stringify({
      startTimeUnixNano: '1791360000123456789',
      endTimeUnixNano: '1791360000123456790',
      storedCounts: { spans: 0, logs: 0, datapoints: 0 },
      requests: [{ signal: 'logs', file: 'logs.json' }],
    })
  )
  fs.writeFileSync(
    path.join(root, 'fixture/logs.json'),
    '{ "doubleValue": -0 }\n'
  )
  return root
}

test('preparation leaves only its successful owned viewer running and refuses previous runtime', async () => {
  const root = setup({})
  const result = await prepare(root)
  try {
    assert.equal(result.start, '2026-10-07T08:00:00.123456789Z')
    assert.equal(result.end, '2026-10-07T08:00:00.123456790Z')
    assert.deepEqual(
      fs.readFileSync(path.join(root, 'runtime/received.bin')),
      fs.readFileSync(path.join(root, 'fixture/logs.json'))
    )
    const command = fs.readFileSync(path.join(root, 'runtime/process.json'))
    await assert.rejects(prepare(root), /EEXIST/)
    assert.deepEqual(
      fs.readFileSync(path.join(root, 'runtime/process.json')),
      command
    )
    assert.equal(result.process.exitCode, null)
  } finally {
    const closed = new Promise<void>(resolve =>
      result.process.once('close', () => resolve())
    )
    result.process.kill('SIGTERM')
    await closed
  }
})

test('partial rejection and store rejection retain evidence and stop/wait only owned processes', async () => {
  for (const settings of [
    { receipt: { partialSuccess: { rejectedLogRecords: '1' } } },
    { rejections: [{ signal: 'logs' }] },
  ]) {
    const root = setup(settings)
    await assert.rejects(prepare(root), /not fully accepted|rejected records/)
    assert.ok(fs.existsSync(path.join(root, 'runtime/ingestion.json')))
    assert.ok(fs.existsSync(path.join(root, 'runtime/failure.json')))
    assert.ok(!fs.existsSync(path.join(root, 'connection.json')))
    const pid = record(readJson(path.join(root, 'runtime/process.json'))).pid
    assert.equal(typeof pid, 'number')
    if (typeof pid !== 'number') throw new Error('Missing owned PID')
    assert.throws(() => process.kill(pid, 0), /ESRCH/)
  }
})

test('owned close result remains available when the viewer exits before cleanup starts', async () => {
  const result = await prepare(setup({}))
  result.process.ref()
  result.process.kill('SIGTERM')
  const exit = await result.closed
  assert.deepEqual(exit, { code: 0, signal: null })
  assert.equal(result.process.kill('SIGTERM'), false)
  assert.deepEqual(await result.closed, exit)
})

test('missing viewer spawn retains runtime failure evidence', async () => {
  const root = setup({})
  fs.renameSync(
    path.join(root, 'otel-desktop-viewer'),
    path.join(root, 'not-the-viewer')
  )
  await assert.rejects(prepare(root), /ENOENT/)
  assert.ok(fs.existsSync(path.join(root, 'runtime/failure.json')))
  assert.ok(!fs.existsSync(path.join(root, 'connection.json')))
})

test('count mismatch and existing connection both retain evidence and clean up owned viewers', async () => {
  for (const scenario of ['counts', 'connection']) {
    const root = setup(scenario === 'counts' ? { counts: [1, 0, 0] } : {})
    if (scenario === 'connection')
      fs.writeFileSync(
        path.join(root, 'connection.json'),
        'previous connection'
      )
    await assert.rejects(prepare(root), /counts differ|EEXIST/)
    const pid = record(readJson(path.join(root, 'runtime/process.json'))).pid
    assert.ok(typeof pid === 'number')
    assert.throws(() => process.kill(pid, 0), /ESRCH/)
    assert.ok(fs.existsSync(path.join(root, 'runtime/failure.json')))
    if (scenario === 'connection')
      assert.equal(
        fs.readFileSync(path.join(root, 'connection.json'), 'utf8'),
        'previous connection'
      )
  }
})
