// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { waitFor } from '@testing-library/svelte'
import WaterfallView, {
  buildWaterfallRows,
  getTraceBounds,
} from './WaterfallView.svelte'
import type { SpanNode } from '@/types/api-types'
import { traceServiceColor } from '@/utils/trace-service-color'
import { renderWithContexts } from '@/test/render-helpers'
import { resetCollapseStoreForTests } from './waterfall-collapse-store'
import { scrollMock } from '@/test/mock-virtual-list'
import {
  columnCell,
  columnOptions,
  columnID,
  loadColumns,
  saveColumns,
} from './waterfall-columns'

vi.mock('@humanspeak/svelte-virtual-list', async () => {
  const { default: MockVirtualList } =
    await import('@/test/MockVirtualList.svelte')
  return { default: MockVirtualList }
})

function spanNode(
  id: string,
  parentID: string | null,
  depth: number
): SpanNode {
  return {
    depth,
    matched: true,
    spanData: {
      spanID: id,
      flags: 0,
      parentSpanID: parentID,
      traceID: 'trace-1',
      name: id,
      kindCode: 1,
      startTime: BigInt(depth) * 1_000_000n,
      endTime: BigInt(depth + 1) * 1_000_000n,
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
      kind: '',
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusMessage: '',
    },
  }
}

describe('Waterfall direct-child counts', () => {
  it('counts children separately from deeper descendants in the rendered tree', () => {
    const spans = [
      spanNode('root', null, 0),
      spanNode('a', 'root', 1),
      spanNode('a1', 'a', 2),
      spanNode('a2', 'a', 2),
      spanNode('b', 'root', 1),
      spanNode('orphan', 'missing', 0),
      spanNode('recovered', 'wrong-parent', 1),
    ]
    const rows = buildWaterfallRows(
      spans,
      getTraceBounds(spans),
      'madoromi-aki-yoru'
    )
    expect(rows.map(row => row.tree.childrenCount)).toEqual([
      2, 2, 0, 0, 0, 1, 0,
    ])
    expect(rows.map(row => row.tree.brokenParent)).toEqual([
      false,
      false,
      false,
      false,
      false,
      true,
      true,
    ])
  })
})

describe('Waterfall attribute values', () => {
  it('keeps built-in fields separate from identically named attributes and preserves precision', () => {
    const span = spanNode('first', null, 0).spanData
    span.startTime = 18446744073709551614n
    span.endTime = 18446744073709551615n
    span.kindCode = 99
    span.kind = 'Unknown (99)'
    span.attributes = [
      { key: 'kind', value: { kind: 'string', value: 'received attribute' } },
    ]
    const field = { source: 'span', key: 'kind', field: true } as const
    const attribute = { source: 'span', key: 'kind' } as const
    expect(columnID(field)).not.toBe(columnID(attribute))
    expect(columnCell(span, field).text).toBe('99 (Unknown (99))')
    expect(columnCell(span, attribute).text).toBe('received attribute')
    expect(
      columnCell(span, {
        source: 'span',
        key: 'startTimeUnixNano',
        field: true,
      })
    ).toMatchObject({ text: '18446744073709551614', numeric: true })
    expect(
      columnCell(span, { source: 'span', key: 'duration', field: true }).title
    ).toContain('1 ns')
    expect(
      columnCell(span, { source: 'span', key: 'parentSpanId', field: true })
        .missing
    ).toBe(true)
    expect(
      columnCell(span, { source: 'span', key: 'flags', field: true })
    ).toMatchObject({ text: '0', missing: false })
    expect(
      columnCell(span, { source: 'scope', key: 'name', field: true })
    ).toMatchObject({ text: '""', missing: false })
  })

  it('defaults service.name on while remembering empty layouts and field identities', () => {
    localStorage.clear()
    expect(loadColumns()).toEqual([{ source: 'resource', key: 'service.name' }])
    saveColumns([])
    expect(loadColumns()).toEqual([])
    const columns = [
      { source: 'scope', key: 'name', field: true },
      { source: 'scope', key: 'name' },
    ] as const
    saveColumns(columns)
    expect(loadColumns()).toEqual(columns)
    localStorage.clear()
  })

  it('keeps owners separate, counts spans once, and retains selected absent keys', () => {
    const span = spanNode('first', null, 0)
    span.spanData.attributes = [
      { key: 'same', value: { kind: 'int64', value: 9223372036854775807n } },
      { key: 'same', value: { kind: 'bool', value: false } },
    ]
    span.spanData.resource.attributes = [
      { key: 'same', value: { kind: 'string', value: 'resource value' } },
    ]
    expect(
      columnOptions([span], [{ source: 'scope', key: 'absent' }]).filter(
        column => !column.field
      )
    ).toMatchObject([
      { source: 'span', key: 'same', count: 1 },
      { source: 'resource', key: 'same', count: 1 },
      { source: 'scope', key: 'absent', count: 0 },
    ])
    expect(
      columnCell(span.spanData, { source: 'span', key: 'same' }).text
    ).toBe('9223372036854775807 | false')
    expect(
      columnCell(span.spanData, { source: 'resource', key: 'same' }).text
    ).toBe('resource value')
    expect(
      columnCell(span.spanData, { source: 'scope', key: 'absent' }).missing
    ).toBe(true)
  })

  it('distinguishes absent, empty, null, zero and negative zero without losing nested values', () => {
    const span = spanNode('first', null, 0).spanData
    span.attributes = [
      { key: 'empty string', value: { kind: 'string', value: '' } },
      { key: 'null', value: { kind: 'empty', value: null } },
      { key: 'zero', value: { kind: 'int64', value: 0n } },
      { key: 'negative zero', value: { kind: 'double', value: -0 } },
      {
        key: 'map',
        value: {
          kind: 'map',
          value: [
            { key: 'a', value: { kind: 'int64', value: 9223372036854775807n } },
            { key: 'a', value: { kind: 'string', value: 'two  spaces' } },
          ],
        },
      },
    ]
    const cell = (key: string) => columnCell(span, { source: 'span', key })
    expect(cell('absent')).toMatchObject({ text: '—', missing: true })
    expect(cell('empty string')).toMatchObject({ text: '""', missing: false })
    expect(cell('null').text).toBe('null')
    expect(cell('zero').text).toBe('0')
    expect(cell('negative zero').text).toBe('-0')
    expect(cell('map').text).toContain('"a": 9223372036854775807,')
    expect(cell('map').text).toContain('"a": "two  spaces"')
  })
})

