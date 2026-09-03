import { describe, expect, it } from 'vitest'
import { loadWithAbortTimeout } from './abortable-load'

describe('bounded benchmark loads', () => {
  it('aborts a stalled load and returns the registered timeout error', async () => {
    let observedSignal: AbortSignal | undefined
    const result = loadWithAbortTimeout(
      signal => {
        observedSignal = signal
        return new Promise((_resolve, reject) => {
          signal.addEventListener('abort', () => reject(signal.reason), {
            once: true,
          })
        })
      },
      5,
      'load timed out'
    )

    await expect(result).rejects.toThrow('load timed out')
    expect(observedSignal?.aborted).toBe(true)
  })

  it('preserves an ordinary load failure without waiting for the timeout', async () => {
    const failure = new Error('load failed')
    await expect(
      loadWithAbortTimeout(async () => Promise.reject(failure), 1_000, 'late')
    ).rejects.toBe(failure)
  })

  it('rejects an invalid timeout before starting the load', async () => {
    let started = false
    await expect(
      loadWithAbortTimeout(
        async () => {
          started = true
          return 'unreachable'
        },
        0,
        'invalid'
      )
    ).rejects.toThrow(/positive safe integer/)
    expect(started).toBe(false)
  })
})
