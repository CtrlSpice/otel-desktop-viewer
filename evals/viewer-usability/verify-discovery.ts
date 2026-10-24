import assert from 'node:assert/strict'
import { record } from './runtime.ts'

export async function verifyDiscovery(endpoint: string, start: bigint, end: bigint) {
  const root = new URL('/', endpoint)
  const response = await fetch(root, { signal: AbortSignal.timeout(10000) })
  assert.equal(response.status, 200, 'Viewer root unavailable')
  const link = response.headers.get('link') ?? ''
  const description = link.match(/<([^>]+)>;\s*rel="service-desc"/)
  assert.ok(description, 'Viewer root does not advertise its API description')
  await response.arrayBuffer()
  const documentURL = new URL(description[1], root)
  const documentResponse = await fetch(documentURL, {
    signal: AbortSignal.timeout(10000),
  })
  assert.equal(documentResponse.status, 200, 'API description unavailable')
  assert.equal(
    documentResponse.headers.get('content-type')?.split(';')[0].trim(),
    'application/json',
    'API discovery returned something other than JSON'
  )
  // Keep the source text: schema bounds can exceed JavaScript's exact integers.
  const documentText = await documentResponse.text()
  const document = record(JSON.parse(documentText))
  assert.equal(typeof document.openrpc, 'string')
  assert.ok(Array.isArray(document.methods))
  const method = document.methods.map(record).find(value => value.name === 'query')
  assert.ok(method, 'API description does not list query')
  assert.ok(Array.isArray(method.params))
  const parameters = method.params.map(record).map(value => value.name)
  assert.ok(parameters.includes('sql') && parameters.includes('limit'))
  assert.ok(Array.isArray(document.servers) && document.servers.length)
  const server = record(document.servers[0])
  assert.equal(typeof server.url, 'string')
  const rpcURL = new URL(String(server.url), documentURL)
  const sql = `SELECT m.name, m.aggregation_temporality, m.is_monotonic,
    d.start_time::VARCHAR AS start_time, d.timestamp::VARCHAR AS timestamp,
    d.int_value::VARCHAR AS int_value
    FROM metrics m JOIN metric_datapoints d ON d.metric_id=m.id
    WHERE m.name='http.server.request.count' AND m.service_name='api-gateway'
    AND d.timestamp BETWEEN ${start}::UBIGINT AND ${end}::UBIGINT
    ORDER BY d.timestamp DESC LIMIT 1`
  const rpcResponse = await fetch(rpcURL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      jsonrpc: '2.0', id: 'discovery-check', method: method.name,
      params: { sql, limit: 1 },
    }),
    signal: AbortSignal.timeout(10000),
  })
  assert.equal(rpcResponse.status, 200)
  const rpc = record(await rpcResponse.json())
  assert.equal(rpc.id, 'discovery-check')
  assert.equal(rpc.error, undefined, 'Discovered query method failed')
  const result = record(rpc.result)
  assert.equal(result.truncated, false)
  assert.deepEqual(result.rows, [[
    'http.server.request.count', 2, true, start.toString(), end.toString(), '104',
  ]])
  return {
    rootURL: root.href, rootLink: link, documentURL: documentURL.href,
    rpcURL: rpcURL.href, documentText, sql, result,
  }
}
