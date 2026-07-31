// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest'
import { itemHref, navigateToItem, parseRoute } from './router'
import { EVENT_PARAM, SPAN_PARAM } from './query-params'
import { setTestUrl } from '@/test/render-helpers'

describe('navigateToItem with itemQuery', () => {
  beforeEach(() => {
    setTestUrl('/logs/log-1?start=100&end=200&span=stale&event=9&agg=rate')
  })

  it('strips stale scoped params and applies a span patch', () => {
    navigateToItem('traces', 'trace-abc', 'push', { [SPAN_PARAM]: 'span-xyz' })
    expect(parseRoute(window.location.href)).toEqual({
      path: '/traces/trace-abc',
      query: { start: '100', end: '200', [SPAN_PARAM]: 'span-xyz' },
    })
  })

  it('applies span and event together', () => {
    navigateToItem('traces', 'trace-abc', 'push', {
      [SPAN_PARAM]: 'span-xyz',
      [EVENT_PARAM]: '2',
    })
    expect(parseRoute(window.location.href)).toEqual({
      path: '/traces/trace-abc',
      query: {
        start: '100',
        end: '200',
        [SPAN_PARAM]: 'span-xyz',
        [EVENT_PARAM]: '2',
      },
    })
  })

  it('leaves the time window when navigating without a patch', () => {
    navigateToItem('traces', 'trace-abc', 'push')
    expect(parseRoute(window.location.href)).toEqual({
      path: '/traces/trace-abc',
      query: { start: '100', end: '200' },
    })
  })
})

describe('itemHref', () => {
  beforeEach(() => {
    setTestUrl('/metrics/m-1?start=50&end=60')
  })

  it('builds a trace path with optional scoped params', () => {
    expect(itemHref('traces', 'trace-1', { [SPAN_PARAM]: 'span-a' })).toBe(
      '/traces/trace-1?start=50&end=60&span=span-a'
    )
  })
})
