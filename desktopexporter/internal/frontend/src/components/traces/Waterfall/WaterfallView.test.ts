// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { tick } from 'svelte'
import { waitFor } from '@testing-library/svelte'
import WaterfallView from './WaterfallView.svelte'
import type { SpanNode } from '@/types/api-types'
import { renderWithContexts } from '@/test/render-helpers'
import { resetCollapseStoreForTests } from './waterfall-collapse-store'
import { scrollMock } from '@/test/mock-virtual-list'

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
      startTime: BigInt(depth) * 1_000_000n,
      endTime: BigInt(depth + 1) * 1_000_000n,
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
      kind: '',
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusMessage: '',
    },
  }
}

/** a → b → c → d → e → f: deep enough that the old heuristic collapsed it. */
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
    ...overrides,
  })
}

function rowIDs(): string[] {
  return [...document.querySelectorAll('tr[data-span-id]')].map(r =>
    r.getAttribute('data-span-id')!
  )
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
  spans[1]!.spanData.statusCode = 'Error'
  spans[2]!.spanData.events = [
    {
      name: 'exception',
      timestamp: 0n,
      attributes: [],
      droppedAttributesCount: 0,
    },
  ]
  spans[4]!.spanData.statusCode = 'Error'
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

    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-1')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-1', onSelectSpan },
    })
    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-2')

    await view.rerender({
      componentProps: { spans, selectedSpanID: 'error-2', onSelectSpan },
    })
    buttonByLabel('Previous error').click()
    expect(onSelectSpan).toHaveBeenLastCalledWith('error-1')
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
      spanData: { ...node.spanData, statusCode: 'Ok', events: [] },
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

    expect(document.body).toHaveTextContent('2 err')
    buttonByLabel('Next error').click()
    expect(onSelectSpan).toHaveBeenCalledWith('error-2')
    expect(onSelectSpan).not.toHaveBeenCalledWith('exception-only')
  })

  it("uses selection scrolling without reopening the reader's collapsed branch", async () => {
    const spans = deepTree()
    spans[5]!.spanData.statusCode = 'Error'
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

describe('WaterfallView collapse ownership', () => {
  beforeEach(() => {
    scrollMock.mockClear()
    resetCollapseStoreForTests()
  })

  // The old default was a heuristic: collapse any parent at depth 4 or with a
  // dozen descendants. Two reports guessed independently that the tree was
  // reacting to its own size, and both were right.
  it('opens fully expanded, however deep the trace', async () => {
    renderTree()
    await tick()
    expect(rowIDs()).toEqual(ALL_IDS)
  })

  // #348. The set used to be assigned by an effect that re-ran whenever the
  // spans array changed identity, so any re-render threw the reader's
  // arrangement away.
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

    // A fresh response arrives while the selection still points into the
    // closed branch -- the exact shape that used to undo the collapse.
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

  // The gap the old toggle had: its label followed the current state, so from
  // a mixed arrangement it only ever offered collapse-all, and there was no
  // way to open everything short of collapsing everything first.
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
        onSelectSpan: vi.fn(),
      },
    })
    await tick()
    const during = rowIDs()
    expect(during).toContain('c')
    expect(during).not.toContain('f')

    // Clearing the search puts back exactly what the reader had: 'e' still
    // collapsed, nothing else touched.
    await rerender({
      componentProps: {
        spans: deepTree(),
        selectedSpanID: null,
        onSelectSpan: vi.fn(),
      },
    })
    await tick()
    expect(rowIDs()).toEqual(['a', 'b', 'c', 'd', 'e'])
  })

  it('center-scrolls a newly selected visible span', async () => {
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
          align: 'center',
          smoothScroll: true,
          shouldThrowOnBounds: false,
        })
      )
    )
  })
})
