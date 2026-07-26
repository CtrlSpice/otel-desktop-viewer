// Only the pure helpers are covered here; navigate, readRoute, navigateCurrentRoute,
// navigateToItem, navigateToSignal, and subscribeToRoute need window/history and are
// deferred to a later DOM-testing stint.
import { describe, expect, it } from 'vitest'
import {
  buildSearch,
  parseRoute,
  signalIdFromPath,
  withoutParams,
  withQueryPatch,
} from './router'

describe('parseRoute', () => {
  it('parses an absolute href into path and query', () => {
    expect(parseRoute('http://example.com/traces/abc?span=1')).toEqual({
      path: '/traces/abc',
      query: { span: '1' },
    })
  })

  it('parses a relative href into path and query', () => {
    expect(parseRoute('/traces/abc?span=1')).toEqual({
      path: '/traces/abc',
      query: { span: '1' },
    })
  })

  it('parses a path with multiple query params', () => {
    expect(parseRoute('/metrics?agg=rate&dp=dp-1')).toEqual({
      path: '/metrics',
      query: { agg: 'rate', dp: 'dp-1' },
    })
  })

  it('parses a path with no query into an empty query object', () => {
    expect(parseRoute('/traces')).toEqual({ path: '/traces', query: {} })
  })
})

describe('buildSearch', () => {
  it('builds a search string from a query object', () => {
    expect(buildSearch({ a: '1', b: '2' })).toBe('?a=1&b=2')
  })

  it('omits empty string values', () => {
    expect(buildSearch({ a: '1', b: '' })).toBe('?a=1')
  })

  it('omits null values', () => {
    expect(buildSearch({ a: '1', b: null as unknown as string })).toBe('?a=1')
  })

  it('omits undefined values', () => {
    expect(buildSearch({ a: '1', b: undefined as unknown as string })).toBe(
      '?a=1'
    )
  })

  it('returns an empty string, not "?", when the result is empty', () => {
    expect(buildSearch({})).toBe('')
    expect(buildSearch({ a: '' })).toBe('')
  })
})

describe('parseRoute and buildSearch round-trip', () => {
  it('recovers the original path and query', () => {
    const path = '/traces/abc'
    const query = { span: '1', agg: 'rate' }
    expect(parseRoute(path + buildSearch(query))).toEqual({ path, query })
  })
})

describe('withQueryPatch', () => {
  it('adds a new key', () => {
    expect(withQueryPatch({ a: '1' }, { b: '2' })).toEqual({ a: '1', b: '2' })
  })

  it('overwrites an existing key', () => {
    expect(withQueryPatch({ a: '1' }, { a: '2' })).toEqual({ a: '2' })
  })

  it('clears a key when patched with null', () => {
    expect(withQueryPatch({ a: '1', b: '2' }, { a: null })).toEqual({
      b: '2',
    })
  })

  it('clears a key when patched with undefined', () => {
    expect(withQueryPatch({ a: '1', b: '2' }, { a: undefined })).toEqual({
      b: '2',
    })
  })

  it('does not mutate the input query object', () => {
    const query = { a: '1' }
    withQueryPatch(query, { a: '2', b: '3' })
    expect(query).toEqual({ a: '1' })
  })
})

describe('withoutParams', () => {
  it('drops the listed keys', () => {
    expect(withoutParams({ a: '1', b: '2', c: '3' }, ['a', 'c'])).toEqual({
      b: '2',
    })
  })

  it('leaves the query unchanged when no listed keys are present', () => {
    expect(withoutParams({ a: '1' }, ['b'])).toEqual({ a: '1' })
  })

  it('does not mutate the input query object', () => {
    const query = { a: '1', b: '2' }
    withoutParams(query, ['a'])
    expect(query).toEqual({ a: '1', b: '2' })
  })
})

describe('signalIdFromPath', () => {
  it('extracts the id from a signal item path', () => {
    expect(signalIdFromPath('traces', '/traces/abc')).toBe('abc')
  })

  it('decodes a URL-encoded id', () => {
    const id = 'abc def/123'
    const path = `/traces/${encodeURIComponent(id)}`
    expect(signalIdFromPath('traces', path)).toBe(id)
  })

  it('returns null for the bare list path', () => {
    expect(signalIdFromPath('traces', '/traces')).toBeNull()
  })

  it('returns null for a different signal path', () => {
    expect(signalIdFromPath('traces', '/metrics/abc')).toBeNull()
  })

  it('returns null for the bare list path with a trailing slash', () => {
    expect(signalIdFromPath('traces', '/traces/')).toBeNull()
  })

  it('takes only the first path segment after the signal prefix', () => {
    expect(signalIdFromPath('traces', '/traces/abc/extra')).toBe('abc')
  })
})
