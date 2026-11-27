import { getContext, onMount, setContext } from 'svelte'

const key = Symbol('data-changes')

/** One connection serves import lookups, freshness indicators and Live. */
export function createDataChanges() {
  let live = $state(false)
  let connected = $state(false)
  let sessionKey = $state(0)
  const listeners = new Set<() => void>()
  const liveListeners = new Set<() => void>()
  const sessionListeners = new Set<() => void>()
  function notifyChange() {
    for (const listener of listeners) listener()
  }
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
    get sessionKey() {
      return sessionKey
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
    subscribeSession(listener: () => void) {
      sessionListeners.add(listener)
      return () => sessionListeners.delete(listener)
    },
    refreshViews: refreshLive,
    notifySessionChanged() {
      sessionKey += 1
      for (const listener of sessionListeners) listener()
      notifyChange()
    },
  }
  onMount(() => {
    const source = new EventSource('/api/changes')
    source.addEventListener('change', () => {
      connected = true
      // The server sends a snapshot on every connection. Even an unchanged
      // revision must resynchronise queries after a disconnect or restart.
      notifyChange()
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
