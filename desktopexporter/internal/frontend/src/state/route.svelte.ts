// Reactive tinro5 route snapshot for components that derive selection from the URL.
//
// Sync seed + subscribe live in readCurrentRoute() (url-state.ts); this hook
// wraps that for Svelte reactivity (back/forward and in-app navigation).

import { readCurrentRoute, type RouteSnapshot } from '@/utils/url-state'
import { router } from 'tinro5'

export type { RouteSnapshot }

export function useRoute(): RouteSnapshot {
  const initial = readCurrentRoute()
  let path = $state(initial.path)
  let query = $state<Record<string, string>>(initial.query)

  $effect(() => {
    const unsubscribe = router.subscribe(route => {
      path = route.path
      query = route.query
    })
    return unsubscribe
  })

  return {
    get path() {
      return path
    },
    get query() {
      return query
    },
  }
}
