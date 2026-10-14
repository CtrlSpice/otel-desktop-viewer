import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { aggregate } from './aggregate.ts'
import { render } from './render.ts'
import { modelSummaries, median } from '../model-summary.ts'
import { pricingSnapshot } from '../pricing.ts'
import goPrices from '../go-api-prices.json' with { type: 'json' }

const secret = 'PRIVATE_SENTINEL_DO_NOT_EMBED'
const metadata = (model: string, seconds = 2) => ({
  model,
  elapsedSeconds: seconds,
  toolCalls: 3,
  failedToolCalls: 1,
  usage: { input: 20, output: 3, reasoning: 2, cacheRead: 4, cacheWrite: 0 },
  usageSteps: [
    { input: 20, output: 3, reasoning: 2, cacheRead: 4, cacheWrite: 0 },
  ],
  workspace: `/Users/private/${secret}`,
  evidence: secret,
  sessionIDs: [secret],
})
let fixtureSession = 0
const row = (model: string, task: string, success: boolean) => ({
  provider: { label: model },
  vars: { task_id: task, goal: secret, binary: secret },
  success,
  response: { output: secret, metadata: { ...metadata(model), sessionIDs: [`fixture_${fixtureSession++}`] } },
  gradingResult: {
    score: success ? 1 : 0,
    reason: secret,
    componentResults: [
      {
        metadata: {
          criteria: [
            {
              id: secret,
              verdict: success ? 'Pass' : 'Fail',
              explanation: secret,
            },
          ],
          artifact: { pass: true, reason: secret, files: [secret] },
        },
      },
    ],
  },
  prompt: secret,
  testCase: { assert: secret },
})

test('different model set; counts retain failures, errors, unknowns and incomplete coverage', () => {
  const pass = row('other/cedar-v2', 'inspect', true)
  const fail = row('other/cedar-v2', 'inspect', false)
  const error = { ...row('other/cedar-v2', 'export', false), failureReason: 2 }
  const missing = {
    provider: { label: 'research/oak' },
    vars: { task_id: 'inspect' },
  }
  const data = aggregate(
    {
      models: ['other/cedar-v2', 'research/oak', 'new/maple'],
      tasks: 2,
      repetitions: 3,
    },
    { results: { results: [pass, fail, error, missing] } },
    undefined,
    undefined
  )
  assert.equal(data.models.length, 3)
  assert.equal(data.expectedAttempts, 18)
  assert.equal(data.attempts, 4)
  assert.deepEqual(
    data.models[0].tasks.map((task) => [
      task.task,
      task.attempts,
      task.pass,
      task.fail,
      task.error,
    ]),
    [
      ['inspect', 2, 1, 1, 0],
      ['export', 1, 0, 0, 1],
    ]
  )
  assert.equal(data.models[0].tasks[0].checks.pass, 1)
  assert.equal(data.models[0].tasks[0].checks.fail, 1)
  assert.equal(data.models[1].result.unknown, 1)
  assert.equal(data.models[1].result.medianSeconds, null)
  assert.equal(data.models[1].result.toolCalls, null)
  assert.equal(data.models[0].investigator.apiCostCad, null)
  assert.equal(data.models[2].result.attempts, 0)
  assert.equal(data.models[2].investigator.toolCalls, null)
  assert.equal(data.grader.sessions, null)
  assert.equal(data.provisional, true)
})

test('error and wrong answer stay distinct; missing duration makes median unknown', () => {
  const fail = row('other/cedar', 'inspect', false)
  const gradingError = { ...row('other/cedar', 'inspect', false), error: secret, failureReason: 1 }
  const providerError = { ...fail, response: { error: secret } }
  const unknown = { ...row('other/cedar', 'inspect', false), success: undefined }
  const data = aggregate(
    { tasks: 1, repetitions: 4 },
    { results: { results: [fail, gradingError, providerError, unknown] } },
    undefined,
    undefined
  )
  assert.deepEqual(
    [
      data.models[0].result.pass,
      data.models[0].result.fail,
      data.models[0].result.error,
      data.models[0].result.unknown,
    ],
    [0, 2, 1, 1]
  )
  assert.equal(data.models[0].result.medianSeconds, null)
  assert.equal(data.models[0].result.timedAttempts, 3)
})

