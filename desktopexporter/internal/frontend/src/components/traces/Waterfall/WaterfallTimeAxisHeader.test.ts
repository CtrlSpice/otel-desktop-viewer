// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/svelte'
import WaterfallTimeAxisHeader, {
  waterfallTimeAxis,
  niceTimeAxis,
} from './WaterfallTimeAxisHeader.svelte'

describe('waterfall name heading alignment', () => {
  it('keeps the span heading free of decorative child counts', () => {
    const target = document.createElement('thead')
    const { container } = render(WaterfallTimeAxisHeader, {
      target,
      props: { traceDurationNs: 1n, spanColWidth: 300 },
    })
    const heading = container.querySelector('[aria-label="Span name"]')!
    expect(heading).toHaveTextContent('name')
    expect(heading.querySelector('.tree-child-count')).toBeNull()
  })
})

describe('waterfall time labels', () => {
  it.each([
    [100_000_000n, '100ms'],
    [200_000_000n, '200ms'],
    [500_000_000n, '500ms'],
    [60_000_000_000n, '1min'],
    [120_000_000_000n, '2min'],
    [3_600_000_000_000n, '1h'],
    [7_200_000_000_000n, '2h'],
  ])('keeps %s ns label intervals', (interval, label) => {
    const axis = waterfallTimeAxis(interval * 2n, 2)
    expect(axis.intervalNs).toBe(interval)
    expect(axis.ticks).toHaveLength(3)
    expect(axis.ticks[1]).toEqual({ offsetPercent: 50, label })
    expect(niceTimeAxis(interval * 2n, 2).ticks).toEqual(axis.ticks)
  })

  it('retains a final partial interval without extending time', () => {
    const axis = waterfallTimeAxis(250_000_000n, 2)
    expect(axis.ticks.map(tick => tick.label)).toEqual([
      '0ms',
      '100ms',
      '200ms',
    ])
    expect(axis.layoutDurationNs).toBe(250_000_000n)
  })

  it.each([0n, -1n])('keeps a single origin for duration %s', duration => {
    expect(waterfallTimeAxis(duration, 6).ticks).toEqual([
      { offsetPercent: 0, label: '0ns' },
    ])
  })
})
