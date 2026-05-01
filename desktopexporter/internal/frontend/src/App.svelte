<script lang="ts">
  import HomePage from '@/pages/HomePage.svelte'
  import MetricsPage from '@/pages/MetricsPage.svelte'
  import LogsPage from '@/pages/LogsPage.svelte'
  import TracesPage from '@/pages/TracesPage.svelte'
  import SignalNavRail from '@/components/SignalNavRail.svelte'
  import { Route, router } from 'tinro5'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  createTimeContext()

  router.mode.history()
</script>

<main
  class="flex h-screen min-w-0 flex-col overflow-hidden bg-base-100 bg-gradient-to-b from-base-100 via-base-100 to-base-200/25 transition-colors duration-300"
>
  <Route path="/">
    <SignalNavRail>
      <div class="mx-auto w-full max-w-[88rem] px-4 pb-10 pt-[var(--layout-gap)] min-[900px]:px-6">
        <HomePage />
      </div>
    </SignalNavRail>
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
</main>
