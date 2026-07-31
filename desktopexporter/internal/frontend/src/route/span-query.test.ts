import { describe, expect, it, vi, afterEach } from 'vitest'
import {
  getEventFromQuery,
  getSpanFromQuery,
  selectSpanEvent,
  setEventInQuery,
  setSpanInQuery,
} from './span-query'
import { parseRoute } from './router'

function stubWindow(href: string) {
  vi.stubGlobal('window', {
    location: { href },
    addEventListener: vi.fn(),
  })
  vi.stubGlobal('history', { pushState: vi.fn(), replaceState: vi.fn() })
}

describe('getEventFromQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns null when event param is absent', () => {
    stubWindow('http://local/traces/t1?span=s1')
    expect(getEventFromQuery()).toBeNull()
  })

  it('parses a valid event index', () => {
    stubWindow('http://local/traces/t1?span=s1&event=2')
    expect(getEventFromQuery()).toBe(2)
  })

  it('returns null for invalid event index', () => {
    stubWindow('http://local/traces/t1?event=-1')
    expect(getEventFromQuery()).toBeNull()
  })
})

describe('setSpanInQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('clears event when span changes', () => {
    stubWindow('http://local/traces/t1?span=s1&event=2&start=0')
    setSpanInQuery('s2')
    const url = (history.replaceState as ReturnType<typeof vi.fn>).mock
      .calls[0]![2] as string
    expect(parseRoute(url)).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's2' },
    })
  })
})

describe('selectSpanEvent', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sets span and event together', () => {
    stubWindow('http://local/traces/t1?start=0')
    selectSpanEvent('s1', 3)
    const url = (history.pushState as ReturnType<typeof vi.fn>).mock
      .calls[0]![2] as string
    expect(parseRoute(url)).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's1', event: '3' },
    })
  })
})

describe('setEventInQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sets event without changing span', () => {
    stubWindow('http://local/traces/t1?span=s1&start=0')
    setEventInQuery(1)
    const url = (history.replaceState as ReturnType<typeof vi.fn>).mock
      .calls[0]![2] as string
    expect(parseRoute(url)).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's1', event: '1' },
    })
  })

  it('clears event when passed null', () => {
    stubWindow('http://local/traces/t1?span=s1&event=1&start=0')
    setEventInQuery(null)
    const url = (history.replaceState as ReturnType<typeof vi.fn>).mock
      .calls[0]![2] as string
    expect(parseRoute(url)).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's1' },
    })
  })
})

describe('getSpanFromQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('reads span from the live URL', () => {
    stubWindow('http://local/traces/t1?span=s1')
    expect(getSpanFromQuery()).toBe('s1')
  })
})
