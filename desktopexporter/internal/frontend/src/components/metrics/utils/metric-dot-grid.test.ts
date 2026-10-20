// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { scaleBand, scaleLinear, scaleTime } from 'd3-scale'
import { metricAxisTickPositions, metricDotPositions, metricDotGuides } from './metric-dot-grid'

describe('metricDotPositions', () => {
  it('puts one intermediate position between every pair of labels', () => {
    expect(metricDotPositions([0, 40, 80], 80)).toEqual([
      0, 20, 40, 60, 80,
    ])
  })

  it('uses each interval independently when label gaps differ', () => {
    expect(metricDotPositions([0, 40, 120], 120)).toEqual([
      0, 20, 40, 80, 120,
    ])
  })

  it('extends the nearest half spacing into partial edge intervals', () => {
    expect(metricDotPositions([22, 62, 102], 124)).toEqual([
      2, 22, 42, 62, 82, 102, 122,
    ])
  })

  it('sorts and deduplicates labels, including reversed value axes', () => {
    expect(metricDotPositions([80, 40, 40, 0], 80)).toEqual(
      metricDotPositions([0, 40, 80], 80)
    )
  })

  it('does not invent an interval when there are fewer than two labels', () => {
    expect(metricDotPositions([], 80)).toEqual([])
    expect(metricDotPositions([40], 80)).toEqual([40])
    expect(metricDotPositions([100], 80)).toEqual([])
  })
})

describe('metricAxisTickPositions', () => {
  it('matches automatic continuous ticks and explicit numeric tick counts', () => {
    const scale = scaleLinear().domain([0, 100]).range([0, 800])
    expect(metricAxisTickPositions(scale, 800, {}, 80)).toEqual(
      scale.ticks(10).map(scale)
    )
    expect(metricAxisTickPositions(scale, 800, { ticks: 5 }, 80)).toEqual(
      scale.ticks(5).map(scale)
    )
  })

  it('centres filtered band ticks instead of using band edges', () => {
    const scale = scaleBand<string>().domain(['a', 'b', 'c', 'd']).range([0, 80])
    expect(metricAxisTickPositions(scale, 80, { ticks: ['a', 'd'] }, 80)).toEqual([10, 70])
  })

  it('keeps all default band labels and respects explicit spacing', () => {
    const scale = scaleBand<string>().domain(['a', 'b', 'c', 'd']).range([0, 80])
    expect(metricAxisTickPositions(scale, 80, {}, 80)).toEqual([10, 30, 50, 70])
    expect(metricAxisTickPositions(scale, 80, { tickSpacing: 40 }, 80)).toEqual([10, 50])
  })

  it('uses supplied time ticks without replacing their uneven gaps', () => {
    const scale = scaleTime().domain([new Date(0), new Date(120)]).range([0, 120])
    expect(metricAxisTickPositions(scale, 120, {
      ticks: [new Date(0), new Date(40), new Date(120)],
    }, 80)).toEqual([0, 40, 120])
  })
})

describe('metricDotGuides', () => {
  it('keeps positions inside the plot, with only the rightmost at 50%', () => {
    const guides = metricDotGuides([0, 40, 80], 80, 'end')
    expect(guides.map(guide => guide.position)).toEqual([0, 20, 40, 60, 80])
    expect(guides.map(guide => guide.opacity)).toEqual([1, 1, 1, 1, 0.5])
  })

  it('adds boundary anchors to uneven edge gaps without moving the existing dots', () => {
    const guides = metricDotGuides([18, 58, 138], 160, 'end')
    expect(guides.map(guide => guide.position)).toEqual([0, 18, 38, 58, 98, 138, 160])
    expect(guides.at(-1)!.opacity).toBe(0.5)
  })

  it('retains a lone label between boundary anchors and leaves an empty axis empty', () => {
    expect(metricDotGuides([], 80, 'start')).toEqual([])
    expect(metricDotGuides([40], 80, 'start')).toEqual([
      { position: 0, opacity: 0.5 },
      { position: 40, opacity: 1 },
      { position: 80, opacity: 1 },
    ])
    expect(metricDotGuides([], 80, 'end')).toEqual([])
    expect(metricDotGuides([40], 80, 'end')).toEqual([
      { position: 0, opacity: 1 },
      { position: 40, opacity: 1 },
      { position: 80, opacity: 0.5 },
    ])
  })

  it('sets only the top row inside the plot to 50%, not the bottom or other rows', () => {
    const guides = metricDotGuides([80, 40, 0], 80, 'start')
    expect(guides.map(guide => guide.position)).toEqual([0, 20, 40, 60, 80])
    expect(guides.map(guide => guide.opacity)).toEqual([0.5, 1, 1, 1, 1])
  })
})
