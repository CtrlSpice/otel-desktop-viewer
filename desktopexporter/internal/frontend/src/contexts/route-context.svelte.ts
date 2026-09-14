import { createContext } from 'svelte'
import { readRoute, subscribeToRoute, type Route } from '@/route'

const [getRouteContext, setRouteContext] = createContext<RouteContext>()

export interface RouteContext {
  get route(): Route
}

export function createRouteContext(): RouteContext {
  let route = $state<Route>(readRoute())

  $effect(() => {
    return subscribeToRoute(() => {
      route = readRoute()
    })
  })

  const ctx: RouteContext = {
    get route() {
      return route
    },
  }

  setRouteContext(ctx)
  return ctx
}

export { getRouteContext }
export type { Route }
