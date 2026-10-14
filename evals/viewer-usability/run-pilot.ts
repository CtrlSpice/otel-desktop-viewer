import fs from 'node:fs'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { readJson, record, runtimeRoot, saveJson, SUITE } from './runtime.ts'
import { freezePricing } from './pricing.ts'
import type { PricingSnapshot } from './pricing.ts'
import { modelSummaries, modelSummaryTable } from './model-summary.ts'
import { cleanEnvironment } from './clean-environment.ts'

export function pilotSummary(
  value: unknown,
  pricing?: PricingSnapshot,
  investigatorRows?: unknown[],
  graderRows: unknown[] = []
) {
  const rows = record(record(value).results).results
  if (!Array.isArray(rows)) throw new Error('Invalid pilot result rows')
  const lines = [
    '# Pilot results',
    '',
    'Separate Sol6.1 grading is provisional until comparison with human-reviewed answers.',
    '',
    '## Investigators',
    '',
    modelSummaryTable(investigatorRows ?? rows, pricing),
    '',
    '## Separate Grader',
    '',
    modelSummaryTable(graderRows, pricing),
    '',
    '## Task results',
    '',
    '| Model | Task | Facts | Seconds | Tool calls | Utility entries | Query entries | Failed tools |',
    '| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |',
  ]
  for (const value of rows) {
    const row = record(value)
    const response = row.response === undefined ? {} : record(row.response)
    const metadata =
      response.metadata === undefined ? {} : record(response.metadata)
    const seconds = metadata.elapsedSeconds
    if (seconds !== undefined && typeof seconds !== 'number')
      throw new Error('Invalid pilot elapsed seconds')
    const cells = [
      record(row.provider).label,
      record(row.vars).task_id,
      row.success
        ? 'pass'
        : response.error ||
            row.failureReason === 2 ||
            (row.error && row.failureReason !== 1 && !row.gradingResult)
          ? 'error'
          : 'fail',
      typeof seconds === 'number' ? Math.round(seconds * 100) / 100 : '?',
      metadata.toolCalls ?? '?',
      metadata.utilityCommandEntries ?? '?',
      metadata.queryCommandEntries ?? '?',
      metadata.failedToolCalls ?? '?',
    ]
    lines.push('| ' + cells.map(String).join(' | ') + ' |')
  }
  return lines.join('\n') + '\n'
}

