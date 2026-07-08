<script lang="ts">
  import HomePage from '@/pages/HomePage.svelte'
  import MetricsPage from '@/pages/MetricsPage.svelte'
  import LogsPage from '@/pages/LogsPage.svelte'
  import TracesPage from '@/pages/TracesPage.svelte'
  import { createRouteContext, getRouteContext } from '@/contexts/route-context.svelte'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  createRouteContext()
  createTimeContext()

  const routeContext = getRouteContext()

  function under(base: string): boolean {
    const path = routeContext.route.path
    return path === base || path.startsWith(base + '/')
  }

  const Page = $derived(
    under('/traces') ? TracesPage
    : under('/metrics') ? MetricsPage
    : under('/logs') ? LogsPage
    : HomePage
  )
</script>

<main
  class="flex h-screen min-w-0 flex-col overflow-hidden bg-base-100 transition-colors duration-300"
>
  <Page />
</main>
