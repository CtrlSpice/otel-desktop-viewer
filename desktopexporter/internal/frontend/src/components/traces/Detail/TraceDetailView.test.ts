// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import TraceDetailView from './TraceDetailView.svelte'
import type { SpanData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const setSpanInQuery = vi.hoisted(() => vi.fn())

vi.mock('@/route', async importOriginal => {
  const actual = await importOriginal<typeof import('@/route')>()
  return { ...actual, setSpanInQuery }
})

function makeSpan(overrides: Partial<SpanData> = {}): SpanData {
  return {
    spanID: 'child-span',
    parentSpanID: 'parent-span',
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
    setSpanInQuery.mockClear()
    setTestUrl('/traces/trace-1?span=child-span&start=0&end=1')
  })

  it('selects the parent span in the current trace on click', async () => {
    renderWithContexts(TraceDetailView, { span: makeSpan() })
    await userEvent.click(screen.getByRole('button', { name: 'parent-span' }))
    expect(setSpanInQuery).toHaveBeenCalledWith('parent-span', 'push')
  })

  it('does not show a parent span link for the root span', () => {
    renderWithContexts(TraceDetailView, {
      span: makeSpan({ parentSpanID: null }),
    })
    expect(screen.queryByRole('button', { name: 'parent-span' })).toBeNull()
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
      banner!.classList.contains('detail-view__paradox--offender')
    ).toBe(false)
  })

  it('escalates for the span that causes the cycle', () => {
    renderWithContexts(TraceDetailView, {
      span: makeSpan(),
      salvaged: true,
      cyclePoint: true,
    })
    const banner = document.querySelector('.detail-view__paradox')
    expect(banner).not.toBeNull()
    expect(banner!.textContent).toContain('causes a cycle')
    expect(
      banner!.classList.contains('detail-view__paradox--offender')
    ).toBe(true)
  })
})
