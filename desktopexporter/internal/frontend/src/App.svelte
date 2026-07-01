<script lang="ts">
  import HomePage from '@/pages/HomePage.svelte'
  import MetricsPage from '@/pages/MetricsPage.svelte'
  import LogsPage from '@/pages/LogsPage.svelte'
  import TracesPage from '@/pages/TracesPage.svelte'
  import { Route, router } from 'tinro5'
  import { createTimeContext } from '@/contexts/time-context.svelte'

  createTimeContext()

  router.mode.history()
</script>

<main
  class="flex h-screen min-w-0 flex-col overflow-hidden bg-base-100 transition-colors duration-300"
>
  <Route path="/">
    <HomePage />
  </Route>
  <Route path="/metrics">
    <MetricsPage />
  </Route>
  <Route path="/logs">
    <LogsPage />
  </Route>
  <Route path="/traces/*">
    <TracesPage />
  </Route>
</main>
