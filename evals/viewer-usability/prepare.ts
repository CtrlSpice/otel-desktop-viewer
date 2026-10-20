import fs from 'node:fs'
import path from 'node:path'
import net from 'node:net'
import { createHash } from 'node:crypto'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { setTimeout as delay } from 'node:timers/promises'
import { datasetManifest, readManifest } from './stage-dataset.ts'
import { integer, record, runtimeRoot, saveJson } from './runtime.ts'
import { stopOwnedGroup } from './run-cancellation.ts'

const floorDivide = (value: bigint, divisor: bigint) =>
  value / divisor - (value % divisor < 0n ? 1n : 0n)

export function rfc3339Nano(value: unknown) {
  const nanos = integer(value)
  const seconds = floorDivide(nanos, 1_000_000_000n)
  const days = floorDivide(seconds, 86400n)
  const daytime = seconds - days * 86400n
  // Gregorian civil date from epoch days, using integer arithmetic throughout.
  const shifted = days + 719468n
  const era = floorDivide(shifted, 146097n)
  const dayOfEra = shifted - era * 146097n
  const yearOfEra =
    (dayOfEra - dayOfEra / 1460n + dayOfEra / 36524n - dayOfEra / 146096n) /
    365n
  let year = yearOfEra + era * 400n
  const dayOfYear =
    dayOfEra - (365n * yearOfEra + yearOfEra / 4n - yearOfEra / 100n)
  const monthIndex = (5n * dayOfYear + 2n) / 153n
  const day = dayOfYear - (153n * monthIndex + 2n) / 5n + 1n
  const month = monthIndex + (monthIndex < 10n ? 3n : -9n)
  if (month <= 2n) year++
  const pad = (value: bigint, width = 2) => String(value).padStart(width, '0')
  return `${pad(year, 4)}-${pad(month)}-${pad(day)}T${pad(daytime / 3600n)}:${pad((daytime / 60n) % 60n)}:${pad(daytime % 60n)}.${pad(nanos - seconds * 1_000_000_000n, 9)}Z`
}

export async function freePorts() {
  const servers = [net.createServer(), net.createServer(), net.createServer()]
  try {
    const ports = []
    for (const server of servers) {
      server.listen(0, '127.0.0.1')
      await once(server, 'listening')
      const address = server.address()
      if (!address || typeof address === 'string')
        throw new Error('No TCP address')
      ports.push(address.port)
    }
    return ports
  } finally {
    await Promise.all(
      servers
        .filter(server => server.listening)
        .map(
          server =>
            new Promise<void>((resolve, reject) =>
              server.close(error => (error ? reject(error) : resolve()))
            )
        )
    )
  }
}

export class IngestionRejectedError extends Error {}

// Counts describe a fresh store, not raw entries or the task's answer key.
// Only spans have a telemetry-row identity; logs and datapoints get new UUIDs.
export function expectedStoredCounts(
  inputs: { signal: string; document: unknown }[]
) {
  const spans = new Set<string>()
  let logs = 0
  let datapoints = 0
  const entries = (value: unknown) => {
    if (value === undefined) return []
    if (!Array.isArray(value)) throw new Error('Expected OTLP collection')
    return value.map(record)
  }
  for (const { signal, document } of inputs) {
    const data = record(document)
    if (signal === 'traces') {
      for (const resource of entries(data.resourceSpans))
        for (const scope of entries(resource.scopeSpans))
          for (const span of entries(scope.spans)) {
            const traceID = String(span.traceId ?? '').toLowerCase()
            const spanID = String(span.spanId ?? '').toLowerCase()
            // Do not turn invalid IDs into an empty successful import while the
            // Collector is still batching. Receiver/store rejection decides that.
            spans.add(JSON.stringify([
              traceID || '0'.repeat(32), spanID || '0'.repeat(16),
            ]))
          }
    } else if (signal === 'logs') {
      for (const resource of entries(data.resourceLogs))
        for (const scope of entries(resource.scopeLogs))
          logs += entries(scope.logRecords).length
    } else if (signal === 'metrics') {
      for (const resource of entries(data.resourceMetrics))
        for (const scope of entries(resource.scopeMetrics))
          for (const metric of entries(scope.metrics))
            for (const kind of ['gauge', 'sum', 'histogram', 'exponentialHistogram'])
              if (metric[kind] !== undefined)
                datapoints += entries(record(metric[kind]).dataPoints).length
    } else throw new Error('Unknown OTLP signal')
  }
  return [spans.size, logs, datapoints]
}

