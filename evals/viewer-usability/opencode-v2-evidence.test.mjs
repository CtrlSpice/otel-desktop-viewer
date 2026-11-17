import assert from 'node:assert/strict'
import http from 'node:http'
import { once } from 'node:events'
import { test } from 'node:test'
import fixtures from './opencode-v2-fixtures.json' with { type: 'json' }
import { aggregateUsage, toolEvidence } from './opencode-v2-evidence.mjs'
import { openV2EventStream } from './opencode-v2-stream.mjs'
import { verifyToolEvent } from './opencode-v2-verify-evidence.mjs'

const makeTool = state => ({ type: 'tool', id: 'call_fixture', name: 'shell', messageID: 'msg_fixture', sessionID: 'ses_fixture',
  state: structuredClone(state), time: { created: 100, ran: 110, ...(['completed', 'error'].includes(state.status) ? { completed: 110 } : {}) } })
const otlpEvent = attributes => ({ name: 'tool.execution', attributes: Object.entries(attributes).map(([key, value]) => ({ key,
  value: typeof value === 'boolean' ? { boolValue: value } : typeof value === 'number' ? { doubleValue: value } : { stringValue: value },
})) })

for (const fixture of fixtures.tools) test(`tool evidence: ${fixture.name}`, () => {
  const tool = makeTool(fixture.state)
  const before = structuredClone(tool)
  const { attributes, errors, timestamp } = toolEvidence(tool)
  const projected = Object.fromEntries(Object.entries(attributes).filter(([key]) => ['tool.status', 'tool.input', 'tool.output', 'tool.is_error'].includes(key)))
  assert.deepEqual(projected, fixture.expected)
  assert.deepEqual(errors, fixture.malformed ? [{ toolID: tool.id, field: 'state.input', reason: 'Missing required V2 tool input' }] : [])
  assert.deepEqual(tool, before, 'Native records must not be changed')
  assert.deepEqual(JSON.parse(attributes['opencode.tool.raw']), tool)
  assert.equal(timestamp, tool.time.completed ?? tool.time.created)
  if (tool.time.completed !== undefined) assert.equal(attributes['tool.duration_ms'], 0, 'Recorded zero duration survives')
  else assert.equal(Object.hasOwn(attributes, 'tool.duration_ms'), false)
  verifyToolEvent(tool, otlpEvent(attributes))
})

for (const fixture of fixtures.usage) test(`usage: ${fixture.name}`, () => {
  const before = structuredClone(fixture.assistants)
  const result = aggregateUsage(fixture.assistants)
  assert.deepEqual(result.usage, fixture.usage)
  assert.deepEqual(result.tokenUsage, fixture.tokenUsage)
  assert.equal(result.usageAvailability.status, fixture.status)
  assert.equal(result.usageAvailability.assistants, fixture.assistants.length)
  for (const [field, count] of Object.entries(result.usageAvailability.recorded)) {
    const values = fixture.assistants.map(message => field.split('.').reduce((value, key) => value?.[key], message.tokens))
    assert.equal(count, values.filter(value => value !== undefined).length)
  }
  assert.match(result.usageAvailability.total, /^unavailable:/)
  assert.equal(Object.hasOwn(result.tokenUsage ?? {}, 'total'), false)
  assert.deepEqual(fixture.assistants, before)
})

test('verifier rejects invented incomplete outcome/input/output even when raw evidence matches', () => {
  const tool = makeTool({ status: 'running' })
  const { attributes } = toolEvidence(tool)
  for (const [key, value] of [['tool.is_error', false], ['tool.input', '{}'], ['tool.output', '{}'], ['tool.duration_ms', 0]]) {
    assert.throws(() => verifyToolEvent(tool, otlpEvent({ ...attributes, [key]: value })))
  }
})

test('verifier checks normalized identity, status, empty fields, duration and OTLP types', () => {
  const tool = makeTool({ status: 'completed', input: {}, content: [] })
  const { attributes } = toolEvidence(tool)
  for (const key of Object.keys(attributes)) {
    const altered = { ...attributes, [key]: 'incorrect' }
    assert.throws(() => verifyToolEvent(tool, otlpEvent(altered)), key)
    const missing = { ...attributes }; delete missing[key]
    assert.throws(() => verifyToolEvent(tool, otlpEvent(missing)), key)
  }
  const event = otlpEvent(attributes)
  event.attributes.push(event.attributes[0])
  assert.throws(() => verifyToolEvent(tool, event), /Duplicate/)
})

// Loopback only: no OpenCode executable, credentials, model, or observer backend.
// With the old stop-only signal, the watchdog ends the fetch and assertions fail.
for (const stage of ['headers', 'body']) for (const source of ['deadline', 'cancellation', 'stop']) {
  test(`SSE stalled ${stage}: ${source} reaches cleanup`, { timeout: 5000 }, async () => {
    const server = http.createServer((_request, response) => {
      if (stage === 'body') { response.writeHead(200, { 'Content-Type': 'text/event-stream' }); response.flushHeaders() }
    })
    const controllers = { deadline: new AbortController(), cancellation: new AbortController(), stop: new AbortController() }
    let watchdog, timer, reachedFinally = false, received = false
    try {
      server.listen(0, '127.0.0.1'); await once(server, 'listening')
      const request = once(server, 'request')
      const pending = openV2EventStream('http://127.0.0.1:' + server.address().port, {
        deadlineSignal: controllers.deadline.signal, cancellationSignal: controllers.cancellation.signal, stopSignal: controllers.stop.signal,
      })
      // Attach a rejection handler immediately, including when headers stall.
      const outcome = pending.then(response => stage === 'body' ? response.body.getReader().read() : response)
      const rejected = assert.rejects(outcome, error => error.message === source || (stage === 'body' && error.name === 'AbortError'))
      await request; received = true
      watchdog = setTimeout(() => controllers.stop.abort(new Error('watchdog')), 1000)
      timer = setTimeout(() => controllers[source].abort(new Error(source)), 20)
      try { await rejected } finally { reachedFinally = true }
      if (source !== 'stop') assert.equal(controllers.stop.signal.aborted, false, 'Deadline/cancellation must work before cleanup abort')
    } finally {
      clearTimeout(timer); clearTimeout(watchdog)
      controllers.stop.abort()
      const closed = once(server, 'close'); server.close(); server.closeAllConnections(); await closed
    }
    assert.equal(received, true, 'Request must reach a server that stalls')
    assert.equal(reachedFinally, true)
  })
}

test('SSE honors already-cancelled attempts before fetching', async () => {
  const cancellation = new AbortController()
  cancellation.abort(new Error('cancelled before request'))
  await assert.rejects(openV2EventStream('http://127.0.0.1:1', {
    deadlineSignal: new AbortController().signal, cancellationSignal: cancellation.signal, stopSignal: new AbortController().signal,
  }), /cancelled before request/)
})
