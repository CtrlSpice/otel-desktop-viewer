// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, screen, waitFor } from '@testing-library/svelte'
import TraceDetailView from '@/components/traces/Detail/TraceDetailView.svelte'
import LogDetailView from '@/components/logs/LogDetailView.svelte'
import { renderWithContexts } from '@/test/render-helpers'
import type { SpanData, LogData, TraceLogSummary } from '@/types/api-types'

const { copyText } = vi.hoisted(() => ({
  copyText: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/utils/clipboard', () => ({ copyText }))

afterEach(() => {
  vi.unstubAllGlobals()
  copyText.mockClear()
})

const resource = { attributes: [], droppedAttributesCount: 0 }
const scope = { ...resource, name: '', version: '' }
const span: SpanData = {
  traceID: 'trace',
  spanID: 'span',
  parentSpanID: null,
  name: 'span',
  flags: 0,
  kind: 'Internal',
  kindCode: 1,
  startTime: 1n,
  endTime: 2n,
  statusCode: 'Ok',
  statusCodeValue: 1,
  statusMessage: '',
  traceState: '',
  events: [
    {
      name: 'old-span-event',
      timestamp: 1n,
      attributes: [],
      droppedAttributesCount: 0,
    },
  ],
  links: [],
  attributes: [],
  resource,
  scope,
  droppedAttributesCount: 0,
  droppedEventsCount: 0,
  droppedLinksCount: 0,
}
const log: LogData = {
  logRef: 'log',
  timestamp: 1n,
  observedTimestamp: 1n,
  traceID: null,
  spanID: null,
  severityText: 'INFO',
  severityNumber: 9,
  body: { kind: 'string', value: 'old' },
  attributes: [],
  resource,
  scope,
  droppedAttributesCount: 0,
  flags: 0,
  eventName: '',
}
const correlated: TraceLogSummary = {
  logRef: 'correlated',
  spanID: 'span',
  timestamp: 1n,
  severityText: 'INFO',
  severityNumber: 9,
  serviceName: '',
  eventName: '',
  bodyPreview: 'independently available A log',
}

describe('raw Details response invalidation', () => {
  it.each(['span', 'log'] as const)(
    'refreshes a same-URL %s preview without remounting it',
    async kind => {
      let release!: () => void
      let pending = new Promise<void>(resolve => {
        release = resolve
      })
      let gets = 0
      const formatted = vi.fn()
      const originalBytes =
        '{"preview":"old","exact":-0.0,"integer":9007199254740993}'
      const nextBytes =
        '{"preview":"next","exact":-0.0,"integer":9007199254740995}'
      const signals: AbortSignal[] = []
      const fetch = vi.fn(async (url: string, init: RequestInit) => {
        signals.push(init.signal as AbortSignal)
        const payload = url.includes('next') ? nextBytes : originalBytes
        if (init.method === 'HEAD')
          return new Response(null, {
            headers: { 'Content-Length': String(payload.length) },
          })
        if ((init.headers as Record<string, string>)?.Range) {
          gets++
          if (gets > 1) await pending
        }
        return new Response(payload, {
          headers: {
            'Content-Length': String(payload.length),
            'Content-Type': 'application/json',
          },
        })
      })
      vi.stubGlobal('fetch', fetch)
      vi.stubGlobal(
        'Worker',
        class {
          onmessage?: (event: { data: string }) => void
          terminate() {}
          postMessage(text: string) {
            void Promise.resolve().then(() => {
              this.onmessage?.({ data: text })
              formatted()
            })
          }
        }
      )
      const traceView =
        kind === 'span'
          ? renderWithContexts(TraceDetailView, { span })
          : undefined
      const logView =
        kind === 'log' ? renderWithContexts(LogDetailView, { log }) : undefined
      const refreshedSpan = Object.freeze({ ...span })
      await fireEvent.click(screen.getByRole('tab', { name: 'JSON' }))
      const editor = await screen.findByLabelText('Raw OTLP JSON')
      await waitFor(() => expect(editor).toHaveTextContent('old'))
      if (traceView)
        await traceView.rerender({
          component: TraceDetailView,
          componentProps: { span: refreshedSpan, logs: [] },
        })
      if (logView)
        await logView.rerender({
          component: LogDetailView,
          componentProps: {
            log: { ...log },
          },
        })
      await waitFor(() => expect(gets).toBe(2))
      if (traceView) {
        await traceView.rerender({
          component: TraceDetailView,
          componentProps: { span: refreshedSpan, logs: [correlated] },
        })
        expect(gets).toBe(2)
        expect(signals.at(-1)?.aborted).toBe(false)
      }
      expect(signals.slice(0, 2).every(signal => signal.aborted)).toBe(true)
      expect(screen.getByLabelText('Raw OTLP JSON')).toBe(editor)
      expect(editor).toHaveTextContent('old')
      if (traceView) {
        expect(screen.getByText('old-span-event')).toBeInTheDocument()
        expect(screen.getByText(correlated.bodyPreview)).toBeInTheDocument()
        expect(screen.getByText('Events').closest('summary')).toHaveTextContent(
          '[2]'
        )
      }
      // Retained records are immutable in production. A new response can
      // invalidate the preview without fabricating new bytes for its identity.
      await fireEvent.click(
        screen.getByRole('button', { name: `Copy ${kind}` })
      )
      await waitFor(() =>
        expect(copyText).toHaveBeenLastCalledWith(originalBytes)
      )
      expect(screen.queryByText('Loading JSON…')).toBeNull()
      await act(async () => release())
      await waitFor(() => expect(formatted).toHaveBeenCalledTimes(2))
      expect(screen.getByLabelText('Raw OTLP JSON')).toBe(editor)
      expect(fetch).toHaveBeenCalledTimes(5)

      // A new identity can be received before its JSON. Copy must still use
      // the preview's displayed identity, not the incoming fields record.
      pending = new Promise<void>(resolve => {
        release = resolve
      })
      if (traceView)
        await traceView.rerender({
          component: TraceDetailView,
          componentProps: {
            span: {
              ...span,
              spanID: 'next',
              events: [
                {
                  name: 'new-span-event',
                  timestamp: 1n,
                  attributes: [],
                  droppedAttributesCount: 0,
                },
                {
                  name: 'second-new-span-event',
                  timestamp: 2n,
                  attributes: [],
                  droppedAttributesCount: 0,
                },
                {
                  name: 'third-new-span-event',
                  timestamp: 3n,
                  attributes: [],
                  droppedAttributesCount: 0,
                },
              ],
            },
            logs: [],
          },
        })
      if (logView)
        await logView.rerender({
          component: LogDetailView,
          componentProps: { log: { ...log, logRef: 'next' } },
        })
      await waitFor(() => expect(gets).toBe(3))
      if (traceView) {
        expect(screen.queryByText('new-span-event')).toBeNull()
        expect(screen.getByText('old-span-event')).toBeInTheDocument()
        expect(screen.getByText(correlated.bodyPreview)).toBeInTheDocument()
        expect(screen.getByText('Events').closest('summary')).toHaveTextContent(
          '[2]'
        )
        await fireEvent.click(screen.getByRole('tab', { name: 'Properties' }))
        expect(screen.getByText('new-span-event')).toBeInTheDocument()
        expect(screen.getByText('Events').closest('summary')).toHaveTextContent(
          '[3]'
        )
        await fireEvent.click(screen.getByRole('tab', { name: 'JSON' }))
        expect(screen.queryByText('new-span-event')).toBeNull()
        expect(screen.getByText(correlated.bodyPreview)).toBeInTheDocument()
        expect(screen.getByText('Events').closest('summary')).toHaveTextContent(
          '[2]'
        )
      }
      const copy = screen.getByRole('button', { name: `Copy ${kind}` })
      await fireEvent.click(copy)
      const oldURL =
        kind === 'span'
          ? '/export/traces/trace?spanId=span'
          : '/export/logs/log'
      await waitFor(() => expect(fetch.mock.calls.at(-1)?.[0]).toBe(oldURL))
      await waitFor(() =>
        expect(copyText).toHaveBeenLastCalledWith(originalBytes)
      )
      expect(screen.getByLabelText('Raw OTLP JSON')).toBe(editor)
      await act(async () => release())
      await waitFor(() => expect(editor).toHaveTextContent('next'))
      if (traceView) {
        expect(screen.getByText('new-span-event')).toBeInTheDocument()
        expect(screen.queryByText('old-span-event')).toBeNull()
        expect(screen.queryByText(correlated.bodyPreview)).toBeNull()
        expect(screen.getByText('Events').closest('summary')).toHaveTextContent(
          '[3]'
        )
      }
      // The displayed-record binding changes only when formatting completes.
      const nextURL =
        kind === 'span'
          ? '/export/traces/trace?spanId=next'
          : '/export/logs/next'
      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: `Copy ${kind}` })
        ).toHaveAttribute('aria-busy', 'false')
      )
      await fireEvent.click(
        screen.getByRole('button', { name: `Copy ${kind}` })
      )
      await waitFor(() => expect(fetch.mock.calls.at(-1)?.[0]).toBe(nextURL))
      await waitFor(() => expect(copyText).toHaveBeenLastCalledWith(nextBytes))
    }
  )
})
