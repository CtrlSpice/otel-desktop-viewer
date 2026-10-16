import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { spawn, spawnSync } from 'node:child_process'
import { once } from 'node:events'
import { permissionStopFixture } from './permission-stop-fixture.ts'
import { viewerSkillFixture } from './test-fixture.ts'
import { runPilot } from './run-pilot.ts'
import { stageDataset, datasetSource } from './stage-dataset.ts'
import { pricingSnapshot } from './pricing.ts'
import apiPrices from './api-prices.json' with { type: 'json' }
import { readJson, record, saveJson, SUITE } from './runtime.ts'

const pricing = pricingSnapshot({
  capturedAt: '2026-10-08T06:00:00Z',
  api: apiPrices,
  exchange: { source: 'synthetic-rate', date: '2026-10-07', cadPerUsd: 1.4257 },
})

function suite(root: string, isolation: unknown, goals: string[]) {
  const directory = path.join(root, 'synthetic-suite')
  fs.mkdirSync(directory)
  fs.symlinkSync(
    path.join(SUITE, 'node_modules'),
    path.join(directory, 'node_modules'),
    'dir'
  )
  fs.writeFileSync(path.join(directory, 'package.json'), '{"type":"module"}')
  const config = {
    prompts: ['{{goal}}\nViewer executable: {{binary}}'],
    providers: [
      {
        id: pathToFileURL(path.join(SUITE, 'provider.ts')).href,
        config: { model: 'openai/gpt-6.1-sol', isolation },
      },
    ],
    evaluateOptions: { maxConcurrency: 1 },
    defaultTest: {
      vars: { binary: path.join(root, 'otel-desktop-viewer') },
      assert: [
        {
          type: 'javascript',
          value: pathToFileURL(path.join(SUITE, 'grade.ts')).href,
        },
      ],
    },
    tests: goals.map(goal => ({ vars: { goal, task_id: 'checkout-failure' } })),
  }
  fs.writeFileSync(
    path.join(directory, 'config.ts'),
    'export default ' + JSON.stringify(config)
  )
  saveJson(path.join(root, 'isolation.json'), isolation)
  process.env.OTEL_EVAL_ISOLATION_FILE = path.join(root, 'isolation.json')
  return directory
}

