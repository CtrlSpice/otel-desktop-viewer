import fs from 'node:fs'
import path from 'node:path'
import { readJson, record } from './runtime.ts'

export function readIsolationCheckSettings(
  file = process.env.OTEL_EVAL_ISOLATION_CHECK_FILE
) {
  if (!file || !path.isAbsolute(file) || fs.statSync(file).mode & 0o077)
    throw new Error(
      'OTEL_EVAL_ISOLATION_CHECK_FILE must reference a private absolute JSON file'
    )

  let settings: Record<string, unknown>
  try {
    settings = record(readJson(file))
  } catch {
    throw new Error('Cannot parse isolation check settings')
  }
  const absolutePath = (key: string) => {
    const value = settings[key]
    if (typeof value !== 'string' || !path.isAbsolute(value))
      throw new Error(`Isolation check ${key} must be an absolute path`)
    return value
  }
  const forbiddenMarkers = settings.forbiddenMarkers
  if (
    !Array.isArray(forbiddenMarkers) ||
    !forbiddenMarkers.every(value => typeof value === 'string')
  )
    throw new Error(
      'Isolation check forbiddenMarkers must be an array of nonempty strings'
    )
  if (forbiddenMarkers.some(value => value.length === 0))
    throw new Error(
      'Isolation check forbiddenMarkers must be an array of nonempty strings'
    )

  return {
    authFile: absolutePath('authFile'),
    personalConfig: absolutePath('personalConfig'),
    originalSuite: absolutePath('originalSuite'),
    opencodeBinary: absolutePath('opencodeBinary'),
    viewerBinary: absolutePath('viewerBinary'),
    forbiddenMarkers,
  }
}
