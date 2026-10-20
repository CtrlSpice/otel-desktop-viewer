// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { SpanData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { SPAN_FIELDS } from '@/constants/fields'
import TraceDetailView from './TraceDetailView.svelte'
import { spanCopyJSON, spanExportURL } from './span-copy'

function makeSpan(): SpanData {
  return {
    traceID: '12345678901234567890123456789012',
    spanID: '1234567890123456',
    parentSpanID: null,
    traceState: '',
    flags: 1,
    name: 'selected span',
    kind: 'Server',
    kindCode: 2,
    startTime: 1_700_000_000_123_456_789n,
    endTime: 1_700_000_001_987_654_321n,
    statusCode: 'Error',
    statusCodeValue: 2,
    statusMessage: 'failed',
    attributes: [],
    resource: { attributes: [], droppedAttributesCount: 1 },
    scope: {
      name: 'instrumentation',
      version: '1.2',
      attributes: [],
      droppedAttributesCount: 2,
    },
    events: [
      {
        name: 'event',
        timestamp: 1_700_000_000_999_999_999n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ],
    links: [],
    droppedAttributesCount: 5,
    droppedEventsCount: 6,
    droppedLinksCount: 7,
  }
}

// Source-only envelope metadata and number spellings cannot be recovered from
// SpanData. Keep this fixture as text, never JSON.stringify a parsed object.
const exported = `{"resourceSpans":[{
  "schemaUrl":"https://opentelemetry.io/schemas/1.21.0",
  "resource":{"attributes":[],"droppedAttributesCount":1},
  "scopeSpans":[{
    "schemaUrl":"https://opentelemetry.io/schemas/1.22.0",
    "scope":{"name":"instrumentation","version":"1.2","droppedAttributesCount":2},
    "spans":[{
      "traceId":"12345678901234567890123456789012",
      "spanId":"1234567890123456","name":"selected span",
      "startTimeUnixNano":"1700000000123456789",
      "attributes":[
        {"key":"same","value":{"intValue":"9223372036854775807"}},
        {"key":"same","value":{"doubleValue":-0.0}},
        {"key":"large","value":{"doubleValue":18446744073709551615}},
        {"key":"exponent","value":{"doubleValue":1.2300e+40}}
      ],
      "events":[{"name":"event","timeUnixNano":"1700000000999999999"}],
      "links":[{"traceId":"linked-trace","spanId":"linked-span","flags":257}],
      "status":{"code":2,"message":"failed"},
      "droppedAttributesCount":5,"droppedEventsCount":6,"droppedLinksCount":7
    }]
  }]
}]}
`

function response(text = exported) {
  return new Response(text, {
    headers: { 'Content-Type': 'application/json; charset=utf-8' },
  })
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('selected span JSON', () => {
  it('reads the selected-span download verbatim, including both schema URLs and numeric lexemes', async () => {
    const fetch = vi.fn().mockResolvedValue(response())
    vi.stubGlobal('fetch', fetch)
    const signal = new AbortController().signal
    const span = makeSpan()
    await expect(spanCopyJSON(span, signal)).resolves.toBe(exported)
    expect(fetch).toHaveBeenCalledExactlyOnceWith(
      `/export/traces/${span.traceID}?spanId=${span.spanID}`,
      { signal, headers: { Accept: 'application/json' } }
    )
    expect(spanExportURL({ traceID: 'trace/1', spanID: 'span&2' })).toBe(
      '/export/traces/trace%2F1?spanId=span%262'
    )
  })

  it.each([
    [404, 'text/plain', 'span not found'],
    [200, 'text/html', '<html>not an export</html>'],
  ])('rejects HTTP %s %s instead of copying it', async (status, type, body) => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(body, { status, headers: { 'Content-Type': type } })
        )
    )
    await expect(
      spanCopyJSON(makeSpan(), new AbortController().signal)
    ).rejects.toThrow()
  })
})

describe('Copy span action', () => {
  it('copies the download source only on explicit keyboard activation, independent of field filters', async () => {
    setTestUrl('/traces/trace?span=selected')
    const user = userEvent.setup()
    const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const fetch = vi.fn().mockResolvedValue(response())
    vi.stubGlobal('fetch', fetch)
    const { container } = renderWithContexts(TraceDetailView, {
      span: makeSpan(),
      columnFilter: [
        SPAN_FIELDS.find(field => 'name' in field && field.name === 'name')!,
      ],
    })
    const button = screen.getByRole('button', { name: 'Copy span' })
    const disclosure = container.querySelector('details')!
    expect(button.closest('summary')).toBeNull()
    await user.click(screen.getByText('selected span'))
    expect(fetch).not.toHaveBeenCalled()
    expect(write).not.toHaveBeenCalled()
    button.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith(exported))
    expect(disclosure.open).toBe(true)
    expect(screen.getByText('Copied span.')).toBeInTheDocument()
    await user.click(disclosure.querySelector('summary')!)
    expect(disclosure.open).toBe(false)
    expect(button.isConnected).toBe(true)
  })

  it.each(['selection', 'unmount'] as const)(
    'cancels an outstanding copy on %s and never writes the stale response',
    async change => {
      const user = userEvent.setup()
      const write = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      let finish!: (value: Response) => void
      const fetch = vi
        .fn()
        .mockImplementationOnce(
          () =>
            new Promise<Response>(resolve => {
              finish = resolve
            })
        )
        .mockResolvedValueOnce(response('new selected span'))
      vi.stubGlobal('fetch', fetch)
      const view = renderWithContexts(TraceDetailView, { span: makeSpan() })
      const button = screen.getByRole('button', { name: 'Copy span' })
      await user.click(button)
      await user.click(button)
      expect(fetch).toHaveBeenCalledTimes(1)
      const signal = fetch.mock.calls[0]![1].signal as AbortSignal
      if (change === 'unmount') view.unmount()
      else {
        await view.rerender({
          component: TraceDetailView,
          componentProps: { span: { ...makeSpan(), spanID: 'next-span' } },
        })
      }
      expect(signal.aborted).toBe(true)
      finish(response())
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(write).not.toHaveBeenCalled()
      if (change === 'selection') {
        await user.click(button)
        await waitFor(() =>
          expect(write).toHaveBeenCalledExactlyOnceWith('new selected span')
        )
        expect(fetch.mock.calls[1]![0]).toContain('?spanId=next-span')
      }
    }
  )

  it('reports a failed download source without copying and permits retry', async () => {
    const user = userEvent.setup()
    const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(new Response('missing', { status: 404 }))
        .mockResolvedValueOnce(response())
    )
    renderWithContexts(TraceDetailView, { span: makeSpan() })
    const button = screen.getByRole('button', { name: 'Copy span' })
    await user.click(button)
    expect(write).not.toHaveBeenCalled()
    expect(screen.getByText('Could not copy span.')).toBeInTheDocument()
    await user.click(button)
    await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith(exported))
  })

  it('offers no span-copy action when no span is selected', () => {
    renderWithContexts(TraceDetailView, { span: undefined })
    expect(screen.queryByRole('button', { name: 'Copy span' })).toBeNull()
  })
})
