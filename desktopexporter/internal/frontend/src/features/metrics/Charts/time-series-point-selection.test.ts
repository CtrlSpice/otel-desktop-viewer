import { describe, expect, it } from 'vitest'
import { timeSeriesChartPointSelection } from './time-series-point-selection'

describe('timeSeriesChartPointSelection', () => {
  it('reads source identity and the series key from LayerChart runtime wrappers', () => {
    expect(
      timeSeriesChartPointSelection(
        {
          data: {
            date: new Date(1),
            value: 42,
            sourceDatapointID: 'source-a',
            seriesKey: 'series-a',
          },
          point: {
            data: { x: new Date(1), y: 42 },
            seriesKey: 'series-a',
          },
        },
        []
      )
    ).toEqual({ seriesKey: 'series-a', sourceDatapointID: 'source-a' })
  })

  it('does not infer identity from LayerChart projected highlight data', () => {
    expect(
      timeSeriesChartPointSelection(
        {
          data: { x: new Date(1), y: 42 },
          series: { key: 'series-a' },
        },
        []
      )
    ).toBeNull()
  })

  it('uses the clicked line projection when the raw datum belongs to another series', () => {
    const date = new Date(1)
    expect(
      timeSeriesChartPointSelection(
        {
          data: {
            date,
            value: 11,
            sourceDatapointID: 'source-a',
            seriesKey: 'series-a',
          },
          point: {
            data: { x: date, y: 22 },
            seriesKey: 'series-b',
          },
        },
        [
          {
            key: 'series-b',
            label: 'series B',
            points: [{ date, value: 22, sourceDatapointID: 'source-b' }],
          },
        ]
      )
    ).toEqual({ seriesKey: 'series-b', sourceDatapointID: 'source-b' })
  })

  it('rejects an ambiguous projected point', () => {
    const date = new Date(1)
    expect(
      timeSeriesChartPointSelection(
        {
          data: {
            date,
            value: 11,
            sourceDatapointID: 'source-a',
            seriesKey: 'series-a',
          },
          point: {
            data: { x: date, y: 22 },
            seriesKey: 'series-b',
          },
        },
        [
          {
            key: 'series-b',
            label: 'series B',
            points: [
              { date, value: 22, sourceDatapointID: 'source-b-1' },
              { date, value: 22, sourceDatapointID: 'source-b-2' },
            ],
          },
        ]
      )
    ).toBeNull()
  })
})
