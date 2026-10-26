// @vitest-environment jsdom
import { expect, it } from 'vitest'
import {
  itemHref,
  navigate,
  navigateToItem,
  readRoute,
  signalHref,
  subscribeToRoute,
} from './router'

it('isolates cross-signal native and routed item links while retaining time and exact selection', () => {
  navigate('/logs?q=body%20CONTAINS%20%22failure%22&start=10&end=20&log=old')
  const href = itemHref('traces', 'trace-1', { span: 'span-1' })
  expect(href).toBe('/traces/trace-1?start=10&end=20&span=span-1')
  expect(signalHref('metrics')).toBe('/metrics?start=10&end=20')
  expect(
    new URL(itemHref('logs', 'log-2'), 'http://local').searchParams.get('q')
  ).toBe('body CONTAINS "failure"')
  navigateToItem('traces', 'trace-1', 'push', { span: 'span-1' })
  expect(window.location.pathname + window.location.search).toBe(href)
  expect(readRoute().query).toEqual({ start: '10', end: '20', span: 'span-1' })
})

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
