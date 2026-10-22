import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { createHash } from 'node:crypto'
import { gunzipSync } from 'node:zlib'

// A byte-preserving receiver, not an instrumentor. Only the owned V2 executable
// receives this endpoint. Wrapper exports use their existing separate receiver.
export async function nativeTelemetryReceiver(root, forward) {
  const directory = path.join(root, 'native-otlp')
  fs.mkdirSync(directory, { mode: 0o700 })
  const receipts = [], pending = new Set()
  const server = http.createServer((request, response) => {
    const task = (async () => {
      const route = request.url
      if (!['/v1/traces', '/v1/logs', '/v1/metrics'].includes(route)) { response.writeHead(404).end(); return }
      const chunks = []; for await (const chunk of request) chunks.push(chunk)
      const bytes = Buffer.concat(chunks)
      const name = String(receipts.length).padStart(5, '0') + '-' + route.split('/').at(-1) + '.otlp'
      const receipt = { name, route, contentType: request.headers['content-type'], contentEncoding: request.headers['content-encoding'],
        bytes: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex'), origin: 'owned-opencode-v2', forwarded: Boolean(forward) }
      receipts.push(receipt)
      fs.writeFileSync(path.join(directory, name), bytes, { flag: 'wx', mode: 0o600 })
      if (forward) {
        const remote = await fetch(forward + route, { method: 'POST', headers: {
          'Content-Type': receipt.contentType, ...(receipt.contentEncoding ? { 'Content-Encoding': receipt.contentEncoding } : {}),
        }, body: bytes, signal: AbortSignal.timeout(30000) })
        receipt.status = remote.status; receipt.reply = await remote.text()
      } else { receipt.status = 200; receipt.reply = '{}' }
      fs.appendFileSync(path.join(directory, 'receipts.jsonl'), JSON.stringify(receipt) + '\n', { mode: 0o600 })
      response.writeHead(receipt.status, { 'Content-Type': 'application/json' }).end(receipt.reply)
    })().catch(error => {
      fs.appendFileSync(path.join(directory, 'errors.log'), String(error) + '\n', { mode: 0o600 })
      response.writeHead(500).end()
    })
    pending.add(task); void task.finally(() => pending.delete(task))
  })
  server.listen(0, '127.0.0.1'); await once(server, 'listening')
  return { endpoint: 'http://127.0.0.1:' + server.address().port, receipts,
    async close() {
      await Promise.all([...pending])
      await new Promise(resolve => server.close(resolve))
      fs.writeFileSync(path.join(directory, 'summary.json'), JSON.stringify({ receipts }, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
    },
  }
}

export function readNativeTelemetry(root, receipts) {
  const spans = [], logs = [], metrics = [], resources = [], scopes = new Map()
  for (const receipt of receipts) {
    let bytes = fs.readFileSync(path.join(root, 'native-otlp', receipt.name))
    if (createHash('sha256').update(bytes).digest('hex') !== receipt.sha256) throw new Error('Native OTLP receipt hash mismatch')
    if (receipt.contentEncoding === 'gzip') bytes = gunzipSync(bytes)
    else if (receipt.contentEncoding) throw new Error('Unsupported native OTLP content encoding; raw bytes retained')
    if (!receipt.contentType?.includes('application/json')) throw new Error('Native OTLP is not JSON; raw bytes retained for separate decoding')
    const body = JSON.parse(bytes)
    for (const [resourceKey, scopeKey, itemsKey, target] of [
      ['resourceSpans', 'scopeSpans', 'spans', spans], ['resourceLogs', 'scopeLogs', 'logRecords', logs], ['resourceMetrics', 'scopeMetrics', 'metrics', metrics],
    ]) for (const resource of body[resourceKey] ?? []) {
      resources.push(resource.resource)
      for (const scope of resource[scopeKey] ?? []) {
        const key = JSON.stringify(scope.scope ?? {})
        const count = scopes.get(key) ?? { scope: scope.scope, spans: 0, logs: 0, metrics: 0 }
        count[itemsKey === 'logRecords' ? 'logs' : itemsKey] += (scope[itemsKey] ?? []).length; scopes.set(key, count)
        target.push(...(scope[itemsKey] ?? []))
      }
    }
  }
  const counts = values => Object.fromEntries([...values.reduce((map, value) => map.set(value, (map.get(value) ?? 0) + 1), new Map())].sort((a, b) => b[1] - a[1]))
  return { spans, logs, metrics, summary: { spans: spans.length, logs: logs.length, metricRecords: metrics.length,
    scopes: [...scopes.values()], resources: [...new Map(resources.map(resource => [JSON.stringify(resource), resource])).values()],
    spanNames: counts(spans.map(span => span.name)), traces: counts(spans.map(span => span.traceId)),
    errors: spans.filter(span => span.status?.code === 2).length,
    wrapperSpans: 0, wrapperToolEvents: 0,
  } }
}

export function nativeTelemetryBoundaries(data, traceparent) {
  const attribute = (span, key) => span.attributes?.find(item => item.key === key)?.value
  const ids = data.spans.map(span => span.traceId + '/' + span.spanId)
  const toolSpans = data.spans.filter(span => span.name === 'Tool.execute')
  const [version, traceID, parentID] = traceparent?.split('-') ?? []
  const matching = version === '00' ? data.spans.filter(span => span.traceId === traceID) : []
  return {
    ...data.summary,
    duplicateSpanRecords: ids.length - new Set(ids).size,
    nativeToolExecuteSpans: toolSpans.length,
    nativeToolSpansWithCallIDs: toolSpans.filter(span => attribute(span, 'gen_ai.tool.call.id')).length,
    nativeGenAIAttributeSpans: data.spans.filter(span => span.attributes?.some(item => item.key.startsWith('gen_ai.'))).length,
    nativeUsageAttributeSpans: data.spans.filter(span => span.attributes?.some(item => /token|usage/i.test(item.key))).length,
    propagation: { traceparent, matchingSpans: matching.length, directChildren: matching.filter(span => span.parentSpanId === parentID).length,
      sessionOrToolWork: matching.filter(span => /^(Session|Tool\.)/.test(span.name)).length },
    toolCounting: 'Native Tool.execute names alone do not establish call identity. Count wrapper tool.execution events by native call ID; use native spans only when a received call ID supports deduplication. Do not infer matches from timing or order.',
    metricsBoundary: 'No metric exporter is wired by the inspected V2 2.0.29 startup layer. Zero received metrics is not a zero-usage measurement.',
  }
}
