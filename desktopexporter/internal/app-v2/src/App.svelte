<script lang="ts">
  import HorizontalNav from './components/HorizontalNav.svelte';
  import HomePage from './pages/HomePage.svelte';
  import MetricsPage from './pages/MetricsPage.svelte';
  import LogsPage from './pages/LogsPage.svelte';
  import TracesPage from './pages/TracesPage.svelte';
  import { Route, router } from 'tinro5';
  import { createTimeContext } from './contexts/time-context.svelte';
  
  // Create the time context at the root level
  createTimeContext();
  
  // Configure router for history mode
  router.mode.history();
</script>

<main class="min-h-screen bg-base-100 transition-colors duration-300">
  <HorizontalNav />
  <div class="container mx-auto p-8 pt-20">
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
  </div>
</main>