test('projection drops private payload; labels cannot inject markup or copy paths', () => {
  const data = aggregate(
    {
      models: ['other/cedar'],
      dataset: `/private/${secret}/testdata/otlp/small`,
      binary: secret,
      isolationSettingsFile: secret,
    },
    { results: { results: [row('other/cedar', 'inspect', true)] } },
    {
      models: [
        {
          model: 'other/cedar',
          name: '<img src=x onerror=alert(1)>',
          attempts: 5,
          apiCostCad: 12,
          estimateCoverage: 'partial',
        },
      ],
    },
    {
      sessions: 7,
      executionErrors: 2,
      judgments: [secret],
      models: [
        {
          name: '/Users/private/grader',
          attempts: 7,
          apiCostCad: 3,
          estimateCoverage: 'complete',
        },
      ],
    }
  )
  const html = render(data)
  assert.equal(data.models[0].investigator.attempts, 5)
  assert.equal(data.models[0].investigator.apiCostCad, 12)
  assert.equal(data.models[0].result.attempts, 1)
  assert.equal(data.grader.models[0].apiCostCad, 3)
  assert.equal(data.grader.models[0].name, 'Separate grader')
  assert.ok(!JSON.stringify(data).includes(secret))
  assert.ok(!html.includes(secret))
  assert.ok(!html.includes('/Users/'))
  assert.ok(!html.includes('<img src=x'))
  assert.ok(html.includes('&lt;img src=x onerror=alert(1)&gt;'))
  assert.ok(html.includes('C$12.000 (partial)'))
  assert.ok(!html.includes('C$12.000 est.'))
  assert.ok(html.includes('Artifact check'))
  assert.equal(data.dataset, 'testdata/otlp/small')
  assert.throws(
    () =>
      aggregate(
        {},
        { results: { results: [row('/Users/private', 'inspect', true)] } },
        undefined,
        undefined
      ),
    /identifier/
  )
})

test('partial scores and signed three-run means stay separate; errors are unscored', () => {
  const attempts = [1, 0.5, 0].map((score, index) => ({
    ...row('other/cedar', 'inspect', score === 1),
    response: { metadata: { ...metadata('other/cedar'), sessionIDs: [`ses_${index}`] } },
    gradingResult: { score },
  }))
  const reviews = attempts.map((attempt, index) => ({
    model: 'other/cedar', task: 'inspect', sessionIDs: [`ses_${index}`],
    score: attempt.gradingResult.score, signedOff: true,
  }))
  const project = (selectedReviews = reviews, selected = attempts) => aggregate(
    { models: ['other/cedar'], tasks: 1, repetitions: 3 },
    { results: { results: selected } }, undefined, undefined, undefined, selectedReviews
  )
  const data = project()
  const task = data.models[0].tasks[0]
  assert.equal(task.partial, 1)
  assert.equal(task.provisionalMean, 0.5)
  assert.deepEqual(task.scores, [1, 0.5, 0])
  assert.equal(task.signedAttempts, 3)
  assert.equal(task.finalMean, 0.5)
  assert.equal(data.provisional, false)
  assert.equal(project(reviews.slice(0, 2)).models[0].tasks[0].finalMean, null)
  const amended = reviews.map((review, index) => ({ ...review, signedOff: index !== 2 }))
  assert.equal(project(amended).models[0].tasks[0].signedAttempts, 2)
  assert.equal(project(amended).models[0].tasks[0].finalMean, null)
  const error = { ...attempts[2], failureReason: 2 }
  const errored = aggregate({}, { results: { results: [error] } }, undefined, undefined, undefined, reviews)
  assert.equal(errored.models[0].tasks[0].provisionalMean, null)
  assert.equal(errored.models[0].tasks[0].signedAttempts, 0)
  const html = render(data)
  assert.match(html, /<dt>Mean<\/dt>/)
  assert.ok(!html.includes('Provisional mean'))
  assert.ok(!html.includes('Final three-run mean'))
  assert.ok(html.includes('class="task-group"'))
  assert.ok(!html.includes('<h4>'))
  assert.match(html, /50.0%/)
  assert.match(html, /Run 1/)
  assert.match(html, /Run 2/)
  assert.match(html, /Run 3/)
  assert.match(html, /<td><span class="badge badge-soft result-badge result-partial">0.5\/1 · signed<\/span><\/td>/)
  assert.ok(!JSON.stringify(data).includes('ses_'))
  assert.ok(!html.includes('ses_'))
  assert.ok(!html.includes('Artifact check'))
  assert.ok(!html.includes('Unknown / not recorded'))
  assert.throws(() => project([...reviews, reviews[0]]), /Duplicate human review/)
  assert.throws(() => project(reviews, [attempts[0], attempts[0], attempts[0]]), /duplicate investigator session/)
  const corrected = render(project(reviews.map(review => ({ ...review, score: 0 }))))
  assert.match(corrected, /<span class="pass" style="flex:0"/)
  assert.match(corrected, /<span class="fail" style="flex:3"/)
  assert.match(corrected, /<strong>0.0%<\/strong>/)
  const unsigned = render(project([]))
  for (const label of ['Human-signed scores', 'Human claim review', 'Signoff coverage', 'Final three-run mean'])
    assert.ok(!unsigned.includes(label))
})

