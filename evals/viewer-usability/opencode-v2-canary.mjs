import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import assert from 'node:assert/strict'
import { once } from 'node:events'
import { randomBytes } from 'node:crypto'
import { tmpdir } from 'node:os'
import Provider, { withSharedHost } from './opencode-v2-provider.mjs'
import { checkConnections } from './opencode-v2-connect.mjs'
import { verifyNoRetry } from './opencode-v2-verify.mjs'
import { nativeTelemetryReceiver, readNativeTelemetry, nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'

const shared = process.argv.includes('--shared-network')
if (process.argv.slice(2).some(arg => !['--shared-network', '--error', '--telemetry', '--forward-observer'].includes(arg)) || shared && process.argv.length !== 3)
  throw new Error('Usage: opencode-v2-canary.mjs --shared-network | [--error] [--telemetry [--forward-observer]]')
if (process.argv.includes('--forward-observer') && !process.argv.includes('--telemetry')) throw new Error('--forward-observer requires --telemetry')
process.umask(0o077)
const root = fs.realpathSync(fs.mkdtempSync(path.join(tmpdir(), 'opencode/opencode-v2-canary-')))
const binary = process.env.OTEL_EVAL_VIEWER_BINARY
assert.ok(binary && path.isAbsolute(binary), 'Set OTEL_EVAL_VIEWER_BINARY to the prebuilt viewer')
const protectedFile = path.join(root, 'protected.txt')
const protectedText = 'protected-canary-' + randomBytes(16).toString('hex')
fs.writeFileSync(protectedFile, protectedText)
const errorMode = process.argv.includes('--error')
const telemetry = process.argv.includes('--telemetry') ? await nativeTelemetryReceiver(root, process.argv.includes('--forward-observer') ? 'http://127.0.0.1:56010' : undefined) : undefined
const traceparent = `00-${randomBytes(16).toString('hex')}-${randomBytes(8).toString('hex')}-01`
const requests = []
const destinations = [], hits = [[], []]
const exportID = '1'.repeat(32)
const exportBytes = JSON.stringify({resourceSpans: []}) + '\n'
if (shared) {
  assert.ok(process.env.OPENCODE_MODELS_PATH, 'Set OPENCODE_MODELS_PATH to the benchmark catalogue')
  for (const routes of hits) {
    const listener = http.createServer((req, res) => {
      routes.push(req.url)
      res.setHeader('Content-Type', 'application/json')
      if (req.url === '/export/traces/' + exportID) { res.end(exportBytes); return }
      res.end(JSON.stringify(req.url === '/rpc' ? { jsonrpc: '2.0', id: 1,
        result: { columns: [{name: 'n', type: 'INTEGER'}], rows: [[1]], truncated: false } } : {}))
    })
    listener.listen(0, '127.0.0.1'); await once(listener, 'listening')
    destinations.push(listener)
  }
}
const origin = server => `http://127.0.0.1:${server.address().port}`
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
  assert.ok(!raw.includes(protectedText), 'Protected source content must not reach the model')
  let tool
  if (shared && requests.length <= 2 && shell) {
    const endpoint = origin(destinations[requests.length - 1])
    tool = { name: 'shell', arguments: JSON.stringify({ command: `./bin/otel-desktop-viewer query 'select 1' --endpoint ${endpoint} --json`,
      description: 'Check shared investigator viewer connection' }) }
  } else if (shared && requests.length === 3 && shell) {
    tool = { name: 'shell', arguments: JSON.stringify({ command: `curl --silent ${origin(destinations[0])}/llms.txt`,
      description: 'Read guidance from the assigned viewer' }) }
  } else if (shared && requests.length <= 7 && shell) {
    const [command, description] = [
      [`./bin/otel-desktop-viewer export trace ${exportID} --endpoint ${origin(destinations[0])} > handoff.json`, 'Save the fixture export'],
      [`./bin/otel-desktop-viewer skills < '${protectedFile}'`, 'Verify protected input is denied'],
      [`./bin/otel-desktop-viewer skills > '${protectedFile}'`, 'Verify protected output is denied'],
      ['./bin/otel-desktop-viewer skills > bin/canary-output', 'Verify the executable directory is read-only'],
    ][requests.length - 4]
    tool = { name: 'shell', arguments: JSON.stringify({ command, description }) }
  } else if (shared && requests.length <= 10 && shell) {
    const command = ['./bin/otel-desktop-viewer --help; ls', 'ls -la bin', 'ls ..'][requests.length - 8]
    tool = { name: 'shell', arguments: JSON.stringify({ command, description: 'Check workspace directory listing' }) }
  } else if (shared && requests.length <= 13 && shell) {
    const command = ['./bin/otel-desktop-viewer --help 2>&1 | head -50; ls -la . bin',
      './bin/otel-desktop-viewer --help | head -n 2', 'head -n 1 handoff.json'][requests.length - 11]
    tool = { name: 'shell', arguments: JSON.stringify({ command, description: 'Check help output through head' }) }
  } else if (shared && requests.length <= 20 && shell) {
    const command = [
      `./bin/otel-desktop-viewer query 'select 1' --endpoint ${origin(destinations[0])} --json | jq -c '.rows[]'`,
      './bin/otel-desktop-viewer --help | grep -iE "^usage$" | wc -l',
      'jq . handoff.json', 'grep -i spans handoff.json', 'wc -c handoff.json',
      'jq -f handoff.json', `jq 'include "handoff"; .'`,
    ][requests.length - 14]
    tool = { name: 'shell', arguments: JSON.stringify({ command, description: 'Check stdin-only viewer output filters' }) }
  } else if (!shared && requests.length === 1 && shell) {
    const attempts = fs.readdirSync(path.join(root, 'runs'))
    assert.equal(attempts.length, 1)
    fs.symlinkSync(protectedFile, path.join(root, 'runs', attempts[0], 'workspace/protected-link'))
    tool = { name: 'shell', arguments: JSON.stringify({ command: './bin/otel-desktop-viewer --help', description: 'Synthetic viewer discovery check' }) }
  }
  if (!shared && requests.length === 2 && read) tool = { name: 'read', arguments: JSON.stringify({ path: protectedFile }) }
  if (!shared && requests.length === 3 && read) tool = { name: 'read', arguments: JSON.stringify({ path: 'protected-link' }) }
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
  const provider = new Provider({ config: { root, model: 'gpt-6.1-sol', baseUrl: origin(server) + '/v1',
    ...(shared ? { sharedHost: true, fixtureEndpoint: origin(destinations[0]) } : {}),
    binary, timeoutMs: 90000, telemetryEndpoint: telemetry?.endpoint } })
  const investigate = () => provider.callApi('Synthetic local-only V2 canary. Follow the scripted tool responses and finish.', { vars: { task_id: 'checkout-failure' }, traceparent })
  const response = shared ? await checkConnections({ root, catalogue: process.env.OPENCODE_MODELS_PATH, runtime: root,
    refs: [], scriptedModelEndpoint: origin(server) + '/v1', evaluateOnHost: host => withSharedHost(host, investigate) }) : await investigate()
  fs.writeFileSync(path.join(root, 'response.json'), JSON.stringify(response, null, 2))
  console.log(JSON.stringify({ root, error: response.error, toolCalls: response.metadata?.toolCalls, failedToolCalls: response.metadata?.failedToolCalls, requests: requests.length }))
  assert.equal(Object.hasOwn(response, 'cost'), false, 'Cost reporting remains disabled')
  if (errorMode) {
    assert.ok(response.error?.includes('Synthetic V2 error detail must survive intact'))
    assert.equal(requests.length, 1, 'V2 must not retry a failed model request')
  } else if (shared) {
    assert.equal(response.error, undefined)
    const tools = JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'tools.json')))
    assert.equal(tools.length, 20)
    assert.equal(tools[0].state.metadata.exit, 0)
    assert.ok(hits[0].includes('/rpc'))
    assert.ok(tools[1].state.metadata?.exit > 0)
    assert.match(tools[1].state.content.map(item => item.text).join(''), /connect: operation not permitted/i)
    assert.deepEqual(hits[1], [])
    assert.equal(tools[2].state.metadata.exit, 0)
    assert.ok(hits[0].includes('/llms.txt'))
    assert.equal(tools[3].state.metadata.exit, 0)
    assert.equal(fs.readFileSync(path.join(response.metadata.workspace, 'handoff.json'), 'utf8'), exportBytes)
    for (const tool of tools.slice(4, 7)) {
      assert.ok(tool.state.metadata?.exit > 0)
      assert.match(tool.state.content.map(item => item.text).join(''), /operation not permitted/i)
    }
    assert.equal(fs.readFileSync(protectedFile, 'utf8'), protectedText)
    assert.equal(fs.existsSync(path.join(response.metadata.workspace, 'bin/canary-output')), false)
    assert.equal(tools[7].state.metadata.exit, 0)
    const chained = tools[7].state.content.map(item => item.text).join('')
    assert.match(chained, /USAGE/)
    assert.match(chained, /handoff\.json/)
    assert.equal(tools[8].state.metadata.exit, 0)
    assert.match(tools[8].state.content.map(item => item.text).join(''), /otel-desktop-viewer/)
    assert.ok(tools[9].state.metadata.exit > 0)
    assert.match(tools[9].state.content.map(item => item.text).join(''), /workspace boundary/)
    const output = index => tools[index].state.content.map(item => item.text).join('').trim()
    assert.equal(tools[10].state.metadata.exit, 0)
    assert.match(output(10), /USAGE/)
    assert.match(output(10), /handoff\.json/)
    assert.equal(tools[11].state.metadata.exit, 0)
    assert.equal(tools[11].state.content.map(item => item.text).join(''), 'otel-desktop-viewer\n\n')
    assert.equal(tools[12].state.metadata.exit, 2)
    assert.equal(tools[13].state.metadata.exit, 0)
    assert.equal(output(13), '[1]')
    assert.equal(tools[14].state.metadata.exit, 0)
    assert.equal(output(14), '1')
    for (const tool of tools.slice(15)) assert.equal(tool.state.metadata.exit, 2)
    assert.equal(requests.length, 21)
    const { receipts } = JSON.parse(fs.readFileSync(path.join(root, 'native-otlp/summary.json')))
    assert.ok(receipts.some(receipt => receipt.route === '/v1/traces' && receipt.status === 200))
  } else {
    assert.equal(response.error, undefined)
    assert.equal(response.output, 'Synthetic V2 complete.')
    assert.equal(response.metadata.toolCalls, 3)
    assert.equal(response.metadata.failedToolCalls, 2)
    assert.equal(requests.length, 4)
    const tools = JSON.parse(fs.readFileSync(path.join(response.metadata.evidence, 'tools.json')))
    assert.equal(tools[0].state.status, 'completed')
    assert.equal(tools[1].state.status, 'error')
    assert.equal(tools[2].state.status, 'error')
    assert.ok(JSON.stringify(tools[0]).includes('USAGE'))
    assert.equal(tools[0].state.metadata.exit, 0)
    assert.ok(!JSON.stringify(tools).includes(protectedText), 'Protected source content must not reach the model')
  }
} finally {
  for (const listener of destinations) { listener.closeAllConnections(); await new Promise(resolve => listener.close(resolve)) }
  server.closeAllConnections()
  await new Promise(resolve => server.close(resolve))
  if (telemetry) {
    await telemetry.close()
    const data = readNativeTelemetry(root, telemetry.receipts)
    if (errorMode) {
      const { metadata } = JSON.parse(fs.readFileSync(path.join(root, 'response.json')))
      const policy = fs.readFileSync(path.join(metadata.evidence, 'policy.jsonl'), 'utf8').trim().split('\n').map(JSON.parse)
      assert.ok(policy.some(event => event.type === 'retry.veto'))
      verifyNoRetry(policy, metadata.sessionID, data.spans)
    }
    const propagated = data.spans.filter(span => span.traceId === traceparent.split('-')[1])
    fs.writeFileSync(path.join(root, 'native-telemetry-summary.json'), JSON.stringify(nativeTelemetryBoundaries(data, traceparent), null, 2) + '\n', { flag: 'wx', mode: 0o600 })
    console.log(JSON.stringify({ root, nativeSpans: data.spans.length, nativeLogs: data.logs.length, nativeMetrics: data.metrics.length, propagatedSpans: propagated.length, scopes: data.summary.scopes }))
    assert.ok(data.spans.length, 'Native tracing must produce actual spans')
    assert.ok(data.logs.length, 'Native logging must produce actual logs')
    assert.ok(telemetry.receipts.every(receipt => receipt.status === 200 && !Object.values(JSON.parse(receipt.reply).partialSuccess ?? {}).some(value => value && value !== '0')), 'Native OTLP must be accepted without partial rejection')
  }
}
