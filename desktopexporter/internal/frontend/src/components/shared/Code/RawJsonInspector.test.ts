// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/svelte'
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
  it('reloads the same export URL when the displayed record is refreshed', async () => {
    const request = await mockExport()
    const view = render(RawJsonInspector, {
      active: true,
      wholeURL: datapoint.url,
      scopes: [datapoint],
      refreshKey: 1,
    })
    await screen.findByLabelText('Raw OTLP JSON')
    expect(request.fetch).toHaveBeenCalledTimes(2)
    await view.rerender({ refreshKey: 2 })
    await waitFor(() => expect(request.fetch).toHaveBeenCalledTimes(4))
    expect(request.signals.slice(0, 2).every(signal => signal.aborted)).toBe(
      true
    )
    await screen.findByLabelText('Raw OTLP JSON')
  })

  it('retains the completed editor and folds across tab changes without fetching again', async () => {
    const request = await mockExport()
    const view = render(RawJsonInspector, {
      active: true,
      wholeURL: datapoint.url,
      scopes: [datapoint],
    })
    const editor = await screen.findByLabelText('Raw OTLP JSON')
    await fireEvent.click(screen.getByRole('button', { name: 'Collapse JSON' }))
    await waitFor(() =>
      expect(editor.querySelector('.cm-foldPlaceholder')).not.toBeNull()
    )
    const fold = editor.querySelector('.cm-foldPlaceholder')
    await view.rerender({ active: false })
    await view.rerender({ active: true })
    expect(screen.getByLabelText('Raw OTLP JSON')).toBe(editor)
    expect(editor.querySelector('.cm-foldPlaceholder')).toBe(fold)
    expect(request.fetch).toHaveBeenCalledTimes(2)
  })

  it.each(['HEAD', 'GET', 'decode', 'format'] as const)(
    'resumes cancelled %s work when JSON is reopened',
    async stage => {
      const request = await mockExport(stage)
      const view = render(RawJsonInspector, {
        active: true,
        wholeURL: datapoint.url,
        scopes: [datapoint],
      })
      await waitFor(() => expect(request.reached).toHaveBeenCalledOnce())
      await view.rerender({ active: false })
      expect(request.signals.every(signal => signal.aborted)).toBe(true)
      await act(async () => request.resume())
      await view.rerender({ active: true })
      await screen.findByLabelText('Raw OTLP JSON')
    }
  )
  it.each(['decode', 'format'] as const)(
    'keeps loading during preview %s',
    async stage => {
      const request = await mockExport(stage)
      render(RawJsonInspector, {
        active: true,
        wholeURL: datapoint.url,
        scopes: [datapoint],
      })
      await waitFor(() => expect(request.reached).toHaveBeenCalledOnce())
      expect(screen.queryByText('Loading JSON…')).toBeNull()
      expect(await screen.findByText('Loading JSON…')).toBeVisible()
      await act(async () => request.resume())
      await screen.findByLabelText('Raw OTLP JSON')
    }
  )

  it('keeps large scopes behind the manual gate and bounds the requested preview', async () => {
    const request = await mockExport('GET')
    request.fetch.mockResolvedValueOnce(
      new Response(null, {
        headers: { 'Content-Length': String(3 * 1024 * 1024) },
      })
    )
    render(RawJsonInspector, {
      active: true,
      wholeURL: datapoint.url,
      scopes: [datapoint],
    })
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Load 2 MiB preview' })
    )
    await waitFor(() => expect(request.reached).toHaveBeenCalledOnce())
    expect(await screen.findByText('Loading JSON…')).toBeVisible()
    expect(request.fetch.mock.calls[1]![1].headers).toEqual({
      Range: 'bytes=0-2097151',
    })
    await act(async () => request.resume())
    await screen.findByLabelText('Raw OTLP JSON')
  })

  it.each(['HEAD', 'GET'] as const)('retries after a %s error', async stage => {
    const request = await mockExport()
    if (stage === 'GET')
      request.fetch.mockResolvedValueOnce(
        new Response(null, {
          headers: { 'Content-Length': String(payload.length) },
        })
      )
    request.fetch.mockResolvedValueOnce(
      new Response('Export failed', { status: 500 })
    )
    render(RawJsonInspector, {
      active: true,
      wholeURL: datapoint.url,
      scopes: [datapoint],
    })
    expect(await screen.findByRole('alert')).toHaveTextContent('Export failed')
    expect(screen.queryByText('Loading JSON…')).toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByLabelText('Raw OTLP JSON')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByText('Loading JSON…')).toBeNull()
    expect(request.fetch).toHaveBeenCalledTimes(stage === 'HEAD' ? 3 : 4)
  })

  it.each(['metric', 'datapoint', 'span', 'log'])(
    'keeps %s loading after immediate HEAD while GET is pending',
    async id => {
      const request = await mockExport('GET')
      render(RawJsonInspector, {
        active: true,
        wholeURL: datapoint.url,
        scopes: [{ ...datapoint, id, label: id }],
      })
      await waitFor(() => expect(request.reached).toHaveBeenCalledOnce())
      // Flush HEAD's finally as well as the effect that starts GET.
      await act(async () => {})
      expect(await screen.findByText('Loading JSON…')).toBeVisible()
      expect(screen.queryByRole('button', { name: /^Load / })).toBeNull()
      expect(screen.queryByLabelText('Raw OTLP JSON')).toBeNull()
      await act(async () => request.resume())
      await screen.findByLabelText('Raw OTLP JSON')
      expect(screen.queryByText('Loading JSON…')).toBeNull()
    }
  )

  it('keeps the new scope loading when an aborted GET completes', async () => {
    const request = await mockExport('GET')
    const view = render(RawJsonInspector, {
      active: true,
      wholeURL: datapoint.url,
      scopes: [datapoint],
    })
    await waitFor(() => expect(request.reached).toHaveBeenCalledOnce())
    let finishHead!: (response: Response) => void
    request.fetch.mockImplementationOnce((_url, init) => {
      request.signals.push(init.signal as AbortSignal)
      return new Promise(resolve => (finishHead = resolve))
    })
    await view.rerender({ scopes: [{ ...datapoint, url: '/export/next' }] })
    expect(request.signals.slice(0, 2).every(signal => signal.aborted)).toBe(
      true
    )
    await act(async () => request.resume())
    expect(await screen.findByText('Loading JSON…')).toBeVisible()
    expect(screen.queryByLabelText('Raw OTLP JSON')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    await act(async () =>
      finishHead(
        new Response(null, {
          headers: { 'Content-Length': String(payload.length) },
        })
      )
    )
    await screen.findByLabelText('Raw OTLP JSON')
    expect(request.fetch.mock.calls.map(([url]) => url)).toEqual([
      datapoint.url,
      datapoint.url,
      '/export/next',
      '/export/next',
    ])
  })

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
    expect(screen.queryByRole('button', { name: /^Copy/ })).toBeNull()

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

  it('retains the editor, folds and its Download identity until the next JSON is ready', async () => {
    const request = await mockExport()
    const view = render(RawJsonInspector, {
      active: true,
      wholeURL: '/export/metrics/example',
      scopes: [datapoint],
    })
    const editor = await screen.findByLabelText('Raw OTLP JSON')
    await fireEvent.click(screen.getByRole('button', { name: 'Collapse JSON' }))
    await waitFor(() =>
      expect(editor.querySelector('.cm-foldPlaceholder')).not.toBeNull()
    )
    const fold = editor.querySelector('.cm-foldPlaceholder')
    let release!: (response: Response) => void
    request.fetch.mockImplementationOnce(
      () => new Promise(resolve => (release = resolve))
    )
    const next = { ...datapoint, url: '/export/metrics/next?datapoint=2' }
    await view.rerender({ scopes: [next], wholeURL: '/export/metrics/next' })
    await waitFor(() => expect(release).toBeDefined())
    expect(screen.getByLabelText('Raw OTLP JSON')).toBe(editor)
    expect(editor.querySelector('.cm-foldPlaceholder')).toBe(fold)
    expect(
      screen.getByRole('link', { name: 'Download datapoint' })
    ).toHaveAttribute('href', datapoint.url)
    expect(screen.queryByText('Loading JSON…')).toBeNull()
    expect(await screen.findByText('Loading JSON…')).toBeVisible()
    expect(screen.getByLabelText('Raw OTLP JSON')).toBe(editor)
    await act(async () =>
      release(
        new Response(null, {
          headers: { 'Content-Length': String(payload.length) },
        })
      )
    )
    await waitFor(() =>
      expect(
        screen.getByRole('link', { name: 'Download datapoint' })
      ).toHaveAttribute('href', next.url)
    )
    expect(screen.getByLabelText('Raw OTLP JSON')).toBe(editor)
    expect(screen.queryByText('Loading JSON…')).toBeNull()
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
  it('automatically fetches metric metadata while keeping the whole metric download URL', async () => {
    const request = await mockExport()
    const url = '/export/metrics/example'
    render(RawJsonInspector, {
      active: true,
      wholeURL: url,
      scopes: [
        {
          id: 'metric',
          label: 'Metric',
          url,
          previewURL: `${url}?includeDatapoints=false`,
        },
      ],
    })
    await screen.findByLabelText('Raw OTLP JSON')
    expect(
      request.fetch.mock.calls.map(([url, init]) => [url, init.method ?? 'GET'])
    ).toEqual([
      [`${url}?includeDatapoints=false`, 'HEAD'],
      [`${url}?includeDatapoints=false`, 'GET'],
    ])
    expect(screen.queryByRole('button', { name: /^Load/ })).toBeNull()
    expect(screen.queryByRole('link', { name: /^Download/ })).toBeNull()
    expect(screen.getByText(/^OTLP JSON/)).toHaveTextContent(
      'without datapoints'
    )
  })

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
