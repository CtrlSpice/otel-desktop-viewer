import { describe, expect, it } from 'vitest'
import { columnCell, columnID, columnOptions } from './waterfall-columns'
import type { WaterfallSpanData } from '@/types/api-types'

describe('waterfall projection columns', () => {
  it('retains the entire catalogue independently of selected previews', () => {
    const columns = columnOptions(
      [],
      [{ source: 'span', key: 'selected' }],
      [
        { source: 'span', key: 'unselected', count: 256 },
        { source: 'resource', key: 'unselected', count: 19 },
        { source: 'scope', key: 'newly-added', count: 1 },
      ]
    )
    expect(
      columns.find(
        c => c.id === columnID({ source: 'span', key: 'unselected' })
      )?.count
    ).toBe(256)
    expect(
      columns.find(
        c => c.id === columnID({ source: 'resource', key: 'unselected' })
      )?.count
    ).toBe(19)
    expect(
      columns.some(c => c.source === 'scope' && c.key === 'newly-added')
    ).toBe(true)
    expect(
      columns.some(
        c => c.field && c.source === 'span' && c.key === 'traceState'
      )
    ).toBe(true)
  })

  it('distinguishes pending, missing, received null and numeric previews', () => {
    const span: WaterfallSpanData = {
      traceID: 'trace',
      spanID: 'span',
      parentSpanID: null,
      traceState: '',
      name: 'name',
      flags: 0,
      kindCode: 1,
      kind: 'Internal',
      startTime: 0n,
      endTime: 1n,
      statusCodeValue: 0,
      statusCode: 'Unset',
      statusMessage: '',
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      events: [],
      resource: { serviceName: '', droppedAttributesCount: 0 },
      scope: { name: '', version: '', droppedAttributesCount: 0 },
      columnValues: [],
    }
    const column = { source: 'span' as const, key: 'value' }
    expect(columnCell(span, column).title).toBe('Loading column preview')
    span.columnValues.push({ ...column, spanID: 'one', values: [] })
    expect(columnCell(span, column).missing).toBe(true)
    span.columnValues[0].values = [{ valueType: 'empty', preview: 'null' }]
    expect(columnCell(span, column)).toMatchObject({
      text: 'null',
      missing: false,
      numeric: false,
    })
    span.columnValues[0].values = [
      { valueType: 'int64', preview: '9007199254740993' },
      { valueType: 'double', preview: '-0' },
    ]
    expect(columnCell(span, column)).toMatchObject({
      text: '9007199254740993 | -0',
      missing: false,
      numeric: true,
    })
  })
})
