import assert from 'node:assert/strict'
import test from 'node:test'
import { groupGradingOutputs, gradingScoreTotals, gradingStatus } from './grading-output.ts'
import type { Presentation } from './aggregate.ts'

const output = (model: string, task: string, score: number | null): Presentation['grader']['outputs'][number] => ({
  graderModel: 'synthetic/grader', investigatorModel: model, task, score, executionError: false, telemetry: null,
  seconds: 2, costCad: 0.25, estimateCoverage: 'complete', inputTokens: 10, outputTokens: 2,
})

test('flat question rows retain each original grade and sum only their grading work', () => {
  const grades = [output('synthetic/b', 'second', 1), output('synthetic/a', 'first', 1), output('synthetic/a', 'first', 0.5), output('synthetic/a', 'first', 0)]
  const before = structuredClone(grades)
  const groups = groupGradingOutputs(grades, ['synthetic/a', 'synthetic/b', 'synthetic/not-started'], ['first', 'second'])
  assert.deepEqual(groups.map(group => group.model), ['synthetic/a', 'synthetic/b'])
  assert.equal(groups[0].questions.length, 1)
  const question = groups[0].questions[0]
  assert.deepEqual(question.grades.map(grade => grade.score), [1, 0.5, 0])
  assert.equal(question.totalSeconds, 6)
  assert.equal(question.costCad, 0.75)
  assert.equal(question.estimateCoverage, 'complete')
  assert.equal(question.inputTokens, 30)
  assert.equal(question.outputTokens, 6)
  assert.equal(groups[0].totals.totalSeconds, 6)
  assert.equal(groups[0].totals.costCad, 0.75)
  assert.deepEqual(grades, before)
})

test('score totals sum points per score column, never average or turn missing/error grades into zero', () => {
  const questions = Array.from({ length: 11 }, (_, index) => ({ grades: [output('synthetic/a', `question-${index}`, index === 0 ? 0.5 : 1)] }))
  questions[0].grades.push({ ...output('synthetic/a', 'question-0', 1), executionError: true })
  assert.deepEqual(gradingScoreTotals(questions, 3), [
    { earned: 10.5, graded: 11, errors: 0 },
    { earned: null, graded: 0, errors: 1 },
    { earned: null, graded: 0, errors: 0 },
  ])
  assert.deepEqual(gradingScoreTotals([{ grades: [output('synthetic/a', 'zero', 0)] }], 1), [{ earned: 0, graded: 1, errors: 0 }])
})

test('token totals stay unknown rather than lose integer precision', () => {
  const grades = [Number.MAX_SAFE_INTEGER, 1].map(tokens => ({ ...output('synthetic/a', 'first', 1), inputTokens: tokens, outputTokens: tokens }))
  const [group] = groupGradingOutputs(grades, [], [])
  assert.equal(group.totals.inputTokens, null)
  assert.equal(group.totals.outputTokens, null)
  assert.equal(group.questions[0].inputTokens, null)
  assert.equal(group.questions[0].outputTokens, null)
})

test('status distinguishes pending grading from execution errors and completed zero scores', () => {
  assert.equal(gradingStatus(0, 3, 0, 0), 'not-started')
  assert.equal(gradingStatus(0, 3, 1, 0), 'in-progress')
  assert.equal(gradingStatus(1, 3, 1, 0), 'in-progress')
  assert.equal(gradingStatus(3, 3, 3, 0), 'done')
  assert.equal(gradingStatus(3, 3, 3, 1), 'needs-attention')
  assert.equal(gradingStatus(0, 3, 1, 1), 'needs-attention')
  assert.equal(gradingStatus(1, null, 1, 0), 'in-progress')
})

test('not-started models and questions can be shown without inventing grades or costs', () => {
  const groups = groupGradingOutputs([output('synthetic/a', 'first', 1)], ['synthetic/a', 'synthetic/b'], ['first', 'second'], true)
  assert.equal(groups.length, 2)
  assert.equal(groups[1].questions.length, 2)
  assert.deepEqual(groups[1].questions[0].grades, [])
  assert.equal(groups[1].totals.totalSeconds, 0)
  assert.equal(groups[1].totals.costCad, null)
  assert.equal(groups[1].totals.estimateCoverage, 'unavailable')
  assert.deepEqual(gradingScoreTotals(groups[1].questions, 3).map(score => score.earned), [null, null, null])
})

test('missing timing and tokens stay unknown, incomplete costs stay partial, errors stay unscored', () => {
  const groups = groupGradingOutputs([
    output('synthetic/a', 'first', 1),
    { ...output('synthetic/a', 'first', null), executionError: true, seconds: null, inputTokens: null, costCad: null, estimateCoverage: 'unavailable' },
    { ...output('synthetic/a', 'second', null), seconds: null, costCad: null, estimateCoverage: 'unavailable' },
  ], [], [])
  const question = groups[0].questions[0]
  assert.equal(question.totalSeconds, null)
  assert.equal(question.inputTokens, null)
  assert.equal(question.costCad, 0.25)
  assert.equal(question.estimateCoverage, 'partial')
  assert.equal(question.grades[1].executionError, true)
  assert.equal(question.grades[1].score, null)
  assert.equal(groups[0].questions[1].costCad, null)
  assert.equal(groups[0].questions[1].estimateCoverage, 'unavailable')
})
