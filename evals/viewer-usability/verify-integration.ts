import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import assert from 'node:assert/strict'
import { prepare, query, post } from './prepare.ts'
import { stageDataset, datasetManifest, readManifest } from './stage-dataset.ts'
import { readJson, record, saveJson, TEMPORARY } from './runtime.ts'
import { verify } from './verify.ts'

const binary = process.env.OTEL_EVAL_VIEWER_BINARY
if (!binary || !path.isAbsolute(binary))
  throw new Error(
    'Set absolute OTEL_EVAL_VIEWER_BINARY for owned integration verification'
  )
const source = process.env.OTLP_DATASET
if (!source || !path.isAbsolute(source))
  throw new Error(
    'Set absolute OTLP_DATASET for owned integration verification'
  )
const root = fs.mkdtempSync(
  path.join(TEMPORARY, 'eval-typescript-owned-integration-')
)
fs.chmodSync(root, 0o700)
console.log(JSON.stringify({ evidence: root, dataset: source, binary }))
fs.copyFileSync(
  binary,
  path.join(root, 'otel-desktop-viewer'),
  fs.constants.COPYFILE_EXCL
)
stageDataset(source, path.join(root, 'fixture'))
const manifest = datasetManifest(
  readManifest(path.join(source, 'manifest.json'))
)
const hashes: Record<string, string> = {}
for (const file of [
  'manifest.json',
  ...manifest.requests.map(request => request.file),
]) {
  const received: Buffer = fs.readFileSync(path.join(source, file))
  const staged: Buffer = fs.readFileSync(path.join(root, 'fixture', file))
  assert.deepEqual(staged, received)
  hashes[file] = crypto.createHash('sha256').update(staged).digest('hex')
}
saveJson(path.join(root, 'staged-byte-proof.json'), { source, hashes })
const ready = await prepare(root)
try {
  const counts = await query(
    ready.endpoint,
    'SELECT (SELECT count(*) FROM spans), (SELECT count(*) FROM logs), (SELECT count(*) FROM metric_datapoints)'
  )
  assert.deepEqual(counts.rows, [manifest.counts])
  const stats = record(
    await post(ready.endpoint + '/rpc', {
      jsonrpc: '2.0',
      id: 1,
      method: 'getStats',
    })
  )
  assert.deepEqual(record(stats.result).rejections, [])
  if (process.argv.includes('--verify-pilot'))
    await verify('historical-pilot-verification', root)
  saveJson(path.join(root, 'integration-proof.json'), {
    node: process.version,
    counts,
    start: ready.start,
    end: ready.end,
    ownedPID: ready.pid,
    binary,
    source,
    externalInferenceAttempted: false,
  })
} finally {
  ready.process.kill('SIGTERM')
  const timeout = setTimeout(() => ready.process.kill('SIGKILL'), 30000)
  let exit
  try {
    exit = await ready.closed
  } finally {
    clearTimeout(timeout)
  }
  saveJson(path.join(root, 'owned-shutdown.json'), {
    pid: ready.pid,
    command: record(readJson(path.join(root, 'runtime/process.json'))).command,
    signalSent: 'SIGTERM',
    waited: true,
    exit,
  })
}
