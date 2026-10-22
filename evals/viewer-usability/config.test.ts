import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import cases from './tasks.json' with { type: 'json' }
import { SUITE, TEMPORARY } from './runtime.ts'

test('actual suite config defines four ordinary providers, eleven questions once, and three native repetitions', () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'pi-config-'))
  fs.writeFileSync(path.join(root, 'connection.json'), JSON.stringify({ binary: '/fixture/viewer', endpoint: 'http://127.0.0.1:8000' }))
  const child = spawnSync(process.execPath, ['--input-type=module', '-e', 'import config from "./config.ts"; console.log(JSON.stringify(config))'], {
    cwd: SUITE, encoding: 'utf8', env: { ...process.env, OTEL_EVAL_RUN_DIR: root, OTEL_EVAL_PI_AUTH_FILE: path.join(root, 'private-auth.json'), OTEL_EVAL_OBSERVER_HTTP: 'http://127.0.0.1:4318' },
  })
  assert.equal(child.status, 0, child.stderr)
  const config = JSON.parse(child.stdout)
  assert.deepEqual(config.providers.map((p: { config: { model: string } }) => p.config.model), ['gpt-6.1-sol', 'gpt-6-luna', 'gpt-5.6-luna', 'claude-opus-5-5'])
  assert.equal(config.tests.length, cases.length)
  assert.deepEqual(config.tests.map((t: { vars: unknown }) => t.vars), cases.map(task => ({ task_id: task.id, goal: task.goal })))
  assert.equal(config.evaluateOptions.repeat, 3)
  assert.equal(config.evaluateOptions.cache, false)
  assert.equal(config.tracing.enabled, true)
  assert.equal(config.providers[3].config.telemetryEndpoint, 'http://127.0.0.1:4318')
  assert.equal(config.extensions, undefined)
  assert.equal(config.defaultTest.vars.binary, './bin/otel-desktop-viewer')
  assert.equal(config.defaultTest.assert[0].type, 'llm-rubric')
  assert.equal(config.defaultTest.assert[0].threshold, 1)
  assert.equal(config.defaultTest.assert[0].value, 'file://' + path.join(SUITE, 'grading-evidence.ts'))
  assert.equal(config.defaultTest.options.provider.config.role, 'grader')
  assert.equal(config.tests.find((t: { vars: { task_id: string } }) => t.vars.task_id === 'trace-export').assert[0].metric, 'export-file')
})
