import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import cases from './tasks.json' with { type: 'json' }
import { providerFixture } from './test-fixture.ts'
import { readJson, record, saveJson, SUITE } from './runtime.ts'

// This invokes the actual pinned loader, but every provider process is a local stub.
const { root, isolation } = providerFixture()
const settings = {
  models: [
    'openai/gpt-6.1-sol',
    'openai/gpt-5.6-sol',
    'openai/gpt-5.6-luna',
    'openai/gpt-5.6-terra',
  ].join('\n'),
  answer: JSON.stringify({
    ...cases[0].expected,
    evidence: ['synthetic loader proof'],
  }),
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
for (const value of rows) {
  const row = record(value)
  assert.equal(row.success, true)
  const response = record(row.response),
    metadata = record(response.metadata)
  assert.equal(metadata.version, '1.18.10')
  assert.equal(response.output, settings.answer)
  assert.equal(record(row.gradingResult).pass, true)
}
saveJson(path.join(root, 'loader-proof.json'), {
  loadedConfig: path.join(SUITE, 'config.ts'),
  loadedProvider: path.join(SUITE, 'provider.ts'),
  loadedGrader: path.join(SUITE, 'grade.ts'),
  syntheticResults: rows.length,
  allPassed: true,
  externalInferenceAttempted: false,
})
