import fs from 'node:fs'
import path from 'node:path'
import { integer, record, runtimeRoot, SUITE } from './runtime.ts'

export function readManifest(file: string): unknown {
  return JSON.parse(
    fs.readFileSync(file, 'utf8'),
    (key: string, value: unknown, context?: { source?: string }) => {
      if (
        (key === 'startTimeUnixNano' || key === 'endTimeUnixNano') &&
        typeof value === 'number'
      ) {
        if (!context?.source || !/^-?\d+$/.test(context.source))
          throw new Error('Manifest timestamps require exact integer tokens')
        // Original numeric tokens keep manifest bounds exact without Number rounding.
        return context.source
      }
      return value
    }
  )
}

export function datasetSource(env = process.env, suite = SUITE) {
  return env.OTLP_DATASET || path.resolve(suite, '../../testdata/otlp/small')
}

export function requestList(value: unknown) {
  const requests = record(value).requests
  if (!Array.isArray(requests))
    throw new Error('Manifest requests must be an array')
  return requests.map(value => {
    const request = record(value)
    if (
      typeof request.file !== 'string' ||
      !request.file ||
      path.isAbsolute(request.file) ||
      request.file.split(/[\\/]/).some(part => part === '..' || part === '.')
    ) {
      throw new Error('Manifest request file must be a relative dataset path')
    }
    if (
      request.signal !== 'traces' &&
      request.signal !== 'logs' &&
      request.signal !== 'metrics'
    ) {
      throw new Error('Unknown manifest request signal')
    }
    return { file: request.file, signal: request.signal }
  })
}

export function datasetManifest(value: unknown) {
  const manifest = record(value)
  const counts = record(manifest.storedCounts)
  return {
    requests: requestList(manifest),
    start: integer(manifest.startTimeUnixNano),
    end: integer(manifest.endTimeUnixNano),
    counts: ['spans', 'logs', 'datapoints'].map(key => {
      const count = counts[key]
      if (
        typeof count !== 'number' ||
        !Number.isSafeInteger(count) ||
        count < 0
      ) {
        throw new Error(
          'Manifest stored counts must be nonnegative exact numbers'
        )
      }
      return count
    }),
  }
}

export function stageDataset(source: string, target: string) {
  const requests = requestList(readManifest(path.join(source, 'manifest.json')))
  fs.mkdirSync(target)
  fs.copyFileSync(
    path.join(source, 'manifest.json'),
    path.join(target, 'manifest.json'),
    fs.constants.COPYFILE_EXCL
  )
  for (const request of requests) {
    const destination = path.join(target, request.file)
    fs.mkdirSync(path.dirname(destination), { recursive: true })
    fs.copyFileSync(
      path.join(source, request.file),
      destination,
      fs.constants.COPYFILE_EXCL
    )
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === import.meta.filename) {
  stageDataset(datasetSource(), path.join(runtimeRoot(), 'fixture'))
  console.log('Shared OTLP dataset staged; no server or model started.')
}
