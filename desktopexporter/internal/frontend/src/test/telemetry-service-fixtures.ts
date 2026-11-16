import { vi } from 'vitest'
import { arrowResponse } from '@/test/arrow-fixtures'
import type { Table } from 'apache-arrow'
import { telemetryDataset } from '@/test/native-telemetry-fixtures'

export function stubArrowResult(table: Table) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(async () => arrowResponse(table))
  )
}

export type StubRpcResponse<T> = {
  jsonrpc?: unknown
  id?: unknown
  result?: T
  error?: { code: number; message: string }
}

export function stubRpcResponse<T>(body: StubRpcResponse<T>) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(async (url, init: RequestInit) => {
      if (String(url).startsWith('/api/data/')) {
        if (body.error) return Response.json(body.error, { status: 400 })
        return arrowResponse(
          telemetryDataset(String(url).split('/').at(-1)!, body.result)
        )
      }
      const request = JSON.parse(String(init.body)) as { id: number }
      const responseBody = 'id' in body ? body : { ...body, id: request.id }
      return {
        ok: true,
        json: async () => responseBody,
      }
    })
  )
}

export function stubRpcResult<T>(result: T) {
  stubRpcResponse({ jsonrpc: '2.0', result })
}
