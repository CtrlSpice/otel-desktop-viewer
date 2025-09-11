<script lang="ts">
  import HorizontalNav from './components/HorizontalNav.svelte';
  import HomePage from './pages/HomePage.svelte';
  import MetricsPage from './pages/MetricsPage.svelte';
  import LogsPage from './pages/LogsPage.svelte';
  import TracesPage from './pages/TracesPage.svelte';
  import { currentRoute } from './stores/router';
</script>

<main class="min-h-screen bg-base-300 transition-colors duration-300">
  <HorizontalNav />
  <div class="container mx-auto p-8 pt-20">
    <!-- Router: Show different pages based on current route -->
    {#if $currentRoute.page === 'home'}
      <HomePage />
    {:else if $currentRoute.page === 'metrics'}
      <MetricsPage />
    {:else if $currentRoute.page === 'logs'}
      <LogsPage />
    {:else if $currentRoute.page === 'traces'}
      <TracesPage />
    {/if}
  </div>
</main>


