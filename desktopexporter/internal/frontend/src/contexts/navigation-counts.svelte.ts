import { getContext, onDestroy, onMount, setContext } from 'svelte'
import { get } from 'svelte/store'
import { evalsEnabled } from '@/state/features'
import { telemetryAPI } from '@/services/telemetry-service'
import { evalAPI } from '@/services/eval-service'
import { getDataChanges } from '@/contexts/data-changes.svelte'
import { createRefreshCoordinator } from '@/services/refresh-coordinator'

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
  async function fetchCounts() {
    request?.abort()
    const controller = new AbortController()
    request = controller
    const current = ++generation
    function publish(value: NavigationCounts) {
      if (current === generation && !controller.signal.aborted)
        counts = { ...counts, ...value }
    }
    await Promise.allSettled([
      telemetryAPI.getStats(controller.signal).then(stats =>
        publish({
          traces: stats.traces.traceCount,
          metrics: stats.metrics.metricCount,
          logs: stats.logs.logCount,
        })
      ),
      telemetryAPI
        .getServiceCount(controller.signal)
        .then(home => publish({ home })),
      __EVALS_BUILT__ && get(evalsEnabled)
        ? evalAPI
            .searchEvalExperiments({ limit: 1 }, controller.signal)
            .then(evals => publish({ evals: evals.total }))
        : publish({ evals: undefined }),
    ])
  }
  const coordinator = createRefreshCoordinator(fetchCounts)
  const refresh = coordinator.request
  const changes = getDataChanges()
  if (changes) onDestroy(changes.subscribe(() => void refresh()))
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
  onDestroy(() => {
    coordinator.dispose()
    request?.abort()
  })
  return state
}

export function getNavigationCounts() {
  return getContext<ReturnType<typeof createNavigationCounts> | undefined>(key)
}
