import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import { prepare, IngestionRejectedError } from './prepare.ts'
import { datasetManifest, readManifest } from './stage-dataset.ts'
import { readJson, record, runtimeRoot, saveJson } from './runtime.ts'

export const CHECKOUT_TRACE = '00000000000000000000000000000011'

const attributes = (value: unknown) => {
  if (value === undefined) return []
  if (!Array.isArray(value)) throw new Error('Invalid attributes')
  return value
    .map(record)
    .sort((a, b) => String(a.key).localeCompare(String(b.key)))
}

// Wrapper grouping and attribute order are not retained ordering guarantees.
// Value arrays, event order, IDs, exact decimal values and payload context remain intact.
export function traceRows(value: unknown) {
  const document = record(value)
  if (!Array.isArray(document.resourceSpans)) throw new Error('Not OTLP traces')
  const rows: Record<string, unknown>[] = []
  for (const rawResource of document.resourceSpans) {
    const resource = record(rawResource)
    const context = record(resource.resource ?? {})
    if (!Array.isArray(resource.scopeSpans))
      throw new Error('Invalid scopeSpans')
    for (const rawScope of resource.scopeSpans) {
      const scope = record(rawScope)
      const instrumentation = record(scope.scope ?? {})
      if (!Array.isArray(scope.spans)) throw new Error('Invalid spans')
      for (const rawSpan of scope.spans) {
        const span = record(rawSpan)
        const events = span.events ?? []
        if (!Array.isArray(events)) throw new Error('Invalid events')
        rows.push({
          resource: {
            attributes: attributes(context.attributes),
            droppedAttributesCount: context.droppedAttributesCount ?? 0,
          },
          resourceSchemaUrl: resource.schemaUrl ?? '',
          scope: {
            name: instrumentation.name ?? '',
            version: instrumentation.version ?? '',
            attributes: attributes(instrumentation.attributes),
            droppedAttributesCount: instrumentation.droppedAttributesCount ?? 0,
          },
          scopeSchemaUrl: scope.schemaUrl ?? '',
          span: {
            traceState: '',
            kind: 0,
            name: '',
            flags: 0,
            droppedAttributesCount: 0,
            droppedEventsCount: 0,
            droppedLinksCount: 0,
            links: [],
            ...span,
            attributes: attributes(span.attributes),
            events: events.map(value => {
              const event = record(value)
              return {
                name: '',
                droppedAttributesCount: 0,
                ...event,
                attributes: attributes(event.attributes),
              }
            }),
            status: { message: '', code: 0, ...record(span.status ?? {}) },
          },
        })
      }
    }
  }
  return rows.sort((a, b) =>
    JSON.stringify([
      record(a.span).traceId,
      record(a.span).spanId,
    ]).localeCompare(
      JSON.stringify([record(b.span).traceId, record(b.span).spanId])
    )
  )
}

export function scratchFiles(workspace: string) {
  const root = fs.realpathSync(workspace)
  const files: string[] = []
  const walk = (directory: string) => {
    for (const entry of fs.readdirSync(directory, {
      withFileTypes: true,
    })) {
      const file = path.join(directory, entry.name)
      const stat = fs.lstatSync(file)
      if (stat.isSymbolicLink()) continue
      if (stat.isDirectory()) walk(file)
      else if (stat.isFile()) files.push(file)
    }
  }
  walk(root)
  return files
}

export async function exportedTrace(
  endpoint: string,
  traceID = CHECKOUT_TRACE
) {
  const response = await fetch(`${endpoint}/export/traces/${traceID}`, {
    signal: AbortSignal.timeout(10000),
  })
  if (!response.ok) throw new Error(`Trace export failed: ${response.status}`)
  return response.json() as Promise<unknown>
}

