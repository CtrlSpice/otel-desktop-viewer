import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { normalize, verifyMetricFacts } from './verify.ts'
import { datasetSource } from './stage-dataset.ts'
import { record } from './runtime.ts'

test('SQL answer comparison ignores row order and nested object key order', () => {
  const received = [
    {
      a: 1,
      b: 2,
      nested: {
        exact: '18446744073709551615',
        time: '1791360000123456789',
      },
    },
    { a: 2, b: 1 },
  ]
  const expected = [
    { b: 1, a: 2 },
    {
      nested: {
        time: '1791360000123456789',
        exact: '18446744073709551615',
      },
      b: 2,
      a: 1,
    },
  ]
  assert.deepEqual(normalize(received), normalize(expected))
  assert.deepEqual(
    normalize([
      { a: 1, b: 2 },
      { a: 2, b: 1 },
    ]),
    normalize([
      { b: 2, a: 1 },
      { b: 1, a: 2 },
    ])
  )
  assert.notDeepEqual(normalize([1, 1, 2]), normalize([1, 2, 2]))
  assert.deepEqual(received[0].nested, {
    exact: '18446744073709551615',
    time: '1791360000123456789',
  })
})

test('metric references verify spike/recovery, full cumulative interval and sparse CPU directly', () => {
  const document = record(
    JSON.parse(
      fs.readFileSync(path.join(datasetSource(), 'metrics.json'), 'utf8')
    )
  )
  const metrics = (document.resourceMetrics as unknown[])
    .map(record)
    .flatMap(resource =>
      (resource.scopeMetrics as unknown[])
        .map(record)
        .flatMap(scope => (scope.metrics as unknown[]).map(record))
    )
  const start = '1791359675000000000',
    end = '1791360275000000000'
  verifyMetricFacts(metrics, start, end)
  const wrongTotal = structuredClone(metrics)
  const sum = record(wrongTotal.find(metric => metric.sum)?.sum)
  const points = sum.dataPoints as Record<string, unknown>[]
  points.at(-1)!.asInt = '101'
  assert.throws(() => verifyMetricFacts(wrongTotal, start, end))
  const wrongCPU = structuredClone(metrics)
  const gauge = record(wrongCPU.find(metric => metric.gauge)?.gauge)
  const cpu = gauge.dataPoints as Record<string, unknown>[]
  cpu[1].timeUnixNano = '1791360004500000000'
  assert.throws(() => verifyMetricFacts(wrongCPU, start, end))
})
