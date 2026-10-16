import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { record, SUITE, TEMPORARY } from './runtime.ts'

test('runner configuration forwards only explicit private isolation settings', () => {
  const root = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-isolation-config-test-')
  )
  fs.writeFileSync(path.join(root, 'connection.json'), '{}', { mode: 0o600 })
  const file = path.join(root, 'isolation.json')
  const settings = {
    permission: { task: 'deny', bash: { '*': 'ask' } },
    authFile: path.join(root, 'auth-reference.json'),
  }
  fs.writeFileSync(file, JSON.stringify(settings), { mode: 0o600 })
  const run = (configured: string) =>
    spawnSync(
      process.execPath,
      [
        '--input-type=module',
        '-e',
        'import c from "./config.ts"; console.log(JSON.stringify({models:[...new Set(c.tests.map(t=>t.vars.eval_model))],tests:c.tests.length,extensions:c.extensions}));',
      ],
      {
        cwd: SUITE,
        env: {
          ...process.env,
          OTEL_EVAL_RUN_DIR: root,
          OTEL_EVAL_ISOLATION_FILE: configured,
        },
        encoding: 'utf8',
      }
    )
  const result = run(file)
  assert.equal(result.status, 0)
  const parsed: unknown = JSON.parse(result.stdout)
  const configuration = record(parsed)
  assert.deepEqual(
    configuration.models,
    [
      'openai/gpt-6.1-sol',
      'openai/gpt-5.6-sol',
      'openai/gpt-6-luna',
      'openai/gpt-5.6-luna',
    ]
  )
  assert.equal(configuration.tests, 44)
  assert.equal((configuration.extensions as string[]).length, 3)
  const missing = run('')
  assert.equal(missing.status, 0)
  const missingProviders: unknown = JSON.parse(missing.stdout)
  assert.equal(record(missingProviders).tests, 44)
  fs.chmodSync(file, 0o644)
  const publicFile = run(file)
  assert.notEqual(publicFile.status, 0)
  assert.match(publicFile.stderr, /private absolute/)
  fs.chmodSync(file, 0o600)
  fs.writeFileSync(
    file,
    '{"authEnvironment":{"OPENAI_API_KEY":"synthetic-secret"}}'
  )
  const inline = run(file)
  assert.notEqual(inline.status, 0)
  assert.match(inline.stderr, /authEnvironmentFile/)
  assert.ok(!inline.stderr.includes('synthetic-secret'))
  fs.writeFileSync(file, '{"private":"synthetic-secret" INVALID')
  const malformed = run(file)
  assert.notEqual(malformed.status, 0)
  assert.match(malformed.stderr, /Cannot parse/)
  assert.ok(!malformed.stderr.includes('synthetic-secret'))
})
