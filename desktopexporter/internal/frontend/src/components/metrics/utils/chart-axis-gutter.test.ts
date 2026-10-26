// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { chartAxisGutter, measureChartAxisGutter } from './chart-axis-gutter'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe('chartAxisGutter', () => {
  it.each([
    [44, 0, 56],
    [44.25, 0, 57],
    [240, 80, 252],
    [40, 100.5, 105],
  ])(
    'reserves measured Y/X widths %s/%s without clipping',
    (y, x, expected) => {
      expect(chartAxisGutter(y, x)).toBe(expected)
    }
  )
})

describe('measureChartAxisGutter', () => {
  it('remeasures changed labels and fonts, coalesces frames and cleans up observers', async () => {
    const frames = new Map<number, FrameRequestCallback>()
    let nextFrame = 0
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      const id = ++nextFrame
      frames.set(id, callback)
      return id
    })
    vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
    let resized = () => {}
    const observe = vi.fn()
    const unobserve = vi.fn()
    const disconnect = vi.fn()
    class Observer implements ResizeObserver {
      constructor(callback: ResizeObserverCallback) {
        resized = () => callback([], this)
      }
      observe = observe
      unobserve = unobserve
      disconnect = disconnect
    }
    vi.stubGlobal('ResizeObserver', Observer)
    const element = document.createElement('div')
    element.innerHTML =
      '<svg><g class="lc-axis placement-left"><text class="lc-axis-tick-label">10,000</text></g><g class="lc-axis placement-bottom"><text class="lc-axis-tick-label">12:00:00</text></g></svg>'
    document.body.append(element)
    let yWidth = 0
    let xWidth = 70
    const yLabel = element.querySelector('.placement-left text')!
    const xLabel = element.querySelector('.placement-bottom text')!
    vi.spyOn(yLabel, 'getBoundingClientRect').mockImplementation(
      () => new DOMRect(0, 0, yWidth, 20)
    )
    vi.spyOn(xLabel, 'getBoundingClientRect').mockImplementation(
      () => new DOMRect(0, 0, xWidth, 20)
    )
    const report = vi.fn()
    const action = measureChartAxisGutter(element, report)
    const flush = () => {
      const pending = [...frames.values()]
      frames.clear()
      pending.forEach(callback => callback(0))
    }
    flush()
    expect(report).not.toHaveBeenCalled()
    yWidth = 140
    resized()
    flush()
    expect(report).toHaveBeenLastCalledWith(152)
    yWidth = 220
    yLabel.textContent = 'longer received range'
    await Promise.resolve()
    flush()
    expect(report).toHaveBeenLastCalledWith(232)
    yWidth = 40
    xWidth = 100
    resized()
    flush()
    expect(report).toHaveBeenLastCalledWith(104)
    expect(frames.size).toBe(0)
    expect(observe).toHaveBeenCalledWith(element)
    action?.destroy?.()
    expect(disconnect).toHaveBeenCalledOnce()
    yLabel.textContent = 'unmounted'
    await Promise.resolve()
    expect(frames.size).toBe(0)
  })
})
