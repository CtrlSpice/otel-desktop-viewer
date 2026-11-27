import { getContext, onDestroy, onMount, setContext } from 'svelte'
import { get } from 'svelte/store'
import { evalsEnabled } from '@/state/features'
import { telemetryAPI } from '@/services/telemetry-service'
import { evalAPI } from '@/features/evals/services/eval-service'
import { getDataChanges } from '@/contexts/data-changes.svelte'
import { createRefreshCoordinator } from '@/services/refresh-coordinator'
import { createRead } from '@/services/reads'

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
  const statsRead = createRead<
    Awaited<ReturnType<typeof telemetryAPI.getStats>>
  >({ group: null })
  const homeRead = createRead<number>({ group: null })
  const evalsRead = createRead<
    Awaited<ReturnType<typeof evalAPI.searchEvalExperiments>>
  >({ group: null })
  async function fetchCounts() {
    function publish(value: NavigationCounts) {
      counts = { ...counts, ...value }
    }
    await Promise.allSettled([
      statsRead.load(
        signal => telemetryAPI.getStats(signal),
        stats =>
          publish({
            traces: stats.traces.traceCount,
            metrics: stats.metrics.metricCount,
            logs: stats.logs.logCount,
          })
      ),
      homeRead.load(
        signal => telemetryAPI.getServiceCount(signal),
        home => publish({ home })
      ),
      __EVALS_BUILT__ && get(evalsEnabled)
        ? evalsRead.load(
            signal => evalAPI.searchEvalExperiments({ limit: 1 }, signal),
            evals => publish({ evals: evals.total })
          )
        : publish({ evals: undefined }),
    ])
  }
  const coordinator = createRefreshCoordinator(fetchCounts)
  const refresh = coordinator.request
  function resetForSession() {
    counts = {}
    homeCounts = null
    homeLinks = {}
    homeSearchActive = false
    statsRead.clear()
    homeRead.clear()
    evalsRead.clear()
  }
  const changes = getDataChanges()
  if (changes) {
    onDestroy(changes.subscribeSession(resetForSession))
    onDestroy(changes.subscribe(() => void refresh()))
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
  onDestroy(() => {
    coordinator.dispose()
  })
  return state
}

export function getNavigationCounts() {
  return getContext<ReturnType<typeof createNavigationCounts> | undefined>(key)
}
