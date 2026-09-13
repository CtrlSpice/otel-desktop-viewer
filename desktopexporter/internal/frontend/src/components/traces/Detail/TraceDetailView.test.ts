// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import TraceDetailView from './TraceDetailView.svelte'
import type { SpanData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeSpan(overrides: Partial<SpanData> = {}): SpanData {
  return {
    spanID: 'child-span',
    parentSpanID: 'parent-span',
    flags: 0,
    traceID: 'trace-1',
    name: 'child',
    startTime: 0n,
    endTime: 1_000_000n,
    statusCode: 'Ok',
    events: [],
    links: [],
    attributes: [],
    resource: { attributes: [], droppedAttributesCount: 0 },
    scope: {
      name: '',
      version: '',
      attributes: [],
      droppedAttributesCount: 0,
    },
    traceState: '',
    kind: 'Internal',
    droppedAttributesCount: 0,
    droppedEventsCount: 0,
    droppedLinksCount: 0,
    statusMessage: '',
    ...overrides,
  }
}

describe('TraceDetailView parent span link', () => {
  beforeEach(() => {
    setTestUrl('/traces/trace-1?span=child-span&start=0&end=1')
  })

  it('selects the parent span in the current trace on click', async () => {
    renderWithContexts(TraceDetailView, { span: makeSpan() })
    const historyLength = window.history.length

    await userEvent.click(screen.getByRole('button', { name: 'parent-span' }))

    expect(window.location.pathname).toBe('/traces/trace-1')
    expect(window.location.search).toBe('?span=parent-span&start=0&end=1')
    expect(window.history.length).toBe(historyLength + 1)
  })

  it('does not show a parent span link for the root span', () => {
    renderWithContexts(TraceDetailView, {
      span: makeSpan({ parentSpanID: null }),
    })
    expect(screen.queryByRole('button', { name: 'parent-span' })).toBeNull()
  })
})

describe('TraceDetailView tabs', () => {
  it('associates the selected tab with the visible detail panel', () => {
    setTestUrl('/traces/trace-1?span=child-span&start=0&end=1')
    renderWithContexts(TraceDetailView, { span: makeSpan() })
    const fields = screen.getByRole('tab', { name: 'Fields' })
    const panel = screen.getByRole('tabpanel')

    expect(fields).toHaveAttribute('aria-controls', panel.id)
    expect(panel).toHaveAttribute('aria-labelledby', fields.id)
  })
})

describe('TraceDetailView paradox banner', () => {
  beforeEach(() => {
    setTestUrl('/traces/trace-1?span=child-span&start=0&end=1')
  })

  it('shows nothing for a healthy span', () => {
    renderWithContexts(TraceDetailView, { span: makeSpan() })
    expect(document.querySelector('.detail-view__paradox')).toBeNull()
  })

  it('repeats the salvage warning for a stranded span', () => {
    renderWithContexts(TraceDetailView, { span: makeSpan(), salvaged: true })
    const banner = document.querySelector('.detail-view__paradox')
    expect(banner).not.toBeNull()
    expect(banner!.textContent).toContain('Recovered from a broken part')
    expect(
      banner!.classList.contains('detail-view__paradox--cycle-point')
    ).toBe(false)
  })

  it('escalates for the retained cycle point', () => {
    renderWithContexts(TraceDetailView, {
      span: makeSpan(),
      salvaged: true,
      cyclePoint: true,
    })
    const banner = document.querySelector('.detail-view__paradox')
    expect(banner).not.toBeNull()
    expect(banner!.textContent).toContain('Cycle detected')
    expect(banner!.textContent).toContain('following parent IDs would loop')
    expect(
      banner!.classList.contains('detail-view__paradox--cycle-point')
    ).toBe(true)
  })
})
