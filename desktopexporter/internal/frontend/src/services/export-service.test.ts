// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { downloadOTLP, readOTLPText } from './export-service'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('OTLP clipboard source', () => {
  it('reads only the requested metric and preserves the exact JSON text', async () => {
    const text =
      '{"value":-0.0,"count":18446744073709551615,"timeUnixNano":"18446744073709551615"}\n'
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(text, {
        headers: { 'Content-Type': 'application/json; charset=utf-8' },
      })
    )
    vi.stubGlobal('fetch', fetchMock)
    const signal = new AbortController().signal
    await expect(
      readOTLPText('metric', 'metric/identity', signal)
    ).resolves.toBe(text)
    expect(fetchMock).toHaveBeenCalledExactlyOnceWith(
      '/export/metrics/metric%2Fidentity',
      { signal, headers: { Accept: 'application/json' } }
    )
  })

  it.each([
    [404, 'text/plain', 'export record not found'],
    [200, 'text/html', '<html>old viewer</html>'],
  ])(
    'rejects an HTTP %s %s response instead of copying it',
    async (status, type, body) => {
      vi.stubGlobal(
        'fetch',
        vi
          .fn()
          .mockResolvedValue(
            new Response(body, { status, headers: { 'Content-Type': type } })
          )
      )
      await expect(
        readOTLPText('metric', 'record', new AbortController().signal)
      ).rejects.toThrow()
    }
  )

  it('rejects a completed response when its selection was cancelled', async () => {
    const controller = new AbortController()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async () => {
        controller.abort()
        return new Response('{}', {
          headers: { 'Content-Type': 'application/json' },
        })
      })
    )
    await expect(
      readOTLPText('metric', 'record', controller.signal)
    ).rejects.toThrow()
  })
})

describe('OTLP downloads', () => {
  it('saves JSON bytes without parsing or changing them', async () => {
    const body = new TextEncoder().encode(
      '{"doubleValue":-0.0,"intValue":"9223372036854775807"}\n'
    )
    const type = 'application/json'
    const createObjectURL = vi.fn((_blob: Blob) => 'blob:export-test')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(body, { headers: { 'Content-Type': type } })
      )
    vi.stubGlobal('fetch', fetchMock)
    let filename = ''
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement
    ) {
      filename = this.download
      expect(this.href).toBe('blob:export-test')
      expect(this.isConnected).toBe(true)
    })

    const signal = new AbortController().signal
    await downloadOTLP('trace', '00000000000000000000000000000001', signal)
    expect(fetchMock).toHaveBeenCalledWith(
      '/export/traces/00000000000000000000000000000001',
      { signal, headers: { Accept: type } }
    )
    const blob = createObjectURL.mock.calls[0]![0]
    expect(Array.from(new Uint8Array(await blob.arrayBuffer()))).toEqual(
      Array.from(body)
    )
    expect(filename).toBe('trace-00000000000000000000000000000001.json')
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:export-test')
    expect(document.querySelector('a[download]')).toBeNull()
  })

  it.each([
    [404, 'text/plain', 'export record not found'],
    [200, 'text/html', '<html>old viewer</html>'],
  ])('does not download an HTTP %s %s response', async (status, type, body) => {
    const createObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL })
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(body, { status, headers: { 'Content-Type': type } })
        )
    )
    await expect(
      downloadOTLP('log', 'record', new AbortController().signal)
    ).rejects.toThrow()
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  it('does not save a response after the selected record was removed', async () => {
    const controller = new AbortController()
    const createObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async () => {
        controller.abort()
        return new Response('{}', {
          headers: { 'Content-Type': 'application/json' },
        })
      })
    )
    await expect(
      downloadOTLP('metric', 'record', controller.signal)
    ).rejects.toThrow()
    expect(createObjectURL).not.toHaveBeenCalled()
  })
})