describe('WaterfallView bigint domain geometry', () => {
  it('uses domain bigint span and event times without reparsing', () => {
    const spans = [spanNode('first', null, 0), spanNode('second', null, 0)]
    spans[0]!.spanData.startTime = 100n
    spans[0]!.spanData.endTime = 200n
    spans[0]!.spanData.events = [
      {
        name: 'midpoint',
        timestamp: 150n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    spans[1]!.spanData.startTime = 200n
    spans[1]!.spanData.endTime = 300n

    const bounds = getTraceBounds(spans)
    const rows = buildWaterfallRows(spans, bounds, 'dark')

    expect(bounds).toEqual({ start: 100n, end: 300n, duration: 200n })
    expect(rows[0]).toMatchObject({
      offsetPercent: 0,
      widthPercent: 50,
      records: [{ id: 'event:first:0', eventIndex: 0 }],
    })
    expect(rows[1]).toMatchObject({ offsetPercent: 50, widthPercent: 50 })
  })

  it('keeps service colours across operations while preserving error semantics', () => {
    const spans = [
      spanNode('first', null, 0),
      spanNode('second', null, 0),
      spanNode('error', null, 0),
    ]
    spans[2]!.spanData.statusCodeValue = 2

    const rows = buildWaterfallRows(
      spans,
      getTraceBounds(spans),
      'madoromi-fuyu-hiru'
    )

    expect(rows.map(row => row.color)).toEqual([
      traceServiceColor('', 'madoromi-fuyu-hiru'),
      traceServiceColor('', 'madoromi-fuyu-hiru'),
      'var(--color-error)',
    ])
    for (const span of spans)
      span.spanData.resource.attributes = [
        {
          key: 'service.name',
          value: { kind: 'string', value: 'one-service' },
        },
      ]
    const serviceRows = buildWaterfallRows(
      spans,
      getTraceBounds(spans),
      'madoromi-fuyu-hiru'
    )
    expect(serviceRows[0].color).toBe(
      traceServiceColor('one-service', 'madoromi-fuyu-hiru')
    )
    expect(serviceRows[1].color).toBe(serviceRows[0].color)
    expect(serviceRows[2].color).toBe('var(--color-error)')
  })
})

function deepTree(): SpanNode[] {
  return [
    spanNode('a', null, 0),
    spanNode('b', 'a', 1),
    spanNode('c', 'b', 2),
    spanNode('d', 'c', 3),
    spanNode('e', 'd', 4),
    spanNode('f', 'e', 5),
  ]
}

const ALL_IDS = ['a', 'b', 'c', 'd', 'e', 'f']

function renderTree(overrides: Record<string, unknown> = {}) {
  return renderWithContexts(WaterfallView, {
    spans: deepTree(),
    selectedSpanID: null,
    onSelectSpan: vi.fn(),
    onSelectTimelineRecord: vi.fn(),
    ...overrides,
  })
}

const SERVICE_COLUMN_ID = '["resource","service.name"]'
type PersistedColumnWidths = Record<
  'span' | 'timeline' | typeof SERVICE_COLUMN_ID,
  number
>

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isPersistedColumnWidths(
  value: unknown
): value is PersistedColumnWidths {
  return (
    value !== null &&
    typeof value === 'object' &&
    'span' in value &&
    SERVICE_COLUMN_ID in value &&
    'timeline' in value &&
    isFiniteNumber(value.span) &&
    isFiniteNumber(value[SERVICE_COLUMN_ID]) &&
    isFiniteNumber(value.timeline)
  )
}

function readPersistedColumnWidths(): PersistedColumnWidths {
  const parsed: unknown = JSON.parse(
    localStorage.getItem('waterfall-column-widths') ?? 'null'
  )
  if (!isPersistedColumnWidths(parsed)) {
    throw new Error('Expected finite persisted waterfall column widths')
  }
  return parsed
}

function rowIDs(): string[] {
  return [...document.querySelectorAll('tr[data-span-id]')].map(r =>
    r.getAttribute('data-span-id')!
  )
}

function spanRow(id: string): HTMLTableRowElement {
  const row = document.querySelector<HTMLTableRowElement>(
    `tr[data-span-id="${id}"]`
  )
  expect(row, `row ${id} should exist`).toBeTruthy()
  return row!
}

function navigationSpans(): SpanNode[] {
  const spans = [
    spanNode('healthy-before', null, 0),
    spanNode('error-1', null, 0),
    spanNode('exception-only', null, 0),
    spanNode('healthy-middle', null, 0),
    spanNode('error-2', null, 0),
    spanNode('healthy-after', null, 0),
  ]
  spans[1]!.spanData.statusCodeValue = 2
  spans[2]!.spanData.events = [
    {
      name: 'exception',
      timestamp: 0n,
      attributes: [],
      droppedAttributesCount: 0,
    },
  ]
  spans[4]!.spanData.statusCodeValue = 2
  return spans
}

function buttonByLabel(label: string): HTMLButtonElement {
  const button = [
    ...document.querySelectorAll<HTMLButtonElement>('button'),
  ].find(candidate => candidate.getAttribute('aria-label') === label)
  expect(button, `button labelled "${label}" should exist`).toBeTruthy()
  return button!
}

/** Collapse a row through its real toggle, the way a reader does. */
async function collapseRow(id: string) {
  const btn = document.querySelector<HTMLElement>(
    `tr[data-span-id="${id}"] button[aria-expanded="true"]`
  )
  expect(btn, `row ${id} should have an expanded toggle`).not.toBeNull()
  btn!.click()
  await tick()
}

describe('WaterfallView error navigation', () => {
  beforeEach(() => {
    scrollMock.mockClear()
    resetCollapseStoreForTests()
  })

  it('walks status errors forward and backward in waterfall order', async () => {
    const spans = navigationSpans()
    const onSelectSpan = vi.fn()
    const view = renderTree({ spans, onSelectSpan })
    await tick()
    const badge = view.getByRole('group', { name: 'Error navigation' })
    expect(badge).toHaveTextContent(/^\[2 err\]$/)

    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-1')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-1', onSelectSpan },
    })
    expect(badge).toHaveTextContent(/^\[1\/2 err\]$/)
    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-2')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-2', onSelectSpan },
    })
    expect(badge).toHaveTextContent(/^\[2\/2 err\]$/)
    buttonByLabel('Previous error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-1')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-1', onSelectSpan },
    })
    expect(badge).toHaveTextContent(/^\[1\/2 err\]$/)
    for (const selectedSpanID of ['healthy-middle', 'exception-only', null]) {
      await view.rerender({
        componentProps: { spans, selectedSpanID, onSelectSpan },
      })
      expect(badge).toHaveTextContent(/^\[2 err\]$/)
    }
  })

  it('chooses the nearest directional status error from a healthy span', async () => {
    const spans = navigationSpans()
    const onSelectSpan = vi.fn()
    const view = renderTree({
      spans,
      selectedSpanID: 'healthy-middle',
      onSelectSpan,
    })
    await tick()

    buttonByLabel('Previous error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-1')

    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-2')

    onSelectSpan.mockClear()
    await view.rerender({
      componentProps: { spans, selectedSpanID: null, onSelectSpan },
    })
    expect(buttonByLabel('Previous error')).toBeDisabled()
    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenCalledWith('error-1')
  })

  it('disables unavailable directions without wrapping or selecting', async () => {
    const spans = navigationSpans()
    const onSelectSpan = vi.fn()
    const view = renderTree({
      spans,
      selectedSpanID: 'error-1',
      onSelectSpan,
    })
    await tick()

    const previous = buttonByLabel('Previous error')
    expect(previous).toBeDisabled()
    previous.click()
    expect(onSelectSpan).not.toHaveBeenCalled()

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-2', onSelectSpan },
    })
    const next = buttonByLabel('Next error')
    expect(next).toBeDisabled()
    next.click()
    expect(onSelectSpan).not.toHaveBeenCalled()

    const healthySpans = spans.map(node => ({
      ...node,
      spanData: { ...node.spanData, statusCodeValue: 1, events: [] },
    }))
    await view.rerender({
      componentProps: {
        spans: healthySpans,
        selectedSpanID: null,
        onSelectSpan,
      },
    })
    expect(
      document.querySelector('button[aria-label="Previous error"]')
    ).not.toBeInTheDocument()
    expect(
      document.querySelector('button[aria-label="Next error"]')
    ).not.toBeInTheDocument()
  })

  it('excludes exception-only spans so navigation agrees with the error badge', async () => {
    const spans = navigationSpans()
    const onSelectSpan = vi.fn()
    renderTree({
      spans,
      selectedSpanID: 'error-1',
      onSelectSpan,
    })
    await tick()

    expect(
      document.querySelector('[role="group"][aria-label="Error navigation"]')
    ).toHaveTextContent(/^\[1\/2 err\]$/)
    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenCalledWith('error-2')
    expect(onSelectSpan).not.toHaveBeenCalledWith('exception-only')
  })

  it("uses selection scrolling without reopening the reader's collapsed branch", async () => {
    const spans = deepTree()
    spans[5]!.spanData.statusCodeValue = 2
    const onSelectSpan = vi.fn()
    const view = renderTree({
      spans,
      selectedSpanID: 'b',
      onSelectSpan,
    })
    await tick()
    await collapseRow('c')
    scrollMock.mockClear()

    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenCalledWith('f')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'f', onSelectSpan },
    })
    await waitFor(() => expect(scrollMock).toHaveBeenCalled())
    expect(rowIDs()).toEqual(['a', 'b', 'c'])
  })
})

