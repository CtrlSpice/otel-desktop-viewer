import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { cleanEnvironment } from './clean-environment.ts'
import { readJson, record, SUITE, TEMPORARY } from './runtime.ts'

test('pinned loader scenarios retain isolated evidence and stop after the first grading error', {
  skip: !fs.existsSync(path.join(SUITE, 'node_modules/promptfoo/package.json')) && 'Pinned Promptfoo dependencies are not installed',
  timeout: 180000,
}, () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-loader-regression-'))
  const result = spawnSync(process.execPath, [path.join(SUITE, 'verify-loader.ts')], {
    env: { ...cleanEnvironment({ root, inherited: {} }), TMPDIR: root, PROMPTFOO_DISABLE_TELEMETRY: '1' },
    encoding: 'utf8', timeout: 170000,
  })
  fs.writeFileSync(path.join(root, 'stdout.log'), result.stdout || '', { mode: 0o600 })
  fs.writeFileSync(path.join(root, 'stderr.log'), result.stderr || '', { mode: 0o600 })
  assert.equal(result.error, undefined)
  assert.equal(result.status, 0, result.stderr + '\nEvidence: ' + root)
  const scenarios = result.stdout.trim().split('\n').map(line => record(JSON.parse(line)))
  assert.equal(scenarios.length, 4)
  const roots = scenarios.map(scenario => String(scenario.evidence))
  assert.equal(new Set(roots).size, 4)
  const proof = record(readJson(path.join(roots[0], 'loader-proof.json')))
  assert.equal(proof.syntheticExecutionErrors, 1)
  assert.equal(proof.unexecutedErrorSlots, 3)
  assert.equal(proof.investigatorResponsesRetained, 13)
  for (const directory of roots) {
    assert.ok(fs.existsSync(path.join(directory, 'evaluation-batches/batch-001.json')))
    assert.ok(fs.readdirSync(directory).some(name => name.endsWith('.stdout')))
    assert.ok(fs.readdirSync(directory).some(name => name.endsWith('.stderr')))
  }
})
