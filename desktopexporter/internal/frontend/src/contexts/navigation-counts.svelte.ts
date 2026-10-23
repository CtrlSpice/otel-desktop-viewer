import { getContext, onDestroy, onMount, setContext } from 'svelte'
import { get } from 'svelte/store'
import { evalsEnabled } from '@/state/features'
import { telemetryAPI } from '@/services/telemetry-service'

export type NavigationCounts = Partial<
  Record<'home' | 'traces' | 'metrics' | 'logs' | 'evals', number>
>
const key = Symbol('navigation-counts')

/** A database-wide reading, independent of route filters and incoming telemetry. */
export function createNavigationCounts() {
  let counts = $state<NavigationCounts>({})
  let homeCounts = $state<NavigationCounts | null>(null)
  let homeSearchActive = $state(false)
  let homeLinks = $state<Partial<Record<keyof NavigationCounts, string>>>({})
  let generation = 0
  let request: AbortController | undefined
  async function serviceCount(signal: AbortSignal) {
    let total = 0
    for (;;) {
      const page = await telemetryAPI.discoverServices(
        { offset: total, limit: 100 },
        signal
      )
      total += page.items.length
      if (!page.truncated) return total
      if (!page.items.length) throw new Error('Service count is incomplete')
    }
  }
  async function refresh() {
    request?.abort()
    const controller = new AbortController()
    request = controller
    const current = ++generation
    const results = await Promise.allSettled([
      telemetryAPI.getStats(),
      serviceCount(controller.signal),
      get(evalsEnabled)
        ? telemetryAPI.searchEvalExperiments({ limit: 1 }, controller.signal)
        : Promise.resolve(undefined),
    ])
    if (current !== generation || controller.signal.aborted) return
    const [stats, services, evals] = results
    counts = {
      traces:
        stats.status === 'fulfilled'
          ? stats.value.traces.traceCount
          : undefined,
      metrics:
        stats.status === 'fulfilled'
          ? stats.value.metrics.metricCount
          : undefined,
      logs:
        stats.status === 'fulfilled' ? stats.value.logs.logCount : undefined,
      home: services.status === 'fulfilled' ? services.value : undefined,
      evals: evals.status === 'fulfilled' ? evals.value?.total : undefined,
    }
  }
  const state = {
    get counts() {
      return homeCounts ? { ...counts, ...homeCounts } : counts
    },
    get matching() {
      return homeCounts !== null && homeSearchActive
    },
    get links() {
      return homeLinks
    },
    setHomeCounts(
      value: NavigationCounts | null,
      links: typeof homeLinks = {},
      searchActive = false
    ) {
      homeCounts = value
      homeLinks = links
      homeSearchActive = searchActive
    },
    refresh,
  }
  setContext(key, state)
  onMount(() => {
    void refresh()
    let enabledBefore = get(evalsEnabled)
    return evalsEnabled.subscribe(enabled => {
      if (enabled && !enabledBefore) void refresh()
      enabledBefore = enabled
    })
  })
  onDestroy(() => request?.abort())
  return state
}

export function getNavigationCounts() {
  return getContext<ReturnType<typeof createNavigationCounts> | undefined>(key)
}
