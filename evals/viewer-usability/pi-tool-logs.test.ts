import test from 'node:test'
import assert from 'node:assert/strict'
import { PiToolLogs } from './pi-tool-logs.ts'

function collected(records: Record<string, unknown>[]) {
  const collector = new PiToolLogs()
  records.forEach((record, index) => collector.observe(record, 1000 + index))
  const logs = collector.drain()
  assert.deepEqual(collector.drain(), [])
  return logs.map(log => [log.eventName, log.attributes, log.timestamp, log.observedTimestamp] as const)
}
const start = { type: 'tool_execution_start', toolCallId: 'call-1', toolName: 'bash', args: { command: 'printf exact' } }
const end = { type: 'tool_execution_end', toolCallId: 'call-1', toolName: 'bash', result: { content: [{ type: 'text', text: 'exact\n' }] }, durationMs: 23, isError: false }
const message = { type: 'message_end', message: { role: 'toolResult', toolCallId: 'call-1', toolName: 'bash', durationMs: 23, isError: false, timestamp: 999 } }

test('one log event retains exact source records, source and observed time without a derived start', () => {
  const events = collected([start, end, message])
  assert.equal(events.length, 1)
  const [name, attributes, timestamp, observed] = events[0]
  assert.equal(name, 'tool.execution')
  assert.equal(timestamp, 999)
  assert.equal(observed, 1001)
  assert.equal(attributes['pi.event.timestamp.source'], 'toolResult.timestamp')
  assert.equal(attributes['tool.duration_ms'], 23)
  assert.equal(attributes['tool.is_error'], false)
  assert.equal(attributes['tool.input'], JSON.stringify(start.args))
  assert.equal(attributes['tool.output'], JSON.stringify(end.result))
  assert.equal(attributes['pi.event.raw'], JSON.stringify(end))
  assert.equal(attributes['pi.tool.start.raw'], JSON.stringify(start))
  assert.equal(attributes['pi.tool.result.raw'], JSON.stringify(message))
  assert.equal(attributes['gen_ai.operation.name'], undefined)
})

test('partial calls retain observed time without inventing completion or an error', () => {
  const [[name, attributes, time]] = collected([start])
  assert.equal(name, 'tool.execution.start')
  assert.equal(time, 1000)
  assert.equal(attributes['pi.event.timestamp.source'], 'collector.observed')
  assert.equal(attributes['tool.is_error'], undefined)
  assert.equal(attributes['tool.duration_ms'], undefined)
})

test('ambiguous IDs and conflicting names never borrow input or message time', () => {
  for (const records of [[start, start, end, message, message], [start, end, end, message],
    [{ ...start, toolName: 'read' }, end, { ...message, message: { ...message.message, toolName: 'read' } }]]) {
    const events = collected(records)
    for (const [name, attributes] of events.filter(event => event[0] === 'tool.execution')) {
      assert.equal(name, 'tool.execution')
      assert.equal(attributes['tool.input'], undefined)
      assert.equal(attributes['pi.event.timestamp.source'], 'collector.observed')
    }
    assert.ok(events.some(event => event[0] === 'tool.execution.start'))
  }
})

test('failed tools remain failed', () => {
  const [[, attributes]] = collected([{ ...end, isError: true }])
  assert.equal(attributes['tool.is_error'], true)
})
