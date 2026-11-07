import { afterEach, describe, expect, it, vi } from 'vitest'
import { Table, Int64, tableToIPC, vectorFromArray } from 'apache-arrow'
import { fetchArrow } from './arrow-fetch'
import {
  telemetryAPI,
  JsonRpcError,
  RequestAbortedError,
} from './telemetry-service'
import { arrowResponse } from '@/test/arrow-fixtures'

afterEach(() => vi.unstubAllGlobals())

const table = new Table({
  id: vectorFromArray([9007199254740993n], new Int64()),
})

describe('Arrow fetch lifecycle', () => {
  it('sends parameters directly and retains exact typed response columns', async () => {
    const fetch = vi.fn().mockResolvedValue(arrowResponse(table))
    vi.stubGlobal('fetch', fetch)
    const controller = new AbortController()
    const frame = await fetchArrow(
      'searchLogSummaries',
      '{"startTime":null}',
      controller.signal
    )
    expect(fetch).toHaveBeenCalledOnce()
    expect(fetch).toHaveBeenCalledWith(
      '/api/data/searchLogSummaries',
      expect.objectContaining({
        method: 'POST',
        body: '{"startTime":null}',
        signal: controller.signal,
      })
    )
    expect(frame.cell('id', 0).integer()).toBe(9007199254740993n)
  })

  it('preserves the backend not-found code for Details error handling', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({ code: -32002, message: 'Log not found' }),
            { status: 404, headers: { 'Content-Type': 'application/json' } }
          )
        )
    )
    await expect(telemetryAPI.getLog('missing')).rejects.toMatchObject({
      code: -32002,
      message: 'Log not found',
    })
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({ code: -32002, message: 'Log not found' }),
            { status: 404, headers: { 'Content-Type': 'application/json' } }
          )
        )
    )
    await expect(telemetryAPI.getLog('missing')).rejects.toBeInstanceOf(
      JsonRpcError
    )
  })

  it('rejects wrong content, missing counts, count mismatches and truncated IPC', async () => {
    const bytes = tableToIPC(table, 'stream')
    for (const response of [
      new Response('<html>proxy error</html>'),
      new Response(new Uint8Array(bytes), {
        headers: { 'Content-Type': 'application/vnd.apache.arrow.stream' },
      }),
      new Response(new Uint8Array(bytes), {
        headers: {
          'Content-Type': 'application/vnd.apache.arrow.stream',
          'X-Arrow-Rows': '2',
        },
      }),
      new Response(new Uint8Array(bytes.subarray(0, bytes.length - 8)), {
        headers: {
          'Content-Type': 'application/vnd.apache.arrow.stream',
          'X-Arrow-Rows': '1',
        },
      }),
    ]) {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
      await expect(fetchArrow('searchLogSummaries', '{}')).rejects.toThrow()
    }
  })

  it('normalizes cancellation before fetch and during body consumption', async () => {
    const controller = new AbortController()
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    controller.abort('superseded')
    await expect(
      telemetryAPI.getLog('old', controller.signal)
    ).rejects.toBeInstanceOf(RequestAbortedError)
    expect(fetch).not.toHaveBeenCalled()

    const active = new AbortController()
    const response = arrowResponse(table)
    const arrayBuffer = response.arrayBuffer.bind(response)
    vi.spyOn(response, 'arrayBuffer').mockImplementation(async () => {
      const data = await arrayBuffer()
      active.abort()
      return data
    })
    fetch.mockResolvedValue(response)
    await expect(
      telemetryAPI.getLog('old', active.signal)
    ).rejects.toBeInstanceOf(RequestAbortedError)
    expect(fetch.mock.calls[0][1].signal).toBe(active.signal)
  })

  it('propagates interrupted HTTP bodies rather than publishing partial rows', async () => {
    const response = arrowResponse(table)
    vi.spyOn(response, 'arrayBuffer').mockRejectedValue(
      new TypeError('connection reset')
    )
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
    await expect(fetchArrow('searchLogSummaries', '{}')).rejects.toThrow(
      'connection reset'
    )
  })
})
