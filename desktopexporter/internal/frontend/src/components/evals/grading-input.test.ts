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

it('ignores non-string optional fields and retains literal scoring labels and source evidence', () => {
  const packet = parseGradingInput(
    '{"question":"Why?","criteria":[{"id":42,"text":"Cause","scoring":{"__proto__":"literal label","":"empty label"}}],"reference":false,"viewerSkill":{},"evidence":{"x":1e3,"x":-0,"large":9007199254740993},"artifact":"literal\\\\n"}'
  )!
  expect(packet.criteria[0].id).toBeUndefined()
  expect(packet.reference).toBeUndefined()
  expect(packet.viewerSkill).toBeUndefined()
  expect(Object.keys(packet.criteria[0].scoring)).toEqual(['__proto__', ''])
  expect(
    Object.getOwnPropertyDescriptor(packet.criteria[0].scoring, '__proto__')
      ?.value
  ).toBe('literal label')
  expect(packet.evidence).toBe('{"x":1e3,"x":-0,"large":9007199254740993}')
  expect(packet.artifact).toBe('literal\\n')
})