describe('WaterfallView scroll to top', () => {
  afterEach(() => vi.restoreAllMocks())

  it('scrolls without changing selection or collapsed branches and retains keyboard focus', async () => {
    resetCollapseStoreForTests()
    const onSelectSpan = vi.fn()
    const view = renderTree({ selectedSpanID: 'b', onSelectSpan })
    await tick()
    await collapseRow('c')
    const visible = rowIDs()
    const viewport = view.container.querySelector<HTMLElement>(
      '.waterfall-vlist-viewport'
    )!
    const label = 'Scroll span tree to top'
    expect(view.queryByRole('button', { name: label })).not.toBeInTheDocument()
    viewport.scrollTop = 200
    viewport.dispatchEvent(new Event('scroll'))
    await tick()
    const button = view.getByRole('button', { name: label })
    button.focus()
    await tick()
    const media = window.matchMedia('(prefers-reduced-motion: reduce)')
    vi.spyOn(window, 'matchMedia').mockReturnValue({ ...media, matches: true })
    scrollMock.mockClear()
    button.click()
    await tick()
    expect(scrollMock).toHaveBeenCalledWith({
      index: 0,
      align: 'top',
      smoothScroll: false,
      shouldThrowOnBounds: false,
    })
    expect(onSelectSpan).not.toHaveBeenCalled()
    expect(rowIDs()).toEqual(visible)
    expect(spanRow('b')).toHaveAttribute('aria-selected', 'true')
    viewport.scrollTop = 0
    viewport.dispatchEvent(new Event('scroll'))
    await tick()
    expect(button).toHaveFocus()
    button.blur()
    await tick()
    expect(view.queryByRole('button', { name: label })).not.toBeInTheDocument()
  })
})

