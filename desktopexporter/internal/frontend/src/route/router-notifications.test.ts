// @vitest-environment jsdom
import { expect, it } from 'vitest'
import { navigate, subscribeToRoute } from './router'

it('keeps notification membership stable when listeners subscribe and unsubscribe', () => {
  const calls: string[] = []
  const unsubscribe: (() => void)[] = []
  let mutate = false
  const later = () => calls.push('later')
  const removed = () => calls.push('removed')
  const removeFirst = subscribeToRoute(() => {
    calls.push('first')
    if (mutate) {
      mutate = false
      removeSecond()
      unsubscribe.push(subscribeToRoute(later))
    }
  })
  const removeSecond = subscribeToRoute(removed)
  unsubscribe.push(removeFirst, removeSecond)
  try {
    calls.length = 0
    mutate = true
    navigate('/metrics')
    // Subscribe invokes immediately, but the new listener must not run again.
    // The removed listener still belongs to the in-flight notification.
    expect(calls).toEqual(['first', 'later', 'removed'])
    calls.length = 0
    navigate('/traces')
    expect(calls).toEqual(['first', 'later'])
  } finally {
    for (const cleanup of unsubscribe) cleanup()
  }
})
