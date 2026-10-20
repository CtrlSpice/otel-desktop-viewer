import fs from 'node:fs'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { readJson, record, runtimeRoot, saveJson, SUITE } from './runtime.ts'
import { freezePricing } from './pricing.ts'
import type { PricingSnapshot } from './pricing.ts'
import { modelSummaries, modelSummaryTable } from './model-summary.ts'
import { cleanEnvironment, observerEnvironment } from './clean-environment.ts'
import { runCancellation, stopOwnedGroup } from './run-cancellation.ts'

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
    '| Model | Task | Completion | Score | Seconds | Tool calls | Utility entries | Query entries | Failed tools |',
    '| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |',
  ]
  for (const value of rows) {
    const row = record(value)
    const response = row.response === undefined ? {} : record(row.response)
    const metadata =
      response.metadata === undefined ? {} : record(response.metadata)
    const seconds = metadata.elapsedSeconds
    if (seconds !== undefined && typeof seconds !== 'number')
      throw new Error('Invalid pilot elapsed seconds')
    const executionError = response.error ||
      row.failureReason === 2 ||
      (row.error && row.failureReason !== 1 && !row.gradingResult)
    const score = executionError || !row.gradingResult
      ? undefined
      : record(row.gradingResult).score
    const cells = [
      record(row.provider).label,
      record(row.vars).task_id,
      executionError ? 'error' : score === 0.5 ? 'partial' : row.success ? 'pass' : 'fail',
      score ?? 'N/A',
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
  onHarnessBlocker?: () => Promise<void>,
  signal?: AbortSignal
) {
  const cancellation = runCancellation(signal)
  try {
    return await pilot(root, suite, suppliedPricing, onHarnessBlocker, cancellation.signal)
  } finally {
    cancellation.dispose()
  }
}

async function pilot(
  root: string,
  suite: string,
  suppliedPricing: PricingSnapshot | undefined,
  onHarnessBlocker: (() => Promise<void>) | undefined,
  signal: AbortSignal
) {
  signal.throwIfAborted()
  if (fs.existsSync(path.join(root, 'harness-blocker.json')))
    throw new Error('Evaluation already stopped by a harness blocker')
  if (fs.existsSync(path.join(root, 'pilot.json')))
    throw new Error('pilot.json exists; refusing to overwrite')
  const pricing = await freezePricing(root, suite, suppliedPricing)
  signal.throwIfAborted()
  const clean = cleanEnvironment({ root, authEnvironment: {} })
  for (const key of ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR', 'OPENCODE_CONFIG_DIR', 'OPENCODE_TEST_MANAGED_CONFIG_DIR']) fs.mkdirSync(clean[key], { recursive: true, mode: 0o700 })
  const env = {
    ...clean,
    ...observerEnvironment(),
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
  let viewerShutdown: Promise<void> | undefined
  const native = new Set<number>()
  const stopping = new Map<number, Promise<void>>()
  const stop = (pid: number) => {
    if (!stopping.has(pid)) stopping.set(pid, stopOwnedGroup(pid, pid === child.pid ? child : undefined).catch(error => {
      failure ??= error instanceof Error ? error : new Error('Owned process shutdown failed')
    }))
    return stopping.get(pid)!
  }
  const shutdown = () => {
    if (child.pid) void stop(child.pid)
    for (const pid of native) void stop(pid)
    viewerShutdown ??= onHarnessBlocker?.().catch(error => {
      failure ??= error instanceof Error ? error : new Error('Owned viewer shutdown failed')
    })
  }
  signal.addEventListener('abort', shutdown, { once: true })
  if (signal.aborted) shutdown()
  child.on('message', message => {
    let event
    try {
      event = record(message)
    } catch {
      return
    }
    if (event.runnerPID !== child.pid || event.root !== fs.realpathSync(root)) return
    if (event.type === 'otel-eval-native-start' && Number.isSafeInteger(event.sessionPID) && Number(event.sessionPID) > 0) {
      native.add(Number(event.sessionPID))
      if (signal.aborted || blocked) void stop(Number(event.sessionPID))
      return
    }
    if (event.type === 'otel-eval-native-stop') {
      native.delete(Number(event.sessionPID))
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
    native.add(Number(event.sessionPID))
    shutdown()
  })
  child.once('error', error => {
    failure = error
  })
  const exit = await new Promise<number | null>(resolve =>
    child.once('close', resolve)
  )
  // An unexpectedly exited worker can also leave a detached attempt behind.
  for (const pid of native) void stop(pid)
  await Promise.all(stopping.values())
  await viewerShutdown
  signal.removeEventListener('abort', shutdown)
  blocked ||= fs.existsSync(path.join(root, 'harness-blocker.json'))
  saveJson(path.join(root, 'pilot-completion.json'), {
    command,
    exit,
    elapsedSeconds: (performance.now() - started) / 1000,
    error: failure?.message,
    ...(signal.aborted ? { status: 'cancelled', cancellation: String(signal.reason) } : {}),
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
  return blocked || signal.aborted ? 1 : exit
}

if (
  process.argv[1] &&
  fs.realpathSync(process.argv[1]) === import.meta.filename
)
  process.exitCode = (await runPilot()) ?? 1
