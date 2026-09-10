import { describe, expect, it, vi, afterEach, type Mock } from 'vitest'
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
  const historyDouble = {
    pushState: vi.fn<History['pushState']>(),
    replaceState: vi.fn<History['replaceState']>(),
  } satisfies Pick<History, 'pushState' | 'replaceState'>
  vi.stubGlobal('history', historyDouble)
  return historyDouble
}

function navigationURL(method: Mock<History['pushState']>): string {
  return method.mock.calls[0]?.[2]?.toString() ?? ''
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
    const { replaceState } = stubWindow(
      'http://local/traces/t1?span=s1&event=2&start=0'
    )
    setSpanInQuery('s2')
    const url = navigationURL(replaceState)
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
    const { pushState } = stubWindow('http://local/traces/t1?start=0')
    selectSpanEvent('s1', 3)
    const url = navigationURL(pushState)
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
    const { replaceState } = stubWindow(
      'http://local/traces/t1?span=s1&start=0'
    )
    setEventInQuery(1)
    const url = navigationURL(replaceState)
    expect(parseRoute(url)).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's1', event: '1' },
    })
  })

  it('clears event when passed null', () => {
    const { replaceState } = stubWindow(
      'http://local/traces/t1?span=s1&event=1&start=0'
    )
    setEventInQuery(null)
    const url = navigationURL(replaceState)
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
