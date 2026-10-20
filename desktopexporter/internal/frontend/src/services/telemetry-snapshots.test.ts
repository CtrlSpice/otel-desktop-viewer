import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  telemetryAPI,
  evalSnapshotDownloadURL,
  JsonRpcError,
  EVAL_SNAPSHOT_MAX_BYTES,
} from './telemetry-service'

afterEach(() => vi.unstubAllGlobals())

function rpc(result: unknown) {
  const fetch = vi.fn(async (_url: string, init: RequestInit) => {
    const request = JSON.parse(String(init.body))
    return {
      ok: true,
      json: async () => ({ jsonrpc: '2.0', id: request.id, result }),
    }
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

describe('snapshot archive RPC boundary', () => {
  it('preserves original text, duplicate keys, numeric spellings and metadata on import', async () => {
    const snapshot = {
      id: 'a'.repeat(64),
      format: 'unknown-json',
      name: 'original.json',
    }
    const fetch = rpc({ snapshot, created: false })
    const original = ' {"x":-0,"x":1.00,"big":9007199254740993,"unicode":"é"}\n'
    const controller = new AbortController()
    expect(
      await telemetryAPI.importEvalSnapshot(
        'local-file',
        'unknown-json',
        'original.json',
        original,
        controller.signal
      )
    ).toEqual({ snapshot, created: false })
    const init = fetch.mock.calls[0][1]
    expect(init.signal).toBe(controller.signal)
    expect(JSON.parse(String(init.body))).toMatchObject({
      method: 'importEvalSnapshot',
      params: {
        source: 'local-file',
        format: 'unknown-json',
        name: 'original.json',
        originalJSON: original,
      },
    })
  })

  it('passes explicit page bounds and cancellation to list, get and delete', async () => {
    const fetch = rpc({ snapshots: [], hasMore: false })
    const controller = new AbortController()
    await telemetryAPI.searchEvalSnapshots(25, 50, controller.signal)
    await telemetryAPI.getEvalSnapshot('b'.repeat(64), controller.signal)
    await telemetryAPI.deleteEvalSnapshot('b'.repeat(64), controller.signal)
    expect(
      fetch.mock.calls.map(([, init]) => JSON.parse(String(init.body)).params)
    ).toEqual([
      { limit: 25, offset: 50 },
      { id: 'b'.repeat(64) },
      { id: 'b'.repeat(64) },
    ])
    expect(
      fetch.mock.calls.every(([, init]) => init.signal === controller.signal)
    ).toBe(true)
    expect(evalSnapshotDownloadURL('b'.repeat(64))).toBe(
      `/api/evals/snapshots/${'b'.repeat(64)}/download`
    )
  })

  it('rejects oversized UTF-8 payloads and escaped envelopes before sending', async () => {
    const fetch = rpc(null)
    await expect(
      telemetryAPI.importEvalSnapshot(
        'file',
        'unknown-json',
        'x.json',
        'é'.repeat(EVAL_SNAPSHOT_MAX_BYTES / 2 + 1)
      )
    ).rejects.toThrow('16 MiB')
    const escaped = JSON.stringify('\\'.repeat(6 * 1024 * 1024))
    expect(new TextEncoder().encode(escaped).length).toBeLessThan(
      EVAL_SNAPSHOT_MAX_BYTES
    )
    await expect(
      telemetryAPI.importEvalSnapshot('file', 'unknown-json', 'x.json', escaped)
    ).rejects.toThrow('20 MiB')
    expect(fetch).not.toHaveBeenCalled()
  })

  it('retains backend error codes for import failures', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url, init: RequestInit) => ({
        ok: true,
        json: async () => ({
          jsonrpc: '2.0',
          id: JSON.parse(String(init.body)).id,
          error: { code: -32602, message: 'Invalid JSON' },
        }),
      }))
    )
    await expect(
      telemetryAPI.importEvalSnapshot('file', 'unknown-json', 'x.json', '{')
    ).rejects.toEqual(new JsonRpcError(-32602, 'Invalid JSON'))
  })
})
