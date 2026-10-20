// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/svelte'
import WaterfallTimeAxisHeader, {
  waterfallTimeAxis,
  waterfallGrid,
} from './WaterfallTimeAxisHeader.svelte'

describe('waterfall name heading alignment', () => {
  it('keeps the span heading free of decorative child counts', () => {
    const target = document.createElement('thead')
    const { container } = render(WaterfallTimeAxisHeader, {
      target,
      props: {
        traceDurationNs: 1n,
        spanColWidth: 300,
        barGrid: waterfallGrid(waterfallTimeAxis(1n, 1), 400),
      },
    })
    const heading = container.querySelector('[aria-label="Span name"]')!
    expect(heading).toHaveTextContent('name')
    expect(heading.querySelector('.tree-child-count')).toBeNull()
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
    expect(axis.ticks.map(tick => tick.label)).toEqual([
      '0ms',
      '100ms',
      '200ms',
    ])
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

describe('waterfall grid bounds', () => {
  it('keeps the time axis intact and guides within the timeline', () => {
    const axis = waterfallTimeAxis(400_000_000n, 2)
    const ticks = structuredClone(axis.ticks)
    const grid = waterfallGrid(axis, 400)
    expect(grid.widthPx).toBe(400)
    expect(grid.guides).toEqual(
      axis.guides.map(guide => ({
        offsetPx: guide.offsetPercent * 4,
        major: guide.major,
      }))
    )
    expect(axis.ticks).toEqual(ticks)
    expect(axis.layoutDurationNs).toBe(400_000_000n)
  })

  it('retains partial final intervals without extending past the timeline', () => {
    const grid = waterfallGrid(waterfallTimeAxis(250_000_000n, 2), 400)
    expect(grid.guides.slice(-3)).toEqual([
      { offsetPx: 320, major: true },
      { offsetPx: 360, major: false },
      { offsetPx: 400, major: false },
    ])
    expect(grid.guides.every(guide => guide.offsetPx <= grid.widthPx)).toBe(
      true
    )
  })

  it('retains whole-nanosecond subdivision spacing', () => {
    const axis = waterfallTimeAxis(2n, 1)
    const grid = waterfallGrid(axis, 200)
    expect(grid.guides.map(guide => guide.offsetPx)).toEqual([0, 100, 200])
    expect(axis.ticks.map(tick => tick.label)).toEqual(['0ns', '2ns'])
  })

  it('does not add guides without measured plot width or a positive trace duration', () => {
    expect(waterfallGrid(waterfallTimeAxis(100n, 2), 0).guides).toEqual([])
    expect(waterfallGrid(waterfallTimeAxis(0n, 2), 400).guides).toEqual([
      { offsetPx: 0, major: true },
    ])
  })
})