test('signed single-run corrections update the displayed model mean as well as the task result', () => {
  const attempt = row('other/cedar', 'inspect', true)
  const data = aggregate({ tasks: 1, repetitions: 1 }, { results: { results: [attempt] } },
    undefined, undefined, undefined, [{ model: 'other/cedar', task: 'inspect',
      sessionIDs: attempt.response.metadata.sessionIDs, signedOff: true, score: 0.5 }])
  assert.equal(data.models[0].result.provisionalMean, 1)
  assert.equal(data.models[0].result.displayMean, 0.5)
  const html = render(data)
  assert.match(html, /<strong>50.0%<\/strong>/)
  assert.match(html, /0.5\/1/)
  assert.match(html, /Automated result/)
  assert.doesNotMatch(html, /<strong>100.0%<\/strong>/)
})

test('grader outputs expose only aggregate fields, not private judging data', () => {
  const data = aggregate({ tasks: 1, repetitions: 1 }, { results: { results: [row('other/cedar', 'checkout-failure', true)] } }, undefined,
    { sessions: 1, executionErrors: 0, models: [{ model: 'openai/gpt-6.1-sol', name: 'Sol 6.1', attempts: 1 }], judgments: [{
      taskID: 'checkout-failure', score: 0.5, status: 'partial',
      investigator: { model: 'other/cedar', evidence: secret },
      metadata: { ...metadata('openai/gpt-6.1-sol'), sessionIDs: [secret] },
      reason: secret, criteria: [{ explanation: secret }],
    }] })
  assert.equal(data.grader.outputs.length, 1)
  assert.equal(data.grader.outputs[0].score, 0.5)
  assert.ok(!JSON.stringify(data).includes(secret))
  const html = render(data)
  assert.ok(html.includes('grader-row'))
  assert.ok(html.includes('<strong>Grader</strong><span>Sol 6.1</span>'))
  assert.ok(html.lastIndexOf('class="model grader-row"') > html.lastIndexOf('class="model"'))
  assert.ok(html.includes('Grading outputs'))
  assert.ok(html.includes('aria-label="Automated grades: 0 × 1/1 · 1 × 0.5/1 · 0 × 0/1"'))
  assert.ok(!html.includes('<strong>1 grades</strong>'))
  assert.ok(html.includes('<span class="badge badge-soft result-badge result-partial">0.5/1</span>'))
  assert.ok(!html.includes(secret))
})

