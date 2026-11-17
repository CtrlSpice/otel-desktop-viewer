import { createSubscriber } from 'svelte/reactivity'

// All mounted relative dates share one timer; the last reader stops it.
const subscribe = createSubscriber(update => {
  const timer = setInterval(update, 30_000)
  return () => clearInterval(timer)
})

export const wallClock = {
  get now(): bigint {
    subscribe()
    return BigInt(Date.now()) * 1_000_000n
  },
}
