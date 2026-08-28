import { describe, expect, it, vi } from 'vitest'
import { createFieldValueCache } from './field-value-cache'

describe('field value cache', () => {
  it('fetches a field once and shares the result', async () => {
    const fetch = vi.fn(async () => ['a', 'b'])
    const cache = createFieldValueCache(fetch, 'traces')
    expect(await cache.values('name')).toEqual(['a', 'b'])
    expect(await cache.values('name')).toEqual(['a', 'b'])
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledWith('traces', 'name', '', 500)
  })

  it('shares one flight between concurrent callers', async () => {
    const fetch = vi.fn(async () => ['a'])
    const cache = createFieldValueCache(fetch, 'traces')
    await Promise.all([cache.values('name'), cache.values('name')])
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('keeps fields separate', async () => {
    const fetch = vi.fn(async (_s: string, field: string) => [field])
    const cache = createFieldValueCache(fetch, 'metrics')
    expect(await cache.values('name')).toEqual(['name'])
    expect(await cache.values('unit')).toEqual(['unit'])
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('evicts a failed fetch so the next call retries', async () => {
    // One unreachable moment must not disable completion for the session.
    let attempt = 0
    const fetch = vi.fn(async () => {
      attempt++
      if (attempt === 1) throw new Error('store down')
      return ['recovered']
    })
    const cache = createFieldValueCache(fetch, 'traces')
    await expect(cache.values('name')).rejects.toThrow('store down')
    expect(await cache.values('name')).toEqual(['recovered'])
  })

  it('evicts by identity, not blindly', async () => {
    // A slow failing fetch must not evict the successful one that replaced
    // it, or the retry's result is thrown away and every call refetches.
    let resolveSlow: (v: string[]) => void = () => {}
    let rejectSlow: (e: Error) => void = () => {}
    const fetch = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise<string[]>((res, rej) => {
            resolveSlow = res
            rejectSlow = rej
          })
      )
      .mockImplementationOnce(async () => ['second'])

    const cache = createFieldValueCache(fetch, 'traces')
    const first = cache.values('name')
    const firstCaught = first.catch(() => 'failed')
    rejectSlow(new Error('slow failure'))
    expect(await firstCaught).toBe('failed')

    const second = cache.values('name')
    expect(await second).toEqual(['second'])
    // The entry now holds the successful fetch, so a third call reuses it.
    expect(await cache.values('name')).toEqual(['second'])
    expect(fetch).toHaveBeenCalledTimes(2)
    void resolveSlow
  })
})
