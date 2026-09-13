<script module lang="ts">
  import type { Component } from 'svelte'

  export type AppPages = {
    home: Component
    traces: Component
    metrics: Component
    logs: Component
  }
</script>

<script lang="ts">
  import { tick } from 'svelte'
  import HomePage from '@/pages/HomePage.svelte'
  import MetricsPage from '@/pages/MetricsPage.svelte'
  import LogsPage from '@/pages/LogsPage.svelte'
  import TracesPage from '@/pages/TracesPage.svelte'
  import {
    createRouteContext,
    getRouteContext,
  } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  const productionPages = {
    home: HomePage,
    traces: TracesPage,
    metrics: MetricsPage,
    logs: LogsPage,
  } satisfies AppPages

  let { pages = productionPages }: { pages?: AppPages } = $props()

  createRouteContext()
  createTimeContext()

  const routeContext = getRouteContext()
  let mainElement = $state<HTMLElement | null>(null)

  function pagePath(path: string): string {
    for (const base of ['/traces', '/metrics', '/logs']) {
      if (path === base || path.startsWith(`${base}/`)) return base
    }
    return '/'
  }

  let lastFocusedPage = pagePath(routeContext.route.path)

  $effect(() => {
    const page = pagePath(routeContext.route.path)
    if (page === lastFocusedPage) return
    lastFocusedPage = page
    void tick().then(() => {
      if (pagePath(routeContext.route.path) === page) mainElement?.focus()
    })
  })

  function under(base: string): boolean {
    const path = routeContext.route.path
    return path === base || path.startsWith(base + '/')
  }

  const Page = $derived(
    under('/traces')
      ? pages.traces
      : under('/metrics')
        ? pages.metrics
        : under('/logs')
          ? pages.logs
          : pages.home
  )
</script>

<main
  bind:this={mainElement}
  tabindex="-1"
  class="flex h-screen min-w-0 flex-col overflow-hidden bg-base-100 transition-colors duration-300"
>
  <Page />
</main>
