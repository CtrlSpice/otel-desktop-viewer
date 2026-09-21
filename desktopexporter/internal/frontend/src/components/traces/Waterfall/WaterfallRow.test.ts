// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render } from '@testing-library/svelte'
import type { WaterfallRowData } from './WaterfallView.svelte'
import type { SpanData, SpanNode } from '@/types/api-types'
import WaterfallRowHarness from '@/test/WaterfallRowHarness.svelte'

// The cycle badge is the only UI that reads `salvaged` / `cyclePoint`
// directly: it must stay silent on every healthy span, appear once a span
// is salvaged, and switch to the heavier cycle-point presentation on the
// retained cut -- with wording that lets a reader tell the two states apart.

function makeSpanData(overrides: Partial<SpanData> = {}): SpanData {
  return {
    traceID: 'trace-1',
    traceState: '',
    spanID: 'span-1',
    parentSpanID: null,
    flags: 0,
    name: 'span-1',
    kindCode: 1,
    kind: 'Internal',
    startTime: 0n,
    endTime: 1_000_000n,
    attributes: [],
    events: [],
    links: [],
    resource: { attributes: [], droppedAttributesCount: 0 },
    scope: {
      name: '',
      version: '',
      attributes: [],
      droppedAttributesCount: 0,
    },
    droppedAttributesCount: 0,
    droppedEventsCount: 0,
    droppedLinksCount: 0,
    statusCode: 'Ok',
    statusCodeValue: 1,
    statusMessage: '',
    ...overrides,
  }
}

function makeRow(spanNodeOverrides: Partial<SpanNode> = {}): WaterfallRowData {
  const spanNode: SpanNode = {
    spanData: makeSpanData(),
    depth: 0,
    matched: true,
    ...spanNodeOverrides,
  }
  return {
    spanNode,
    color: '#8899aa',
    isError: false,
    offsetPercent: 0,
    widthPercent: 50,
    tree: { childrenCount: 0, isLastChild: false, ancestorHasNextSibling: [] },
    records: [],
  }
}

function renderRow(
  row: WaterfallRowData,
  matched = false,
  callbacks: {
    onRowClick?: () => void
    onSelectTimelineRecord?: () => void
  } = {}
) {
  return render(WaterfallRowHarness, {
    props: {
      row,
      barGridPercents: [],
      selected: false,
      tabbable: false,
      visible: true,
      subtreeCollapsed: false,
      rowIndex: 1,
      spanColWidth: 200,
      serviceColWidth: 100,
      timelineWidth: 400,
      traceStart: 0n,
      traceEnd: 1_000_000n,
      matched,
      onRowClick: callbacks.onRowClick ?? (() => {}),
      onToggleExpand: () => {},
      onSelectTimelineRecord: callbacks.onSelectTimelineRecord ?? (() => {}),
    },
  })
}

function cycleBadge(container: HTMLElement): HTMLElement | null {
  return container.querySelector('.waterfall-row__cycle')
}

describe('WaterfallRow direct match badge', () => {
  it('shows a visible badge only for a direct search match', () => {
    const { queryByText: queryWithoutMatch } = renderRow(makeRow())
    expect(queryWithoutMatch('Match')).toBeNull()

    const { getByText } = renderRow(makeRow(), true)
    expect(getByText('Match')).toBeVisible()
  })
})

describe('WaterfallRow timeline layout', () => {
  it('keeps the duration after the bar and retains the production bar', () => {
    const { container } = renderRow(makeRow())
    const label = container.querySelector('.waterfall-row__bar-label')
    const bar = container.querySelector('.waterfall-row__bar')
    expect(label?.getAttribute('style')?.replace(/\s+/g, ' ')).toContain(
      'calc(50% + var(--bar-label-gap))'
    )
    expect(label).not.toHaveClass('waterfall-row__bar-label--inside')
    expect(bar).toHaveClass('waterfall-row__bar')
  })
})

describe('WaterfallRow activity selection', () => {
  it('does not turn a popover row click into a span-row click', async () => {
    const row = makeRow()
    row.records = [
      {
        kind: 'event',
        id: 'event:span-1:0',
        timestamp: 10n,
        eventIndex: 0,
        event: {
          name: 'ready',
          timestamp: 10n,
          attributes: [],
          droppedAttributesCount: 0,
        },
      },
    ]
    const onRowClick = vi.fn()
    const onSelectTimelineRecord = vi.fn()
    const view = renderRow(row, false, {
      onRowClick,
      onSelectTimelineRecord,
    })

    await fireEvent.focus(view.getByRole('button', { name: 'Select 1 event' }))
    await fireEvent.click(view.getByRole('button', { name: /Event.*ready/ }))

    expect(onSelectTimelineRecord).toHaveBeenCalledTimes(1)
    expect(onRowClick).not.toHaveBeenCalled()
  })
})

describe('WaterfallRow cycle badge', () => {
  it('renders no badge on a healthy span', () => {
    const { container } = renderRow(makeRow())
    expect(cycleBadge(container)).toBeNull()
  })

  it('renders no badge when cyclePoint is set but salvaged is not', () => {
    // The wire format never emits this state, but the component still guards
    // against a malformed payload on its own.
    const { container } = renderRow(
      makeRow({ salvaged: undefined, cyclePoint: true })
    )
    expect(cycleBadge(container)).toBeNull()
  })

  it('shows the recovered badge on a salvaged, non-cycle-point span', () => {
    const { container } = renderRow(
      makeRow({ salvaged: true, cyclePoint: false })
    )
    const badge = cycleBadge(container)
    expect(badge).not.toBeNull()
    expect(badge).not.toHaveClass('waterfall-row__cycle--cycle-point')
    expect(badge!.getAttribute('aria-label')).toContain(
      'Recovered from a broken part of this trace'
    )
    expect(badge).toHaveAttribute('role', 'img')
    expect(badge!.getAttribute('title')).toContain(
      'Recovered from a broken part of this trace'
    )
    // Warning glyph in the warning tint: no cycle-point escalation or biohazard.
    expect(badge!.textContent).toContain('⚠')
    expect(badge!.querySelector('svg')).toBeNull()
    expect(badge!.classList.contains('waterfall-row__cycle--cycle-point')).toBe(
      false
    )
  })

  it('shows the cycle-point badge and distinct wording when cyclePoint is true', () => {
    const { container } = renderRow(
      makeRow({ salvaged: true, cyclePoint: true })
    )
    const badge = cycleBadge(container)
    expect(badge).not.toBeNull()
    expect(badge).toHaveClass('waterfall-row__cycle--cycle-point')
    expect(badge!.getAttribute('aria-label')).toContain('Cycle detected')
    // The biohazard svg, not the warning glyph.
    expect(badge!.querySelector('svg')).not.toBeNull()
    expect(badge!.textContent).not.toContain('⚠')
    expect(badge!.classList.contains('waterfall-row__cycle--cycle-point')).toBe(
      true
    )
  })

  it('gives the cycle point and other recovered spans distinguishable labels', () => {
    const { container: recoveredContainer } = renderRow(
      makeRow({ salvaged: true, cyclePoint: false })
    )
    const { container: cyclePointContainer } = renderRow(
      makeRow({ salvaged: true, cyclePoint: true })
    )
    const recoveredLabel =
      cycleBadge(recoveredContainer)!.getAttribute('aria-label')
    const cyclePointLabel =
      cycleBadge(cyclePointContainer)!.getAttribute('aria-label')
    expect(recoveredLabel).not.toBe(cyclePointLabel)
  })
})