export async function postBytes(
  url: string,
  data: Uint8Array
): Promise<unknown> {
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: Buffer.from(data),
    signal: AbortSignal.timeout(10000),
  })
  if (!response.ok) {
    if (response.status === 400 && new URL(url).pathname.startsWith('/v1/'))
      throw new IngestionRejectedError('OTLP payload rejected: HTTP 400')
    throw new Error(`HTTP request failed: ${response.status}`)
  }
  return response.json()
}

export const post = (url: string, data: unknown) =>
  postBytes(url, Buffer.from(JSON.stringify(data)))

export function checkReceipt(value: unknown) {
  const receipt = record(value)
  const partial =
    receipt.partialSuccess === undefined ? {} : record(receipt.partialSuccess)
  if (
    partial.errorMessage ||
    ['rejectedSpans', 'rejectedLogRecords', 'rejectedDataPoints'].some(
      key => integer(partial[key] ?? 0) !== 0n
    )
  ) {
    throw new IngestionRejectedError(
      'Fixture request was not fully accepted: ' + JSON.stringify(partial)
    )
  }
}

export async function query(endpoint: string, sql: string) {
  const response = record(
    await post(endpoint + '/rpc', {
      jsonrpc: '2.0',
      id: 1,
      method: 'query',
      params: { sql, limit: 100 },
    })
  )
  if ('error' in response) throw new Error(JSON.stringify(response.error))
  const result = record(response.result)
  if (
    !Array.isArray(result.rows) ||
    !result.rows.every(Array.isArray) ||
    !Array.isArray(result.columns)
  ) {
    throw new Error('Invalid query result')
  }
  if (typeof result.truncated !== 'boolean')
    throw new Error('Invalid query truncation flag')
  return {
    ...result,
    truncated: result.truncated,
    rows: result.rows.map(row => Array.from(row, (value: unknown) => value)),
    columns: result.columns.map(value => {
      const column = record(value)
      if (typeof column.name !== 'string')
        throw new Error('Invalid query column')
      return { ...column, name: column.name }
    }),
  }
}

