import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import { query } from './prepare.ts'
import { datasetManifest, readManifest } from './stage-dataset.ts'
import { readJson, record, runtimeRoot, saveJson } from './runtime.ts'
import { exportedTrace, traceRows } from './verify-export.ts'
import { verifyDiscovery } from './verify-discovery.ts'

export function normalize(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(normalize).sort((left, right) => {
      const a = JSON.stringify(left),
        b = JSON.stringify(right)
      return a < b ? -1 : a > b ? 1 : 0
    })
  }
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value)
        .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
        .map(([key, item]) => [key, normalize(item)])
    )
  }
  return value
}

export async function verify(name = 'verification', root = runtimeRoot()) {
  if (!name || name === '.' || name === '..' || /[\\/]/.test(name)) {
    throw new Error(
      'Verification output must be a directory name inside the external runtime.'
    )
  }
  const manifest = datasetManifest(
    readManifest(path.join(root, 'fixture/manifest.json'))
  )
  const connection = record(readJson(path.join(root, 'connection.json')))
  if (typeof connection.endpoint !== 'string')
    throw new Error('Invalid connection endpoint')
  const endpoint = connection.endpoint
  const evidence = path.join(root, name)
  fs.mkdirSync(evidence)
  saveJson(
    path.join(evidence, 'openrpc-discovery.json'),
    await verifyDiscovery(endpoint, manifest.start, manifest.end)
  )
  async function sql(name: string, statement: string) {
    const result = await query(endpoint, statement)
    saveJson(path.join(evidence, name + '.json'), {
      sql: statement,
      result,
    })
    if (result.truncated) throw new Error('Verifier result truncated')
    return result.rows.map(row =>
      Object.fromEntries(
        result.columns.map((column, index) => [column.name, row[index]])
      )
    )
  }
  const START = manifest.start.toString(),
    END = manifest.end.toString()
  const roots = await sql(
    'roots',
    `SELECT trace_id_wire(trace_id) AS traceID FROM spans WHERE parent_span_id IS NULL AND service_name='api-gateway' AND start_time BETWEEN ${START}::UBIGINT AND ${END}::UBIGINT ORDER BY start_time`
  )
  assert.equal(roots.length, 32)
  const sourceRows = manifest.requests
    .filter(request => request.signal === 'traces')
    .flatMap(request =>
      traceRows(readJson(path.join(root, 'fixture', request.file)))
    )
  const stored: Record<string, unknown>[] = []
  for (const row of roots) {
    assert.equal(typeof row.traceID, 'string')
    const document = await exportedTrace(endpoint, String(row.traceID))
    saveJson(path.join(evidence, 'trace-' + row.traceID + '.json'), document)
    const rows = traceRows(document)
    assert.deepEqual(
      rows,
      sourceRows.filter(source => record(source.span).traceId === row.traceID)
    )
    stored.push(...rows)
  }
  const attr = (owner: Record<string, unknown>, key: string) => {
    assert.ok(Array.isArray(owner.attributes))
    return record(
      owner.attributes.map(record).find(attribute => attribute.key === key)
        ?.value
    )
  }
  const rootRows = stored.filter(row => !record(row.span).parentSpanId)
  const customers = rootRows.map(
    row => attr(record(row.span), 'customer.id').stringValue
  )
  assert.equal(new Set(customers).size, 32)
  assert.deepEqual(
    customers,
    Array.from(
      { length: 32 },
      (_, index) => 'customer-' + String(index + 1).padStart(3, '0')
    )
  )
  for (const [index, row] of rootRows.entries()) {
    const value = attr(record(row.span), 'order.sequence')
    assert.deepEqual(value, {
      [index === 7 ? 'stringValue' : 'intValue']: String(
        9007199254740993n + BigInt(index)
      ),
    })
  }
  const cohorts = [0, 8, 16, 24].map(offset => {
    const rows = rootRows.slice(offset, offset + 8)
    const region = attr(record(rows[0].resource), 'cloud.region').stringValue
    const version = attr(
      record(rows[0].resource),
      'service.version'
    ).stringValue
    for (const row of rows) {
      assert.equal(
        attr(record(row.resource), 'cloud.region').stringValue,
        region
      )
      assert.equal(
        attr(record(row.resource), 'service.version').stringValue,
        version
      )
    }
    return {
      region,
      version,
      roots: rows.length,
      failures: rows.filter(row => record(record(row.span).status).code === 2)
        .length,
    }
  })
  assert.deepEqual(cohorts, [
    { region: 'west', version: '1.0.0', roots: 8, failures: 0 },
    { region: 'east', version: '1.0.0', roots: 8, failures: 0 },
    { region: 'west', version: '1.1.0', roots: 8, failures: 4 },
    { region: 'east', version: '1.0.0', roots: 8, failures: 0 },
  ])
  const span = (checkout: number, name: string) =>
    record(
      stored.find(
        row =>
          record(row.span).traceId ===
            checkout.toString(16).padStart(32, '0') &&
          record(row.span).name === name
      )?.span
    )
  const duration = (value: Record<string, unknown>) =>
    String(
      BigInt(String(value.endTimeUnixNano)) -
        BigInt(String(value.startTimeUnixNano))
    )
  for (const [checkout, durations] of [
    [18, ['600000000', '195000000', '80000000']],
    [19, ['1500000000', '1350000000', '1250000000']],
  ] as const) {
    assert.deepEqual(
      ['POST /api/v2/orders', 'payment/charge', 'stripe/create-charge'].map(
        name => duration(span(checkout, name))
      ),
      durations
    )
    assert.equal(record(span(checkout, 'POST /api/v2/orders').status).code, 1)
  }
  const provider = span(17, 'stripe/create-charge')
  assert.equal(record(provider.status).code, 2)
  const failedRoot = span(17, 'POST /api/v2/orders')
  assert.equal(record(failedRoot.status).code, 2)
  assert.equal(duration(failedRoot), '1200000000')
  assert.deepEqual(attr(failedRoot, 'http.response.status_code'), {
    intValue: '402',
  })
  const exception = record((provider.events as unknown[])[0])
  assert.deepEqual(attr(exception, 'exception.type'), {
    stringValue: 'PaymentDeclinedError',
  })
  assert.deepEqual(attr(exception, 'exception.message'), {
    stringValue: 'Card declined: insufficient funds',
  })
  const logs = await sql(
    'trace-logs',
    "SELECT json_extract_string(l.body,'$.value') AS body, CASE WHEN l.span_id IS NULL THEN 'trace-only' WHEN s.span_id IS NULL THEN 'missing-span' ELSE 'matching-span' END AS association FROM logs l LEFT JOIN spans s ON l.trace_id=s.trace_id AND l.span_id=s.span_id WHERE l.trace_id='00000000-0000-0000-0000-000000000011'::UUID ORDER BY body"
  )
  assert.deepEqual(logs, [
    {
      body: 'Delayed payment audit received',
      association: 'missing-span',
    },
    {
      body: 'POST /api/v2/orders returned 402',
      association: 'matching-span',
    },
    {
      body: 'Payment declined: insufficient_funds',
      association: 'matching-span',
    },
    { body: 'Request audit recorded', association: 'trace-only' },
    {
      body: 'Stock below threshold; reservation succeeded',
      association: 'matching-span',
    },
  ])
  const metrics = await sql(
    'metrics',
    'SELECT id::VARCHAR AS id,name,service_name FROM metrics ORDER BY name'
  )
  assert.equal(metrics.length, 4)
  assert.ok(metrics.every(metric => metric.service_name === 'api-gateway'))
  const metricDocuments: Record<string, unknown>[] = []
  for (const metric of metrics) {
    const response = await fetch(`${endpoint}/export/metrics/${metric.id}`, {
      signal: AbortSignal.timeout(10000),
    })
    if (!response.ok) throw new Error('Metric export failed')
    const document = record(await response.json())
    saveJson(path.join(evidence, 'metric-' + metric.id + '.json'), document)
    for (const resource of (document.resourceMetrics as unknown[]).map(record))
      for (const scope of (resource.scopeMetrics as unknown[]).map(record))
        metricDocuments.push(...(scope.metrics as unknown[]).map(record))
  }
  verifyMetricFacts(metricDocuments, START, END)
  saveJson(path.join(evidence, 'answers.json'), {
    roots: rootRows.length,
    customers: customers.length,
    cohorts,
    logs,
    metrics,
    correlationOnly: true,
    providerInternalCauseObserved: false,
  })
  console.log(
    'Approved investigation facts verified against the ingested small store.'
  )
}

