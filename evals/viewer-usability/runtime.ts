import fs from 'node:fs'
import path from 'node:path'
import { tmpdir } from 'node:os'

export const SUITE = import.meta.dirname
export const TEMPORARY = tmpdir()

export function runtimeRoot() {
  const configured = process.env.OTEL_EVAL_RUN_DIR
  if (!configured || !path.isAbsolute(configured)) {
    throw new Error(
      'Set OTEL_EVAL_RUN_DIR to an existing absolute directory outside any repository.'
    )
  }
  const root = fs.realpathSync(configured)
  if (!fs.statSync(root).isDirectory())
    throw new Error('OTEL_EVAL_RUN_DIR must be a directory.')
  for (let parent = root; ; parent = path.dirname(parent)) {
    if (fs.existsSync(path.join(parent, '.git'))) {
      throw new Error(
        'Evaluation runtime and agent directories must be outside repositories.'
      )
    }
    if (parent === path.dirname(parent)) break
  }
  return root
}

export function record(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('Expected a JSON object')
  }
  return Object.fromEntries(Object.entries(value))
}

export function readJson(file: string): unknown {
  return JSON.parse(fs.readFileSync(file, 'utf8'))
}

export function integer(value: unknown): bigint {
  if (typeof value === 'string' && /^-?\d+$/.test(value)) return BigInt(value)
  if (typeof value === 'number' && Number.isSafeInteger(value))
    return BigInt(value)
  throw new Error('Expected an exact integer')
}

export function saveJson(file: string, value: unknown) {
  fs.writeFileSync(file, JSON.stringify(value, null, 2) + '\n', {
    flag: 'wx',
    mode: 0o600,
  })
}
