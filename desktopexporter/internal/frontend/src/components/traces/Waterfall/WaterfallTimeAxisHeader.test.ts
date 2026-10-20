// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/svelte'
import WaterfallTimeAxisHeader, {
  waterfallTimeAxis,
  waterfallGrid,
} from './WaterfallTimeAxisHeader.svelte'

describe('waterfall name heading alignment', () => {
  it.each([0, 4, 123])('reserves the root count slot for %s children', rootChildCount => {
    const target = document.createElement('thead')
    const { container } = render(WaterfallTimeAxisHeader, {
      target,
      props: {
        traceDurationNs: 1n,
        spanColWidth: 300,
        rootChildCount,
        barGrid: waterfallGrid(waterfallTimeAxis(1n, 1), 400, 112),
      },
    })
    const heading = container.querySelector('[aria-label="Span name"]')!
    const spacer = heading.querySelector('[aria-hidden="true"]')
    if (rootChildCount > 0) {
      expect(heading).toHaveClass('waterfall-time-axis-header__th-span--parent')
      expect(spacer).toHaveTextContent(String(rootChildCount))
      expect(spacer).toHaveClass('invisible', 'tabular-nums', 'font-normal')
      expect(spacer).toHaveClass('waterfall-time-axis-header__count-spacer')
    } else {
      expect(spacer).toBeNull()
      expect(heading).not.toHaveClass('waterfall-time-axis-header__th-span--parent')
    }
  })
})

describe('waterfall time-grid subdivisions', () => {
  it.each([
    [100_000_000n, '100ms'],
    [200_000_000n, '200ms'],
    [500_000_000n, '500ms'],
    [60_000_000_000n, '1min'],
    [120_000_000_000n, '2min'],
  ])(
    'adds three guides per %s ns label interval without adding labels',
    (interval, label) => {
      const axis = waterfallTimeAxis(interval * 2n, 2)
      expect(axis.intervalNs).toBe(interval)
      expect(axis.ticks).toHaveLength(3)
      expect(axis.ticks[1]).toEqual({ offsetPercent: 50, label })
      expect(axis.guides).toHaveLength(9)
      expect(axis.guides[1]).toEqual({
        offsetPercent: 12.5,
        major: false,
      })
      expect(axis.guides.filter(guide => guide.major)).toEqual([
        { offsetPercent: 0, major: true },
        { offsetPercent: 50, major: true },
        { offsetPercent: 100, major: true },
      ])
    }
  )

  it('continues the minor grid into the final partial interval without extending time', () => {
    const axis = waterfallTimeAxis(250_000_000n, 2)
    expect(axis.ticks.map(tick => tick.label)).toEqual(['0ms', '100ms', '200ms'])
    expect(axis.guides.map(guide => guide.offsetPercent)).toEqual([
      0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100,
    ])
    expect(axis.guides.at(-1)?.major).toBe(false)
    expect(axis.layoutDurationNs).toBe(250_000_000n)
  })

  it('keeps nanosecond guides at whole nanoseconds', () => {
    expect(waterfallTimeAxis(1n, 1).guides).toEqual([
      { offsetPercent: 0, major: true },
      { offsetPercent: 100, major: true },
    ])
    expect(waterfallTimeAxis(2n, 1).guides).toEqual([
      { offsetPercent: 0, major: true },
      { offsetPercent: 50, major: false },
      { offsetPercent: 100, major: true },
    ])
    expect(waterfallTimeAxis(20n, 1).guides).toEqual([
      { offsetPercent: 0, major: true },
      { offsetPercent: 25, major: false },
      { offsetPercent: 50, major: false },
      { offsetPercent: 75, major: false },
      { offsetPercent: 100, major: true },
    ])
  })

  it.each([0n, -1n])('keeps a single origin for duration %s', duration => {
    const axis = waterfallTimeAxis(duration, 6)
    expect(axis.guides).toEqual([{ offsetPercent: 0, major: true }])
    expect(axis.ticks).toEqual([{ offsetPercent: 0, label: '0ns' }])
  })
})

describe('waterfall horizontal grid continuation', () => {
  it('keeps the time axis intact and fades the last five guide columns', () => {
    const axis = waterfallTimeAxis(400_000_000n, 2)
    const ticks = structuredClone(axis.ticks)
    const grid = waterfallGrid(axis, 400, 200)
    expect(grid.widthPx).toBe(600)
    expect(grid.guides.slice(0, -5)).toEqual(
      axis.guides.slice(0, -1).map(guide => ({
        offsetPx: guide.offsetPercent * 4,
        major: guide.major,
        opacity: 1,
      }))
    )
    expect(grid.guides.slice(-5)).toEqual([
      { offsetPx: 400, major: true, opacity: 4 / 5 },
      { offsetPx: 450, major: false, opacity: 3 / 5 },
      { offsetPx: 500, major: false, opacity: 2 / 5 },
      { offsetPx: 550, major: false, opacity: 1 / 5 },
      { offsetPx: 600, major: true, opacity: 0 },
    ])
    expect(axis.ticks).toEqual(ticks)
    expect(axis.layoutDurationNs).toBe(400_000_000n)
  })

  it('continues across partial final intervals without extending past available space', () => {
    const grid = waterfallGrid(waterfallTimeAxis(250_000_000n, 2), 400, 112)
    expect(grid.guides.slice(-3)).toEqual([
      { offsetPx: 400, major: false, opacity: 2 / 5 },
      { offsetPx: 440, major: false, opacity: 1 / 5 },
      { offsetPx: 480, major: true, opacity: 0 },
    ])
    expect(grid.guides.every(guide => guide.offsetPx <= grid.widthPx)).toBe(true)
  })

  it('retains whole-nanosecond subdivision spacing', () => {
    const axis = waterfallTimeAxis(2n, 1)
    const grid = waterfallGrid(axis, 200, 250)
    expect(grid.guides.map(guide => guide.offsetPx)).toEqual([0, 100, 200, 300, 400])
    expect(axis.ticks.map(tick => tick.label)).toEqual(['0ns', '2ns'])
  })

  it('does not add guides without measured plot width or a positive trace duration', () => {
    expect(waterfallGrid(waterfallTimeAxis(100n, 2), 0, 112).guides).toEqual([])
    expect(waterfallGrid(waterfallTimeAxis(0n, 2), 400, 112).guides).toEqual([
      { offsetPx: 0, major: true, opacity: 1 },
    ])
  })

  it('keeps the original guides when the right gutter has no room', () => {
    const axis = waterfallTimeAxis(400_000_000n, 2)
    expect(waterfallGrid(axis, 400, 0).guides).toHaveLength(axis.guides.length)
  })
})
