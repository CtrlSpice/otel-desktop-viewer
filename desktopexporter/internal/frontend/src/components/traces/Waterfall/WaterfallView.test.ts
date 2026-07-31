// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { tick } from 'svelte'
import { waitFor } from '@testing-library/svelte'
import WaterfallView from './WaterfallView.svelte'
import type { SpanNode } from '@/types/api-types'
import { renderWithContexts } from '@/test/render-helpers'
import { scrollMock } from '@/test/mock-virtual-list'

vi.mock('@humanspeak/svelte-virtual-list', async () => {
  const { default: MockVirtualList } =
    await import('@/test/MockVirtualList.svelte')
  return { default: MockVirtualList }
})

function spanNode(
  id: string,
  parentId: string | null,
  depth: number
): SpanNode {
  return {
    depth,
    matched: true,
    spanData: {
      spanID: id,
      parentSpanID: parentId,
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

/** a → b → c → d → e → f; auto-collapse hides f under collapsed e. */
function deepCollapsedTree(): SpanNode[] {
  return [
    spanNode('a', null, 0),
    spanNode('b', 'a', 1),
    spanNode('c', 'b', 2),
    spanNode('d', 'c', 3),
    spanNode('e', 'd', 4),
    spanNode('f', 'e', 5),
  ]
}

describe('WaterfallView span selection reveal', () => {
  beforeEach(() => {
    scrollMock.mockClear()
  })

  it('expands collapsed ancestors and center-scrolls the selected span', async () => {
    renderWithContexts(WaterfallView, {
      spans: deepCollapsedTree(),
      selectedSpanID: 'f',
      onSelectSpan: vi.fn(),
    })

    await waitFor(() => {
      expect(document.querySelector('tr[data-span-id="f"]')).toBeInTheDocument()
    })

    expect(scrollMock).toHaveBeenCalledWith(
      expect.objectContaining({
        index: expect.any(Number),
        align: 'center',
        smoothScroll: true,
        shouldThrowOnBounds: false,
      })
    )
  })

  it('keeps a deep span hidden until it is selected', async () => {
    renderWithContexts(WaterfallView, {
      spans: deepCollapsedTree(),
      selectedSpanID: 'a',
      onSelectSpan: vi.fn(),
    })
    await tick()

    expect(document.querySelector('tr[data-span-id="f"]')).toBeNull()
  })
})
