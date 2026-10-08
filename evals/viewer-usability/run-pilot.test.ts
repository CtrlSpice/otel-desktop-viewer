import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { runPilot, pilotSummary } from './run-pilot.ts'
import { readJson, record, TEMPORARY } from './runtime.ts'

test('pilot subprocess keeps sharing/telemetry disabled, failed evidence and original results', async () => {
  const root = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-typescript-pilot-test-')
  )
  const suite = path.join(root, 'synthetic-suite')
  fs.mkdirSync(path.join(suite, 'node_modules/.bin'), { recursive: true })
  fs.writeFileSync(
    path.join(suite, 'node_modules/.bin/promptfoo'),
    `#!${process.execPath}
const fs = require('node:fs');
const path = require('node:path');
fs.writeFileSync('observation.json',JSON.stringify({args:process.argv.slice(2),cwd:process.cwd(),
  telemetry:process.env.PROMPTFOO_DISABLE_TELEMETRY,state:process.env.PROMPTFOO_CONFIG_DIR}));
console.log('synthetic failed pilot stdout'); console.error('synthetic failed pilot stderr');
fs.writeFileSync('pilot.json',JSON.stringify({results:{results:[{provider:{label:'synthetic'},vars:{task_id:'synthetic'},
  success:false,response:{error:'synthetic failure',metadata:{elapsedSeconds:1.234,toolCalls:0}}}]}}));
process.exitCode = 7;
`,
    { mode: 0o700 }
  )
  assert.equal(await runPilot(root, suite), 7)
  const observation = record(readJson(path.join(root, 'observation.json')))
  assert.equal(observation.telemetry, '1')
  assert.equal(observation.state, path.join(root, 'promptfoo-state'))
  assert.equal(observation.cwd, fs.realpathSync(root))
  assert.ok(Array.isArray(observation.args))
  assert.ok(observation.args.includes('--no-share'))
  assert.ok(observation.args.includes(path.join(suite, 'config.ts')))
  assert.equal(
    record(readJson(path.join(root, 'pilot-completion.json'))).exit,
    7
  )
  assert.match(
    fs.readFileSync(path.join(root, 'pilot.stderr.log'), 'utf8'),
    /synthetic failed/
  )
  assert.match(
    fs.readFileSync(path.join(root, 'pilot-summary.md'), 'utf8'),
    /synthetic \| synthetic \| error \| 1.23/
  )
  const retained = fs.readFileSync(path.join(root, 'pilot.json'))
  await assert.rejects(runPilot(root, suite), /refusing to overwrite/)
  assert.deepEqual(fs.readFileSync(path.join(root, 'pilot.json')), retained)
})

test('pilot spawn failure keeps protected completion and logs', async () => {
  const root = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-typescript-pilot-spawn-test-')
  )
  await assert.rejects(
    runPilot(root, path.join(root, 'missing-suite')),
    /ENOENT/
  )
  assert.match(
    String(record(readJson(path.join(root, 'pilot-completion.json'))).error),
    /ENOENT/
  )
  assert.equal(
    fs.statSync(path.join(root, 'pilot.stderr.log')).mode & 0o777,
    0o600
  )
})

test('pilot summary distinguishes fact failures and provider errors', () => {
  const results = [
    { success: true },
    { success: false },
    { success: false, response: { error: 'failure' } },
  ].map(row => ({
    provider: { label: 'model' },
    vars: { task_id: 'task' },
    ...row,
  }))
  const summary = pilotSummary({ results: { results } })
  for (const status of ['pass', 'fail', 'error'])
    assert.ok(summary.includes(`| model | task | ${status} |`))
})
