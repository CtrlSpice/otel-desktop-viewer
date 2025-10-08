<script lang="ts">
  import { HugeiconsIcon } from "@hugeicons/svelte"
  import {
    HomeIcon,
    BarChartHorizontalIcon,
    Chart03Icon,
    File01Icon,
  } from "@hugeicons/core-free-icons"
  import ThemeToggle from "@/components/ThemeToggle.svelte"
  import { router } from "tinro5"

  // Navigation items
  // prettier-ignore
  let navItems = [
    { id: "home", icon: HomeIcon, label: "Home", path: "/" },
    { id: "traces", icon: BarChartHorizontalIcon, label: "Traces", path: "/traces" },
    { id: "metrics", icon: Chart03Icon, label: "Metrics", path: "/metrics" },
    { id: "logs", icon: File01Icon, label: "Logs", path: "/logs" },
  ]

  // Current path state
  let currentPath = $state(router.path);

  // Subscribe to router changes
  router.subscribe((route) => {
    currentPath = route.path;
  });

  function handleNavClick(path: string) {
    router.goto(path);
  }
</script>

<!-- Horizontal Navigation Bar -->
<nav
  class="fixed top-0 left-0 right-0 h-12 bg-base-200 border-b border-base-300 flex items-center justify-between px-4 z-40"
>
  <!-- Left side - Navigation items -->
  <div class="flex items-end h-full pt-2">
    {#each navItems as item}
      <button
        class="nav-button {currentPath === item.path
          ? 'nav-button-active'
          : 'nav-button-inactive'}"
        onclick={() => handleNavClick(item.path)}
      >
        <HugeiconsIcon icon={item.icon} size={16} />
        <span class="text-xs font-medium">{item.label}</span>
      </button>
    {/each}
  </div>

  <!-- Right side - Theme toggle -->
  <div class="flex items-center">
    <ThemeToggle />
  </div>
</nav>

<style>
  .nav-button {
    @apply flex items-center justify-center gap-1.5 px-3 rounded-t-lg transition-colors duration-200 mr-1 h-full w-24;
  }

  .nav-button-active {
    @apply bg-primary text-primary-content border-b-2 border-primary-content;
  }

  .nav-button-inactive {
    @apply text-base-content hover:bg-base-300 border-b-2 border-transparent;
  }

  /* Light theme overrides */
  :global([data-theme="light"]) .nav-button-active {
    @apply bg-primary-content text-primary border-b-2 border-primary;
  }

  :global([data-theme="light"]) .nav-button-inactive {
    @apply text-base-content/70 hover:bg-base-200 hover:text-base-content;
  }
</style>
