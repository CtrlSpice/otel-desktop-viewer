// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { buildWaterfallRows, getTraceBounds } from './waterfall-layout'
import { spanNode } from '@/test/waterfall-fixtures'
import {
  spanNameColor,
  traceServiceColor,
} from '@/utils/colours/trace-service-color'
import { savedColorSlots } from '@/utils/colours/color-slots'
import { THEMES } from '@/themes'

describe('Waterfall direct-child counts', () => {
  it('counts children separately from deeper descendants in the rendered tree', () => {
    const spans = [
      spanNode('root', null, 0),
      spanNode('a', 'root', 1),
      spanNode('a1', 'a', 2),
      spanNode('a2', 'a', 2),
      spanNode('b', 'root', 1),
      spanNode('orphan', 'missing', 0),
      spanNode('recovered', 'wrong-parent', 1),
    ]
    const rows = buildWaterfallRows(
      spans,
      getTraceBounds(spans),
      'madoromi-aki-yoru'
    )
    expect(rows.map(row => row.tree.childrenCount)).toEqual([
      2, 2, 0, 0, 0, 1, 0,
    ])
    expect(rows.map(row => row.tree.brokenParent)).toEqual([
      false,
      false,
      false,
      false,
      false,
      true,
      true,
    ])
  })
})

describe('WaterfallView bigint domain geometry', () => {
  it('uses domain bigint span and event times without reparsing', () => {
    const spans = [spanNode('first', null, 0), spanNode('second', null, 0)]
    spans[0]!.spanData.startTime = 100n
    spans[0]!.spanData.endTime = 200n
    spans[0]!.spanData.events = [
      {
        name: 'midpoint',
        timestamp: 150n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    spans[1]!.spanData.startTime = 200n
    spans[1]!.spanData.endTime = 300n

    const bounds = getTraceBounds(spans)
    const rows = buildWaterfallRows(spans, bounds, 'dark')

    expect(bounds).toEqual({ start: 100n, end: 300n, duration: 200n })
    expect(rows[0]).toMatchObject({
      offsetPercent: 0,
      widthPercent: 50,
      records: [{ id: 'event:first:0', eventIndex: 0 }],
    })
    expect(rows[1]).toMatchObject({ offsetPercent: 50, widthPercent: 50 })
  })

  it.each([undefined, 'one-service'])(
    'colours a single %s service by exact span name while preserving errors',
    service => {
      const spans = [
        spanNode('first', null, 0),
        spanNode('second', null, 0),
        spanNode('repeated', null, 0),
        spanNode('error', null, 0),
        spanNode('exception', null, 0),
      ]
      spans[2]!.spanData.name = 'first'
      spans[3]!.spanData.statusCodeValue = 2
      spans[4]!.spanData.events = [
        {
          name: 'exception',
          timestamp: 0n,
          attributes: [],
          droppedAttributesCount: 0,
        },
      ]
      if (service !== undefined) {
        for (const span of spans)
          span.spanData.resource.attributes = [
            { key: 'service.name', value: { kind: 'string', value: service } },
          ]
      }
      const servicesBefore = savedColorSlots('services')

      const rows = buildWaterfallRows(
        spans,
        getTraceBounds(spans),
        'madoromi-fuyu-hiru'
      )

      expect(rows.map(row => row.color)).toEqual([
        spanNameColor('first', 'madoromi-fuyu-hiru'),
        spanNameColor('second', 'madoromi-fuyu-hiru'),
        spanNameColor('first', 'madoromi-fuyu-hiru'),
        'var(--color-error)',
        'var(--color-error)',
      ])
      expect(rows[0].color).not.toBe(rows[1].color)
      expect(savedColorSlots('services')).toEqual(servicesBefore)
      const reordered = buildWaterfallRows(
        [...spans].reverse(),
        getTraceBounds(spans),
        'madoromi-fuyu-hiru'
      )
      expect(reordered.map(row => row.color)).toEqual(
        rows.map(row => row.color).reverse()
      )
    }
  )

  it('uses service colours for multi-service traces, including unmatched services', () => {
    const spans = [
      spanNode('first', null, 0),
      spanNode('second', null, 0),
      spanNode('error', null, 0),
    ]
    for (const [index, span] of spans.entries())
      span.spanData.resource.attributes = [
        {
          key: 'service.name',
          value: {
            kind: 'string',
            value: index === 2 ? 'other-service' : 'one-service',
          },
        },
      ]
    spans[2]!.matched = false
    spans[2]!.spanData.statusCodeValue = 2
    const rows = buildWaterfallRows(
      spans,
      getTraceBounds(spans),
      'madoromi-fuyu-hiru'
    )
    expect(rows[0].color).toBe(
      traceServiceColor('one-service', 'madoromi-fuyu-hiru')
    )
    expect(rows[1].color).toBe(rows[0].color)
    expect(rows[2].color).toBe('var(--color-error)')
    spans[2]!.spanData.statusCodeValue = 1
    expect(
      buildWaterfallRows(spans, getTraceBounds(spans), 'madoromi-fuyu-hiru')[2]
        .color
    ).toBe(traceServiceColor('other-service', 'madoromi-fuyu-hiru'))
  })

  it('keeps exact span names distinct and their colours stable across all themes', () => {
    const spans = [spanNode('first', null, 0), spanNode('second', null, 0)]
    spans[1].spanData.name = 'first '
    const colours = new Set<string>()
    for (const theme of THEMES) {
      const rows = buildWaterfallRows(spans, getTraceBounds(spans), theme.id)
      expect(rows[0].color).not.toBe(rows[1].color)
      colours.add(rows[0].color)
    }
    expect(colours.size).toBe(1)
  })
})
