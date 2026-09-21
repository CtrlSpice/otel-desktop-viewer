// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import TraceDetailView from './TraceDetailView.svelte'
import type { SpanData, TraceLogSummary } from '@/types/api-types'
import { SPAN_FIELDS } from '@/constants/fields'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeSpan(overrides: Partial<SpanData> = {}): SpanData {
  return {
    spanID: 'child-span',
    parentSpanID: 'parent-span',
    flags: 0,
    traceID: 'trace-1',
    name: 'child',
    kindCode: 1,
    startTime: 0n,
    endTime: 1_000_000n,
    statusCode: 'Ok',
    statusCodeValue: 1,
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

function spanField(name: string) {
  return SPAN_FIELDS.find(field => 'name' in field && field.name === name)!
}

describe('TraceDetailView numeric enum fields', () => {
  beforeEach(() => {
    setTestUrl('/traces/trace-1?span=child-span&start=0&end=1')
  })

  it('shows only the authoritative negative kind code when filtered by kindCode', () => {
    renderWithContexts(TraceDetailView, {
      span: makeSpan({ kind: 'Unknown (-1)', kindCode: -1 }),
      columnFilter: [spanField('kindCode')],
    })

    const table = screen.getByRole('table', { name: 'Span fields' })
    expect(table).toHaveTextContent('kind code (int64):')
    expect(table).toHaveTextContent('-1')
    expect(table).not.toHaveTextContent('kind (string):')
    expect(table).not.toHaveTextContent('Unknown (-1)')
    expect(
      table.closest('details')?.querySelector('summary')
    ).toHaveTextContent(/Span\s*1 field/)
  })

  it('shows only the authoritative unknown status code when filtered by statusCodeValue', () => {
    renderWithContexts(TraceDetailView, {
      span: makeSpan({ statusCode: 'Unknown (99)', statusCodeValue: 99 }),
      columnFilter: [spanField('statusCodeValue')],
    })

    const table = screen.getByRole('table', { name: 'Span fields' })
    expect(table).toHaveTextContent('status code value (int64):')
    expect(table).toHaveTextContent('99')
    expect(table).not.toHaveTextContent('status code (string):')
    expect(table).not.toHaveTextContent('Unknown (99)')
    expect(
      screen.getByRole('img', { name: 'Unrecognised status code' })
    ).toBeInTheDocument()
    expect(
      table.closest('details')?.querySelector('summary')
    ).toHaveTextContent(/Span\s*1 field/)
  })

  it.each([
    { statusCodeValue: 99, statusCode: 'Unknown (99)' },
    { statusCodeValue: -1, statusCode: 'Unknown (-1)' },
  ])(
    'marks unknown status code $statusCodeValue without hiding its received value',
    ({ statusCodeValue, statusCode }) => {
      renderWithContexts(TraceDetailView, {
        span: makeSpan({ statusCode, statusCodeValue }),
      })

      const table = screen.getByRole('table', { name: 'Span fields' })
      expect(table).toHaveTextContent(statusCode)
      expect(table).toHaveTextContent(statusCodeValue.toString())
      expect(
        screen.getByRole('img', { name: 'Unrecognised status code' })
      ).toBeInTheDocument()
    }
  )

  it.each([
    { statusCodeValue: 0, statusCode: 'Unset' },
    { statusCodeValue: 1, statusCode: 'Ok' },
    { statusCodeValue: 2, statusCode: 'Error' },
  ])(
    'does not mark recognised status code $statusCodeValue as a defect',
    ({ statusCodeValue, statusCode }) => {
      renderWithContexts(TraceDetailView, {
        span: makeSpan({ statusCode, statusCodeValue }),
      })

      expect(
        screen.queryByRole('img', { name: 'Unrecognised status code' })
      ).not.toBeInTheDocument()
    }
  )
})

describe('TraceDetailView parent span link', () => {
  beforeEach(() => {
    setTestUrl('/traces/trace-1?span=child-span&event=3&start=0&end=1')
  })

  it('selects the parent span and clears the selected event', async () => {
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

    const rootLabel = screen.getByText('(root)')
    const heading = rootLabel.closest('summary')
    expect(heading).toHaveTextContent(/^Span\s*\(root\)\s*\d+ fields/)
    const nameKey = document.querySelector('.detail-fields .detail-cell__key')
    expect(nameKey).toHaveTextContent(/^name\s*\(string\):$/)
    expect(nameKey).not.toHaveTextContent('(root)')
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

  it('uses one Activity count and opens separate event and log groups', () => {
    const logs: TraceLogSummary[] = [
      {
        id: 'log-1',
        timestamp: 5n,
        spanID: 'child-span',
        severityText: 'INFO',
        severityNumber: 9,
        serviceName: 'checkout',
        eventName: '',
        bodyPreview: '',
      },
    ]
    renderWithContexts(TraceDetailView, {
      span: makeSpan({
        events: [
          {
            name: 'ready',
            timestamp: 4n,
            attributes: [],
            droppedAttributesCount: 0,
          },
        ],
      }),
      logs,
      selectedLogID: 'log-1',
    })

    expect(screen.queryByRole('tab', { name: /Events/ })).toBeNull()
    expect(screen.getByRole('tab', { name: /Activity.*2/ })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByText('Events')).toBeVisible()
    expect(screen.getByText('Logs')).toBeVisible()
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
