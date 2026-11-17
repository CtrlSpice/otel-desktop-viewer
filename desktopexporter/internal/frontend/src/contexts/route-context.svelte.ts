import { createContext } from 'svelte'
import {
  readRoute,
  subscribeToRoute,
  type Route,
  type SignalName,
} from '@/route'

const [getRouteContext, setRouteContext] = createContext<RouteContext>()

export interface RouteContext {
  get route(): Route
  queryFor(signal: SignalName): string | undefined
}

export function createRouteContext(): RouteContext {
  let route = $state<Route>(readRoute())
  const queries = $state<Partial<Record<SignalName, string>>>({})
  function remember(current: Route) {
    for (const signal of ['traces', 'metrics', 'logs'] as const) {
      if (
        current.path === `/${signal}` ||
        current.path.startsWith(`/${signal}/`)
      )
        queries[signal] = current.query.q ?? ''
    }
  }
  remember(readRoute())

  $effect(() => {
    return subscribeToRoute(() => {
      const current = readRoute()
      route = current
      remember(current)
    })
  })

  const ctx: RouteContext = {
    queryFor(signal) {
      return queries[signal] || undefined
    },
    get route() {
      return route
    },
  }

  setRouteContext(ctx)
  return ctx
}

export { getRouteContext }
export type { Route }