test('pinned Promptfoo stops scheduling and grading after a split viewer denial and stops/waits its owned viewer', async () => {
  const previous = { ...process.env }
  try {
    const { root, isolation } = permissionStopFixture({
      mode: 'split',
      blockGoal: 'BLOCK_VIEWER_HELP',
      descendant: true,
    })
    const binary = process.env.OTEL_EVAL_VIEWER_BINARY
    assert.ok(
      binary && path.isAbsolute(binary),
      'Set absolute OTEL_EVAL_VIEWER_BINARY'
    )
    fs.copyFileSync(binary, path.join(root, 'otel-desktop-viewer'))
    stageDataset(datasetSource(), path.join(root, 'fixture'))
    const directory = suite(root, isolation, [
      'COMPLETED_FIRST',
      'BLOCK_VIEWER_HELP',
      'MUST_NOT_START',
    ])
    assert.equal(
      record(readJson(path.join(SUITE, 'node_modules/promptfoo/package.json')))
        .version,
      '0.124.1'
    )
    saveJson(path.join(root, 'pricing-snapshot.json'), pricing)
    const runner = path.join(directory, 'runner.ts')
    fs.writeFileSync(
      runner,
      `import {prepare} from ${JSON.stringify(pathToFileURL(path.join(SUITE, 'prepare.ts')).href)};
import {runPilot} from ${JSON.stringify(pathToFileURL(path.join(SUITE, 'run-pilot.ts')).href)};
import {ownedViewerStop} from ${JSON.stringify(pathToFileURL(path.join(SUITE, 'owned-viewer-stop.ts')).href)};
const root=${JSON.stringify(root)};
const ready=await prepare(root),stop=ownedViewerStop(root,ready);
try {process.exitCode=(await runPilot(root,${JSON.stringify(directory)},undefined,stop))??1;}
finally {await stop();}
`
    )
    const child = spawn(process.execPath, [runner], {
      cwd: root,
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let stdout = '',
      stderr = ''
    child.stdout.on('data', data => {
      stdout += data
    })
    child.stderr.on('data', data => {
      stderr += data
    })
    const [exit] = await once(child, 'close')
    fs.writeFileSync(path.join(root, 'runner.stdout'), stdout, { mode: 0o600 })
    fs.writeFileSync(path.join(root, 'runner.stderr'), stderr, { mode: 0o600 })
    assert.equal(exit, 1, stderr)
    const started = fs
      .readFileSync(path.join(root, 'started.jsonl'), 'utf8')
      .trim()
      .split('\n')
      .map(line => JSON.parse(line))
    const blockedIndex = started.findIndex(
      entry => !entry.grader && entry.prompt.includes('BLOCK_VIEWER_HELP')
    )
    assert.ok(blockedIndex >= 0)
    assert.equal(
      started.length,
      blockedIndex + 1,
      'No investigator or grader starts after the blocked session'
    )
    assert.ok(
      started.slice(0, blockedIndex).some(entry => entry.grader),
      'Completed grading remains'
    )
    assert.equal(
      started.some(entry => entry.prompt.includes('MUST_NOT_START')),
      false
    )
    assert.equal(fs.existsSync(path.join(root, 'continued-tool-work')), false)
    const blocker = record(readJson(path.join(root, 'harness-blocker.json')))
    assert.equal(blocker.status, 'harness-blocker')
    assert.equal(blocker.role, 'investigator')
    assert.equal(blocker.model, 'openai/gpt-6.1-sol')
    assert.equal(blocker.task, 'checkout-failure')
    assert.equal(
      record(readJson(path.join(root, 'pilot-completion.json'))).status,
      'harness-blocker'
    )
    assert.throws(() => process.kill(Number(blocker.sessionPID), 0), /ESRCH/)
    const descendant = Number(
      fs.readFileSync(path.join(root, 'descendant.pid'), 'utf8')
    )
    const remaining = spawnSync(
      'ps',
      ['-p', String(descendant), '-o', 'stat='],
      { encoding: 'utf8' }
    )
    saveJson(path.join(root, 'descendant-shutdown.json'), {
      pid: descendant,
      status: remaining.status,
      state: remaining.stdout.trim(),
    })
    assert.ok(
      remaining.status === 1 || remaining.stdout.trim().startsWith('Z'),
      'Owned SIGTERM-resistant descendant is killed after the parent closes'
    )
    const shutdown = record(readJson(path.join(root, 'owned-shutdown.json')))
    assert.throws(() => process.kill(Number(shutdown.pid), 0), /ESRCH/)
    assert.equal(shutdown.waited, true)
    assert.ok(
      Date.parse(String(shutdown.completedAt)) -
        Date.parse(String(blocker.detectedAt)) <
        10000,
      'Viewer shutdown starts immediately, not after runner grace'
    )
    const blockedEvidence = String(blocker.evidence)
    assert.ok(
      fs
        .readFileSync(path.join(blockedEvidence, 'stdout.jsonl'), 'utf8')
        .endsWith('\n')
    )
    assert.ok(
      fs.existsSync(path.join(blockedEvidence, 'partial-commands.json'))
    )
    const runs = fs.readdirSync(path.join(root, 'runs'))
    assert.ok(
      runs.some(
        name =>
          name.startsWith('grader-') &&
          fs.existsSync(path.join(root, 'runs', name, 'grading.json'))
      )
    )
    assert.ok(
      runs.some(
        name =>
          name.startsWith('investigator-') &&
          fs.existsSync(path.join(root, 'runs', name, 'answer.txt'))
      )
    )
    console.log('Pinned Promptfoo abort evidence: ' + root)
    fs.unlinkSync(path.join(root, 'otel-desktop-viewer'))
    for (const name of fs.readdirSync(path.join(root, 'workspaces'))) {
      const copy = path.join(
        root,
        'workspaces',
        name,
        'scratch/bin/otel-desktop-viewer'
      )
      if (fs.existsSync(copy)) fs.unlinkSync(copy)
    }
  } finally {
    process.env = previous
  }
})

test('pinned Promptfoo continues and grades protected/lifecycle denials, ordinary errors and quoted denials', async () => {
  const previous = { ...process.env }
  try {
    for (const mode of [
      'protected',
      'lifecycle',
      'not-found',
      'search-empty',
      'rpc-error',
      'quoted',
    ]) {
      const { root, isolation } = permissionStopFixture({ mode })
      const binary = path.join(root, 'otel-desktop-viewer')
      fs.copyFileSync(viewerSkillFixture(root), binary)
      saveJson(path.join(root, 'connection.json'), { binary })
      const directory = suite(root, isolation, ['SYNTHETIC_CONTINUE'])
      assert.equal(await runPilot(root, directory, pricing), 0, mode)
      assert.equal(
        fs.existsSync(path.join(root, 'harness-blocker.json')),
        false,
        mode
      )
      const started = fs
        .readFileSync(path.join(root, 'started.jsonl'), 'utf8')
        .trim()
        .split('\n')
        .map(line => JSON.parse(line))
      assert.equal(started.filter(entry => entry.grader).length, 3, mode)
      assert.equal(started.filter(entry => !entry.grader).length, 3, mode)
      console.log('Pinned Promptfoo continue evidence (' + mode + '): ' + root)
    }
  } finally {
    process.env = previous
  }
})
