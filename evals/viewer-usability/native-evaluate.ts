import fs from 'node:fs'
import path from 'node:path'
import config from './config.ts'
import { readJson, record, runtimeRoot, saveJson } from './runtime.ts'
import { afterAll } from './native-lifecycle.ts'

const promptfooPackage = 'promptfoo'
const { evaluate } = await import(promptfooPackage)

export async function runNativeEvaluation(suite = config, repeat = 3, resultName = 'pilot.json') {
  if (path.basename(resultName) !== resultName)
    throw new Error('Result filename must stay inside the run directory')
  try {
    const root = runtimeRoot()
    const stop = new AbortController()
    const result = await evaluate(suite, {
      cache: false,
      writeLatestResults: false,
      share: false,
      maxConcurrency: 1,
      repeat,
      abortSignal: stop.signal,
      // Promptfoo invokes this after the row and its grading evidence are saved.
      progressCallback: () => {
        if (fs.existsSync(path.join(root, 'execution-error.json'))) stop.abort()
      },
    })
    const summary = await result.toEvaluateSummary()
    const executionErrorFile = path.join(root, 'execution-error.json')
    const executionError = fs.existsSync(executionErrorFile) ? record(readJson(executionErrorFile)) : undefined
    // Hook replacement controls execution; Promptfoo still labels the inert provider.
    for (const row of summary.results) {
      const model = row.response?.metadata?.model ?? row.vars?.eval_model
      if (typeof model !== 'string') throw new Error('Missing saved attempt model')
      if (model !== row.vars?.eval_model) throw new Error('Saved attempt model mismatch')
      row.provider = { id: 'opencode:sdk:' + model, label: model }
      // Promptfoo treats thrown JavaScript assertions as failed answers. Preserve
      // our explicit grader execution failure as an error in the saved result.
      if (row === summary.results.at(-1) && executionError?.role === 'grader' && executionError.model === model && executionError.task === row.vars?.task_id) {
        row.failureReason = 2
        row.success = false
      }
    }
    saveJson(path.join(root, resultName), { results: summary })
    return ['harness-blocker.json', 'execution-error.json'].some(file => fs.existsSync(path.join(root, file))) ? 1 : 0
  } finally {
    await afterAll()
  }
}

if (process.argv[1] && fs.existsSync(process.argv[1]) && fs.realpathSync(process.argv[1]) === import.meta.filename) process.exitCode = await runNativeEvaluation()