describe('WaterfallView collapse ownership', () => {
  beforeEach(() => {
    scrollMock.mockClear()
    resetCollapseStoreForTests()
  })

  it('opens fully expanded, however deep the trace', async () => {
    renderTree()
    await tick()
    expect(rowIDs()).toEqual(ALL_IDS)
  })

  it("keeps the reader's collapse when the spans array changes identity", async () => {
    const { rerender } = renderTree()
    await tick()
    await collapseRow('c')
    expect(rowIDs()).toEqual(['a', 'b', 'c'])

    await rerender({
      componentProps: {
        spans: deepTree(),
        selectedSpanID: null,
        onSelectSpan: vi.fn(),
      },
    })
    await tick()
    expect(rowIDs()).toEqual(['a', 'b', 'c'])
  })

  // The component is torn down by loading states, trace switches, and layout
  // changes. State lives outside it precisely so that none of those read as
  // "the waterfall collapsed itself".
  it("keeps the reader's collapse across unmount and remount", async () => {
    const first = renderTree()
    await tick()
    await collapseRow('c')
    first.unmount()

    renderTree()
    await tick()
    expect(rowIDs()).toEqual(['a', 'b', 'c'])
  })

  // The rule the reveal must obey: if the reader closed the branch, the
  // selection living inside it is not a reason to open it again.
  it('does not reopen a collapsed branch that contains the selection', async () => {
    const { rerender } = renderTree({ selectedSpanID: 'f' })
    await waitFor(() => {
      expect(document.querySelector('tr[data-span-id="f"]')).toBeInTheDocument()
    })

    await collapseRow('c')
    expect(rowIDs()).toEqual(['a', 'b', 'c'])

    await rerender({
      componentProps: {
        spans: deepTree(),
        selectedSpanID: 'f',
        onSelectSpan: vi.fn(),
      },
    })
    await tick()
    expect(rowIDs()).toEqual(['a', 'b', 'c'])
  })

  // Selecting a hidden span scrolls toward it -- to the nearest visible
  // ancestor -- rather than expanding anything.
  it('scrolls to the nearest visible ancestor of a hidden selection', async () => {
    const { rerender } = renderTree()
    await tick()
    await collapseRow('c')
    scrollMock.mockClear()

    await rerender({
      componentProps: {
        spans: deepTree(),
        selectedSpanID: 'f',
        onSelectSpan: vi.fn(),
      },
    })
    await waitFor(() => expect(scrollMock).toHaveBeenCalled())
    expect(rowIDs()).toEqual(['a', 'b', 'c'])
  })

  it('collapse-all and expand-all are separate, always-live, idempotent', async () => {
    renderTree()
    await tick()

    const byLabel = (label: string) =>
      [...document.querySelectorAll('button')].find(
        b => b.getAttribute('aria-label') === label
      )!
    const collapseAll = () => byLabel('Collapse all spans')
    const expandAll = () => byLabel('Expand all spans')

    // Both present at once, in every state -- not a toggle.
    expect(collapseAll()).toBeTruthy()
    expect(expandAll()).toBeTruthy()

    collapseAll().click()
    await tick()
    expect(rowIDs()).toEqual(['a'])

    // Idempotent: invoking again from the state it produced changes nothing.
    collapseAll().click()
    await tick()
    expect(rowIDs()).toEqual(['a'])

    expandAll().click()
    await tick()
    expect(rowIDs()).toEqual(ALL_IDS)
    expandAll().click()
    await tick()
    expect(rowIDs()).toEqual(ALL_IDS)
  })

  it('expand-all works from a mixed arrangement', async () => {
    renderTree()
    await tick()
    await collapseRow('c')
    expect(rowIDs()).toEqual(['a', 'b', 'c'])

    ;[...document.querySelectorAll('button')]
      .find(b => b.getAttribute('aria-label') === 'Expand all spans')!
      .click()
    await tick()
    expect(rowIDs()).toEqual(ALL_IDS)
  })

  // Search is a lens: it shapes the tree while active and leaves the reader's
  // arrangement untouched underneath.
  it("restores the reader's arrangement when a search clears", async () => {
    const { rerender } = renderTree()
    await tick()
    await collapseRow('e')
    expect(rowIDs()).toEqual(['a', 'b', 'c', 'd', 'e'])

    // A search response: only 'c' matches, so its childless subtree collapses
    // and unrelated branches fold away.
    const searched = deepTree().map(n => ({
      ...n,
      matched: n.spanData.spanID === 'c',
    }))
    await rerender({
      componentProps: {
        spans: searched,
        selectedSpanID: null,
        searchActive: true,
        onSelectSpan: vi.fn(),
      },
    })
    await tick()
    const during = rowIDs()
    expect(during).toContain('c')
    expect(during).not.toContain('f')
    expect(spanRow('c')).toHaveClass('waterfall-row--matched')
    expect(spanRow('c')).not.toHaveTextContent('Match')
    expect(spanRow('b')).not.toHaveTextContent('Match')

    // Clearing the search puts back exactly what the reader had: 'e' still
    // collapsed, nothing else touched.
    await rerender({
      componentProps: {
        spans: deepTree(),
        selectedSpanID: null,
        searchActive: false,
        onSelectSpan: vi.fn(),
      },
    })
    await tick()
    expect(rowIDs()).toEqual(['a', 'b', 'c', 'd', 'e'])
  })

  it('labels every direct match when an active search matches every span', async () => {
    renderTree({ searchActive: true })
    await tick()

    expect(document.querySelectorAll('.waterfall-row--matched')).toHaveLength(
      ALL_IDS.length
    )
    expect(spanRow('a')).not.toHaveClass('waterfall-row--joined-before')
    expect(spanRow('b')).toHaveClass(
      'waterfall-row--joined-before',
      'waterfall-row--joined-after'
    )
    expect(spanRow('f')).not.toHaveClass('waterfall-row--joined-after')
  })

  it('reveals a newly selected span without centering a visible row', async () => {
    const { rerender } = renderTree()
    await tick()
    scrollMock.mockClear()

    await rerender({
      componentProps: {
        spans: deepTree(),
        selectedSpanID: 'f',
        onSelectSpan: vi.fn(),
      },
    })
    await waitFor(() =>
      expect(scrollMock).toHaveBeenCalledWith(
        expect.objectContaining({
          index: expect.any(Number),
          align: 'nearest',
          smoothScroll: true,
          shouldThrowOnBounds: false,
        })
      )
    )
  })
})

