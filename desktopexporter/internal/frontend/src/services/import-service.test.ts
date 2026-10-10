import { afterEach, describe, expect, it, vi } from 'vitest'
import { createFileImporter, importOTLPFile } from './import-service'
import { telemetryAPI } from './telemetry-service'
import { OTLP_REQUEST_BYTES } from './otlp-file'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function receiver(
  reply: (
    url: RequestInfo | URL,
    init?: RequestInit
  ) => Promise<Response> = async () => new Response('{}')
) {
  vi.spyOn(telemetryAPI, 'getImportConfig').mockResolvedValue({
    otlpHttpPort: 54318,
  })
  const send = vi.fn(reply)
  vi.stubGlobal('fetch', send)
  return send
}

describe('OTLP file sender', () => {
  it('uses the configured port, viewer hostname and standard signal paths with unchanged values', async () => {
    const send = receiver()
    const file = new File(
      [
        '{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"doubleValue":-0}}]}]}],"resourceSpans":[{}],"resourceMetrics":[{}]}',
      ],
      'mixed.json'
    )
    expect(
      await importOTLPFile(
        file,
        'http://example.test:8000/logs?ignored=yes#x',
        new AbortController().signal
      )
    ).toEqual([])
    expect(send).toHaveBeenCalledTimes(3)
    expect(send.mock.calls.map(call => String(call[0]))).toEqual([
      'http://example.test:54318/v1/traces',
      'http://example.test:54318/v1/logs',
      'http://example.test:54318/v1/metrics',
    ])
    const init = send.mock.calls[1][1] as RequestInit
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' })
    expect(await (init.body as Blob).text()).toContain('"doubleValue":-0')
  })

  it('does not send earlier wrappers when the end of the file is malformed', async () => {
    const send = receiver()
    const file = new File(
      ['{"resourceLogs":[{}]}\n{"resourceSpans":'],
      'broken.jsonl'
    )
    await expect(
      importOTLPFile(
        file,
        'http://localhost:8000',
        new AbortController().signal
      )
    ).rejects.toThrow()
    expect(send).not.toHaveBeenCalled()
  })

  it('reports profiles as coming soon and unknown wrappers as invalid without sending either', async () => {
    const send = receiver()
    const file = new File(
      ['{"resourceProfiles":[{}],"garbage":[{}]}'],
      'other.json'
    )
    const issues = await importOTLPFile(
      file,
      'http://localhost:8000',
      new AbortController().signal
    )
    expect(issues).toEqual([
      expect.stringContaining('Profiles support coming soon'),
      expect.stringContaining('Invalid OTLP wrapper: garbage'),
    ])
    expect(send).not.toHaveBeenCalled()
    expect(telemetryAPI.getImportConfig).not.toHaveBeenCalled()
  })

  it('reports receiver rejection and partial success while continuing with other signals', async () => {
    const send = receiver()
    send.mockResolvedValueOnce(
      new Response('{"message":"Invalid trace ID"}', { status: 400 })
    )
    send.mockResolvedValueOnce(
      new Response(
        '{"partialSuccess":{"rejectedLogRecords":"1","errorMessage":"log rejected"}}'
      )
    )
    const issues = await importOTLPFile(
      new File(
        ['{"resourceSpans":[{}],"resourceLogs":[{}],"resourceMetrics":[{}]}'],
        'mixed.json'
      ),
      'http://localhost:8000',
      new AbortController().signal
    )
    expect(issues).toEqual(['traces: Invalid trace ID', 'logs: log rejected'])
    expect(send).toHaveBeenCalledTimes(3)
  })

  it.each([
    { status: 400, message: '' },
    { status: 500, message: '' },
    { status: 400, message: ' \t\n ' },
    { status: 500, message: ' \t\n ' },
  ])(
    'reports HTTP $status when its JSON error message is blank',
    async ({ status, message }) => {
      receiver(
        async () => new Response(JSON.stringify({ message }), { status })
      )
      const issues = await importOTLPFile(
        new File(['{"resourceSpans":[{}]}'], 'trace.json'),
        'http://localhost:8000',
        new AbortController().signal
      )
      expect(issues).toEqual([`traces: HTTP ${status}`])
    }
  )

  it('reports a blank-message HTTP failure through the file queue and continues to the next file', async () => {
    const send = receiver()
    send.mockResolvedValueOnce(
      new Response('{"code":13,"message":""}', { status: 500 })
    )
    const report = vi.fn()
    const importer = createFileImporter('http://localhost:8000', report)
    await importer.enqueue([
      new File(['{"resourceSpans":[{}]}'], 'failed.json'),
      new File(['{"resourceLogs":[{}]}'], 'next.json'),
    ])
    expect(send).toHaveBeenCalledTimes(2)
    expect(report).toHaveBeenCalledOnce()
    expect(report).toHaveBeenCalledWith(
      expect.objectContaining({
        fileName: 'failed.json',
        reason: 'traces: HTTP 500',
      })
    )
  })

  it.each([
    'null',
    '42',
    '{"partialSuccess":42}',
    '{"partialSuccess":{"rejectedSpans":-1}}',
  ])('reports malformed receiver responses: %s', async body => {
    receiver(async () => new Response(body))
    const issues = await importOTLPFile(
      new File(['{"resourceSpans":[{}]}'], 'trace.json'),
      'http://localhost:8000',
      new AbortController().signal
    )
    expect(issues[0]).toContain('Invalid OTLP')
  })

  it('sends large files in capped batches with only one request in flight', async () => {
    let release!: () => void
    const held = new Promise<Response>(resolve => {
      release = () => resolve(new Response('{}'))
    })
    const send = receiver()
    send.mockReturnValueOnce(held)
    const resource = `{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"${'x'.repeat(11 * 1024 * 1024)}"}}]}]}`
    const file = new File(
      [`{"resourceLogs":[${resource},${resource}]}`],
      'large.json'
    )
    const importing = importOTLPFile(
      file,
      'http://localhost:8000',
      new AbortController().signal
    )
    try {
      await vi.waitFor(() => expect(send).toHaveBeenCalledTimes(1))
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(send).toHaveBeenCalledTimes(1)
    } finally {
      release()
    }
    expect(await importing).toEqual([])
    expect(send).toHaveBeenCalledTimes(2)
    for (const call of send.mock.calls) {
      const init = call[1] as RequestInit
      expect((init.body as Blob).size).toBeLessThanOrEqual(OTLP_REQUEST_BYTES)
    }
  }, 20000)

  it('serializes separate selections and continues after a broken file', async () => {
    let release!: () => void
    const held = new Promise<Response>(resolve => {
      release = () => resolve(new Response('{}'))
    })
    const send = receiver()
    send.mockReturnValueOnce(held)
    const report = vi.fn()
    const importer = createFileImporter('http://localhost:8000', report)
    const first = importer.enqueue([
      new File(['{"resourceSpans":[{}]}'], 'first.json'),
    ])
    const next = importer.enqueue([
      new File(['{'], 'broken.json'),
      new File(['{"resourceLogs":[{}]}'], 'last.json'),
    ])
    try {
      await vi.waitFor(() => expect(send).toHaveBeenCalledTimes(1))
      expect(report).not.toHaveBeenCalled()
    } finally {
      release()
    }
    await Promise.all([first, next])
    expect(send).toHaveBeenCalledTimes(2)
    expect(report).toHaveBeenCalledOnce()
    expect(report.mock.calls[0][0].fileName).toBe('broken.json')
  })

  it('cancels an active send and skips queued files without reporting cancellation as a defect', async () => {
    const send = receiver()
    send.mockImplementation(
      (_url, init) =>
        new Promise((_resolve, reject) => {
          ;(init as RequestInit).signal?.addEventListener('abort', () =>
            reject(new DOMException('Aborted', 'AbortError'))
          )
        })
    )
    const report = vi.fn()
    const importer = createFileImporter('http://localhost:8000', report)
    const pending = importer.enqueue([
      new File(['{"resourceSpans":[{}]}'], 'first.json'),
      new File(['{"resourceLogs":[{}]}'], 'last.json'),
    ])
    await vi.waitFor(() => expect(send).toHaveBeenCalledOnce())
    importer.cancel()
    await pending
    expect(send).toHaveBeenCalledOnce()
    expect(report).not.toHaveBeenCalled()
  })
})
