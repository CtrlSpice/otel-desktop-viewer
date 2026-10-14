import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { runPilot, pilotSummary } from './run-pilot.ts'
import { readJson, record, TEMPORARY, SUITE, saveJson } from './runtime.ts'
import apiPrices from './api-prices.json' with { type: 'json' }
import { pricingSnapshot } from './pricing.ts'

const pricing = pricingSnapshot({
  capturedAt: '2026-10-08T06:00:00Z',
  api: apiPrices,
  exchange: { source: 'synthetic-rate', date: '2026-10-07', cadPerUsd: 1.4257 },
})

test('pilot subprocess keeps sharing/telemetry disabled, failed evidence and original results', async () => {
  const root = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-typescript-pilot-test-')
  )
  const suite = path.join(root, 'synthetic-suite')
  fs.mkdirSync(path.join(suite, 'node_modules/.bin'), { recursive: true })
  fs.writeFileSync(
    path.join(suite, 'native-evaluate.ts'),
    `#!${process.execPath}
const fs = require('node:fs');
const path = require('node:path');
fs.writeFileSync('observation.json',JSON.stringify({args:process.argv.slice(2),cwd:process.cwd(),
  telemetry:process.env.PROMPTFOO_DISABLE_TELEMETRY,state:process.env.PROMPTFOO_CONFIG_DIR}));
console.log('synthetic failed pilot stdout'); console.error('synthetic failed pilot stderr');
fs.writeFileSync('pilot.json',JSON.stringify({results:{results:[{provider:{label:'synthetic'},vars:{task_id:'synthetic'},
  success:false,response:{error:'synthetic failure',metadata:{elapsedSeconds:1.234,toolCalls:0}}}]}}));
fs.mkdirSync('runs/investigator-synthetic',{recursive:true});
fs.mkdirSync('runs/grader-synthetic',{recursive:true});
const investigator={role:'investigator',model:'openai/gpt-6-luna',elapsedSeconds:1.234,toolCalls:0,usage:{input:123,output:2,reasoning:0,cacheRead:0,cacheWrite:0}};
const grader={role:'grader',model:'openai/gpt-6.1-sol',elapsedSeconds:0.5,toolCalls:0,usage:{input:3,output:2,reasoning:0,cacheRead:0,cacheWrite:0}};
fs.writeFileSync('runs/investigator-synthetic/metrics.json',JSON.stringify(investigator));
fs.writeFileSync('runs/grader-synthetic/grading.json',JSON.stringify({status:'execution-error',metadata:grader,investigator}));
process.exitCode = 7;
`,
    { mode: 0o700 }
  )
  assert.equal(await runPilot(root, suite, pricing), 7)
  const observation = record(readJson(path.join(root, 'observation.json')))
  assert.equal(observation.telemetry, '1')
  assert.equal(observation.state, path.join(root, 'promptfoo-state'))
  assert.equal(observation.cwd, fs.realpathSync(root))
  assert.ok(Array.isArray(observation.args))
  assert.deepEqual(observation.args, [])
  const entrypoint = fs.readFileSync(path.join(SUITE, 'native-evaluate.ts'), 'utf8')
  assert.match(entrypoint, /share: false/)
  assert.match(entrypoint, /repeat = 3/)
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
    /synthetic \| synthetic \| error \| N\/A \| 1.23/
  )
  const retained = fs.readFileSync(path.join(root, 'pilot.json'))
  assert.deepEqual(readJson(path.join(root, 'pricing-snapshot.json')), pricing)
  assert.ok(fs.existsSync(path.join(root, 'model-summary.json')))
  const investigators = record(
    readJson(path.join(root, 'model-summary.json'))
  ).models
  assert.ok(Array.isArray(investigators))
  assert.equal(record(investigators[0]).model, 'openai/gpt-6-luna')
  assert.equal(record(investigators[0]).inputTokens, 123)
  const grading = record(readJson(path.join(root, 'grader-summary.json')))
  assert.equal(grading.executionErrors, 1)
  assert.ok(Array.isArray(grading.models))
  assert.equal(record(grading.models[0]).model, 'openai/gpt-6.1-sol')
  assert.equal(record(grading.models[0]).inputTokens, 3)
  await assert.rejects(runPilot(root, suite, pricing), /refusing to overwrite/)
  assert.deepEqual(fs.readFileSync(path.join(root, 'pilot.json')), retained)
})

test('pilot spawn failure keeps protected completion and logs', async () => {
  const root = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-typescript-pilot-spawn-test-')
  )
  assert.equal(await runPilot(root, path.join(root, 'missing-suite'), pricing), 1)
  assert.equal(record(readJson(path.join(root, 'pilot-completion.json'))).exit, 1)
  assert.match(fs.readFileSync(path.join(root, 'pilot.stderr.log'), 'utf8'), /Cannot find module/)
  assert.equal(
    fs.statSync(path.join(root, 'pilot.stderr.log')).mode & 0o777,
    0o600
  )
})

test('pilot summary preserves partial credit and leaves execution errors unscored', () => {
  const results = [
    { success: true, gradingResult: { pass: true, score: 1 } },
    {
      success: false,
      failureReason: 1,
      error: 'Incomplete explanation',
      gradingResult: { pass: false, score: 0.5 },
    },
    {
      success: false,
      failureReason: 1,
      error: 'Incorrect cause',
      gradingResult: { pass: false, score: 0 },
    },
    { success: false, response: { error: 'failure' } },
    {
      success: false,
      failureReason: 2,
      error: 'Grader execution failure',
      gradingResult: null,
    },
  ].map(row => ({
    provider: { label: 'model' },
    vars: { task_id: 'task' },
    ...row,
  }))
  const summary = pilotSummary({ results: { results } })
  for (const [status, score] of [['pass', 1], ['partial', 0.5], ['fail', 0], ['error', 'N/A']])
    assert.ok(summary.includes(`| model | task | ${status} | ${score} |`))
  assert.equal(summary.split('| model | task | error |').length - 1, 2)
})

test('actual standalone pilot entrypoint propagates nonzero child exit', () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-pilot-entrypoint-'))
  const suite = path.join(root, 'suite')
  fs.mkdirSync(path.join(suite, 'node_modules/.bin'), { recursive: true })
  for (const file of [
    'run-pilot.ts',
    'runtime.ts',
    'pricing.ts',
    'model-summary.ts',
    'clean-environment.ts',
    'openai-models.json',
  ])
    fs.copyFileSync(path.join(SUITE, file), path.join(suite, file))
  fs.writeFileSync(path.join(suite, 'package.json'), '{"type":"module"}')
  fs.writeFileSync(
    path.join(suite, 'native-evaluate.ts'),
    `#!${process.execPath}\nprocess.exitCode = 7;\n`,
    { mode: 0o700 }
  )
  saveJson(path.join(root, 'pricing-snapshot.json'), pricing)
  const result = spawnSync(
    process.execPath,
    [path.join(suite, 'run-pilot.ts')],
    {
      env: { ...process.env, OTEL_EVAL_RUN_DIR: root },
      encoding: 'utf8',
    }
  )
  assert.equal(result.status, 7, result.stderr)
  assert.equal(
    record(readJson(path.join(root, 'pilot-completion.json'))).exit,
    7
  )
})
