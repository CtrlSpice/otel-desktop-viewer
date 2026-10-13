import fs from 'node:fs'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { readJson, record, runtimeRoot, saveJson, SUITE } from './runtime.ts'
import { freezePricing } from './pricing.ts'
import type { PricingSnapshot } from './pricing.ts'
import { modelSummaries, modelSummaryTable } from './model-summary.ts'

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
  suppliedPricing?: PricingSnapshot
) {
  if (fs.existsSync(path.join(root, 'pilot.json')))
    throw new Error('pilot.json exists; refusing to overwrite')
  const pricing = await freezePricing(root, suite, suppliedPricing)
  const env = {
    ...process.env,
    PROMPTFOO_DISABLE_TELEMETRY: '1',
    PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'),
  }
  const command = [
    path.join(suite, 'node_modules/.bin/promptfoo'),
    'eval',
    '-c',
    path.join(suite, 'config.ts'),
    '--no-cache',
    '--repeat',
    '3',
    '--no-share',
    '-o',
    path.join(root, 'pilot.json'),
    '--no-progress-bar',
    '--no-table',
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
    stdio: ['ignore', out, err],
  })
  fs.closeSync(out)
  fs.closeSync(err)
  let failure: Error | undefined
  child.once('error', error => {
    failure = error
  })
  const exit = await new Promise<number | null>(resolve =>
    child.once('close', resolve)
  )
  saveJson(path.join(root, 'pilot-completion.json'), {
    command,
    exit,
    elapsedSeconds: (performance.now() - started) / 1000,
    error: failure?.message,
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
  return exit
}

if (
  process.argv[1] &&
  fs.realpathSync(process.argv[1]) === import.meta.filename
)
  process.exitCode = (await runPilot()) ?? 1
