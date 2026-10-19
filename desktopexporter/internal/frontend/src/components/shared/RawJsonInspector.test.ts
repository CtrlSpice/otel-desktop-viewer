// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/svelte'
import RawJsonInspector from './RawJsonInspector.svelte'

afterEach(() => vi.unstubAllGlobals())

const payload = '{"value":42}'
const datapoint = {
  id: 'datapoint',
  label: 'Datapoint',
  url: '/export/metrics/example?datapoint=1',
}
const aggregate = {
  id: 'datapoint',
  label: 'Datapoint',
  unavailable: 'Select an individual datapoint to inspect its raw JSON.',
}

async function mockExport(pauseAt?: 'HEAD' | 'GET' | 'decode' | 'format') {
  let resume!: () => void
  const pending = new Promise<void>(resolve => (resume = resolve))
  const reached = vi.fn()
  const signals: AbortSignal[] = []
  const terminate = vi.fn()
  async function pause(stage: string) {
    if (stage === pauseAt) {
      reached()
      await pending
    }
  }
  const response = new Response(payload, {
    headers: { 'Content-Length': String(payload.length) },
  })
  const blob = await response.blob()
  const arrayBuffer = blob.arrayBuffer.bind(blob)
  vi.spyOn(blob, 'arrayBuffer').mockImplementation(async () => {
    await pause('decode')
    return arrayBuffer()
  })
  vi.spyOn(response, 'blob').mockResolvedValue(blob)
  const fetch = vi.fn(async (_url: string, init: RequestInit) => {
    signals.push(init.signal as AbortSignal)
    const method = init.method ?? 'GET'
    await pause(method)
    return method === 'HEAD'
      ? new Response(null, {
          headers: { 'Content-Length': String(payload.length) },
        })
      : response
  })
  vi.stubGlobal('fetch', fetch)
  vi.stubGlobal(
    'Worker',
    class {
      onmessage?: (event: { data: string }) => void
      terminate = terminate
      postMessage(text: string) {
        void pause('format').then(() =>
          this.onmessage?.({ data: JSON.stringify(JSON.parse(text), null, 2) })
        )
      }
    }
  )
  return { fetch, signals, reached, resume, terminate }
}

function expectEmptyPreview() {
  expect(screen.getByText(/^OTLP JSON/)).toHaveTextContent(/^OTLP JSON$/)
  expect(screen.queryByLabelText('Raw OTLP JSON')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Copy OTLP JSON' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Collapse JSON' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Expand JSON' })).toBeDisabled()
}

describe('RawJsonInspector selection changes', () => {
  it.each([
    { name: 'aggregate bucket', scopes: [aggregate] },
    { name: 'empty selection', scopes: [] },
  ])('clears the loaded datapoint for an $name', async ({ scopes }) => {
    const request = await mockExport()
    const view = render(RawJsonInspector, {
      active: true,
      wholeURL: '/export/metrics/example',
      scopes: [datapoint],
    })
    await screen.findByLabelText('Raw OTLP JSON')
    expect(screen.getByText(/^OTLP JSON/)).toHaveTextContent('OTLP JSON · 12 B')
    expect(screen.getByRole('button', { name: 'Copy OTLP JSON' })).toBeVisible()

    await view.rerender({ scopes })
    expectEmptyPreview()
    expect(
      screen.getByText(
        scopes[0]?.unavailable ?? 'Select a record to inspect its raw JSON.'
      )
    ).toBeVisible()
    expect(request.signals.every(signal => signal.aborted)).toBe(true)

    await view.rerender({ scopes: [datapoint] })
    await screen.findByLabelText('Raw OTLP JSON')
    expect(request.fetch).toHaveBeenCalledTimes(4)
  })

  it.each(['HEAD', 'GET', 'decode', 'format'] as const)(
    'cancels pending %s work when the datapoint becomes unavailable',
    async stage => {
      const request = await mockExport(stage)
      const view = render(RawJsonInspector, {
        active: true,
        wholeURL: '/export/metrics/example',
        scopes: [datapoint],
      })
      await waitFor(() => expect(request.reached).toHaveBeenCalledOnce())
      await view.rerender({ scopes: [aggregate] })
      expect(request.signals.every(signal => signal.aborted)).toBe(true)
      if (stage === 'format') expect(request.terminate).toHaveBeenCalledOnce()
      expectEmptyPreview()

      // Even work already beyond fetch (body decoding or a queued worker
      // message) must not restore the old record after cancellation.
      await act(async () => request.resume())
      expectEmptyPreview()
      expect(screen.getByText(aggregate.unavailable)).toBeVisible()
      expect(request.fetch).toHaveBeenCalledTimes(stage === 'HEAD' ? 1 : 2)
    }
  )
})

describe('RawJsonInspector metadata', () => {
  it('keeps the format indicator and explains its size and provenance on hover instead of in paragraphs', () => {
    const { container } = render(RawJsonInspector, {
      active: false,
      wholeURL: '/export/logs/example',
      scopes: [{ id: 'log', label: 'Log', url: '/export/logs/example' }],
    })
    const metadata = screen.getByRole('status')
    expect(metadata).toHaveTextContent('OTLP JSON')
    expect(metadata.getAttribute('title')).toContain(
      'UTF-8 bytes before display formatting'
    )
    expect(metadata.getAttribute('title')).toContain(
      'not the original request bytes'
    )
    expect(metadata).toHaveAccessibleDescription(
      metadata.getAttribute('title')!
    )
    expect(container.querySelector('.raw-inspector__hint')).toBeNull()
    expect(container.querySelector('p')).toBeNull()
    expect(screen.queryByRole('link', { name: 'Download log' })).toBeNull()
    const folding = screen.getByRole('group', { name: 'JSON folding' })
    expect(folding.closest('.raw-inspector__controls')).not.toBeNull()
    for (const label of ['Collapse JSON', 'Expand JSON']) {
      const button = screen.getByRole('button', { name: label })
      expect(button).toHaveClass('btn-xs', 'chart-header-control')
      expect(button).toHaveTextContent('')
      expect(button).toHaveAttribute('data-tip', label)
      expect(button).toBeDisabled()
      expect(button.querySelector('svg')).not.toBeNull()
    }
  })

  it('retains a narrower scoped download without repeating the whole-record header export', () => {
    render(RawJsonInspector, {
      active: false,
      wholeURL: '/export/traces/example',
      scopes: [
        {
          id: 'span',
          label: 'Span',
          url: '/export/traces/example?spanId=0000000000000001',
        },
      ],
    })
    expect(screen.getByRole('link', { name: 'Download span' })).toHaveAttribute(
      'href',
      '/export/traces/example?spanId=0000000000000001'
    )
    expect(
      screen.queryByRole('link', { name: 'Download full OTLP' })
    ).toBeNull()
  })
})