export function verifyMetricFacts(
  metrics: Record<string, unknown>[],
  start: string,
  end: string
) {
  const histogram = record(metrics.find(metric => metric.histogram)?.histogram)
  assert.equal(histogram.aggregationTemporality, 1)
  const points = (histogram.dataPoints as unknown[]).map(record)
  const spike = points.find(
    point => point.timeUnixNano === '1791360025000000000'
  )!
  assert.equal(spike.startTimeUnixNano, '1791360000000000000')
  assert.equal(spike.count, '16')
  assert.ok(Math.abs(Number(spike.sum) / 16 - 0.421875) < 1e-12)
  assert.equal(spike.max, 1.5)
  assert.deepEqual(spike.bucketCounts, ['1', '11', '2', '2'])
  assert.deepEqual(spike.explicitBounds, [0.1, 0.5, 1])
  const next = points[points.indexOf(spike) + 1]
  assert.ok(Math.abs(Number(next.sum) / Number(next.count) - 0.175) < 1e-12)
  assert.ok(
    Math.abs(Number(points.at(-1)!.sum) / Number(points.at(-1)!.count) - 0.15) <
      1e-12
  )
  const exemplar = record((spike.exemplars as unknown[])[0])
  assert.equal(exemplar.asDouble, 1.2)
  assert.equal(exemplar.traceId, '00000000000000000000000000000011')
  assert.equal(exemplar.spanId, '0000000000000001')
  const sum = record(metrics.find(metric => metric.sum)?.sum)
  assert.equal(sum.aggregationTemporality, 2)
  assert.equal(sum.isMonotonic, true)
  const snapshots = (sum.dataPoints as unknown[]).map(record)
  assert.equal(snapshots[0].asInt, '3')
  assert.equal(snapshots.at(-1)!.asInt, '104')
  assert.equal(snapshots.at(-1)!.timeUnixNano, end)
  for (const [index, point] of snapshots.entries()) {
    assert.equal(point.startTimeUnixNano, start)
    if (index)
      assert.ok(
        BigInt(String(point.asInt)) >=
          BigInt(String(snapshots[index - 1].asInt))
      )
  }
  const gauge = record(metrics.find(metric => metric.gauge)?.gauge)
  const cpu = (gauge.dataPoints as unknown[]).map(record)
  assert.equal(Math.min(...cpu.map(point => Number(point.asDouble))), 0.1)
  assert.equal(Math.max(...cpu.map(point => Number(point.asDouble))), 0.18)
  assert.equal(
    cpu.find(point => point.timeUnixNano === '1791360000000000000')!.asDouble,
    0.14
  )
  assert.equal(
    cpu.find(point => point.timeUnixNano === '1791360025000000000')!.asDouble,
    0.16
  )
  for (const [index, point] of cpu.entries()) {
    const time = BigInt(String(point.timeUnixNano))
    assert.ok(time < 1791360004000000000n || time > 1791360005500000000n)
    if (index)
      assert.equal(
        time - BigInt(String(cpu[index - 1].timeUnixNano)),
        25000000000n
      )
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === import.meta.filename)
  await verify(process.argv[2])
