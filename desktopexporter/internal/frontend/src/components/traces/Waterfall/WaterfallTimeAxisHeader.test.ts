// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/svelte'
import WaterfallTimeAxisHeader, {
  waterfallTimeAxis,
} from './WaterfallTimeAxisHeader.svelte'

describe('waterfall name heading alignment', () => {
  it.each([0, 4, 123])('reserves the root count slot for %s children', rootChildCount => {
    const target = document.createElement('thead')
    const { container } = render(WaterfallTimeAxisHeader, {
      target,
      props: { traceDurationNs: 1n, spanColWidth: 300, rootChildCount },
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
