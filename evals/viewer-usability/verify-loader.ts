import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import cases from './tasks.json' with { type: 'json' }
import { providerFixture } from './test-fixture.ts'
import { readJson, record, saveJson, SUITE } from './runtime.ts'
import rubrics from './rubrics.json' with { type: 'json' }
import { pilotSummary } from './run-pilot.ts'

// This invokes the actual pinned loader, but every provider process is a local stub.
const { root, isolation } = providerFixture()
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
fs.writeFileSync(
  path.join(root, 'stub-settings.json'),
  JSON.stringify(settings)
)
saveJson(path.join(root, 'connection.json'), {
  binary: '/synthetic/viewer',
  endpoint: 'http://127.0.0.1:1',
  start: '2026-10-07T08:00:00Z',
  end: '2026-10-07T08:10:00Z',
})
saveJson(path.join(root, 'isolation.json'), isolation)
const pinned = record(
  readJson(path.join(SUITE, 'node_modules/promptfoo/package.json'))
)
assert.equal(pinned.version, '0.124.0')
const command = [
  process.execPath,
  path.join(SUITE, 'node_modules/promptfoo/dist/src/entrypoint.js'),
  'eval',
  '-c',
  path.join(SUITE, 'config.ts'),
  '--filter-first-n',
  '1',
  '--no-cache',
  '--no-share',
  '-o',
  path.join(root, 'loader-results.json'),
  '--no-progress-bar',
  '--no-table',
]
const result = spawnSync(command[0], command.slice(1), {
  cwd: root,
  encoding: 'utf8',
  timeout: 120000,
  env: {
    ...process.env,
    OTEL_EVAL_RUN_DIR: root,
    OTEL_EVAL_ISOLATION_FILE: path.join(root, 'isolation.json'),
    PROMPTFOO_DISABLE_TELEMETRY: '1',
    PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'),
  },
})
fs.writeFileSync(path.join(root, 'loader.stdout'), result.stdout || '', {
  flag: 'wx',
  mode: 0o600,
})
fs.writeFileSync(path.join(root, 'loader.stderr'), result.stderr || '', {
  flag: 'wx',
  mode: 0o600,
})
saveJson(path.join(root, 'loader-command.json'), {
  command,
  status: result.status,
  signal: result.signal,
  error: result.error?.message,
  node: process.version,
  promptfoo: pinned.version,
  externalInferenceAttempted: false,
})
console.log(JSON.stringify({ evidence: root, status: result.status }))
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
fs.writeFileSync(
  path.join(root, 'stub-settings.json'),
  JSON.stringify({ ...settings, graderAnswer: 'invalid grader response' })
)
const errorCommand = [...command]
errorCommand[errorCommand.indexOf('-o') + 1] = path.join(
  root,
  'loader-errors.json'
)
const failed = spawnSync(errorCommand[0], errorCommand.slice(1), {
  cwd: root,
  encoding: 'utf8',
  timeout: 120000,
  env: {
    ...process.env,
    OTEL_EVAL_RUN_DIR: root,
    OTEL_EVAL_ISOLATION_FILE: path.join(root, 'isolation.json'),
    PROMPTFOO_DISABLE_TELEMETRY: '1',
    PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'),
  },
})
saveJson(path.join(root, 'loader-error-command.json'), {
  status: failed.status,
  externalInferenceAttempted: false,
})
const errorRows = record(
  record(readJson(path.join(root, 'loader-errors.json'))).results
).results
assert.ok(Array.isArray(errorRows))
assert.equal(errorRows.length, 4)
assert.equal(
  pilotSummary({ results: { results: errorRows } }).split(' | error |').length -
    1,
  4
)
for (const value of errorRows) {
  const row = record(value)
  assert.equal(row.success, false)
  assert.match(String(row.error), /Invalid grader output/)
  assert.equal(row.failureReason, 2)
  assert.equal(row.gradingResult, null)
}
fs.writeFileSync(
  path.join(root, 'stub-settings.json'),
  JSON.stringify({
    ...settings,
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
)
const factCommand = [...command]
factCommand[factCommand.indexOf('-o') + 1] = path.join(
  root,
  'loader-factual-failures.json'
)
const factual = spawnSync(factCommand[0], factCommand.slice(1), {
  cwd: root,
  encoding: 'utf8',
  timeout: 120000,
  env: {
    ...process.env,
    OTEL_EVAL_RUN_DIR: root,
    OTEL_EVAL_ISOLATION_FILE: path.join(root, 'isolation.json'),
    PROMPTFOO_DISABLE_TELEMETRY: '1',
    PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'),
  },
})
assert.equal(factual.error, undefined)
saveJson(path.join(root, 'loader-factual-command.json'), {
  status: factual.status,
  externalInferenceAttempted: false,
})
const factRows = record(
  record(readJson(path.join(root, 'loader-factual-failures.json'))).results
).results
assert.ok(Array.isArray(factRows))
assert.equal(factRows.length, 4)
for (const value of factRows) {
  const row = record(value)
  assert.equal(row.failureReason, 1)
  assert.equal(record(row.gradingResult).pass, false)
  assert.ok(row.error)
}
assert.equal(
  pilotSummary({ results: { results: factRows } }).split(' | fail |').length -
    1,
  4
)
const investigatorRuns = fs
  .readdirSync(path.join(root, 'runs'))
  .filter(name => name.startsWith('investigator-'))
assert.equal(investigatorRuns.length, 12)
for (const run of investigatorRuns)
  assert.ok(
    ['Inventory failed.', settings.answer].includes(
      fs.readFileSync(path.join(root, 'runs', run, 'answer.txt'), 'utf8')
    )
  )
saveJson(path.join(root, 'loader-proof.json'), {
  loadedConfig: path.join(SUITE, 'config.ts'),
  loadedProvider: path.join(SUITE, 'provider.ts'),
  loadedGrader: path.join(SUITE, 'grade.ts'),
  syntheticResults: rows.length,
  syntheticExecutionErrors: errorRows.length,
  syntheticFactualFailures: factRows.length,
  investigatorResponsesRetained: investigatorRuns.length,
  separateFreshGraderContexts: true,
  privatePromptSeparation: true,
  allPassed: true,
  externalInferenceAttempted: false,
})
