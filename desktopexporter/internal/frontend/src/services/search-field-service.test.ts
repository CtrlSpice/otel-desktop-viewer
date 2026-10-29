import { afterEach, expect, it, vi } from 'vitest'
import { getDynamicAttributes } from './search-field-service'
import { telemetryAPI } from './telemetry-service'
import type { FieldDefinition } from '@/search/model'

afterEach(() => vi.restoreAllMocks())

it('shares in-flight discovery, then fetches fresh definitions for later requests', async () => {
  let resolve!: (fields: FieldDefinition[]) => void
  const fetch = vi
    .spyOn(telemetryAPI, 'getTraceAttributeDefinitions')
    .mockImplementationOnce(
      () =>
        new Promise(done => {
          resolve = done
        })
    )
    .mockResolvedValue([])
  const first = getDynamicAttributes('traces')
  const second = getDynamicAttributes('traces')
  expect(second).toBe(first)
  expect(fetch).toHaveBeenCalledTimes(1)
  resolve([])
  await Promise.all([first, second])
  await getDynamicAttributes('traces')
  expect(fetch).toHaveBeenCalledTimes(2)
})

it('does not retain a failed request or mix signal catalogues', async () => {
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  const traces = vi
    .spyOn(telemetryAPI, 'getTraceAttributeDefinitions')
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValue([])
  const logs = vi
    .spyOn(telemetryAPI, 'getLogAttributeDefinitions')
    .mockResolvedValue([])
  await Promise.all([
    getDynamicAttributes('traces'),
    getDynamicAttributes('logs'),
  ])
  await getDynamicAttributes('traces')
  expect(traces).toHaveBeenCalledTimes(2)
  expect(logs).toHaveBeenCalledTimes(1)
})
