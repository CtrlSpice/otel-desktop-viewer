import fs from 'node:fs'
import path from 'node:path'
import { readJson, record, runtimeRoot, saveJson } from './runtime.ts'

// A model pause is distinct from a failure of the shared harness or grader.
export function modelExecutionErrors(root = runtimeRoot()) {
  const file = path.join(root, 'model-errors.json')
  if (!fs.existsSync(file)) return []
  const value = readJson(file)
  if (!Array.isArray(value)) throw new Error('Invalid model error ledger')
  return value.map(record)
}

export function recordExecutionError(error: {
  model: unknown
  task: unknown
  evidence: unknown
  modelFailure: boolean
}) {
  const root = runtimeRoot()
  const entry = { status: 'execution-error', role: 'investigator', model: error.model, task: error.task, evidence: error.evidence }
  if (error.modelFailure && typeof error.model === 'string' && !fs.existsSync(path.join(root, 'harness-blocker.json'))) {
    const errors = modelExecutionErrors(root)
    if (errors.some(item => item.model === error.model)) return
    const file = path.join(root, 'model-errors.json')
    const temporary = file + '.tmp'
    saveJson(temporary, [...errors, entry])
    fs.renameSync(temporary, file)
  } else {
    const file = path.join(root, 'execution-error.json')
    if (!fs.existsSync(file)) saveJson(file, entry)
  }
}
