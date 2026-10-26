import test from 'node:test'
import assert from 'node:assert/strict'
import { claudeEvidence } from './claude-provider.ts'

test('Claude tool counts deduplicate streamed message snapshots and match error results by call ID', () => {
  const message = { type: 'assistant', message: { model: 'claude-opus-5-5', content: [{ type: 'tool_use', id: 'one', name: 'Bash' }] } }
  const events = [message, message,
    { type: 'assistant', message: { model: '<synthetic>', content: [{ type: 'text', text: 'Not logged in' }] } },
    { type: 'assistant', message: { model: 'claude-opus-5-5', content: [{ type: 'tool_use', id: 'two', name: 'Read' }] } },
    { type: 'user', message: { content: [{ type: 'tool_result', tool_use_id: 'one', is_error: true }, { type: 'tool_result', tool_use_id: 'two' }] } },
    { type: 'user', message: { content: [{ type: 'tool_result', tool_use_id: 'one', is_error: true }] } },
  ]
  assert.deepEqual(claudeEvidence(events), { toolCalls: 2, failedToolCalls: 1, actualModels: ['claude-opus-5-5'] })
})