export async function prepare(root = runtimeRoot(), selfTelemetryEndpoint?: string, signal?: AbortSignal) {
  signal?.throwIfAborted()
  const manifest = datasetManifest(
    readManifest(path.join(root, 'fixture/manifest.json'))
  )
  const window = {
    start: rfc3339Nano(manifest.start.toString()),
    end: rfc3339Nano(manifest.end.toString()),
  }
  const runtime = path.join(root, 'runtime')
  fs.mkdirSync(runtime)
  const [ui, grpc, http] = await freePorts()
  signal?.throwIfAborted()
  const binary = path.join(root, 'otel-desktop-viewer')
  const endpoint = `http://127.0.0.1:${ui}`
  const command = [
    binary,
    '--open-browser=false',
    '--host',
    '127.0.0.1',
    '--browser-port',
    String(ui),
    '--grpc',
    String(grpc),
    '--http',
    String(http),
    '--db',
    path.join(runtime, 'fixture.duckdb'),
    '--db-max-size',
    '0',
    ...(selfTelemetryEndpoint ? ['--self-telemetry-endpoint', selfTelemetryEndpoint] : []),
  ]
  const out = fs.openSync(path.join(runtime, 'viewer.stdout'), 'wx', 0o600)
  let err: number
  try {
    err = fs.openSync(path.join(runtime, 'viewer.stderr'), 'wx', 0o600)
  } catch (error) {
    fs.closeSync(out)
    throw error
  }
  const child = spawn(binary, command.slice(1), {
    cwd: runtime,
    detached: true,
    stdio: ['ignore', out, err],
  })
  fs.closeSync(out)
  fs.closeSync(err)
  let spawnError: Error | undefined
  child.on('error', error => {
    spawnError = error
  })
  const closed = new Promise<{ code: number | null; signal: string | null }>(
    resolve => child.once('close', (code, signal) => resolve({ code, signal }))
  )
  let stopping: Promise<void> | undefined
  const stop = () => stopping ??= (child.pid ? stopOwnedGroup(child.pid, child) : closed.then(() => {}))
  const cancel = () => { void stop().catch(() => {}) }
  signal?.addEventListener('abort', cancel, { once: true })
  try {
    signal?.throwIfAborted()
    saveJson(path.join(runtime, 'process.json'), {
      pid: child.pid,
      command,
      binary_sha256: createHash('sha256')
        .update(fs.readFileSync(binary))
        .digest('hex'),
    })
    let ready = false
    for (let attempt = 0; attempt < 100; attempt++) {
      signal?.throwIfAborted()
      if (spawnError) throw spawnError
      if (child.exitCode !== null || child.signalCode !== null)
        throw new Error('Owned viewer exited; inspect viewer.stderr')
      try {
        await query(endpoint, 'SELECT 1')
        ready = true
        break
      } catch {
        await delay(100)
      }
    }
    if (!ready) throw new Error('Owned viewer did not become ready')
    const receipts: Record<string, unknown> = {}
    const inputs: { signal: string; document: unknown }[] = []
    for (const request of manifest.requests) {
      signal?.throwIfAborted()
      const bytes = fs.readFileSync(path.join(root, 'fixture', request.file))
      const receipt = await postBytes(
        `http://127.0.0.1:${http}/v1/${request.signal}`,
        bytes
      )
      receipts[request.file] = receipt
      fs.writeFileSync(
        path.join(runtime, 'ingestion.json'),
        JSON.stringify(receipts, null, 2) + '\n',
        { mode: 0o600 }
      )
      checkReceipt(receipt)
      inputs.push({
        signal: request.signal,
        document: JSON.parse(bytes.toString('utf8')),
      })
    }
    const expectedCounts = expectedStoredCounts(inputs)
    saveJson(path.join(runtime, 'expected-counts.json'), expectedCounts)
    let counts
    for (let attempt = 0; attempt < 100; attempt++) {
      signal?.throwIfAborted()
      counts = await query(
        endpoint,
        'SELECT (SELECT count(*) FROM spans), (SELECT count(*) FROM logs), (SELECT count(*) FROM metric_datapoints)'
      )
      fs.writeFileSync(
        path.join(runtime, 'counts.json'),
        JSON.stringify(counts, null, 2) + '\n',
        { mode: 0o600 }
      )
      const stats = record(
        await post(endpoint + '/rpc', {
          jsonrpc: '2.0',
          id: 1,
          method: 'getStats',
        })
      )
      fs.writeFileSync(
        path.join(runtime, 'stats.json'),
        JSON.stringify(stats, null, 2) + '\n',
        { mode: 0o600 }
      )
      if ('error' in stats) throw new Error(JSON.stringify(stats.error))
      const rejections = record(stats.result).rejections
      if (!Array.isArray(rejections))
        throw new Error('Invalid fixture store rejection diagnostics')
      if (rejections.some(value => {
        const rejection = record(value)
        return rejection.signal !== 'traces' ||
          rejection.kind !== 'span_already_stored'
      }))
        throw new IngestionRejectedError(
          'Fixture store reported rejected records; inspect stats.json'
        )
      if (JSON.stringify(counts.rows) === JSON.stringify([expectedCounts]))
        break
      if (attempt === 99)
        throw new Error(
          'Fixture ingestion visibility not confirmed before deadline: expected ' +
          JSON.stringify(expectedCounts) +
          ', observed ' + JSON.stringify(counts)
        )
      await delay(100)
    }
    const connection = { binary, endpoint, ...window }
    signal?.throwIfAborted()
    saveJson(path.join(root, 'connection.json'), connection)
    child.unref()
    return {
      ready: true,
      pid: child.pid,
      ...connection,
      process: child,
      closed,
    }
  } catch (error) {
    const failure = signal?.aborted ? signal.reason : error
    try {
      saveJson(path.join(runtime, 'failure.json'), {
        error: failure instanceof Error ? failure.message : String(failure),
      })
    } finally {
      await stop()
    }
    throw failure
  } finally {
    signal?.removeEventListener('abort', cancel)
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === import.meta.filename) {
  const { process: ownedProcess, closed, ...connection } = await prepare(runtimeRoot(), process.env.OTEL_EVAL_OBSERVER_GRPC)
  ownedProcess.unref()
  console.log(JSON.stringify(connection))
}