describe('WaterfallView error navigation, vim brackets', () => {
  beforeEach(() => {
    scrollMock.mockClear()
    resetCollapseStoreForTests()
  })

  function grid(): HTMLElement {
    const el = document.querySelector<HTMLElement>('[role="treegrid"]')
    expect(el, 'treegrid host should exist').toBeTruthy()
    return el!
  }

  function press(key: string) {
    grid().dispatchEvent(
      new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    )
  }

  it('pages errors with [ and ], anchored to the selection like the badge', async () => {
    const spans = navigationSpans()
    const onSelectSpan = vi.fn()
    const view = renderTree({ spans, onSelectSpan })
    await tick()

    press(']')
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-1')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-1', onSelectSpan },
    })
    press(']')
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-2')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-2', onSelectSpan },
    })
    press('[')
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-1')
  })

  it('refuses to wrap at either end, like the chevrons', async () => {
    const spans = navigationSpans()
    const onSelectSpan = vi.fn()
    renderTree({ spans, onSelectSpan, selectedSpanID: 'error-2' })
    await tick()

    press(']')
    expect(onSelectSpan).not.toHaveBeenCalled()
  })
})

describe('WaterfallView hierarchy semantics', () => {
  beforeEach(() => resetCollapseStoreForTests())

  it('exposes the complete logical tree through virtualized rows', async () => {
    renderTree()
    await tick()

    const treegrid = document.querySelector('[role="treegrid"]')
    expect(treegrid).toHaveAttribute('aria-rowcount', '6')
    expect(treegrid).toHaveAttribute('aria-colcount', '3')

    expect(spanRow('a')).toHaveAttribute('role', 'row')
    expect(spanRow('a')).toHaveAttribute('aria-rowindex', '1')
    expect(spanRow('b')).toHaveAttribute('aria-rowindex', '2')
    expect(spanRow('b')).toHaveAttribute('aria-level', '2')
    expect(spanRow('b')).toHaveAttribute('aria-expanded', 'true')
    expect(spanRow('a').cells[0]).toHaveAttribute('role', 'rowheader')
    expect(spanRow('a').cells[1]).toHaveAttribute('role', 'gridcell')
    expect(spanRow('a').cells[2]).toHaveAttribute('role', 'gridcell')
  })

  it('keeps full-tree row positions when a branch is collapsed', async () => {
    const spans = [
      spanNode('root', null, 0),
      spanNode('branch', 'root', 1),
      spanNode('hidden', 'branch', 2),
      spanNode('sibling', 'root', 1),
    ]
    renderTree({ spans })
    await tick()
    await collapseRow('branch')

    expect(rowIDs()).toEqual(['root', 'branch', 'sibling'])
    expect(document.querySelector('[role="treegrid"]')).toHaveAttribute(
      'aria-rowcount',
      '4'
    )
    expect(spanRow('sibling')).toHaveAttribute('aria-rowindex', '4')
  })
})

