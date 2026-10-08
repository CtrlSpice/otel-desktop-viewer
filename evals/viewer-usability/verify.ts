import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import cases from './tasks.json' with { type: 'json' }
import { query } from './prepare.ts'
import { datasetManifest, readManifest } from './stage-dataset.ts'
import { readJson, record, runtimeRoot, saveJson } from './runtime.ts'

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
  async function sql(name: string, statement: string) {
    const result = await query(endpoint, statement)
    saveJson(path.join(evidence, name + '.json'), { sql: statement, result })
    if (result.truncated) throw new Error('Verifier result truncated')
    return result.rows.map(row =>
      Object.fromEntries(
        result.columns.map((column, index) => [column.name, row[index]])
      )
    )
  }
  const START = manifest.start.toString(),
    END = manifest.end.toString()
  const scope = (alias = 's') =>
    `${alias}.start_time BETWEEN ${START}::UBIGINT AND ${END}::UBIGINT AND ${alias}.service_name='checkout'`
  const owned = `FROM spans s CROSS JOIN unnest(s.attribute_ids) ids(id) JOIN attributes a ON a.id=ids.id WHERE ${scope()}`
  const actual: Record<string, unknown> = {}
  actual['typed-values'] = {
    attempt: await sql(
      'attempt',
      `SELECT DISTINCT json_extract_string(a.value,'$.kind') AS kind,json_extract_string(a.value,'$.value') AS value ${owned} AND a.key='attempt' ORDER BY kind`
    ),
    methods: await sql(
      'methods',
      `WITH owners AS (SELECT DISTINCT s.trace_id,s.span_id,json_extract_string(a.value,'$.value') AS value ${owned} AND a.key='http.request.method') SELECT value,count(*) AS count,(SELECT count(*) FROM owners) AS denominator,count(*)::DOUBLE/(SELECT count(*) FROM owners) AS relativeFrequency FROM owners GROUP BY value ORDER BY value`
    ),
  }
  const resourceFrequency: Record<string, unknown> = {}
  for (const [label, table, service, predicate] of [
    ['spans', 'spans', 'checkout', `s.start_time BETWEEN ${START} AND ${END}`],
    [
      'logs',
      'logs',
      'worker',
      `coalesce(nullif(s.timestamp,0),s.observed_timestamp) BETWEEN ${START} AND ${END}`,
    ],
  ]) {
    resourceFrequency[label] = await sql(
      label + '-regions',
      `WITH owners AS (SELECT json_extract_string(a.value,'$.value') AS region FROM ${table} s JOIN resources r ON r.id=s.resource_id CROSS JOIN unnest(r.attribute_ids) ids(id) JOIN attributes a ON a.id=ids.id WHERE ${predicate} AND s.service_name='${service}' AND a.key='region') SELECT region,count(*) AS count,(SELECT count(*) FROM owners) AS denominator FROM owners GROUP BY region ORDER BY region`
    )
  }
  actual['resource-frequency'] = resourceFrequency
  actual['trace-investigation'] = {
    errorSpans: await sql(
      'errors',
      "SELECT trace_id_wire(trace_id) AS traceID,span_id_wire(span_id) AS spanID,service_name AS service FROM spans WHERE trace_id='00000000-0000-0000-0000-00000000000a'::UUID AND status_code=2"
    ),
    logs: await sql(
      'trace-logs',
      "SELECT json_extract_string(l.body,'$.value') AS body, CASE WHEN l.span_id IS NULL THEN 'trace-only' WHEN s.span_id IS NULL THEN 'missing-span' ELSE 'matching-span' END AS association FROM logs l LEFT JOIN spans s ON l.trace_id=s.trace_id AND l.span_id=s.span_id WHERE l.trace_id='00000000-0000-0000-0000-00000000000a'::UUID ORDER BY body"
    ),
  }
  const childTime: Record<string, unknown> = {}
  for (const [label, source, parent, key] of [
    [
      'events',
      'events e JOIN spans s ON s.trace_id=e.trace_id AND s.span_id=e.span_id',
      'struct_pack(trace_id := s.trace_id,span_id := s.span_id)',
      'exception.type',
    ],
    [
      'exemplars',
      'exemplars e JOIN metric_datapoints d ON d.id=e.metric_datapoint_id JOIN metrics s ON s.id=d.metric_id',
      'd.id',
      'sample.label',
    ],
  ]) {
    childTime[label] = await sql(
      label,
      `SELECT json_extract_string(a.value,'$.value') AS value,count(DISTINCT ${parent}) AS count FROM ${source} CROSS JOIN unnest(e.attribute_ids) ids(id) JOIN attributes a ON a.id=ids.id WHERE e.timestamp BETWEEN ${START} AND ${END} AND s.service_name='worker' AND a.key='${key}' GROUP BY value`
    )
  }
  const outside = await sql(
    'parent-outside',
    `SELECT EXISTS(SELECT 1 FROM events e JOIN spans s ON e.trace_id=s.trace_id AND e.span_id=s.span_id WHERE e.timestamp BETWEEN ${START} AND ${END} AND s.start_time NOT BETWEEN ${START} AND ${END}) AS outside`
  )
  childTime.hasParentOutsideWindow = outside[0].outside
  actual['child-time'] = childTime
  const inventory = (
    await sql(
      'customers',
      `SELECT count(DISTINCT json_extract_string(a.value,'$.value')) AS distinctCustomers,min(json_extract_string(a.value,'$.value')) AS first,max(json_extract_string(a.value,'$.value')) AS last ${owned} AND a.key='customer.id'`
    )
  )[0]
  inventory.complete = true
  inventory.missingKeyPresent =
    (await sql('missing', `SELECT a.id ${owned} AND a.key='customer.missing'`))
      .length > 0
  actual['complete-customer-inventory'] = inventory
  actual['custom-aggregation'] = {
    groups: await sql(
      'aggregate',
      `SELECT json_extract_string(a.value,'$.value') AS method,count(*) AS spanCount,count(*) FILTER(WHERE s.status_code=2) AS errorCount,sum(s.end_time::HUGEINT-s.start_time::HUGEINT)::VARCHAR AS totalDurationNs ${owned} AND a.key='http.request.method' GROUP BY method ORDER BY method`
    ),
  }
  for (const task of cases)
    assert.deepEqual(
      normalize(actual[task.id]),
      normalize(task.expected),
      task.id
    )
  saveJson(path.join(evidence, 'answers.json'), actual)
  console.log('All six expected answers verified against the ingested store.')
}

if (process.argv[1] && path.resolve(process.argv[1]) === import.meta.filename)
  await verify(process.argv[2])