test('grader breakdown counts original grades and keeps execution errors and missing outputs separate', () => {
  const investigator = 'other/cedar', grader = 'openai/gpt-6.1-sol'
  const judgments = [{ score: 1 }, { score: 0.5 }, { score: 0 }, { score: 1, status: 'execution-error' }, { score: null }]
    .map(result => ({ ...result, taskID: 'inspect', investigator: { model: investigator }, metadata: metadata(grader) }))
  const data = aggregate({ tasks: 1, repetitions: 1 }, { results: { results: [row(investigator, 'inspect', true)] } }, undefined,
    { models: [{ model: grader, name: 'Sol 6.1', attempts: 6 }], judgments })
  const html = render(data)
  assert.ok(html.includes('aria-label="Automated grades: 1 × 1/1 · 1 × 0.5/1 · 1 × 0/1"'))
  assert.ok(html.includes('<span>1 execution error</span>'))
  assert.ok(html.includes('<span>1 unscored</span>'))
  assert.ok(html.includes('<span>5/6 outputs recorded</span>'))
})

test('single-run result badges distinguish completion, partial credit and no credit; errors stay unscored', () => {
  for (const [score, status, label] of [[1, 'complete', '1/1'], [0.5, 'partial', '0.5/1'], [0, 'no-credit', '0/1']] as const) {
    const attempt = { ...row('other/cedar', 'inspect', score === 1), gradingResult: { score } }
    const data = aggregate({ tasks: 1, repetitions: 1 }, { results: { results: [attempt] } }, undefined, undefined)
    assert.ok(render(data).includes(`<span class="badge badge-soft result-badge result-${status}">${label}</span>`))
    const errored = { ...attempt, failureReason: 2 }
    const errorHtml = render(aggregate({ tasks: 1, repetitions: 1 }, { results: { results: [errored] } }, undefined, undefined))
    assert.ok(errorHtml.includes('<dt>Result</dt><dd>Execution error</dd>'))
    assert.ok(!errorHtml.includes(`<span class="badge badge-soft result-badge result-${status}">`))
  }
})

test('task and grader cost projections retain response timestamps for peak pricing', () => {
  const model = 'opencode-go/deepseek-v4-pro'
  const make = (timestamp?: number) => {
    const attempt = row(model, 'inspect', true)
    const metrics = { ...attempt.response.metadata, usageSteps: [{ ...attempt.response.metadata.usageSteps[0], timestamp }] }
    const prices = { capturedAt: '2026-10-14T00:00:00Z', api: goPrices,
      exchange: { source: 'synthetic', date: '2026-10-14', cadPerUsd: 1 } }
    return aggregate({ tasks: 1, repetitions: 1 }, { results: { results: [{ ...attempt, response: { metadata: metrics } }] } }, undefined,
      { judgments: [{ taskID: 'inspect', score: 1, status: 'pass', investigator: { model }, metadata: metrics }] }, prices)
  }
  const offPeak = make(Date.parse('2026-10-14T00:59:59Z'))
  const peak = make(Date.parse('2026-10-14T01:00:00Z'))
  assert.equal(peak.models[0].tasks[0].apiCostCad, offPeak.models[0].tasks[0].apiCostCad! * 2)
  assert.equal(peak.grader.outputs[0].costCad, offPeak.grader.outputs[0].costCad! * 2)
  assert.equal(make().models[0].tasks[0].apiCostCad, null)
})

