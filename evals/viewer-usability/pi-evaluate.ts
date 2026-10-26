import fs from 'node:fs'
import path from 'node:path'
import { parseArgs } from 'node:util'
import { readJson, record, runtimeRoot } from './runtime.ts'
import { cancelPiAttempts } from './pi-provider.ts'
import { cancelClaudeAttempts } from './claude-provider.ts'
import { runCancellation } from './run-cancellation.ts'

const { values } = parseArgs({ options: { task: { type: 'string' }, model: { type: 'string' } } })
const root = runtimeRoot()
for (const file of ['promptfoo.json', 'promptfoo-rows.jsonl', 'runs'])
  if (fs.existsSync(path.join(root, file))) throw new Error('Use a fresh run directory; existing evidence must not be overwritten')
process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
const { default: config } = await import('./config.ts')
const tests = values.task ? config.tests.filter(test => test.vars.task_id === values.task) : config.tests
if (!tests.length) throw new Error('Unknown task: ' + values.task)
const providers = values.model ? config.providers.filter(provider => provider.config.model === values.model) : config.providers
if (!providers.length) throw new Error('Unknown configured model: ' + values.model)
const promptfooPackage = 'promptfoo'
const { evaluate } = await import(promptfooPackage)
const cancellation = runCancellation()
let cleanup: Promise<unknown> | undefined
const cancel = () => {
  cleanup ??= Promise.all([cancelPiAttempts(), cancelClaudeAttempts()])
  void cleanup.catch(() => {}) // Await and report cleanup failures in finally.
}
cancellation.signal.addEventListener('abort', cancel, { once: true })
try {
  // Promptfoo owns provider × test × repetition expansion and its native export.
  await evaluate({ ...config, tests, providers }, { ...config.evaluateOptions, share: false, abortSignal: cancellation.signal })
} finally {
  await (cleanup ?? Promise.all([cancelPiAttempts(), cancelClaudeAttempts()]))
  cancellation.signal.removeEventListener('abort', cancel)
  cancellation.dispose()
}
console.log('Native Promptfoo export: ' + path.join(root, 'promptfoo.json'))
const rows = record(record(readJson(path.join(root, 'promptfoo.json'))).results).results
if (!Array.isArray(rows)) throw new Error('Missing native result rows')
if (cancellation.signal.aborted || rows.some(value => {
  const row = record(value)
  return row.failureReason === 2 || record(row.response ?? {}).error
})) process.exitCode = 1