export async function runPilot(
  root = runtimeRoot(),
  suite = SUITE,
  suppliedPricing?: PricingSnapshot,
  onHarnessBlocker?: () => Promise<void>
) {
  if (fs.existsSync(path.join(root, 'harness-blocker.json')))
    throw new Error('Evaluation already stopped by a harness blocker')
  if (fs.existsSync(path.join(root, 'pilot.json')))
    throw new Error('pilot.json exists; refusing to overwrite')
  const pricing = await freezePricing(root, suite, suppliedPricing)
  const clean = cleanEnvironment({ root, authEnvironment: {} })
  for (const key of ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR', 'OPENCODE_CONFIG_DIR', 'OPENCODE_TEST_MANAGED_CONFIG_DIR']) fs.mkdirSync(clean[key], { recursive: true, mode: 0o700 })
  const env = {
    ...clean,
    OTEL_EVAL_RUN_DIR: root,
    OTEL_EVAL_ISOLATION_FILE: process.env.OTEL_EVAL_ISOLATION_FILE,
    PROMPTFOO_DISABLE_TELEMETRY: '1',
    PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'),
  }
  const command = [
    process.execPath,
    path.join(suite, 'native-evaluate.ts'),
  ]
  const started = performance.now()
  const out = fs.openSync(path.join(root, 'pilot.stdout.log'), 'wx', 0o600)
  let err: number
  try {
    err = fs.openSync(path.join(root, 'pilot.stderr.log'), 'wx', 0o600)
  } catch (error) {
    fs.closeSync(out)
    throw error
  }
  const child = spawn(command[0], command.slice(1), {
    cwd: root,
    env,
    stdio: ['ignore', out, err, 'ipc'],
    detached: process.platform !== 'win32',
  })
  fs.closeSync(out)
  fs.closeSync(err)
  let failure: Error | undefined
  let blocked = false
  let cleanup: Promise<void> | undefined
  let viewerShutdown: Promise<void> | undefined
  child.on('message', message => {
    let event
    try {
      event = record(message)
    } catch {
      return
    }
    if (
      blocked ||
      event.type !== 'otel-eval-harness-blocker' ||
      event.runnerPID !== child.pid ||
      event.root !== fs.realpathSync(root) ||
      !Number.isSafeInteger(event.sessionPID) ||
      Number(event.sessionPID) <= 0 ||
      !fs.existsSync(path.join(root, 'harness-blocker.json'))
    )
      return
    const blocker = record(readJson(path.join(root, 'harness-blocker.json')))
    if (
      blocker.runnerPID !== child.pid ||
      blocker.sessionPID !== event.sessionPID ||
      blocker.status !== 'harness-blocker'
    )
      return
    blocked = true
    // Only the spawned runner and the session it reports owning are signalled.
    const signalOwned = (signal: NodeJS.Signals) => {
      for (const pid of [child.pid!, Number(event.sessionPID)]) {
        try {
          if (process.platform === 'win32') process.kill(pid, signal)
          else process.kill(-pid, signal)
        } catch (error) {
          if (!(
            error instanceof Error &&
            'code' in error &&
            error.code === 'ESRCH'
          ))
            throw error
        }
      }
    }
    viewerShutdown = onHarnessBlocker?.().catch(error => {
      failure =
        error instanceof Error
          ? error
          : new Error('Owned viewer shutdown failed')
    })
    signalOwned('SIGTERM')
    cleanup = new Promise(resolve => {
      setTimeout(() => {
        signalOwned('SIGKILL')
        resolve()
      }, 30000)
    })
  })
  child.once('error', error => {
    failure = error
  })
  const exit = await new Promise<number | null>(resolve =>
    child.once('close', resolve)
  )
  await cleanup
  await viewerShutdown
  blocked ||= fs.existsSync(path.join(root, 'harness-blocker.json'))
  saveJson(path.join(root, 'pilot-completion.json'), {
    command,
    exit,
    elapsedSeconds: (performance.now() - started) / 1000,
    error: failure?.message,
    ...(blocked
      ? {
          status: 'harness-blocker',
          harnessBlocker: path.join(root, 'harness-blocker.json'),
        }
      : {}),
  })
  if (fs.existsSync(path.join(root, 'pilot.json'))) {
    const result = readJson(path.join(root, 'pilot.json'))
    const rows = record(record(result).results).results
    if (!Array.isArray(rows)) throw new Error('Invalid pilot result rows')
    const runs = path.join(root, 'runs')
    // Promptfoo drops providerResponse when an assertion throws. Retained process
    // evidence still accounts for investigator usage on grading execution errors.
    const investigators = fs.existsSync(runs)
      ? fs
          .readdirSync(runs)
          .filter(name => name.startsWith('investigator-'))
          .map(name => {
            const metrics = path.join(runs, name, 'metrics.json')
            const file = fs.existsSync(metrics)
              ? metrics
              : path.join(runs, name, 'failure.json')
            return record(readJson(file))
          })
          .map(metadata => ({
            provider: { label: metadata.model },
            response: { metadata },
          }))
      : rows
    saveJson(path.join(root, 'model-summary.json'), {
      pricing,
      models: modelSummaries(investigators, pricing),
    })
    const grading = fs.existsSync(runs)
      ? fs
          .readdirSync(runs)
          .filter(name => name.startsWith('grader-'))
          .map(name => path.join(runs, name, 'grading.json'))
          .filter(file => fs.existsSync(file))
          .map(file => record(readJson(file)))
      : []
    const graderRows = grading.map(report => ({
      provider: { label: 'openai/gpt-6.1-sol' },
      response: { metadata: report.metadata },
    }))
    saveJson(path.join(root, 'grader-summary.json'), {
      provisional: true,
      sessions: grading.length,
      executionErrors: grading.filter(
        report => report.status === 'execution-error'
      ).length,
      models: modelSummaries(graderRows, pricing),
      judgments: grading,
    })
    fs.writeFileSync(
      path.join(root, 'pilot-summary.md'),
      pilotSummary(result, pricing, investigators, graderRows),
      { flag: 'wx', mode: 0o600 }
    )
  }
  if (failure) throw failure
  return blocked ? 1 : exit
}

if (
  process.argv[1] &&
  fs.realpathSync(process.argv[1]) === import.meta.filename
)
  process.exitCode = (await runPilot()) ?? 1
