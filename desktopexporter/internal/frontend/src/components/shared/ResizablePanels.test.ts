// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { createRawSnippet } from 'svelte'
import { render } from '@testing-library/svelte'
import ResizablePanels from './ResizablePanels.svelte'

const leftPanel = createRawSnippet(() => ({ render: () => '<p>main</p>' }))
const rightPanel = createRawSnippet(() => ({ render: () => '<p>detail</p>' }))

// jsdom has no layout, so the component's ResizeObserver never fires on its
// own. This stub records instances so a test can hand the component a
// container measurement, which is all the real observer ever did for it.
const observers: { cb: ResizeObserverCallback }[] = []
beforeEach(() => {
  observers.length = 0
  localStorage.clear()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      cb: ResizeObserverCallback
      constructor(cb: ResizeObserverCallback) {
        this.cb = cb
        observers.push(this)
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
})

async function measure(width: number, height = 600) {
  await tick()
  for (const o of observers) {
    o.cb(
      [{ contentRect: { width, height } } as ResizeObserverEntry],
      null as unknown as ResizeObserver
    )
  }
  await tick()
}

function leftFraction(): number {
  const shell = document.querySelector<HTMLElement>('.panel-shell')
  if (!shell) throw new Error('no panel shell rendered')
  return Number.parseFloat(shell.style.flex)
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
    expect(leftFraction()).toBeCloseTo(1 - 448 / FLEX_SPACE, 5)
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
    expect(leftFraction()).toBeCloseTo(0.6, 5)
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
    // Saved on a wide window, loaded on a narrow one: the old behaviour
    // rejected the value wholesale and silently reset the split.
    localStorage.setItem('split', '0.95')
    render(ResizablePanels, {
      leftPanel,
      rightPanel,
      storageKey: 'split',
      minRightWidth: 0.2,
    })
    await measure(1000)
    expect(leftFraction()).toBeCloseTo(0.8, 5)
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
    expect(leftFraction()).toBeCloseTo(0.7, 5)
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
    expect(leftFraction()).toBeCloseTo(500 / FLEX_SPACE, 5)
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
    expect(leftFraction()).toBeCloseTo(600 / FLEX_SPACE, 5)
  })
})
