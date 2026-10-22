import { expect, it } from 'vitest'
import { parseGradingInput } from './grading-input'

it('separates grader instructions while preserving exact evidence numeric tokens', () => {
  const input =
    'Grade only the answer.\n\n' +
    '{"question":"Why?","criteria":[{"id":"cause","text":"Identify the cause.","scoring":{"Fail":"Wrong cause","Pass":"Supported cause"}}],"evidence":{"timestamp":1791360000950000001,"value":-0},"viewerSkill":"# Viewer"}'
  const packet = parseGradingInput(input)!
  expect(packet.instructions).toBe('Grade only the answer.')
  expect(packet.criteria[0].scoring).toEqual({
    Fail: 'Wrong cause',
    Pass: 'Supported cause',
  })
  expect(packet.evidence).toBe('{"timestamp":1791360000950000001,"value":-0}')
  expect(packet.viewerSkill).toBe('# Viewer')
})

it('leaves unrelated, malformed and truncated rubrics to the original-text view', () => {
  for (const input of [
    'Plain criterion',
    '{"question":"Why?"}',
    '{"question":"Why?","criteria":[{"text":"Cause","scoring":{"Pass":1}}]}',
    'Instructions\n{"question":"Why?","criteria":',
  ]) {
    expect(parseGradingInput(input)).toBeUndefined()
  }
})
