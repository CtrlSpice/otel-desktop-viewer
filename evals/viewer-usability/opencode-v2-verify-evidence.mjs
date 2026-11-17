import assert from 'node:assert/strict'

// Check normalized attributes independently of the producer projection. Equality
// of opencode.tool.raw alone cannot detect invented success or empty output.
export function verifyToolEvent(tool, event) {
  assert.equal(event.eventName ?? event.name, 'tool.execution')
  if (event.eventName) {
    assert.equal(event.timeUnixNano, String(BigInt(tool.time.completed ?? tool.time.created) * 1000000n))
    assert.ok(BigInt(event.observedTimeUnixNano) > 0n)
  }
  const attributes = new Map(event.attributes.map(attribute => [attribute.key, attribute.value]))
  assert.equal(attributes.size, event.attributes.length, 'Duplicate tool-event attributes')
  const text = (key, value) => assert.deepEqual(attributes.get(key), { stringValue: value }, key)
  text('gen_ai.tool.name', tool.name)
  text('gen_ai.tool.call.id', tool.id)
  text('opencode.session.id', tool.sessionID)
  text('opencode.message.id', tool.messageID)
  text('opencode.runtime.version', '2.0.29')
  text('opencode.tool.raw', JSON.stringify(tool))
  text('tool.status', tool.state.status)
  if (Object.hasOwn(tool.state, 'input')) text('tool.input', JSON.stringify(tool.state.input))
  else assert.equal(attributes.has('tool.input'), false, 'Invented tool input')
  if (Object.hasOwn(tool.state, 'content')) text('tool.output', JSON.stringify(tool.state.content))
  else if (Object.hasOwn(tool.state, 'error')) text('tool.output', JSON.stringify(tool.state.error))
  else assert.equal(attributes.has('tool.output'), false, 'Invented tool output')
  if (tool.state.status === 'completed' || tool.state.status === 'error') {
    assert.deepEqual(attributes.get('tool.is_error'), { boolValue: tool.state.status === 'error' }, 'tool.is_error')
  } else assert.equal(attributes.has('tool.is_error'), false, 'Incomplete tool has no recorded outcome')
  if (tool.time.completed !== undefined && tool.time.ran !== undefined) {
    const duration = attributes.get('tool.duration_ms')
    assert.ok(duration && Object.keys(duration).length === 1 && ('intValue' in duration || 'doubleValue' in duration), 'Numeric duration required')
    assert.equal(Number(duration.intValue ?? duration.doubleValue), tool.time.completed - tool.time.ran, 'tool.duration_ms')
  } else assert.equal(attributes.has('tool.duration_ms'), false, 'Invented tool duration')
  text('opencode.event.timestamp.source', tool.time.completed === undefined ? 'tool.time.created' : 'tool.time.completed')
}