const run = process.env.EVAL_PRESENTATION_RUN
test(
  'saved run: every model/task/repetition and saved role totals match',
  { skip: !run },
  () => {
    const read = (file: string) =>
      JSON.parse(fs.readFileSync(path.join(run!, file), 'utf8'))
    const manifest = read('run-manifest.json'),
      pilot = read('pilot.json'),
      summary = read('model-summary.json'),
      grader = read('grader-summary.json'),
      pricing = pricingSnapshot(read('pricing-snapshot.json'))
    const data = aggregate(manifest, pilot, summary, grader, pricing)
    assert.equal(data.attempts, pilot.results.results.length)
    assert.deepEqual(
      data.models.map((model) => model.model),
      manifest.models
    )
    assert.equal(
      data.expectedAttempts,
      manifest.models.length * manifest.tasks * manifest.repetitions
    )
    let accounted = 0
    for (const model of data.models) {
      const saved = summary.models.find(
        (value: { model: string }) => value.model === model.model
      )
      for (const key of [
        'attempts',
        'medianSeconds',
        'apiCostCad',
        'toolCalls',
        'failedTools',
        'inputTokens',
        'outputTokens',
        'cacheReadTokens',
        'cacheWriteTokens',
        'estimateCoverage',
      ] as const)
        assert.equal(model.investigator[key], saved[key])
      for (const task of model.tasks) {
        const selected = pilot.results.results.filter(
          (value: { provider: { label: string }; vars: { task_id: string } }) =>
            value.provider.label === model.model &&
            value.vars.task_id === task.task
        )
        accounted += selected.length
        assert.equal(task.attempts, selected.length)
        assert.equal(task.attempts, manifest.repetitions)
        assert.equal(
          task.pass,
          selected.filter((value: { success: boolean }) => value.success).length
        )
        assert.equal(
          task.fail + task.partial + task.error + task.unknown + task.pass,
          selected.length
        )
        const reports = selected.flatMap(
          (value: {
            gradingResult?: {
              componentResults?: {
                metadata?: {
                  criteria?: { verdict: string }[]
                  artifact?: { pass: boolean }
                }
              }[]
            }
          }) => value.gradingResult?.componentResults ?? []
        )
        const criteria = reports.flatMap(
          (value: { metadata?: { criteria?: { verdict: string }[] } }) =>
            value.metadata?.criteria ?? []
        )
        assert.equal(
          task.checks.pass,
          criteria.filter(
            (value: { verdict: string }) => value.verdict === 'Pass'
          ).length
        )
        assert.equal(
          task.checks.fail,
          criteria.filter(
            (value: { verdict: string }) => value.verdict === 'Fail'
          ).length
        )
        assert.equal(
          task.checks.partial,
          criteria.filter((value: { verdict: string }) => value.verdict === 'Partial').length
        )
        assert.equal(
          task.checks.artifactPass,
          reports.filter(
            (value: { metadata?: { artifact?: { pass: boolean } } }) =>
              value.metadata?.artifact?.pass === true
          ).length
        )
        assert.equal(
          task.checks.artifactFail,
          reports.filter(
            (value: { metadata?: { artifact?: { pass: boolean } } }) =>
              value.metadata?.artifact?.pass === false
          ).length
        )
        assert.equal(
          task.medianSeconds,
          median(
            selected.map(
              (value: { response: { metadata: { elapsedSeconds: number } } }) =>
                value.response.metadata.elapsedSeconds
            )
          )
        )
        const calculated = modelSummaries(selected, pricing)[0]
        for (const key of [
          'medianSeconds',
          'apiCostCad',
          'toolCalls',
          'failedTools',
          'inputTokens',
          'outputTokens',
          'cacheReadTokens',
          'cacheWriteTokens',
          'estimateCoverage',
        ] as const)
          assert.equal(task[key], calculated[key])
      }
    }
    assert.equal(accounted, data.attempts)
    assert.equal(
      data.models.reduce((sum, model) => sum + model.result.pass, 0),
      pilot.results.results.filter((row: { success?: boolean }) => row.success === true).length
    )
    assert.equal(
      data.models.reduce((sum, model) => sum + model.result.fail + model.result.partial + model.result.error + model.result.unknown, 0),
      pilot.results.results.filter((row: { success?: boolean }) => row.success !== true).length
    )
    assert.equal(data.grader.sessions, grader.sessions)
    assert.equal(data.grader.executionErrors, grader.executionErrors)
    assert.equal(data.grader.models[0].apiCostCad, grader.models[0].apiCostCad)
    const html = render(data)
    for (const forbidden of [
      '/Users/',
      '/private/var/',
      '/var/folders/',
      'sessionIDs',
      'usageSteps',
      'componentResults',
      'isolationSettingsFile',
      'gradingResult',
      '"judgments"',
      '"rubric"',
    ])
      assert.ok(!html.includes(forbidden), forbidden)
    // Check every retained answer and grader explanation without publishing them.
    for (const row of pilot.results.results) {
      if (
        typeof row.response?.output === 'string' &&
        row.response.output.length > 80
      )
        assert.ok(!html.includes(row.response.output))
      for (const component of row.gradingResult?.componentResults ?? [])
        for (const criterion of component.metadata?.criteria ?? [])
          if (
            typeof criterion.explanation === 'string' &&
            criterion.explanation.length > 80
          )
            assert.ok(!html.includes(criterion.explanation))
    }
  }
)
