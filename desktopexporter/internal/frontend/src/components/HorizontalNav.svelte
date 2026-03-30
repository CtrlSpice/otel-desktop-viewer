<script lang="ts">
  import { HugeiconsIcon } from "@hugeicons/svelte"
  import {
    HomeIcon,
    BarChartHorizontalIcon,
    Chart03Icon,
    FirePitIcon,
  } from "@hugeicons/core-free-icons"
  import ThemeToggle from "@/components/ThemeToggle.svelte"
  import { router } from "tinro5"

  /** True when this top-level nav item should show as active (list or detail under that signal). */
  function isNavItemActive(path: string, itemId: string): boolean {
    switch (itemId) {
      case "home":
        return path === "/"
      case "traces":
        return path === "/traces" || path.startsWith("/trace/")
      case "metrics":
        return path === "/metrics" || path.startsWith("/metrics/")
      case "logs":
        return path === "/logs" || path.startsWith("/logs/")
      default:
        return false
    }
  }

  // Navigation items
  // prettier-ignore
  let navItems = [
    { id: "home", icon: HomeIcon, label: "Home", path: "/" },
    { id: "traces", icon: BarChartHorizontalIcon, label: "Traces", path: "/traces" },
    { id: "metrics", icon: Chart03Icon, label: "Metrics", path: "/metrics" },
    { id: "logs", icon: FirePitIcon, label: "Logs", path: "/logs" },
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
  class="fixed top-0 left-0 right-0 z-40 flex h-14 min-w-0 items-center justify-between gap-2 border-b border-base-300/50 bg-base-100/80 px-4 backdrop-blur-md backdrop-saturate-150 min-[900px]:px-6"
>
  <div class="flex min-w-0 flex-1 items-center overflow-x-auto overflow-y-hidden">
    <div class="flex flex-nowrap items-center gap-1 pr-2">
    {#each navItems as item}
      {@const active = isNavItemActive(currentPath, item.id)}
      <button
        type="button"
        class="nav-button {active ? 'nav-button-active' : 'nav-button-inactive'}"
        aria-current={active ? "page" : undefined}
        onclick={() => handleNavClick(item.path)}
      >
        <HugeiconsIcon icon={item.icon} size={17} />
        <span>{item.label}</span>
      </button>
    {/each}
    </div>
  </div>

  <div class="flex shrink-0 items-center">
    <ThemeToggle />
  </div>
</nav>

<style lang="postcss">
  .nav-button {
    @apply flex items-center justify-center gap-2 rounded-lg px-4 py-2 text-sm font-medium tracking-tight transition-[color,background-color,box-shadow] duration-200;
  }

  .nav-button-active {
    @apply bg-primary/15 text-primary shadow-sm shadow-primary/10 ring-1 ring-primary/20;
  }

  .nav-button-inactive {
    @apply border border-transparent text-base-content/55 hover:bg-base-200/80 hover:text-base-content;
  }
</style>
