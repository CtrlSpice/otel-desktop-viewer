import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import assert from 'node:assert/strict'
import { once } from 'node:events'
import { randomBytes } from 'node:crypto'
import Provider from './opencode-v2-provider.mjs'
import { nativeTelemetryReceiver, readNativeTelemetry, nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'

if (process.argv.slice(2).some(arg => !['--error', '--telemetry', '--forward-observer'].includes(arg))) throw new Error('Usage: opencode-v2-canary.mjs [--error] [--telemetry [--forward-observer]]')
if (process.argv.includes('--forward-observer') && !process.argv.includes('--telemetry')) throw new Error('--forward-observer requires --telemetry')
process.umask(0o077)
const root = fs.mkdtempSync('/Users/moya/.local/share/otel-desktop-viewer/evals/opencode-v2-canary-')
const errorMode = process.argv.includes('--error')
const telemetry = process.argv.includes('--telemetry') ? await nativeTelemetryReceiver(root, process.argv.includes('--forward-observer') ? 'http://127.0.0.1:56010' : undefined) : undefined
const traceparent = `00-${randomBytes(16).toString('hex')}-${randomBytes(8).toString('hex')}-01`
const requests = []
const server = http.createServer(async (req, res) => {
  let raw = ''; for await (const chunk of req) raw += chunk
  const body = JSON.parse(raw)
  requests.push({ route: req.url, body })
  fs.writeFileSync(path.join(root, 'requests.json'), JSON.stringify(requests, null, 2))
  if (errorMode) {
    res.writeHead(429, { 'Content-Type': 'application/json' }).end(JSON.stringify({ error: { type: 'rate_limit_error', message: 'Synthetic V2 error detail must survive intact' } }))
    return
  }
  const shell = body.tools?.find(tool => tool.name === 'shell')
  const read = body.tools?.find(tool => tool.name === 'read')
  let tool
  if (requests.length === 1 && shell) tool = { name: 'shell', arguments: JSON.stringify({ command: './bin/otel-desktop-viewer --help', description: 'Synthetic viewer discovery check' }) }
  if (requests.length === 2 && read) tool = { name: 'read', arguments: JSON.stringify({ path: '/Users/moya/.local/share/otel-desktop-viewer/evals/native-otel-six-gLWzRd/promptfoo.json' }) }
  const item = tool
    ? { id: 'fc_fixture_' + requests.length, call_id: 'call_fixture_' + requests.length, type: 'function_call', ...tool, status: 'completed' }
    : { id: 'msg_fixture_' + requests.length, type: 'message', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: 'Synthetic V2 complete.', annotations: [] }] }
  const response = { id: 'resp_' + requests.length, object: 'response', created_at: Math.floor(Date.now() / 1000), model: 'gpt-6.1-sol', status: 'completed', output: [item],
    usage: { input_tokens: 20, output_tokens: 10, total_tokens: 30, input_tokens_details: { cached_tokens: 0 }, output_tokens_details: { reasoning_tokens: 0 } } }
  if (!body.stream) { res.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify(response)); return }
  res.writeHead(200, { 'Content-Type': 'text/event-stream' })
  let sequence = 0
  const emit = (type, data) => res.write('event: ' + type + '\ndata: ' + JSON.stringify({ type, sequence_number: sequence++, ...data }) + '\n\n')
  emit('response.created', { response: { ...response, status: 'in_progress', output: [] } })
  emit('response.output_item.added', { output_index: 0, item: { ...item, status: 'in_progress', ...(tool ? { arguments: '' } : { content: [] }) } })
  if (tool) {
    emit('response.function_call_arguments.delta', { item_id: item.id, output_index: 0, delta: item.arguments })
    emit('response.function_call_arguments.done', { item_id: item.id, output_index: 0, arguments: item.arguments })
  } else {
    const part = item.content[0]
    emit('response.content_part.added', { item_id: item.id, output_index: 0, content_index: 0, part: { ...part, text: '' } })
    emit('response.output_text.delta', { item_id: item.id, output_index: 0, content_index: 0, delta: part.text })
    emit('response.output_text.done', { item_id: item.id, output_index: 0, content_index: 0, text: part.text })
    emit('response.content_part.done', { item_id: item.id, output_index: 0, content_index: 0, part })
  }
  emit('response.output_item.done', { output_index: 0, item })
  emit('response.completed', { response })
  res.end()
})
server.listen(0, '127.0.0.1'); await once(server, 'listening')
try {
  const provider = new Provider({ config: { root, model: 'gpt-6.1-sol', baseUrl: `http://127.0.0.1:${server.address().port}/v1`,
    binary: '/Users/moya/.local/share/otel-desktop-viewer/evals/native-otel-six-gLWzRd/otel-desktop-viewer', timeoutMs: 90000, telemetryEndpoint: telemetry?.endpoint } })
  const response = await provider.callApi('Synthetic local-only V2 canary. Follow the scripted tool responses and finish.', { vars: { task_id: 'checkout-failure' }, traceparent })
  fs.writeFileSync(path.join(root, 'response.json'), JSON.stringify(response, null, 2))
  console.log(JSON.stringify({ root, error: response.error, toolCalls: response.metadata?.toolCalls, failedToolCalls: response.metadata?.failedToolCalls, requests: requests.length }))
  if (errorMode) {
    assert.ok(response.error?.includes('Synthetic V2 error detail must survive intact'))
    assert.equal(requests.length, 1, 'V2 must not retry a failed model request')
  } else {
    assert.equal(response.error, undefined)
    assert.equal(response.output, 'Synthetic V2 complete.')
    assert.equal(response.metadata.toolCalls, 2)
    assert.equal(response.metadata.failedToolCalls, 1)
    assert.equal(requests.length, 3)
    const tools = JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'tools.json')))
    assert.equal(tools[0].state.status, 'completed')
    assert.equal(tools[1].state.status, 'error')
    assert.ok(JSON.stringify(tools[0]).includes('USAGE'))
    assert.equal(tools[0].state.metadata.exit, 0)
    assert.ok(!JSON.stringify(tools[1]).includes('evalId'), 'Protected source content must not reach the model')
  }
} finally {
  server.closeAllConnections()
  await new Promise(resolve => server.close(resolve))
  if (telemetry) {
    await telemetry.close()
    const data = readNativeTelemetry(root, telemetry.receipts)
    const propagated = data.spans.filter(span => span.traceId === traceparent.split('-')[1])
    fs.writeFileSync(path.join(root, 'native-telemetry-summary.json'), JSON.stringify(nativeTelemetryBoundaries(data, traceparent), null, 2) + '\n', { flag: 'wx', mode: 0o600 })
    console.log(JSON.stringify({ root, nativeSpans: data.spans.length, nativeLogs: data.logs.length, nativeMetrics: data.metrics.length, propagatedSpans: propagated.length, scopes: data.summary.scopes }))
    assert.ok(data.spans.length, 'Native tracing must produce actual spans')
    assert.ok(data.logs.length, 'Native logging must produce actual logs')
    assert.ok(telemetry.receipts.every(receipt => receipt.status === 200 && !Object.values(JSON.parse(receipt.reply).partialSuccess ?? {}).some(value => value && value !== '0')), 'Native OTLP must be accepted without partial rejection')
  }
}
