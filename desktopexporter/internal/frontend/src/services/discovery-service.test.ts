import { afterEach, expect, it, vi } from 'vitest'
import { telemetryAPI } from './telemetry-service'
import { stubRpcResult } from '@/test/telemetry-service-fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('requests only a scalar service count, including counts beyond one summary page', async () => {
  stubRpcResult(250)
  const controller = new AbortController()
  expect(await telemetryAPI.getServiceCount(controller.signal)).toBe(250)
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(fetch).toHaveBeenCalledWith(
    '/rpc',
    expect.objectContaining({ signal: controller.signal })
  )
  const request = JSON.parse(vi.mocked(fetch).mock.calls[0][1]!.body as string)
  expect(request.method).toBe('getServiceCount')
  expect(request.params).toBeUndefined()
})
