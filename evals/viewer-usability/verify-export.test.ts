import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { gzipSync, gunzipSync } from 'node:zlib'
import { create, Header } from 'tar'
import {
  traceRows,
  verifyExport,
  scratchFiles,
  CHECKOUT_TRACE,
  exportPayloads,
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

async function archive(root: string, files: Record<string, Buffer | string>) {
  const source = fs.mkdtempSync(path.join(root, 'archive-source-'))
  for (const [name, bytes] of Object.entries(files)) {
    const file = path.join(source, name)
    fs.mkdirSync(path.dirname(file), { recursive: true })
    fs.writeFileSync(file, bytes)
  }
  return create({ cwd: source, gzip: true, portable: true }, ['.']).concat()
}

function repeatedArchive(first: string, second: string, bytes: Buffer) {
  const blocks: Buffer[] = []
  for (const [name, content] of [
    [first, bytes],
    [second, Buffer.from('not JSON')],
  ] as const) {
    const header = new Header({ path: name, type: 'File', size: content.length })
    header.encode()
    blocks.push(
      header.block!, content, Buffer.alloc((512 - content.length % 512) % 512)
    )
  }
  return gzipSync(Buffer.concat([...blocks, Buffer.alloc(1024)]))
}

test('duplicate archive file paths reject the whole handoff before replay', async () => {
  const { root, workspace, metadata, document } = fixture()
  const bytes = Buffer.from(JSON.stringify(document))
  const file = path.join(workspace, 'duplicate.tar.gz')
  for (const [first, second] of [
    ['trace.json', 'trace.json'],
    ['trace.json', './trace.json'],
    ['dir/../trace.json', 'trace.json'],
    ['handoff//trace.json', 'handoff/trace.json'],
    ['/trace.json', 'trace.json'],
  ]) {
    const packed = repeatedArchive(first, second, bytes)
    await assert.rejects(exportPayloads(packed), /Duplicate archive member path/)
    fs.writeFileSync(file, packed)
    assert.equal((await verifyExport(metadata, root)).pass, false)
  }
  assert.ok(!fs.readdirSync(root).some(name => name.startsWith('export-replay-')))
})

test('archive members preserve bytes and names without extracting paths or following links', async () => {
  const { root, document } = fixture()
  const bytes = Buffer.from(JSON.stringify(document, null, 2) + '\n')
  const name = 'handoff/' + 'long-directory/'.repeat(12) + 'trace.json'
  const packed = await archive(root, { [name]: bytes, 'README.txt': 'handoff' })
  const payloads = await exportPayloads(packed)
  assert.deepEqual(
    payloads.find(value => value.member === './' + name)?.bytes,
    bytes
  )
  assert.equal(payloads.length, 2)
  assert.deepEqual(await exportPayloads(bytes), [{ bytes }])

  const outside = path.join(root, 'outside.json')
  fs.writeFileSync(outside, bytes)
  const source = fs.mkdtempSync(path.join(root, 'archive-links-'))
  fs.symlinkSync(outside, path.join(source, 'linked.json'))
  const linked = await create({ cwd: source, gzip: true }, [
    'linked.json',
  ]).concat()
  assert.deepEqual(await exportPayloads(linked), [])

  // Construct hostile member names using tar's own header encoder. Reading must
  // never write either an absolute target or a parent-relative target.
  const target = path.join(root, 'must-not-be-written.json')
  const blocks: Buffer[] = []
  for (const member of [target, '../must-not-be-written.json']) {
    const header = new Header({
      path: member,
      type: 'File',
      size: bytes.length,
    })
    header.encode()
    blocks.push(
      header.block!,
      bytes,
      Buffer.alloc((512 - bytes.length % 512) % 512)
    )
  }
  blocks.push(Buffer.alloc(1024))
  const named = await exportPayloads(gzipSync(Buffer.concat(blocks)))
  assert.deepEqual(named.map(value => value.member), [
    target,
    '../must-not-be-written.json',
  ])
  assert.ok(named.every(value => value.bytes.equals(bytes)))
  assert.equal(fs.existsSync(target), false)
  assert.deepEqual(fs.readFileSync(outside), bytes)
})

test('malformed, truncated and checksum-invalid archives cannot supply partial candidates', async () => {
  const { root } = fixture()
  const packed = await archive(root, {
    'first.json': '{}',
    'second.json': '{}',
  })
  const damaged = gunzipSync(packed)
  damaged[0] ^= 1
  for (const bytes of [
    gzipSync(Buffer.from('not a tar archive')),
    packed.subarray(0, packed.length - 8),
    gzipSync(damaged),
    gzipSync(gunzipSync(packed).subarray(0, 1200)),
  ]) {
    await assert.rejects(exportPayloads(bytes))
  }
})

test('archives with wrong or changed trace contents fail without replay', async () => {
  const { root, workspace, metadata, document } = fixture()
  const file = path.join(workspace, 'handoff.tar.gz')
  for (const payload of [
    'not JSON',
    JSON.stringify({ resourceSpans: [] }),
    JSON.stringify(document).replace('9007199254741009', '9007199254741008'),
  ]) {
    fs.writeFileSync(file, await archive(root, { 'trace.json': payload }))
    assert.equal((await verifyExport(metadata, root)).pass, false)
  }
})

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

test('wrong one-span export is a content failure, not an ingestion timeout', async () => {
  const { root, workspace, metadata, document } = fixture()
  const scope = document.resourceSpans[0].scopeSpans[0]
  scope.spans = [scope.spans[0]]
  scope.spans[0].traceId = '00000000000000000000000000000012'
  const file = path.join(workspace, 'wrong-one-span.json')
  fs.writeFileSync(file, JSON.stringify(document))
  const result = await verifyExport(metadata, root)
  assert.equal(result.pass, false)
  assert.match(result.reason, /complete six-span trace/)
  assert.ok(!fs.readdirSync(root).some(name => name.startsWith('export-replay-')))
})

test('dedup-only diagnostics allow matching contents but matching counts do not hide changed data or IDs', async t => {
  for (const changedField of ['unchanged', 'name', 'spanId']) {
    const { root, workspace, metadata, document } = fixture()
    t.diagnostic(`${changedField} dedup replay evidence: ${root}`)
    const file = path.join(workspace, 'trace.json')
    fs.writeFileSync(file, JSON.stringify(document))
    const changed = structuredClone(document)
    if (changedField !== 'unchanged')
      changed.resourceSpans[0].scopeSpans[0].spans[0][changedField] =
        changedField === 'name' ? 'wrong retained name' : 'ffffffffffffffff'
    const binary = path.join(root, 'stub-viewer')
    fs.writeFileSync(binary, `#!${process.execPath}
const http = require('node:http');
const args = process.argv.slice(2);
const port = flag => Number(args[args.indexOf(flag)+1]);
const rpc = http.createServer(async (req,res) => {
  if (req.url.startsWith('/export/')) {
    res.end(JSON.stringify(${JSON.stringify(changed)})); return;
  }
  let body = ''; for await (const chunk of req) body += chunk;
  const request = JSON.parse(body);
   const result = request.method === 'getStats' ? {rejections:[{signal:'traces',kind:'span_already_stored',occurrences:1}]} :
    {columns:[{name:'count'}],rows:request.params.sql === 'SELECT 1' ? [[1]] : [[6,0,0]],truncated:false};
  res.end(JSON.stringify({jsonrpc:'2.0',id:1,result}));
});
const receiver = http.createServer(async (req,res) => {
  for await (const chunk of req) {} res.end('{}');
});
rpc.listen(port('--browser-port'),'127.0.0.1');
receiver.listen(port('--http'),'127.0.0.1');
process.on('SIGTERM',()=>{rpc.close();receiver.close();rpc.closeAllConnections();receiver.closeAllConnections();});
`, { mode: 0o700 })
    saveJson(path.join(root, 'connection.json'), { binary })
    const result = await verifyExport(metadata, root)
    assert.equal(result.pass, changedField === 'unchanged')
    const replay = fs.readdirSync(root).find(name => name.startsWith('export-replay-'))!
    const proof = record(readJson(path.join(root, replay, 'export-proof.json')))
    assert.equal(proof.pass, changedField === 'unchanged')
    if (changedField !== 'unchanged')
      assert.match(String(proof.reason), /does not retain the expected contents/)
    assert.deepEqual(record(record(readJson(path.join(root, replay, 'runtime/stats.json'))).result).rejections,
      [{ signal: 'traces', kind: 'span_already_stored', occurrences: 1 }])
    assert.deepEqual(record(readJson(path.join(root, replay, 'runtime/counts.json'))).rows, [[6, 0, 0]])
    assert.equal(record(readJson(path.join(root, replay, 'owned-shutdown.json'))).waited, true)
  }
})

test('fresh real dedup-only export retains matching target IDs and contents',
  { skip: !process.env.OTEL_EVAL_VIEWER_BINARY }, async t => {
    const { root, workspace, metadata, document } = fixture()
    t.diagnostic(`Dedup export evidence: ${root}`)
    const scope = document.resourceSpans[0].scopeSpans[0]
    const extra = structuredClone(scope.spans[0])
    extra.traceId = '00000000000000000000000000000012'
    scope.spans.push(extra, structuredClone(extra))
    const bytes = Buffer.from(JSON.stringify(document, null, 2) + '\n')
    const file = path.join(workspace, 'dedup.json')
    fs.writeFileSync(file, bytes)
    saveJson(path.join(root, 'connection.json'), { binary: process.env.OTEL_EVAL_VIEWER_BINARY })
    const result = await verifyExport(metadata, root)
    assert.equal(result.pass, true)
    assert.deepEqual(result.files, [file])
    const replay = fs.readdirSync(root).find(name => name.startsWith('export-replay-'))!
    assert.deepEqual(fs.readFileSync(path.join(root, replay, 'fixture/trace.json')), bytes)
    assert.deepEqual(record(readJson(path.join(root, replay, 'runtime/counts.json'))).rows, [[7, 0, 0]])
    const rejections = record(record(readJson(path.join(root, replay, 'runtime/stats.json'))).result).rejections as Record<string, unknown>[]
    assert.ok(rejections.length)
    assert.ok(rejections.every(rejection => rejection.signal === 'traces' && rejection.kind === 'span_already_stored'))
    assert.equal(record(readJson(path.join(root, replay, 'export-proof.json'))).pass, true)
    const shutdown = record(readJson(path.join(root, replay, 'owned-shutdown.json')))
    assert.equal(shutdown.waited, true)
    assert.equal(record(shutdown.exit).code, 0)
    assert.throws(() => process.kill(Number(shutdown.pid), 0), /ESRCH/)
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

test(
  'archive-only handoff verifies contained bytes and reports the delivered archive path',
  { skip: !process.env.OTEL_EVAL_VIEWER_BINARY },
  async () => {
    const { root, workspace, metadata, document } = fixture()
    const bytes = Buffer.from(JSON.stringify(document, null, 2) + '\n')
    const file = path.join(workspace, 'handoff.tar.gz')
    const packed = await archive(root, {
      'handoff/trace.json': bytes,
      'handoff/copy.json': bytes,
      'README.txt': 'Import the OTLP JSON in handoff/trace.json.',
      'unrelated.json': '{}',
    })
    fs.writeFileSync(file, packed)
    fs.writeFileSync(
      path.join(workspace, 'broken.tar.gz'),
      packed.subarray(0, -8)
    )
    fs.writeFileSync(
      path.join(workspace, 'duplicate.tar.gz'),
      repeatedArchive('trace.json', './trace.json', bytes)
    )
    saveJson(path.join(root, 'connection.json'), {
      binary: process.env.OTEL_EVAL_VIEWER_BINARY,
    })
    const result = await verifyExport(metadata, root)
    assert.equal(result.pass, true)
    assert.deepEqual(result.files, [file])
    assert.deepEqual(fs.readFileSync(file), packed)
    assert.deepEqual(scratchFiles(workspace).sort(), [
      path.join(workspace, 'broken.tar.gz'),
      path.join(workspace, 'duplicate.tar.gz'),
      file,
    ])
    const replays = fs
      .readdirSync(root)
      .filter(name => name.startsWith('export-replay-'))
    assert.equal(replays.length, 2)
    const members: string[] = []
    for (const replay of replays) {
      assert.deepEqual(
        fs.readFileSync(path.join(root, replay, 'fixture/trace.json')),
        bytes
      )
      const proof = record(
        readJson(path.join(root, replay, 'export-proof.json'))
      )
      assert.equal(proof.file, file)
      assert.equal(proof.pass, true)
      members.push(String(proof.member))
      const shutdown = record(
        readJson(path.join(root, replay, 'owned-shutdown.json'))
      )
      assert.equal(shutdown.waited, true)
      assert.equal(record(shutdown.exit).code, 0)
      assert.throws(() => process.kill(Number(shutdown.pid), 0), /ESRCH/)
    }
    assert.deepEqual(members.sort(), [
      './handoff/copy.json', './handoff/trace.json',
    ])
  }
)
