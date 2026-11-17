import { getContext, onMount, setContext } from 'svelte'

const key = Symbol('data-changes')

/** One connection serves import lookups, freshness indicators and Live. */
export function createDataChanges() {
  let live = $state(false)
  let connected = $state(false)
  const listeners = new Set<() => void>()
  const liveListeners = new Set<() => void>()
  function refreshLive() {
    for (const listener of liveListeners) listener()
  }
  const state = {
    get live() {
      return live
    },
    get connected() {
      return connected
    },
    setLive(enabled: boolean) {
      live = enabled
      if (enabled) refreshLive()
    },
    subscribe(listener: () => void) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    subscribeLive(listener: () => void) {
      liveListeners.add(listener)
      return () => liveListeners.delete(listener)
    },
    refreshViews: refreshLive,
  }
  onMount(() => {
    const source = new EventSource('/api/changes')
    source.addEventListener('change', () => {
      connected = true
      // The server sends a snapshot on every connection. Even an unchanged
      // revision must resynchronise queries after a disconnect or restart.
      for (const listener of listeners) listener()
      if (live) refreshLive()
    })
    source.onerror = () => (connected = false)
    return () => source.close()
  })
  setContext(key, state)
  return state
}

export function getDataChanges() {
  return getContext<ReturnType<typeof createDataChanges> | undefined>(key)
}
