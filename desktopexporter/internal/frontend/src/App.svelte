<script lang="ts">
  import HorizontalNav from '@/components/HorizontalNav.svelte';
  import HomePage from '@/pages/HomePage.svelte';
  import MetricsPage from '@/pages/MetricsPage.svelte';
  import LogsPage from '@/pages/LogsPage.svelte';
  import TracesPage from '@/pages/TracesPage.svelte';
  import TraceDetailPage from '@/pages/TraceDetailPage.svelte';
  import { Route, router } from 'tinro5';
  import { createTimeContext } from '@/contexts/time-context.svelte';
  
  // Create the time context at the root level
  createTimeContext();
  
  // Configure router for history mode
  router.mode.history();
</script>

<main
  class="flex h-screen min-w-0 flex-col bg-base-100 bg-gradient-to-b from-base-100 via-base-100 to-base-200/25 transition-colors duration-300"
>
  <HorizontalNav />
  <div
    class="mx-auto flex min-h-0 w-full min-w-0 flex-1 flex-col px-4 pb-10 min-[900px]:px-6"
    style="padding-top: calc(var(--nav-height) + var(--layout-gap));"
  >
    <!-- Router: Show different pages based on current route -->
    <Route path="/">
      <HomePage />
    </Route>
    <Route path="/metrics">
      <MetricsPage />
    </Route>
    <Route path="/logs">
      <LogsPage />
    </Route>
    <Route path="/traces">
      <TracesPage />
    </Route>
    <Route path="/trace/:traceID">
      <TraceDetailPage />
    </Route>
  </div>
</main>


