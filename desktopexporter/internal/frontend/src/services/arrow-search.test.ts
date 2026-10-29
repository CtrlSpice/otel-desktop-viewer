import { expect, it, vi } from 'vitest'
import { ArrowFrame } from './arrow-data'
import {
  attributeDefinitions,
  attributeMatch,
  attributeSampleText,
} from './arrow-search'
import { telemetryDataset } from '@/test/native-telemetry-fixtures'

it('retains typed nested path segments and metadata ownership', () => {
  const frame = new ArrowFrame(
    telemetryDataset('getMetricAttributeDefinitions', [
      {
        name: '',
        owner: 'datapoint',
        attributeScope: 'datapoint',
        path: ['attributes', 'nested', 0, '0', ''],
        type: 'int64',
      },
      {
        name: 'revision',
        owner: 'metric',
        attributeScope: 'metadata',
        path: ['metadata', 'revision'],
        type: 'string',
      },
    ])
  )
  const fields = attributeDefinitions(frame)
  expect(fields[0]).toMatchObject({
    owner: 'datapoint',
    path: ['attributes', 'nested', 0, '0', ''],
    type: 'int64',
  })
  expect(fields[1]).toMatchObject({
    owner: 'metric',
    path: ['metadata', 'revision'],
    type: 'string',
  })
})

it('opens selected samples lazily and preserves exact completion spelling', () => {
  const frame = new ArrowFrame(
    telemetryDataset('searchAttributeMatches', [
      {
        name: 'value',
        owner: 'span',
        attributeScope: 'span',
        path: ['attributes', 'value'],
        type: 'int64',
        matchCount: 8,
        sampleValues: [
          { kind: 'int64', value: '9007199254740993' },
          { kind: 'double', value: '0x8000000000000000' },
          { kind: 'map', value: [] },
        ],
      },
    ])
  )
  const read = vi.spyOn(frame, 'cell')
  const match = attributeMatch(frame, 0)
  expect(read).not.toHaveBeenCalled()
  expect(match.matchCount).toBe(8)
  expect(read).not.toHaveBeenCalledWith('sampleValues', 0)
  const samples = match.sampleValues
  expect(samples[0]).toEqual({ kind: 'int64', value: 9007199254740993n })
  expect(Object.is(samples[1].value, -0)).toBe(true)
  expect(samples.flatMap(attributeSampleText)).toEqual([
    '9007199254740993',
    '0x8000000000000000',
  ])
  expect(attributeSampleText({ kind: 'double', value: NaN })).toEqual([
    '0x7ff8000000000000',
  ])
  expect(attributeSampleText({ kind: 'double', value: Infinity })).toEqual([
    '0x7ff0000000000000',
  ])
})
