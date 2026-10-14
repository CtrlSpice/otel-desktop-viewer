import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import assert from 'node:assert/strict'
import { prepare, expectedStoredCounts, IngestionRejectedError } from './prepare.ts'
import { TEMPORARY, readJson, record } from './runtime.ts'
import { stageDataset, datasetSource, datasetManifest, readManifest } from './stage-dataset.ts'

const stub = `#!${process.execPath}
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const args = process.argv.slice(2);
const settings = JSON.parse(fs.readFileSync(path.join(__dirname,'settings.json')));
const port = flag => Number(args[args.indexOf(flag)+1]);
let polls = 0, sends = 0;
const rpc = http.createServer(async (req,res) => {
  let body = ''; for await (const chunk of req) body += chunk;
  const request = JSON.parse(body);
  const result = request.method === 'getStats' ? {rejections:settings.rejections || []} :
    {columns:[{name:'count'}],rows:request.params.sql === 'SELECT 1' ? [[1]] : [settings.delayed && ++polls < 4 ? [0,0,0] : settings.counts || [0,0,0]],truncated:false};
  fs.writeFileSync(path.join(process.cwd(),'polls.json'),JSON.stringify({polls,sends}));
  res.end(JSON.stringify({jsonrpc:'2.0',id:1,result}));
});
const receiver = http.createServer(async (req,res) => {
  const chunks = []; for await (const chunk of req) chunks.push(chunk);
  fs.writeFileSync(path.join(process.cwd(),'received.bin'),Buffer.concat(chunks));
  sends++;
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
    '{ "resourceLogs": [], "doubleValue": -0 }\n'
  )
  return root
}

test('invalid-only span input waits for explicit store rejection rather than passing on initial zero counts',
  { skip: !process.env.OTEL_EVAL_VIEWER_BINARY }, async () => {
    const root = setup({})
    fs.copyFileSync(process.env.OTEL_EVAL_VIEWER_BINARY!, path.join(root, 'otel-desktop-viewer'))
    const manifestFile = path.join(root, 'fixture/manifest.json')
    const manifest = record(readJson(manifestFile))
    manifest.requests = [{ signal: 'traces', file: 'trace.json' }]
    fs.writeFileSync(manifestFile, JSON.stringify(manifest))
    fs.writeFileSync(path.join(root, 'fixture/trace.json'), JSON.stringify({
      resourceSpans: [{ scopeSpans: [{ spans: [{ name: 'no span ID' }] }] }],
    }))
    await assert.rejects(prepare(root), IngestionRejectedError)
    const stats = record(readJson(path.join(root, 'runtime/stats.json')))
    assert.ok((record(stats.result).rejections as unknown[]).length)
    assert.ok(!fs.existsSync(path.join(root, 'connection.json')))
    const processInfo = record(readJson(path.join(root, 'runtime/process.json')))
    assert.throws(() => process.kill(Number(processInfo.pid), 0), /ESRCH/)
  })

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
    { counts: [1, 0, 0], rejections: [{ signal: 'logs' }] },
    { rejections: [{ signal: 'traces', kind: 'span_refused', message: 'span_already_stored' }] },
    { rejections: [{ signal: 'logs', kind: 'span_already_stored' }] },
    { rejections: [
      { signal: 'traces', kind: 'span_already_stored' },
      { signal: 'traces', kind: 'span_refused' },
    ] },
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
    const root = setup(scenario === 'counts' ? {
      counts: [1, 0, 0],
      rejections: [{ signal: 'traces', kind: 'span_already_stored' }],
    } : {})
    if (scenario === 'connection')
      fs.writeFileSync(
        path.join(root, 'connection.json'),
        'previous connection'
      )
    await assert.rejects(prepare(root), error => {
      assert.ok(error instanceof Error)
      assert.ok(!(error instanceof IngestionRejectedError))
      assert.match(error.message, /visibility not confirmed before deadline|EEXIST/)
      return true
    })
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

test('input counts deduplicate span pairs across requests, not owners, logs or datapoints', () => {
  const trace = (traceId: string, spanId: string) => ({ resourceSpans: [{
    scopeSpans: [{ spans: [{ traceId, spanId }] }],
  }] })
  assert.deepEqual(expectedStoredCounts([
    { signal: 'traces', document: trace('a'.repeat(32), 'f'.repeat(16)) },
    { signal: 'traces', document: trace('A'.repeat(32), 'F'.repeat(16)) },
    { signal: 'traces', document: trace('b'.repeat(32), 'f'.repeat(16)) },
    { signal: 'logs', document: { resourceLogs: [{ scopeLogs: [{ logRecords: [{}, {}] }] }] } },
    { signal: 'metrics', document: { resourceMetrics: [{ scopeMetrics: [{ metrics:
      ['gauge', 'sum', 'histogram', 'exponentialHistogram'].map(kind => ({ [kind]: { dataPoints: [{}, {}] } })),
    }] }] } },
  ]), [2, 2, 8])
})

test('accepted receipt waits for input counts without resending or using reference counts', async () => {
  const root = setup({ delayed: true, counts: [0, 1, 0] })
  fs.writeFileSync(path.join(root, 'fixture/logs.json'),
    '{ "resourceLogs": [{"scopeLogs":[{"logRecords":[{"body":{"doubleValue":-0}}]}]}] }\n')
  const result = await prepare(root)
  try {
    assert.deepEqual(readJson(path.join(root, 'runtime/expected-counts.json')), [0, 1, 0])
    assert.deepEqual(readJson(path.join(root, 'runtime/polls.json')), { polls: 4, sends: 1 })
    assert.deepEqual(fs.readFileSync(path.join(root, 'runtime/received.bin')),
      fs.readFileSync(path.join(root, 'fixture/logs.json')))
  } finally {
    result.process.kill('SIGTERM')
    await result.closed
  }
})

test('fresh real store matches derived shared-dataset counts and repeated spans retain rejection evidence',
  { skip: !process.env.OTEL_EVAL_VIEWER_BINARY }, async t => {
    for (const repeated of [false, true]) {
      const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-readiness-real-'))
      t.diagnostic(`${repeated ? 'Repeated spans' : 'Shared dataset'} evidence: ${root}`)
      stageDataset(datasetSource(), path.join(root, 'fixture'))
      fs.copyFileSync(process.env.OTEL_EVAL_VIEWER_BINARY!, path.join(root, 'otel-desktop-viewer'))
      const manifestFile = path.join(root, 'fixture/manifest.json')
      const raw = record(readManifest(manifestFile))
      const manifest = datasetManifest(raw)
      const inputs = manifest.requests.map(request => ({
        signal: request.signal,
        document: readJson(path.join(root, 'fixture', request.file)),
      }))
      const expected = expectedStoredCounts(inputs)
      assert.deepEqual(expected, manifest.counts)
      if (repeated) {
        const traces = manifest.requests.filter(request => request.signal === 'traces')
        assert.ok(traces.length)
        raw.requests = [...manifest.requests, ...traces]
        fs.writeFileSync(manifestFile, JSON.stringify(raw))
        assert.deepEqual(expectedStoredCounts([...inputs,
          ...inputs.filter(input => input.signal === 'traces')]), expected)
      }
      const ready = await prepare(root)
      try {
        assert.deepEqual(readJson(path.join(root, 'runtime/expected-counts.json')), expected)
        assert.deepEqual(record(readJson(path.join(root, 'runtime/counts.json'))).rows, [expected])
        if (repeated) {
          const stats = record(readJson(path.join(root, 'runtime/stats.json')))
          const rejections = record(stats.result).rejections as Record<string, unknown>[]
          assert.ok(rejections.length)
          assert.ok(rejections.every(rejection =>
            rejection.signal === 'traces' && rejection.kind === 'span_already_stored'))
        }
      } finally {
        ready.process.kill('SIGTERM')
        assert.equal((await ready.closed).code, 0)
        assert.throws(() => process.kill(Number(ready.pid), 0), /ESRCH/)
      }
    }
  })
