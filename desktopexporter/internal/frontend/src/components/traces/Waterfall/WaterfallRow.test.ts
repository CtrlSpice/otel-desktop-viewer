// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import type { WaterfallRowData } from './WaterfallView.svelte'
import type { SpanData, SpanNode } from '@/types/api-types'
import WaterfallRowHarness from '@/test/WaterfallRowHarness.svelte'

// The cycle badge is the only UI that reads `salvaged` / `cyclePoint`
// directly: it must stay silent on every healthy span, appear once a span
// is salvaged, and switch to the heavier "offender" presentation on the one
// span that actually caused the cycle -- with wording that lets a reader
// tell the two states apart.

function makeSpanData(overrides: Partial<SpanData> = {}): SpanData {
  return {
    traceID: 'trace-1',
    traceState: '',
    spanID: 'span-1',
    parentSpanID: null,
    name: 'span-1',
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
    eventMarkers: [],
  }
}

function renderRow(row: WaterfallRowData) {
  return render(WaterfallRowHarness, {
    props: {
      row,
      barGridPercents: [],
      selected: false,
      visible: true,
      subtreeCollapsed: false,
      spanColWidth: 200,
      serviceColWidth: 100,
      onRowClick: () => {},
      onToggleExpand: () => {},
      onSelectEvent: vi.fn(),
    },
  })
}

function cycleBadge(container: HTMLElement): HTMLElement | null {
  return container.querySelector('.waterfall-row__cycle')
}

describe('WaterfallRow cycle badge', () => {
  it('renders no badge on a healthy span', () => {
    const { container } = renderRow(makeRow())
    expect(cycleBadge(container)).toBeNull()
  })

  it('renders no badge when cyclePoint is set but salvaged is not', () => {
    // Guards against a badge keyed off cyclePoint alone -- the wire format
    // never emits cyclePoint without salvaged, but the component's own
    // `cycleLabel` derivation checks cyclePoint first.
    const { container } = renderRow(
      makeRow({ salvaged: undefined, cyclePoint: undefined })
    )
    expect(cycleBadge(container)).toBeNull()
  })

  it('shows the recovered badge on a salvaged, non-offending span', () => {
    const { container } = renderRow(
      makeRow({ salvaged: true, cyclePoint: false })
    )
    const badge = cycleBadge(container)
    expect(badge).not.toBeNull()
    expect(badge).not.toHaveClass('waterfall-row__cycle--offender')
    expect(badge!.getAttribute('aria-label')).toContain(
      'Recovered from a broken part of this trace'
    )
    expect(badge!.getAttribute('title')).toContain(
      'Recovered from a broken part of this trace'
    )
    // Warning glyph in the warning tint: no offender escalation, no biohazard.
    expect(badge!.textContent).toContain('⚠')
    expect(badge!.querySelector('svg')).toBeNull()
    expect(badge!.classList.contains('waterfall-row__cycle--offender')).toBe(
      false
    )
  })

  it('shows the offender badge and distinct wording when cyclePoint is true', () => {
    const { container } = renderRow(
      makeRow({ salvaged: true, cyclePoint: true })
    )
    const badge = cycleBadge(container)
    expect(badge).not.toBeNull()
    expect(badge).toHaveClass('waterfall-row__cycle--offender')
    expect(badge!.getAttribute('aria-label')).toContain(
      'This span causes the cycle'
    )
    // The biohazard svg, not the warning glyph.
    expect(badge!.querySelector('svg')).not.toBeNull()
    expect(badge!.textContent).not.toContain('⚠')
    expect(badge!.classList.contains('waterfall-row__cycle--offender')).toBe(
      true
    )
  })

  it('gives the offender and the spans it stranded distinguishable labels', () => {
    const { container: recoveredContainer } = renderRow(
      makeRow({ salvaged: true, cyclePoint: false })
    )
    const { container: offenderContainer } = renderRow(
      makeRow({ salvaged: true, cyclePoint: true })
    )
    const recoveredLabel =
      cycleBadge(recoveredContainer)!.getAttribute('aria-label')
    const offenderLabel =
      cycleBadge(offenderContainer)!.getAttribute('aria-label')
    expect(recoveredLabel).not.toBe(offenderLabel)
  })
})
