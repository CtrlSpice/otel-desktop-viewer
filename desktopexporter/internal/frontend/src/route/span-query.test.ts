import { describe, expect, it, vi, afterEach, type Mock } from 'vitest'
import {
  getEventFromQuery,
  getSpanFromQuery,
  parseEventIndex,
  selectSpanEvent,
  selectSpanLog,
  setEventInQuery,
  setLogInQuery,
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

  it.each(['1junk', '1.5', '-1', '9007199254740992', ''])(
    'returns null for invalid event index %j',
    value => {
      stubWindow(`http://local/traces/t1?event=${value}`)
      expect(getEventFromQuery()).toBeNull()
    }
  )
})

describe('parseEventIndex', () => {
  it('accepts only complete non-negative safe-integer decimal strings', () => {
    expect(parseEventIndex('0')).toBe(0)
    expect(parseEventIndex('12')).toBe(12)
    expect(parseEventIndex('01')).toBe(1)
    expect(parseEventIndex(undefined)).toBeNull()
    expect(parseEventIndex('')).toBeNull()
    expect(parseEventIndex('1junk')).toBeNull()
    expect(parseEventIndex('1.5')).toBeNull()
    expect(parseEventIndex('-1')).toBeNull()
    expect(parseEventIndex('9007199254740992')).toBeNull()
  })
})

describe('setSpanInQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('clears event and log when span changes', () => {
    const { replaceState } = stubWindow(
      'http://local/traces/t1?span=s1&event=2&log=l1&start=0'
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
    const { pushState } = stubWindow(
      'http://local/traces/t1?start=0&log=old-log'
    )
    selectSpanEvent('s1', 3)
    const url = navigationURL(pushState)
    expect(parseRoute(url)).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's1', event: '3' },
    })
  })
})

describe('selectSpanLog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sets span and log together and clears event', () => {
    const { pushState } = stubWindow('http://local/traces/t1?start=0&event=2')
    selectSpanLog('s1', 'l1')
    expect(parseRoute(navigationURL(pushState))).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's1', log: 'l1' },
    })
  })
})

describe('setEventInQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sets event without changing span', () => {
    const { replaceState } = stubWindow(
      'http://local/traces/t1?span=s1&start=0&log=l1'
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

describe('setLogInQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sets log without changing span and clears event', () => {
    const { replaceState } = stubWindow(
      'http://local/traces/t1?span=s1&event=1&start=0'
    )
    setLogInQuery('l1')
    expect(parseRoute(navigationURL(replaceState))).toEqual({
      path: '/traces/t1',
      query: { start: '0', span: 's1', log: 'l1' },
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
