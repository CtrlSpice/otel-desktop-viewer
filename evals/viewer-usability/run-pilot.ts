import fs from 'node:fs'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { readJson, record, runtimeRoot, saveJson, SUITE } from './runtime.ts'

export function pilotSummary(value: unknown) {
  const rows = record(record(value).results).results
  if (!Array.isArray(rows)) throw new Error('Invalid pilot result rows')
  const lines = [
    '# Pilot results',
    '',
    'Automated fact checks only; transcript interpretation pending.',
    '',
    '| Model | Task | Facts | Seconds | Tool calls | Utility entries | Query entries | Failed tools |',
    '| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |',
  ]
  for (const value of rows) {
    const row = record(value)
    const response = row.response === undefined ? {} : record(row.response)
    const metadata =
      response.metadata === undefined ? {} : record(response.metadata)
    const seconds = metadata.elapsedSeconds ?? 0
    if (typeof seconds !== 'number')
      throw new Error('Invalid pilot elapsed seconds')
    const cells = [
      record(row.provider).label,
      record(row.vars).task_id,
      row.success ? 'pass' : response.error ? 'error' : 'fail',
      Math.round(seconds * 100) / 100,
      metadata.toolCalls ?? '?',
      metadata.utilityCommandEntries ?? '?',
      metadata.queryCommandEntries ?? '?',
      metadata.failedToolCalls ?? '?',
    ]
    lines.push('| ' + cells.map(String).join(' | ') + ' |')
  }
  return lines.join('\n') + '\n'
}

export async function runPilot(root = runtimeRoot(), suite = SUITE) {
  if (fs.existsSync(path.join(root, 'pilot.json')))
    throw new Error('pilot.json exists; refusing to overwrite')
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
    fs.writeFileSync(
      path.join(root, 'pilot-summary.md'),
      pilotSummary(readJson(path.join(root, 'pilot.json'))),
      { flag: 'wx', mode: 0o600 }
    )
  }
  if (failure) throw failure
  return exit
}

if (process.argv[1] && path.resolve(process.argv[1]) === import.meta.filename)
  await runPilot()
