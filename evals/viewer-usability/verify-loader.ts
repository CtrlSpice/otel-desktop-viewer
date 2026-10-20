import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import cases from './tasks.json' with { type: 'json' }
import { providerFixture, viewerSkillFixture } from './test-fixture.ts'
import { readJson, record, saveJson, SUITE } from './runtime.ts'
import rubrics from './rubrics.json' with { type: 'json' }
import { pilotSummary } from './run-pilot.ts'
import { cleanEnvironment } from './clean-environment.ts'

// This invokes the actual pinned loader, but every provider process is a local stub.
const settings = {
  models: [
    'openai/gpt-6.1-sol',
    'openai/gpt-5.6-sol',
    'openai/gpt-6-luna',
    'openai/gpt-5.6-luna',
  ].join('\n'),
  answer:
    'The payment was declined for insufficient funds, according to the provider exception and payment log.',
}
function scenario(overrides: Record<string, unknown> = {}) {
  const { root, isolation } = providerFixture()
  fs.writeFileSync(
    path.join(root, 'stub-settings.json'),
    JSON.stringify({ ...settings, ...overrides })
  )
  saveJson(path.join(root, 'connection.json'), {
    binary: viewerSkillFixture(root),
    endpoint: 'http://127.0.0.1:1',
    start: '2026-10-07T08:00:00Z',
    end: '2026-10-07T08:10:00Z',
  })
  saveJson(path.join(root, 'isolation.json'), isolation)
  return root
}
const root = scenario()
const pinned = record(
  readJson(path.join(SUITE, 'node_modules/promptfoo/package.json'))
)
assert.equal(pinned.version, '0.124.1')
const command = [
  process.execPath,
  '--input-type=module',
  '-e',
  `import config from ${JSON.stringify(path.join(SUITE, 'config.ts'))}; import {runNativeEvaluation} from ${JSON.stringify(path.join(SUITE, 'native-evaluate.ts'))}; process.exitCode=await runNativeEvaluation({...config,tests:config.tests.slice(0,4)},1,process.argv[1]);`,
  'loader-results.json',
]
function runLoader(root: string, command: string[], prefix: string) {
  const result = spawnSync(command[0], command.slice(1), {
    cwd: root,
    encoding: 'utf8',
    timeout: 300000,
    env: {
      ...cleanEnvironment({ root, inherited: {} }),
      OTEL_EVAL_RUN_DIR: root,
      OTEL_EVAL_ISOLATION_FILE: path.join(root, 'isolation.json'),
      PROMPTFOO_DISABLE_TELEMETRY: '1',
      PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'),
    },
  })
  fs.writeFileSync(path.join(root, prefix + '.stdout'), result.stdout || '', {
    flag: 'wx',
    mode: 0o600,
  })
  fs.writeFileSync(path.join(root, prefix + '.stderr'), result.stderr || '', {
    flag: 'wx',
    mode: 0o600,
  })
  saveJson(path.join(root, prefix + '-command.json'), {
    command,
    status: result.status,
    signal: result.signal,
    error: result.error?.message,
    node: process.version,
    promptfoo: pinned.version,
    externalInferenceAttempted: false,
  })
  console.log(JSON.stringify({ evidence: root, status: result.status }))
  assert.equal(result.error, undefined, 'Loader process failed; inspect ' + root)
  return result
}
const result = runLoader(root, command, 'loader')
assert.equal(
  result.status,
  0,
  'Pinned Promptfoo loader failed; inspect protected loader logs'
)
const rows = record(
  record(readJson(path.join(root, 'loader-results.json'))).results
).results
assert.ok(Array.isArray(rows))
assert.equal(rows.length, 4)
assert.equal(
  pilotSummary({ results: { results: rows } }).split(' | pass |').length - 1,
  4
)
for (const value of rows) {
  const row = record(value)
  assert.equal(row.success, true)
  const response = record(row.response),
    metadata = record(response.metadata)
  assert.equal(metadata.version, '1.18.10')
  assert.equal(response.output, settings.answer)
  assert.equal(record(row.gradingResult).pass, true)
  assert.equal(record(row.gradingResult).score, 1)
  const prompt = fs.readFileSync(
    path.join(String(metadata.evidence), 'prompt.txt'),
    'utf8'
  )
  assert.ok(prompt.startsWith(cases[0].goal + '\n'))
  assert.ok(!prompt.includes('reported-cause'))
  assert.ok(!prompt.includes('insufficient_funds'))
  assert.ok(!prompt.includes('Return one JSON'))
  const grading = record(row.gradingResult)
  assert.ok(Array.isArray(grading.componentResults))
  const report = record(record(grading.componentResults[0]).metadata)
  const grader = record(report.metadata)
  assert.equal(grader.role, 'grader')
  assert.equal(grader.model, 'openai/gpt-6.1-sol')
  assert.notEqual(grader.workspace, metadata.workspace)
  assert.notDeepEqual(grader.sessionIDs, metadata.sessionIDs)
  assert.ok(
    !JSON.stringify(row.vars).includes(
      String(rubrics['checkout-failure'].reference)
    )
  )
  assert.ok(!JSON.stringify(row.vars).includes('reported-cause'))
}
// A malformed grading response is a loader execution error, not an incorrect answer.
const errorRoot = scenario({ graderAnswer: 'invalid grader response' })
const errorCommand = [...command]
errorCommand[errorCommand.length - 1] = 'loader-errors.json'
const failed = runLoader(errorRoot, errorCommand, 'loader-error')
assert.equal(failed.status, 1)
const errorRows = record(
  record(readJson(path.join(errorRoot, 'loader-errors.json'))).results
).results
assert.ok(Array.isArray(errorRows))
assert.equal(errorRows.length, 1)
assert.equal(
  pilotSummary({ results: { results: errorRows } }).split(' | error |').length -
    1,
  1
)
const deferred = record(readJson(path.join(errorRoot, 'deferred-slots.json')))
assert.ok(Array.isArray(deferred.slots))
assert.equal(deferred.slots.length, 3)
assert.deepEqual(deferred.slots.map(value => record(value).model), settings.models.split('\n').slice(1))
assert.ok(deferred.slots.every(value => record(value).task === cases[0].id && record(value).repetition === 0))
for (const value of errorRows) {
  const row = record(value)
  assert.equal(row.success, false)
  assert.match(String(row.error), /Invalid grader output/)
  assert.equal(row.failureReason, 2)
  assert.equal(row.gradingResult, null)
}
const factRoot = scenario({
  answer: 'Inventory failed.',
  graderAnswer: JSON.stringify({
    criteria: [
      {
        id: 'reported-cause',
        verdict: 'Fail',
        explanation: 'Incorrectly blames inventory.',
      },
    ],
  }),
})
const factCommand = [...command]
factCommand[factCommand.length - 1] = 'loader-factual-failures.json'
const factual = runLoader(factRoot, factCommand, 'loader-factual')
assert.equal(factual.status, 0)
const factRows = record(
  record(readJson(path.join(factRoot, 'loader-factual-failures.json'))).results
).results
assert.ok(Array.isArray(factRows))
assert.equal(factRows.length, 4)
for (const value of factRows) {
  const row = record(value)
  assert.equal(row.failureReason, 1)
  assert.equal(record(row.gradingResult).pass, false)
  assert.equal(record(row.gradingResult).score, 0)
  assert.ok(row.error)
}
assert.equal(
  pilotSummary({ results: { results: factRows } }).split(' | fail |').length -
    1,
  4
)
const partialAnswer = 'The payment was declined.'
const partialRoot = scenario({
  answer: partialAnswer,
  graderAnswer: JSON.stringify({ criteria: [{
    id: 'reported-cause', verdict: 'Partial',
    explanation: 'Identifies the decline but omits insufficient funds.',
  }] }),
})
const partialCommand = [...command]
partialCommand[partialCommand.length - 1] = 'loader-partial-results.json'
const partial = runLoader(partialRoot, partialCommand, 'loader-partial')
assert.equal(partial.status, 0)
const partialRows = record(
  record(readJson(path.join(partialRoot, 'loader-partial-results.json'))).results
).results
assert.ok(Array.isArray(partialRows))
assert.equal(partialRows.length, 4)
for (const value of partialRows) {
  const row = record(value)
  assert.equal(row.success, false)
  assert.equal(row.failureReason, 1)
  const grading = record(row.gradingResult)
  assert.equal(grading.pass, false)
  assert.equal(grading.score, 0.5)
  assert.ok(Array.isArray(grading.componentResults))
  const report = record(record(grading.componentResults[0]).metadata)
  const grader = record(report.metadata)
  const saved = record(readJson(path.join(String(grader.evidence), 'grading.json')))
  assert.equal(saved.status, 'partial')
  assert.equal(saved.score, 0.5)
}
assert.equal(
  pilotSummary({ results: { results: partialRows } }).split(' | partial | 0.5 |').length - 1,
  4
)
const roots = [root, errorRoot, factRoot, partialRoot]
const investigatorRuns = roots.flatMap(directory => fs
  .readdirSync(path.join(directory, 'runs'))
  .filter(name => name.startsWith('investigator-'))
  .map(name => path.join(directory, 'runs', name)))
assert.equal(investigatorRuns.length, 13)
for (const run of investigatorRuns)
  assert.ok(
    ['Inventory failed.', settings.answer, partialAnswer].includes(
      fs.readFileSync(path.join(run, 'answer.txt'), 'utf8')
    )
  )
saveJson(path.join(root, 'loader-proof.json'), {
  scenarios: { pass: root, executionError: errorRoot, factualFailure: factRoot, partial: partialRoot },
  loadedConfig: path.join(SUITE, 'config.ts'),
  loadedProvider: path.join(SUITE, 'native-lifecycle.ts'),
  loadedGrader: path.join(SUITE, 'grade.ts'),
  syntheticResults: rows.length,
  syntheticExecutionErrors: errorRows.length,
  unexecutedErrorSlots: deferred.slots.length,
  syntheticFactualFailures: factRows.length,
  syntheticPartialResults: partialRows.length,
  investigatorResponsesRetained: investigatorRuns.length,
  separateFreshGraderContexts: true,
  privatePromptSeparation: true,
  allPassed: true,
  externalInferenceAttempted: false,
})
