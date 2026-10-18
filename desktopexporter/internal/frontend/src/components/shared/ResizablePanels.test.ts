// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { createRawSnippet } from 'svelte'
import { fireEvent, render } from '@testing-library/svelte'
import ResizablePanels from './ResizablePanels.svelte'

const leftPanel = createRawSnippet(() => ({ render: () => '<p>main</p>' }))
const rightPanel = createRawSnippet(() => ({ render: () => '<p>detail</p>' }))

describe('ResizablePanels collapse', () => {
  it.each([1000, 600])(
    'preserves mounted content and the split at width %i',
    async width => {
      const editableDetail = createRawSnippet(() => ({
        render: () => '<input aria-label="Detail draft" />',
      }))
      const { container, rerender } = render(ResizablePanels, {
        leftPanel,
        rightPanel: editableDetail,
        rightPanelID: 'detail',
        storageKey: 'split',
        defaultLeftWidth: 0.6,
      })
      await measure(width)
      const main = container.querySelector('.panel-shell')!
      const detail = container.querySelector('#detail')!
      const input = detail.querySelector('input')!
      input.value = 'unfinished draft'
      const fraction = leftFraction()

      await rerender({ rightCollapsed: true })
      expect((main as HTMLElement).style.flexGrow).toBe('1')
      expect(detail).toHaveProperty('inert', true)
      expect(container.querySelector('[role="separator"]')).toHaveProperty(
        'inert',
        true
      )
      expect(container.querySelector('.panel-shell')).toBe(main)
      expect(leftFraction()).toBe(fraction)

      await rerender({ rightCollapsed: false })
      expect(container.querySelector('#detail')).toBe(detail)
      expect(detail).toHaveProperty('inert', false)
      expect(input).toHaveValue('unfinished draft')
      expect(leftFraction()).toBe(fraction)
    }
  )
})

// jsdom has no layout, so the component's ResizeObserver never fires on its
// own. This stub records instances so a test can hand the component a
// container measurement, which is all the real observer ever did for it.
class ResizeObserverStub implements ResizeObserver {
  constructor(readonly cb: ResizeObserverCallback) {
    observers.push(this)
  }
  observe(_target: Element, _options?: ResizeObserverOptions) {}
  unobserve(_target: Element) {}
  disconnect() {}
}

const observers: ResizeObserverStub[] = []
beforeEach(() => {
  observers.length = 0
  localStorage.clear()
  vi.stubGlobal('ResizeObserver', ResizeObserverStub)
})

async function measure(width: number, height = 600) {
  await tick()
  const entry = {
    target: document.body,
    contentRect: new DOMRect(0, 0, width, height),
    borderBoxSize: [],
    contentBoxSize: [],
    devicePixelContentBoxSize: [],
  } satisfies ResizeObserverEntry
  for (const o of observers) {
    o.cb([entry], o)
  }
  await tick()
}

function leftFraction(): number {
  const divider = document.querySelector('[role="separator"]')
  if (!divider) throw new Error('no divider rendered')
  return Number(divider.getAttribute('aria-valuenow')) / 100
}

function rightWidthPx(): number {
  const detail = document.querySelector<HTMLElement>('.panel-detail')
  if (!detail) throw new Error('no detail panel rendered')
  return Number.parseFloat(detail.style.flexBasis)
}

/* The fractions are flex-grow shares of the flex space: the container
 * minus the divider and two gaps. In jsdom the divider measures 0 and
 * the gap falls back to 8px, so a 1000px container has 984px of flex
 * space. The rem conversion falls back to a 16px root the same way. */
const FLEX_SPACE = 1000 - 2 * 8

describe('ResizablePanels rem default', () => {
  it('lands the right pane on the rem width once the container measures', async () => {
    render(ResizablePanels, { leftPanel, rightPanel, defaultRightRem: 28 })
    await measure(1000)
    // 28rem = 448px of 984px flex space.
    expect(rightWidthPx()).toBeCloseTo(448, 5)
  })

  it('lets a stored split beat the rem default', async () => {
    localStorage.setItem('split', '0.6')
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      defaultRightRem: 28,
      storageKey: 'split',
    })
    await measure(1000)
    expect(rightWidthPx()).toBeCloseTo(0.4 * FLEX_SPACE, 5)
  })

  it('falls back to the fraction default without a measurement', async () => {
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      defaultLeftWidth: 0.58,
    })
    await tick()
    expect(leftFraction()).toBeCloseTo(0.58, 5)
  })
})

