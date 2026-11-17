// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/svelte'
import MetricAxes from './MetricAxes.svelte'
import {
  axisTime,
  axisValue,
  axisCount,
  axisBuckets,
  axisBucketBounds,
} from './MetricChartPlot.svelte'

describe('MetricAxes', () => {
  it('centres the 1px border on the existing plot bounds without an accessible control', async () => {
    const { container, rerender } = render(MetricAxes, {
      padding: { top: 24, right: 72, bottom: 72, left: 72 },
    })
    const axes = container.querySelector<HTMLElement>('.metric-axes')!
    expect(axes).toHaveAttribute('aria-hidden', 'true')
    expect(axes.style.top).toBe('23.5px')
    expect(axes.style.left).toBe('71.5px')
    expect(axes.style.right).toBe('71.5px')
    expect(axes.style.bottom).toBe('71.5px')
    await rerender({ padding: { top: 24, right: 72, bottom: 96, left: 72 } })
    expect(axes.style.bottom).toBe('95.5px')
    expect(axes).not.toHaveAttribute('tabindex')
  })

  it('disables the separate native rules while keeping axis labels and formatting', () => {
    const axes = [
      axisTime('UTC'),
      axisValue('bytes'),
      axisCount(),
      axisBuckets('ms'),
      axisBucketBounds('ms'),
    ]
    expect(axes.every(axis => axis.rule === false)).toBe(true)
    expect(axisValue('bytes').label).toBe('bytes')
    expect(axisCount().label).toBe('count')
    expect(axisBuckets('ms').label).toBe('value (ms)')
    expect(axisTime('UTC').format(new Date(0))).toBeTruthy()
  })

  it('anchors Y-axis values beside the rule and units above it', () => {
    for (const axis of [
      axisValue('tokens'),
      axisCount(),
      axisBucketBounds('ms'),
    ]) {
      expect(axis.tickLabelProps).toEqual({ dx: -8, textAnchor: 'end' })
      expect(axis.labelProps).toEqual({
        x: 0,
        y: 0,
        dy: -8,
        textAnchor: 'start',
        verticalAnchor: 'end',
      })
    }
  })
})