describe('WaterfallView row keyboard navigation', () => {
  beforeEach(() => {
    scrollMock.mockClear()
    resetCollapseStoreForTests()
  })

  async function press(
    row: HTMLTableRowElement,
    key: string,
    expectedFocusedID: string
  ) {
    row.dispatchEvent(
      new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    )
    await waitFor(() => expect(spanRow(expectedFocusedID)).toHaveFocus())
  }

  function keydown(
    target: HTMLElement,
    key: string,
    modifiers: KeyboardEventInit = {}
  ): KeyboardEvent {
    const event = new KeyboardEvent('keydown', {
      key,
      bubbles: true,
      cancelable: true,
      ...modifiers,
    })
    target.dispatchEvent(event)
    return event
  }

  it('enters through the first span and traverses rows with Vim or arrow keys', async () => {
    const onSelectSpan = vi.fn()
    renderTree({ onSelectSpan })
    await tick()

    expect(spanRow('a')).toHaveAttribute('tabindex', '0')
    for (const id of ALL_IDS.slice(1)) {
      expect(spanRow(id)).toHaveAttribute('tabindex', '-1')
    }

    spanRow('a').focus()
    await press(spanRow('a'), 'j', 'b')
    expect(onSelectSpan).toHaveBeenLastCalledWith('b')

    await press(spanRow('b'), 'ArrowDown', 'c')
    expect(onSelectSpan).toHaveBeenLastCalledWith('c')

    await press(spanRow('c'), 'k', 'b')
    expect(onSelectSpan).toHaveBeenLastCalledWith('b')

    await press(spanRow('b'), 'ArrowUp', 'a')
    expect(onSelectSpan).toHaveBeenLastCalledWith('a')
  })

  it('enters through the selected span when it is visible', async () => {
    renderTree({ selectedSpanID: 'c' })
    await tick()

    expect(spanRow('a')).toHaveAttribute('tabindex', '-1')
    expect(spanRow('c')).toHaveAttribute('tabindex', '0')
  })

  it('moves through pages and accepts Ctrl+Home and Ctrl+End treegrid navigation', async () => {
    const onSelectSpan = vi.fn()
    renderTree({ onSelectSpan })
    await tick()

    spanRow('a').focus()
    expect(keydown(spanRow('a'), 'PageDown').defaultPrevented).toBe(true)
    await waitFor(() => expect(spanRow('f')).toHaveFocus())

    expect(
      keydown(spanRow('f'), 'Home', { ctrlKey: true }).defaultPrevented
    ).toBe(true)
    await waitFor(() => expect(spanRow('a')).toHaveFocus())

    expect(
      keydown(spanRow('a'), 'End', { ctrlKey: true }).defaultPrevented
    ).toBe(true)
    await waitFor(() => expect(spanRow('f')).toHaveFocus())
    expect(onSelectSpan).toHaveBeenLastCalledWith('f')
  })

  it('leaves selected trace-ID text available to native copy shortcuts', async () => {
    const traceID = '0123456789abcdef0123456789abcdef'
    const onSelectSpan = vi.fn()
    const spans = deepTree()
    spans[0]!.spanData.traceID = traceID
    spans[0]!.spanData.name = traceID
    renderTree({ spans, selectedSpanID: 'a', onSelectSpan })
    await tick()

    const first = spanRow('a')
    first.focus()
    const traceIDText = first.querySelector<HTMLElement>(
      '.waterfall-row__title .table-value-cell__inspect'
    )
    expect(traceIDText).toHaveTextContent(traceID)
    const selection = window.getSelection()!
    const range = document.createRange()
    range.selectNodeContents(traceIDText!)
    selection.removeAllRanges()
    selection.addRange(range)

    expect(selection.toString()).toBe(traceID)
    for (const [key, modifiers] of [
      ['c', { metaKey: true }],
      ['c', { ctrlKey: true }],
    ] as const) {
      const event = keydown(traceIDText!, key, modifiers)
      expect(event.defaultPrevented).toBe(false)
      expect(selection.toString()).toBe(traceID)
      expect(first).toHaveFocus()
    }
    expect(onSelectSpan).not.toHaveBeenCalled()
  })

  it('leaves browser and platform modifier shortcuts unhandled', async () => {
    const onSelectSpan = vi.fn()
    renderTree({ onSelectSpan })
    await tick()

    const first = spanRow('a')
    first.focus()
    for (const [key, modifiers] of [
      ['j', { ctrlKey: true }],
      ['PageDown', { ctrlKey: true }],
      ['ArrowLeft', { ctrlKey: true }],
      ['ArrowLeft', { altKey: true }],
    ] as const) {
      const event = keydown(first, key, modifiers)
      expect(event.defaultPrevented).toBe(false)
      expect(first).toHaveFocus()
    }
    expect(onSelectSpan).not.toHaveBeenCalled()
  })

  it('keeps unmodified ArrowLeft collapse navigation', async () => {
    renderTree()
    await tick()

    const first = spanRow('a')
    first.focus()
    expect(keydown(first, 'ArrowLeft').defaultPrevented).toBe(true)
    expect(spanRow('a')).toHaveFocus()
  })

  it('keeps the treegrid scroll viewport out of the row tab sequence', async () => {
    renderTree()
    await tick()

    expect(
      document.querySelector('[role="treegrid"][aria-label="Span waterfall"]')
    ).toHaveAttribute('tabindex', '-1')
  })
})