export async function verifyExport(
  metadata: Record<string, unknown>,
  root = runtimeRoot()
) {
  const workspace = metadata.workspace
  const isolationRoot = metadata.isolationRoot
  if (
    typeof workspace !== 'string' ||
    typeof isolationRoot !== 'string' ||
    !fs
      .realpathSync(isolationRoot)
      .startsWith(fs.realpathSync(path.join(root, 'workspaces')) + path.sep) ||
    fs.realpathSync(workspace) !==
      path.join(fs.realpathSync(isolationRoot), 'scratch')
  )
    throw new Error(
      'Export verifier requires owned investigator scratch metadata'
    )
  const manifest = datasetManifest(
    readManifest(path.join(root, 'fixture/manifest.json'))
  )
  const expected = manifest.requests
    .filter(request => request.signal === 'traces')
    .flatMap(request =>
      traceRows(readJson(path.join(root, 'fixture', request.file)))
    )
    .filter(row => record(row.span).traceId === CHECKOUT_TRACE)
  assert.equal(
    expected.length,
    6,
    'Approved trace reference requires six spans'
  )
  const candidates: { file: string; bytes: Buffer; spanCount: number }[] = []
  for (const file of scratchFiles(workspace)) {
    const descriptor = fs.openSync(
      file,
      fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW
    )
    let bytes: Buffer
    try {
      bytes = fs.readFileSync(descriptor)
    } finally {
      fs.closeSync(descriptor)
    }
    try {
      const rows = traceRows(JSON.parse(bytes.toString('utf8')))
      assert.deepEqual(
        rows.filter(row => record(row.span).traceId === CHECKOUT_TRACE),
        expected
      )
      candidates.push({ file, bytes, spanCount: rows.length })
    } catch {}
  }
  if (!candidates.length)
    return {
      pass: false,
      reason:
        'No file in this investigator scratch retains the complete six-span trace and context.',
      files: [] as string[],
    }
  const connection = record(readJson(path.join(root, 'connection.json')))
  if (typeof connection.binary !== 'string')
    throw new Error('Missing replay binary')
  const files: string[] = []
  for (const candidate of candidates) {
    const replay = fs.mkdtempSync(path.join(root, 'export-replay-'))
    fs.chmodSync(replay, 0o700)
    fs.mkdirSync(path.join(replay, 'fixture'))
    fs.copyFileSync(
      connection.binary,
      path.join(replay, 'otel-desktop-viewer'),
      fs.constants.COPYFILE_EXCL
    )
    fs.writeFileSync(path.join(replay, 'fixture/trace.json'), candidate.bytes, {
      mode: 0o600,
    })
    saveJson(path.join(replay, 'fixture/manifest.json'), {
      startTimeUnixNano: manifest.start.toString(),
      endTimeUnixNano: manifest.end.toString(),
      requests: [{ signal: 'traces', file: 'trace.json' }],
      storedCounts: { spans: candidate.spanCount, logs: 0, datapoints: 0 },
    })
    let ready: Awaited<ReturnType<typeof prepare>>
    try {
      ready = await prepare(replay)
    } catch (error) {
      if (!(error instanceof IngestionRejectedError)) throw error
      saveJson(path.join(replay, 'export-proof.json'), {
        file: candidate.file,
        pass: false,
        reason: error.message,
      })
      continue
    }
    try {
      assert.deepEqual(traceRows(await exportedTrace(ready.endpoint)), expected)
      files.push(candidate.file)
      saveJson(path.join(replay, 'export-proof.json'), {
        file: candidate.file,
        retainedSpans: 6,
        pass: true,
      })
    } catch (error) {
      if (!(error instanceof assert.AssertionError)) throw error
      saveJson(path.join(replay, 'export-proof.json'), {
        file: candidate.file,
        pass: false,
        reason: 'Replayed trace does not retain the expected contents.',
      })
    } finally {
      ready.process.kill('SIGTERM')
      const timer = setTimeout(() => ready.process.kill('SIGKILL'), 30000)
      try {
        saveJson(path.join(replay, 'owned-shutdown.json'), {
          pid: ready.pid,
          waited: true,
          exit: await ready.closed,
        })
      } finally {
        clearTimeout(timer)
      }
    }
  }
  return {
    pass: files.length > 0,
    reason: files.length
      ? 'Produced file replayed unchanged into an owned viewer and retains all six spans, values, context and parents.'
      : 'No candidate file was fully accepted and retained the expected trace contents.',
    files,
  }
}
