import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import {
  traceRows,
  verifyExport,
  scratchFiles,
  CHECKOUT_TRACE,
} from './verify-export.ts'
import { stageDataset, datasetSource } from './stage-dataset.ts'
import { readJson, record, saveJson, TEMPORARY } from './runtime.ts'

function fixture() {
  const root = fs.realpathSync(
    fs.mkdtempSync(path.join(TEMPORARY, 'investigation-export-test-'))
  )
  stageDataset(datasetSource(), path.join(root, 'fixture'))
  const isolationRoot = path.join(root, 'workspaces/session')
  const workspace = path.join(isolationRoot, 'scratch')
  fs.mkdirSync(workspace, { recursive: true })
  const document = record(readJson(path.join(root, 'fixture/traces.json')))
  const resources = (document.resourceSpans as unknown[])
    .map(record)
    .map(resource => ({
      ...resource,
      scopeSpans: (resource.scopeSpans as unknown[])
        .map(record)
        .map(scope => ({
          ...scope,
          spans: (scope.spans as unknown[])
            .map(record)
            .filter(span => span.traceId === CHECKOUT_TRACE),
        }))
        .filter(scope => scope.spans.length),
    }))
    .filter(resource => resource.scopeSpans.length)
  return {
    root,
    workspace,
    metadata: { workspace, isolationRoot },
    document: { resourceSpans: resources },
  }
}

test('export comparison keeps exact values, parent IDs, context and value-array order', () => {
  const { document } = fixture()
  const rows = traceRows(document)
  assert.equal(rows.length, 6)
  assert.equal(rows.filter(row => record(row.span).parentSpanId).length, 5)
  const changed = structuredClone(document)
  changed.resourceSpans[0].scopeSpans[0].spans[0].endTimeUnixNano =
    '1791360000000000001'
  assert.notDeepEqual(traceRows(changed), rows)
  const context = structuredClone(document)
  Object.assign(context.resourceSpans[0], { resource: { attributes: [] } })
  assert.notDeepEqual(traceRows(context), rows)
  const values = [{ intValue: '9007199254740993' }, { stringValue: '2' }]
  Object.assign(document.resourceSpans[0].scopeSpans[0].spans[0], {
    attributes: [
      { key: 'array-order-proof', value: { arrayValue: { values } } },
    ],
  })
  const ordered = structuredClone(traceRows(document))
  values.reverse()
  assert.notDeepEqual(traceRows(document), ordered)
})

test('missing, malformed, wrong, tampered and out-of-scratch exports fail without replay', async () => {
  const { root, workspace, metadata, document } = fixture()
  assert.equal((await verifyExport(metadata, root)).pass, false)
  const file = path.join(workspace, 'arbitrary-name')
  for (const bytes of [
    'not JSON',
    '{}',
    JSON.stringify({ resourceSpans: [] }),
    JSON.stringify(document).replace(
      CHECKOUT_TRACE,
      '00000000000000000000000000000012'
    ),
    JSON.stringify(document).replace('9007199254741009', '9007199254741008'),
  ]) {
    fs.writeFileSync(file, bytes)
    assert.equal((await verifyExport(metadata, root)).pass, false)
  }
  const outside = path.join(root, 'outside.json')
  fs.writeFileSync(outside, JSON.stringify(document))
  fs.symlinkSync(outside, path.join(workspace, 'outside-link'))
  fs.symlinkSync(root, path.join(workspace, 'outside-directory'))
  assert.deepEqual(scratchFiles(workspace), [file])
  assert.equal((await verifyExport(metadata, root)).pass, false)
  await assert.rejects(
    verifyExport({ workspace: root, isolationRoot: root }, root),
    /owned investigator/
  )
})

test(
  'valid file with unconstrained name replays actual bytes and shuts down owned receiver',
  { skip: !process.env.OTEL_EVAL_VIEWER_BINARY },
  async () => {
    const { root, workspace, metadata, document } = fixture()
    fs.mkdirSync(path.join(workspace, 'handoff'))
    const file = path.join(workspace, 'handoff/for-another-engineer')
    const bytes = Buffer.from(JSON.stringify(document, null, 2) + '\n')
    fs.writeFileSync(file, bytes)
    saveJson(path.join(root, 'connection.json'), {
      binary: process.env.OTEL_EVAL_VIEWER_BINARY,
    })
    const result = await verifyExport(metadata, root)
    assert.equal(result.pass, true)
    assert.deepEqual(result.files, [file])
    const replay = fs
      .readdirSync(root)
      .find(name => name.startsWith('export-replay-'))!
    assert.deepEqual(
      fs.readFileSync(path.join(root, replay, 'fixture/trace.json')),
      bytes
    )
    const shutdown = record(
      readJson(path.join(root, replay, 'owned-shutdown.json'))
    )
    assert.equal(shutdown.waited, true)
    assert.equal(record(shutdown.exit).code, 0)
    assert.throws(() => process.kill(Number(shutdown.pid), 0), /ESRCH/)
  }
)

test(
  'malformed extra records fail only their candidate and preserve valid exports',
  { skip: !process.env.OTEL_EVAL_VIEWER_BINARY },
  async () => {
    const { root, workspace, metadata, document } = fixture()
    saveJson(path.join(root, 'connection.json'), {
      binary: process.env.OTEL_EVAL_VIEWER_BINARY,
    })
    const invalid = structuredClone(document)
    invalid.resourceSpans[0].scopeSpans[0].spans.push({
      name: 'malformed extra span',
    })
    const bad = path.join(workspace, 'a-invalid.json')
    fs.writeFileSync(bad, JSON.stringify(invalid))
    assert.equal((await verifyExport(metadata, root)).pass, false)
    const good = path.join(workspace, 'b-valid.json')
    fs.writeFileSync(good, JSON.stringify(document))
    const rejected = structuredClone(document)
    rejected.resourceSpans[0].scopeSpans[0].spans.push({
      traceId: 'not-a-trace-id',
      spanId: 'not-a-span-id',
      name: 'invalid extra span',
      startTimeUnixNano: '1',
      endTimeUnixNano: '2',
    })
    fs.writeFileSync(
      path.join(workspace, 'c-rejected.json'),
      JSON.stringify(rejected)
    )
    const result = await verifyExport(metadata, root)
    assert.equal(result.pass, true)
    assert.deepEqual(result.files, [good])
    for (const directory of fs
      .readdirSync(root)
      .filter(name => name.startsWith('export-replay-'))) {
      const proof = record(
        readJson(path.join(root, directory, 'export-proof.json'))
      )
      assert.equal(proof.pass, proof.file === good)
      const processInfo = record(
        readJson(path.join(root, directory, 'runtime/process.json'))
      )
      assert.throws(() => process.kill(Number(processInfo.pid), 0), /ESRCH/)
    }
    fs.writeFileSync(
      path.join(root, 'connection.json'),
      JSON.stringify({ binary: path.join(root, 'missing-binary') })
    )
    await assert.rejects(verifyExport(metadata, root), /ENOENT/)
  }
)
