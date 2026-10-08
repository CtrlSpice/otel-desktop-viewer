import test from 'node:test'
import assert from 'node:assert/strict'
import grade, { matches } from './grade.ts'
import cases from './tasks.json' with { type: 'json' }
test('known answers pass and every incorrect top-level fact fails', () => {
  for (const task of cases) {
    const context = { vars: { task_id: task.id } }
    assert.equal(
      grade(
        JSON.stringify({ ...task.expected, evidence: ['viewer response'] }),
        context
      ).pass,
      true
    )
    for (const key of Object.keys(task.expected)) {
      assert.equal(
        grade(
          JSON.stringify({
            ...task.expected,
            [key]: null,
            evidence: ['viewer response'],
          }),
          context
        ).pass,
        false,
        task.id + '/' + key
      )
    }
  }
})
test('typed integers cannot be flattened or rounded', () => {
  assert.equal(
    matches(
      { kind: 'string', value: '9007199254740993' },
      { kind: 'int64', value: '9007199254740993' }
    ),
    false
  )
  assert.equal(matches('9007199254740992', '9007199254740993'), false)
})
test('array order does not matter but duplicate rows do', () => {
  assert.equal(matches([2, 1], [1, 2]), true)
  assert.equal(matches([1, 1], [1, 2]), false)
})
test('plain/fenced JSON allowed, prose-only and missing evidence fail', () => {
  const context = { vars: { task_id: cases[0].id } }
  const answer = { ...cases[0].expected, evidence: 'checked' }
  assert.equal(
    grade('```json\n' + JSON.stringify(answer) + '\n```', context).pass,
    true
  )
  assert.equal(grade('I think there are some spans.', context).pass, false)
  assert.equal(grade(JSON.stringify(cases[0].expected), context).pass, false)
})
