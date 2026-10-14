import fs from 'node:fs'
import path from 'node:path'
import config from './config.ts'
import { runtimeRoot, saveJson } from './runtime.ts'
import { afterAll } from './native-lifecycle.ts'

const promptfooPackage = 'promptfoo'
const { evaluate } = await import(promptfooPackage)

export async function runNativeEvaluation(suite = config, repeat = 3, resultName = 'pilot.json') {
  if (path.basename(resultName) !== resultName)
    throw new Error('Result filename must stay inside the run directory')
  try {
    const root = runtimeRoot()
    const result = await evaluate(suite, {
      cache: false,
      writeLatestResults: false,
      share: false,
      maxConcurrency: 1,
      repeat,
    })
    const summary = await result.toEvaluateSummary()
    // Hook replacement controls execution; Promptfoo still labels the inert provider.
    for (const row of summary.results) {
      const model = row.response?.metadata?.model ?? row.vars?.eval_model
      if (typeof model !== 'string') throw new Error('Missing saved attempt model')
      if (model !== row.vars?.eval_model) throw new Error('Saved attempt model mismatch')
      row.provider = { id: 'opencode:sdk:' + model, label: model }
    }
    saveJson(path.join(root, resultName), { results: summary })
    return fs.existsSync(path.join(root, 'harness-blocker.json')) ? 1 : 0
  } finally {
    await afterAll()
  }
}

if (process.argv[1] && fs.existsSync(process.argv[1]) && fs.realpathSync(process.argv[1]) === import.meta.filename) process.exitCode = await runNativeEvaluation()
