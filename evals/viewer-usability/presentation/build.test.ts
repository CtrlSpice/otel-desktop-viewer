import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { build } from './build.ts'
import { prepareReview, recordReview, signoffReview } from '../human-review.ts'
import { investigatorEvidence } from '../evidence.ts'
import { saveJson, TEMPORARY } from '../runtime.ts'
import tasks from '../tasks.json' with { type: 'json' }
import rubrics from '../rubrics.json' with { type: 'json' }

test('build consumes validated human review without exposing private audit data', async () => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(TEMPORARY, 'presentation-review-test-')))
  const run = path.join(root, 'run'), review = path.join(root, 'review')
  const evidence = path.join(run, 'runs/investigator-test')
  const graderEvidence = path.join(run, 'runs/grader-test')
  const scratch = path.join(run, 'workspaces/clean-context-test/scratch')
  for (const directory of [evidence, graderEvidence, scratch])
    fs.mkdirSync(directory, { recursive: true, mode: 0o700 })
  const answer = 'The payment was declined. PRIVATE_REVIEW_CANARY'
  const metadata = {
    model: 'synthetic/model', role: 'investigator', task: 'checkout-failure',
    evidence, sessionIDs: ['ses_test'], workspace: scratch, archivedWorkspace: scratch,
    isolationRoot: path.dirname(scratch), toolCalls: 0, exit: { code: 0, timedOut: false },
  }
  fs.writeFileSync(path.join(evidence, 'answer.txt'), answer, { mode: 0o600 })
  fs.writeFileSync(path.join(evidence, 'stdout.jsonl'), '', { mode: 0o600 })
  fs.writeFileSync(path.join(evidence, 'prompt.txt'), 'Synthetic question', { mode: 0o600 })
  saveJson(path.join(evidence, 'commands.json'), [])
  saveJson(path.join(evidence, 'metrics.json'), metadata)
  const packet = investigatorEvidence(metadata, 'checkout-failure', answer, run)
  const grader = { model: 'synthetic/grader', role: 'grader', task: 'checkout-failure', evidence: graderEvidence,
    sessionIDs: ['ses_grader'], exit: { code: 0, timedOut: false } }
  const report = { provisional: true, role: 'grader', taskID: 'checkout-failure',
    metadata: grader, investigator: metadata, investigatorEvidence: packet,
    status: 'pass', score: 1, criteria: [{ id: 'reported-cause', verdict: 'Pass', score: 1,
      explanation: 'PRIVATE_REVIEW_CANARY' }] }
  saveJson(path.join(graderEvidence, 'metrics.json'), grader)
  saveJson(path.join(graderEvidence, 'grading.json'), report)
  const rubric = rubrics['checkout-failure']
  fs.writeFileSync(path.join(graderEvidence, 'prompt.txt'), JSON.stringify({
    question: tasks[0].goal, ...rubric, answer, evidence: packet,
  }), { mode: 0o600 })
  saveJson(path.join(run, 'run-manifest.json'), { models: ['synthetic/model'], tasks: 1, repetitions: 1 })
  saveJson(path.join(run, 'pilot.json'), { results: { results: [{
    provider: { label: 'synthetic/model' }, vars: { task_id: 'checkout-failure' },
    success: true, response: { output: answer, metadata }, gradingResult: { score: 1 },
  }] } })
  await prepareReview(run, 'runs/grader-test/grading.json', review)
  recordReview(review, { schemaVersion: 1, reviewer: 'PRIVATE_REVIEW_CANARY',
    completion: { action: 'correct', score: 0.5, reason: 'Incomplete cause.',
      evidence: [{ source: 'answer', quote: 'The payment was declined.' }] },
    claims: [], findings: [], toolReviews: [],
  })
  signoffReview(review, 1, 'PRIVATE_REVIEW_CANARY', true)
  const output = path.join(root, 'index.html')
  const data = build(run, output, [review])
  const task = data.models[0].tasks[0]
  assert.deepEqual(task.scores, [1])
  assert.deepEqual(task.signedScores, [0.5])
  assert.equal(task.signedAttempts, 1)
  assert.equal(task.finalMean, null)
  assert.equal(task.claimReviewedAttempts, 1)
  assert.ok(!JSON.stringify(data).includes('PRIVATE_REVIEW_CANARY'))
  const html = fs.readFileSync(output, 'utf8')
  assert.ok(!html.includes('PRIVATE_REVIEW_CANARY'))
  assert.ok(!html.includes(root))
  assert.ok(!html.includes('How we test'))
  assert.ok(!html.includes('Dataset: Unknown'))
  assert.throws(() => build(run, path.join(root, 'duplicate.html'), [review, review]), /Duplicate human review/)
  fs.appendFileSync(path.join(evidence, 'answer.txt'), 'changed')
  assert.throws(() => build(run, path.join(root, 'tampered.html'), [review]), /changed|mismatch/i)
  assert.equal(fs.existsSync(path.join(root, 'tampered.html')), false)
})
