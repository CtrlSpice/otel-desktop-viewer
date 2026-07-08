import { setContext, getContext } from 'svelte'
import { readRoute, subscribeToRoute, type Route } from '@/route'

const KEY = 'route'

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

  setContext(KEY, ctx)
  return ctx
}

export function getRouteContext(): RouteContext {
  return getContext<RouteContext>(KEY)
}

export type { Route }