describe('WaterfallView column separators', () => {
  let containerWidth = 800

  beforeEach(() => {
    containerWidth = 800
    localStorage.clear()
    resetCollapseStoreForTests()
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(
      () => containerWidth
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  function separators(): HTMLElement[] {
    return [...document.querySelectorAll<HTMLElement>('[role="separator"]')]
  }

  function press(
    separator: HTMLElement,
    key: string,
    shiftKey = false
  ): KeyboardEvent {
    const event = new KeyboardEvent('keydown', {
      key,
      shiftKey,
      bubbles: true,
      cancelable: true,
    })
    separator.dispatchEvent(event)
    return event
  }

  it('exposes one focusable separator per boundary with its reachable range', async () => {
    renderTree()
    await tick()

    const bars = separators()
    expect(bars).toHaveLength(2)
    for (const bar of bars) {
      expect(bar).toHaveAttribute('tabindex', '0')
      expect(Number(bar.getAttribute('aria-valuemin'))).toBeLessThanOrEqual(
        Number(bar.getAttribute('aria-valuenow'))
      )
      expect(Number(bar.getAttribute('aria-valuenow'))).toBeLessThanOrEqual(
        Number(bar.getAttribute('aria-valuemax'))
      )
      expect(Number(bar.getAttribute('aria-valuemin'))).toBeGreaterThanOrEqual(
        0
      )
      expect(Number(bar.getAttribute('aria-valuemax'))).toBeLessThanOrEqual(100)
      expect(bar).toHaveAttribute(
        'aria-valuetext',
        `${bar.getAttribute('aria-valuenow')} percent from left`
      )
    }

    expect(document.querySelectorAll('.resize-handle')).toHaveLength(0)
    expect(
      document.querySelector('[role="treegrid"][aria-label="Span waterfall"]')
    ).toBeInTheDocument()
  })

  it('nudges through the shared cascade and persists each keyboard change', async () => {
    localStorage.setItem(
      'waterfall-column-widths',
      JSON.stringify({ span: 200, [SERVICE_COLUMN_ID]: 100, timeline: 500 })
    )
    renderTree()
    await tick()

    const spanBar = separators()[0]!
    spanBar.focus()
    const start = Number.parseFloat(spanBar.style.left)
    const startPercent = Number(spanBar.getAttribute('aria-valuenow'))

    const right = press(spanBar, 'ArrowRight')
    expect(right.defaultPrevented).toBe(true)
    await tick()
    expect(Number.parseFloat(spanBar.style.left)).toBe(start + 16)
    expect(Number(spanBar.getAttribute('aria-valuenow'))).toBeGreaterThan(
      startPercent
    )
    expect(spanBar).toHaveFocus()

    const afterRight = readPersistedColumnWidths()
    expect(afterRight.span).toBeGreaterThan(start)

    const left = press(spanBar, 'ArrowLeft', true)
    expect(left.defaultPrevented).toBe(true)
    await tick()
    expect(Number.parseFloat(spanBar.style.left)).toBe(start - 48)
  })

  it.each([
    { span: 200, service: 200, timeline: 400 },
    { span: 200, service: 300, [SERVICE_COLUMN_ID]: 200, timeline: 400 },
  ])(
    'preserves upgraded service widths without overriding newer widths: %j',
    async stored => {
      containerWidth = 800
      localStorage.setItem('waterfall-column-widths', JSON.stringify(stored))
      renderTree()
      await tick()

      expect(Number.parseFloat(separators()[0]!.style.left)).toBe(200)
      expect(Number.parseFloat(separators()[1]!.style.left)).toBe(400)
      press(separators()[0]!, 'ArrowRight')
      await tick()
      expect(readPersistedColumnWidths()).toEqual({
        span: 216,
        [SERVICE_COLUMN_ID]: 184,
        timeline: 400,
      })
    }
  )

  it('uses Home to restore and persist the current-container default', async () => {
    containerWidth = 600
    renderTree()
    await tick()

    const spanBar = separators()[0]!
    const defaultPosition = spanBar.getAttribute('aria-valuenow')
    press(spanBar, 'ArrowRight')
    await tick()
    expect(spanBar).not.toHaveAttribute('aria-valuenow', defaultPosition)

    const home = press(spanBar, 'Home')
    expect(home.defaultPrevented).toBe(true)
    await tick()
    expect(spanBar).toHaveAttribute('aria-valuenow', defaultPosition)

    const stored = readPersistedColumnWidths()
    expect(
      stored.span + stored[SERVICE_COLUMN_ID] + stored.timeline
    ).toBeCloseTo(600, 6)
    expect(stored.span).toBeGreaterThanOrEqual(140)
    expect(stored[SERVICE_COLUMN_ID]).toBeGreaterThanOrEqual(100)
    expect(stored.timeline).toBeGreaterThanOrEqual(240)
  })

  it('salvages finite widths while ignoring malformed entries', async () => {
    const stored = {
      span: 400,
      [SERVICE_COLUMN_ID]: 'wide',
      timeline: null,
      removedColumn: 123,
    }
    localStorage.setItem('waterfall-column-widths', JSON.stringify(stored))

    renderTree()
    await tick()

    expect(Number.parseFloat(separators()[0]!.style.left)).toBeGreaterThan(300)
    expect(
      JSON.parse(localStorage.getItem('waterfall-column-widths')!)
    ).toEqual(stored)
  })

  it('falls back to default widths when persisted JSON is malformed', async () => {
    localStorage.setItem('waterfall-column-widths', '{not json')

    expect(() => renderTree()).not.toThrow()
    await tick()

    expect(Number.parseFloat(separators()[0]!.style.left)).toBe(140)
    expect(Number.parseFloat(separators()[1]!.style.left)).toBe(240)
  })

  it('rejects extreme finite widths before layout arithmetic overflows', async () => {
    localStorage.setItem(
      'waterfall-column-widths',
      JSON.stringify({
        span: Number.MAX_VALUE,
        [SERVICE_COLUMN_ID]: Number.MAX_VALUE,
        timeline: Number.MAX_VALUE,
      })
    )

    renderTree()
    await tick()

    for (const separator of separators()) {
      expect(Number.isFinite(Number.parseFloat(separator.style.left))).toBe(
        true
      )
      expect(separator.getAttribute('aria-valuenow')).not.toContain('NaN')
      expect(separator.getAttribute('aria-valuenow')).not.toContain('Infinity')
    }
  })

  it('keeps resizing usable when width persistence is blocked', async () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    renderTree()
    await tick()

    const spanBar = separators()[0]!
    const start = Number.parseFloat(spanBar.style.left)
    expect(() => press(spanBar, 'ArrowRight')).not.toThrow()
    await tick()
    expect(Number.parseFloat(spanBar.style.left)).toBe(start + 16)
  })
})

describe('WaterfallView virtual row identity', () => {
  it('keeps focus on the same span when rows reorder', async () => {
    const spans = navigationSpans()
    const view = renderTree({ spans, selectedSpanID: 'error-1' })
    await tick()

    const focused = document.querySelector<HTMLElement>(
      'tr[data-span-id="error-1"]'
    )!
    focused.focus()

    await view.rerender({
      componentProps: {
        spans: [...spans].reverse(),
        selectedSpanID: 'error-1',
        onSelectSpan: vi.fn(),
      },
    })
    await tick()

    expect(document.activeElement).toHaveAttribute('data-span-id', 'error-1')
  })
})
