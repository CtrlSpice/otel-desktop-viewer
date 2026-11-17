import fs from 'node:fs'
import path from 'node:path'
import { readJson, record, runtimeRoot } from './runtime.ts'

export function modelExecutionErrors(root = runtimeRoot()) {
  const file = path.join(root, 'model-errors.json')
  if (!fs.existsSync(file)) return []
  const value = readJson(file)
  if (!Array.isArray(value)) throw new Error('Invalid model error ledger')
  return value.map(record)
}