describe('ResizablePanels stored splits', () => {
  it('clamps an out-of-range stored split instead of reverting to default', async () => {
    localStorage.setItem('split', '0.95')
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      storageKey: 'split',
      minRightWidth: 0.2,
    })
    await measure(1000)
    expect(rightWidthPx()).toBeCloseTo(0.2 * FLEX_SPACE, 5)
  })

  it('ignores a stored value that is not a number', async () => {
    localStorage.setItem('split', 'extra wide')
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      storageKey: 'split',
      defaultLeftWidth: 0.7,
    })
    await measure(1000)
    expect(rightWidthPx()).toBeCloseTo(0.3 * FLEX_SPACE, 5)
  })
})

describe('ResizablePanels shared width', () => {
  it('keeps the same detail width when the available main space changes', async () => {
    render(ResizablePanels, { leftPanel, rightPanel, rightWidthRem: 30 })
    await measure(1200)
    expect(rightWidthPx()).toBeCloseTo(480, 5)
    await measure(1000)
    expect(rightWidthPx()).toBeCloseTo(480, 5)
  })

  it('uses the shared width instead of a stored fractional split', async () => {
    localStorage.setItem('split', '0.8')
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      rightWidthRem: 30,
      storageKey: 'split',
    })
    await measure(1000)
    expect(rightWidthPx()).toBeCloseTo(480, 5)
  })

  it('saves horizontal changes in rem and resets to the shared default', async () => {
    const onRightWidthChange = vi.fn()
    const { getByRole } = render(ResizablePanels, {
      leftPanel,
      rightPanel,
      rightWidthRem: 30,
      onRightWidthChange,
      defaultRightRem: 28,
      storageKey: 'split',
    })
    await measure(1000)
    const separator = getByRole('separator')
    await fireEvent.keyDown(separator, { key: 'ArrowLeft' })
    expect(onRightWidthChange).toHaveBeenLastCalledWith(
      (480 + FLEX_SPACE * 0.01) / 16
    )
    expect(localStorage.getItem('split')).toBeNull()
    await fireEvent.dblClick(separator)
    expect(onRightWidthChange.mock.lastCall![0]).toBeCloseTo(28, 5)
  })

  it('does not overwrite the shared width while resizing a stacked layout', async () => {
    const onRightWidthChange = vi.fn()
    const { getByRole } = render(ResizablePanels, {
      leftPanel,
      rightPanel,
      rightWidthRem: 30,
      onRightWidthChange,
      storageKey: 'split',
    })
    await measure(600)
    await fireEvent.keyDown(getByRole('separator'), { key: 'ArrowUp' })
    expect(onRightWidthChange).not.toHaveBeenCalled()
    expect(localStorage.getItem('split')).not.toBeNull()
    await measure(1200)
    expect(rightWidthPx()).toBeCloseTo(480, 5)
  })
})

describe('ResizablePanels pixel floors', () => {
  it('converts a pixel floor against the flex space, not the container', async () => {
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      defaultLeftWidth: 0.2,
      minLeftWidth: 0.1,
      minLeftPx: 500,
    })
    await measure(1000)
    // 500px of 984px flex space -- against the raw container this would
    // be 0.5 and the rendered pane would come out 8px short.
    expect(rightWidthPx()).toBeCloseTo(FLEX_SPACE - 500, 5)
  })

  it('lets the main-pane floor win over the rem default when they conflict', async () => {
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      defaultRightRem: 28,
      minLeftPx: 600,
    })
    await measure(1000)
    // The rem default would put the left pane at 536px of flex space;
    // the 600px floor overrides it.
    expect(rightWidthPx()).toBeCloseTo(FLEX_SPACE - 600, 5)
  })
})
