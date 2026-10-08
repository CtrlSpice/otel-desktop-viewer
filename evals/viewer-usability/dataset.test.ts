import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { spawnSync } from 'node:child_process'
import {
  stageDataset,
  datasetSource,
  readManifest,
  datasetManifest,
} from './stage-dataset.ts'
import { checkReceipt, postBytes, rfc3339Nano } from './prepare.ts'
import { TEMPORARY, SUITE } from './runtime.ts'

const scratch = () =>
  fs.mkdtempSync(path.join(TEMPORARY, 'eval-typescript-dataset-test-'))

test('manifest staging preserves exact bytes, multiple requests and existing evidence', () => {
  const root = scratch(),
    source = path.join(root, 'source'),
    target = path.join(root, 'fixture')
  fs.mkdirSync(source)
  fs.writeFileSync(
    path.join(source, 'manifest.json'),
    JSON.stringify({
      requests: [
        { signal: 'logs', file: 'logs-extra.json' },
        { signal: 'logs', file: 'logs-other.json' },
      ],
    })
  )
  fs.writeFileSync(
    path.join(source, 'logs-extra.json'),
    '{ "doubleValue": -0, "intValue": "-9223372036854775808", "timeUnixNano":"18446744073709551615" }\n'
  )
  fs.writeFileSync(
    path.join(source, 'logs-other.json'),
    '{"resourceLogs":[]}\n'
  )
  fs.writeFileSync(path.join(source, 'not-listed.json'), '{}')
  stageDataset(source, target)
  assert.deepEqual(fs.readdirSync(target).sort(), [
    'logs-extra.json',
    'logs-other.json',
    'manifest.json',
  ])
  for (const name of fs.readdirSync(target))
    assert.deepEqual(
      fs.readFileSync(path.join(target, name)),
      fs.readFileSync(path.join(source, name))
    )
  assert.throws(() => stageDataset(source, target), /EEXIST/)
  assert.deepEqual(
    fs.readFileSync(path.join(target, 'logs-extra.json')),
    fs.readFileSync(path.join(source, 'logs-extra.json'))
  )
})

test('default selection and explicit selection use the same staging CLI', () => {
  assert.equal(
    datasetSource({}, '/repo/evals/viewer-usability'),
    '/repo/testdata/otlp/small'
  )
  const root = scratch(),
    selected = path.join(root, 'selected'),
    run = path.join(root, 'run')
  fs.mkdirSync(selected)
  fs.mkdirSync(run)
  fs.writeFileSync(
    path.join(selected, 'manifest.json'),
    '{"requests":[{"signal":"logs","file":"logs.json"}]}'
  )
  fs.writeFileSync(path.join(selected, 'logs.json'), 'selected bytes')
  const result = spawnSync(
    process.execPath,
    [path.join(SUITE, 'stage-dataset.ts')],
    {
      env: { ...process.env, OTLP_DATASET: selected, OTEL_EVAL_RUN_DIR: run },
      encoding: 'utf8',
    }
  )
  assert.equal(result.status, 0, result.stderr)
  assert.equal(
    fs.readFileSync(path.join(run, 'fixture/logs.json'), 'utf8'),
    'selected bytes'
  )
})

test('HTTP ingestion sends negative zero, signed/unsigned 64-bit tokens and whitespace unchanged', async () => {
  const payload = Buffer.from(
    '{ "resourceLogs": [], "doubleValue": -0, "intValue":"-9223372036854775808", "unsigned":"18446744073709551615", "exact":"9007199254740993" }\n'
  )
  let received: Buffer | undefined
  const server = http.createServer(async (req, res) => {
    const chunks: Buffer[] = []
    for await (const chunk of req) chunks.push(Buffer.from(chunk))
    received = Buffer.concat(chunks)
    assert.equal(req.url, '/v1/logs')
    assert.equal(req.headers['content-type'], 'application/json')
    res.end('{"partialSuccess":{}}')
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  try {
    const address = server.address()
    assert.ok(address && typeof address !== 'string')
    assert.deepEqual(
      await postBytes(`http://127.0.0.1:${address.port}/v1/logs`, payload),
      { partialSuccess: {} }
    )
    assert.deepEqual(received, payload)
  } finally {
    await new Promise<void>(resolve => server.close(() => resolve()))
  }
})

test('nanosecond windows use exact integer civil-date arithmetic', () => {
  for (const [value, expected] of [
    ['0', '1970-01-01T00:00:00.000000000Z'],
    ['1791360000123456789', '2026-10-07T08:00:00.123456789Z'],
    ['18446744073709551615', '2554-07-21T23:34:33.709551615Z'],
    ['-1', '1969-12-31T23:59:59.999999999Z'],
    ['951782400000000001', '2000-02-29T00:00:00.000000001Z'],
  ])
    assert.equal(rfc3339Nano(value), expected)
  assert.throws(() => rfc3339Nano(1791360000123456789), /exact integer/)
})

test('numeric JSON manifest bounds retain signed/unsigned 64-bit source tokens', () => {
  const root = scratch(),
    file = path.join(root, 'manifest.json')
  const bytes =
    '{"startTimeUnixNano":-9223372036854775808,"endTimeUnixNano":18446744073709551615,"requests":[],"storedCounts":{"spans":0,"logs":0,"datapoints":0}}'
  fs.writeFileSync(file, bytes)
  const manifest = datasetManifest(readManifest(file))
  assert.equal(manifest.start, -9223372036854775808n)
  assert.equal(manifest.end, 18446744073709551615n)
  assert.equal(
    rfc3339Nano(manifest.start.toString()),
    '1677-09-21T00:12:43.145224192Z'
  )
  assert.equal(fs.readFileSync(file, 'utf8'), bytes)
})

test('partial success cannot pass as full ingestion', () => {
  checkReceipt({})
  checkReceipt({ partialSuccess: {} })
  for (const key of [
    'rejectedSpans',
    'rejectedLogRecords',
    'rejectedDataPoints',
  ]) {
    checkReceipt({ partialSuccess: { [key]: '0' } })
    for (const value of [1, '1', '18446744073709551615']) {
      assert.throws(
        () => checkReceipt({ partialSuccess: { [key]: value } }),
        /not fully accepted/
      )
    }
  }
  assert.throws(
    () =>
      checkReceipt({
        partialSuccess: { errorMessage: 'Rejected fixture data' },
      }),
    /not fully accepted/
  )
})
