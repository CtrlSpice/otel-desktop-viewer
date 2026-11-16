import { afterEach, expect, it, vi } from 'vitest'
import { telemetryAPI } from './telemetry-service'
import { stubRpcResult } from '@/test/telemetry-service-fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('reads ordered field completions and empty results through native transport', async () => {
  const values = ['quote"\\%_雪', 'second', '']
  stubRpcResult(values)
  expect(
    await telemetryAPI.getFieldValueCompletions('traces', 'name', '%_', 10)
  ).toEqual(values)
  expect(vi.mocked(fetch).mock.calls[0][0]).toBe(
    '/api/data/getFieldValueCompletions'
  )
  stubRpcResult([])
  expect(
    await telemetryAPI.getFieldValueCompletions('traces', 'name', 'absent', 10)
  ).toEqual([])
})
