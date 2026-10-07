// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { downloadOTLP } from './export-service'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('OTLP downloads', () => {
  it.each(['json', 'protobuf'] as const)(
    'saves %s bytes without parsing or changing them',
    async format => {
      const body =
        format === 'json'
          ? new TextEncoder().encode(
              '{"doubleValue":-0.0,"intValue":"9223372036854775807"}\n'
            )
          : new Uint8Array([0, 10, 255, 128, 0, 13, 10])
      const type =
        format === 'json' ? 'application/json' : 'application/x-protobuf'
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
      vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(
        function (this: HTMLAnchorElement) {
          filename = this.download
          expect(this.href).toBe('blob:export-test')
          expect(this.isConnected).toBe(true)
        }
      )

      const signal = new AbortController().signal
      await downloadOTLP(
        'trace',
        '00000000000000000000000000000001',
        format,
        signal
      )
      expect(fetchMock).toHaveBeenCalledWith(
        `/export/traces/00000000000000000000000000000001?format=${format}`,
        { signal, headers: { Accept: type } }
      )
      const blob = createObjectURL.mock.calls[0]![0]
      expect(Array.from(new Uint8Array(await blob.arrayBuffer()))).toEqual(
        Array.from(body)
      )
      expect(filename).toBe(
        `trace-00000000000000000000000000000001.${format === 'json' ? 'json' : 'pb'}`
      )
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:export-test')
      expect(document.querySelector('a[download]')).toBeNull()
    }
  )

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
      downloadOTLP('log', 'record', 'json', new AbortController().signal)
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
      downloadOTLP('metric', 'record', 'json', controller.signal)
    ).rejects.toThrow()
    expect(createObjectURL).not.toHaveBeenCalled()
  })
})
