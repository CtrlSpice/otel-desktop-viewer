import { afterEach, expect, it, vi } from 'vitest'
import { getDynamicAttributes } from './search-field-service'
import { telemetryAPI } from './telemetry-service'
import type { FieldDefinition } from '@/search/model'

afterEach(() => vi.restoreAllMocks())

it('keeps discovery abort signals with each editor and fetches fresh definitions', async () => {
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
  const firstOwner = new AbortController()
  const secondOwner = new AbortController()
  const first = getDynamicAttributes('traces', firstOwner.signal)
  const second = getDynamicAttributes('traces', secondOwner.signal)
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(fetch.mock.calls.map(call => call[0])).toEqual([
    firstOwner.signal,
    secondOwner.signal,
  ])
  resolve([])
  await Promise.all([first, second])
  await getDynamicAttributes('traces')
  expect(fetch).toHaveBeenCalledTimes(3)
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
