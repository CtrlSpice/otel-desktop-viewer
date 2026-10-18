// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/svelte'
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
    const field = table.querySelector('[data-field="kind"]')!
    expect(field.querySelector('.detail-property__type')).toHaveTextContent(
      'enum'
    )
    expect(field.querySelector('.detail-property__value')).toHaveTextContent(
      /^-1$/
    )
    expect(table.querySelectorAll('[data-field]')).toHaveLength(1)
    expect(table).not.toHaveTextContent('Unknown (-1)')
    expect(
      table.closest('details')?.querySelector('summary')
    ).toHaveTextContent(/^Span$/)
  })

  it('shows only the authoritative unknown status code when filtered by statusCodeValue', () => {
    renderWithContexts(TraceDetailView, {
      span: makeSpan({ statusCode: 'Unknown (99)', statusCodeValue: 99 }),
      columnFilter: [spanField('statusCodeValue')],
    })

    const table = screen.getByRole('table', { name: 'Span fields' })
    const field = table.querySelector('[data-field="status.code"]')!
    expect(field.querySelector('.detail-property__type')).toHaveTextContent(
      'enum'
    )
    expect(field.querySelector('.detail-property__value')).toHaveTextContent(
      /^99$/
    )
    expect(table.querySelectorAll('[data-field]')).toHaveLength(1)
    expect(table).not.toHaveTextContent('Unknown (99)')
    expect(
      screen.getByRole('img', { name: 'Unrecognised status code' })
    ).toBeInTheDocument()
    expect(
      table.closest('details')?.querySelector('summary')
    ).toHaveTextContent(/^Span$/)
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

    const nameKey = document.querySelector(
      '[data-field="name"] .detail-property__key'
    )
    expect(nameKey).toHaveTextContent(/^name$/)
    expect(nameKey).not.toHaveTextContent('(root)')
  })
})

describe('TraceDetailView detail bubbles', () => {
  it('renders span, resource and scope fields in one flat table without changing stored keys', () => {
    const span = makeSpan({
      attributes: [
        {
          key: 'service.name',
          value: { kind: 'string', value: 'span-service' },
        },
      ],
      resource: {
        attributes: [
          {
            key: 'service.name',
            value: { kind: 'string', value: 'resource-service' },
          },
        ],
        droppedAttributesCount: 1,
      },
      scope: {
        name: 'test-scope',
        version: '1.2',
        attributes: [{ key: 'build', value: { kind: 'int64', value: 42n } }],
        droppedAttributesCount: 2,
      },
    })
    renderWithContexts(TraceDetailView, { span })
    const table = screen.getByRole('table', { name: 'Span fields' })
    for (const [key, origin, type, value] of [
      ['service.name', '', 'string', 'span-service'],
      ['service.name', 'resource', 'string', 'resource-service'],
      ['name', 'scope', 'string', 'test-scope'],
      ['version', 'scope', 'string', '1.2'],
      ['build', 'scope', 'int64', '42'],
      ['droppedAttributesCount', 'resource', 'uint32', '1'],
      ['droppedAttributesCount', 'scope', 'uint32', '2'],
    ]) {
      const label = origin ? `Copy ${key} (${origin})` : `Copy ${key}`
      const row = within(table)
        .getByRole('button', { name: label })
        .closest('tr')!
      expect(row.querySelector('.detail-property__key')).toHaveTextContent(key!)
      expect(row.querySelector('.detail-property__type')).toHaveTextContent(
        type!
      )
      expect(row.querySelector('.detail-property__body')).toHaveTextContent(
        value!
      )
      if (origin)
        expect(row.querySelector('.detail-property__origin')).toHaveTextContent(
          origin
        )
      else expect(row.querySelector('.detail-property__origin')).toBeNull()
    }
    expect(table).not.toHaveTextContent('resource.service.name')
    expect(table.closest('details')!.querySelectorAll('details')).toHaveLength(
      0
    )
    expect(span.resource.attributes[0]!.key).toBe('service.name')
    expect(span.scope.attributes[0]!.value).toEqual({
      kind: 'int64',
      value: 42n,
    })
  })

  it('stacks the former tabs as independently collapsible sections', async () => {
    setTestUrl('/traces/trace-1?span=child-span&start=0&end=1')
    renderWithContexts(TraceDetailView, {
      span: makeSpan({
        events: [
          {
            name: 'ready',
            timestamp: 1n,
            attributes: [],
            droppedAttributesCount: 0,
          },
        ],
        links: [
          {
            traceID: 'linked-trace',
            spanID: 'linked-span',
            traceState: '',
            attributes: [],
            droppedAttributesCount: 0,
            flags: 0,
          },
        ],
      }),
    })
    expect(screen.queryByRole('tablist')).toBeNull()
    const fields = screen
      .getByText('Span', { exact: true, selector: '.collapse-title > span' })
      .closest('details')!
    const activity = screen
      .getByText('Events', { exact: true })
      .closest('details')!
    const links = screen.getByText('Links', { exact: true }).closest('details')!
    expect(fields.open).toBe(true)
    expect(activity.open).toBe(true)
    expect(links.open).toBe(true)
    await userEvent.click(fields.querySelector('summary')!)
    expect(fields.open).toBe(false)
    expect(activity.open).toBe(true)
    expect(links.open).toBe(true)
  })

  it('keeps zero-count Events and Links headers visible without expandable bodies', async () => {
    renderWithContexts(TraceDetailView, { span: makeSpan() })
    for (const label of ['Events', 'Links']) {
      const section = screen.getByRole('region', { name: label })
      expect(section).toHaveTextContent(
        label === 'Events' ? '0 events' : '0 links'
      )
      await userEvent.click(within(section).getByText(label, { exact: true }))
      expect(
        section.querySelector('details, summary, .collapse-content')
      ).toBeNull()
    }
  })

  it('combines span events and correlated logs under Events with provenance badges', () => {
    const logs: TraceLogSummary[] = [
      {
        logRef: 'log-1',
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
      selectedLogRef: 'log-1',
    })

    expect(
      screen.getByText('Events', { exact: true }).closest('summary')
    ).toHaveTextContent('2 events')
    expect(
      screen.getByText('Events', { exact: true }).closest('details')
    ).toHaveAttribute('open')
    expect(screen.getByText('Span event', { exact: true })).toBeVisible()
    expect(screen.getByText('Log', { exact: true })).toBeVisible()
    expect(
      screen
        .getByText('Events', { exact: true })
        .closest('details')!
        .querySelectorAll('details')
    ).toHaveLength(0)
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
