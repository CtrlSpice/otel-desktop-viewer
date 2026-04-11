<script lang="ts">
  import type { Component } from 'svelte'
  import {
    HomeIcon,
    BarChartHorizontalIcon,
    ChartHistogramIcon,
    FirePitIcon,
  } from '@/icons'
  import ThemeToggle from '@/components/ThemeToggle.svelte'
  import { router } from 'tinro5'

  type NavItem = {
    id: string
    label: string
    path: string
    icon: Component
  }

  const navItems: NavItem[] = [
    { id: 'home', label: 'Home', path: '/', icon: HomeIcon },
    {
      id: 'traces',
      label: 'Traces',
      path: '/traces',
      icon: BarChartHorizontalIcon,
    },
    {
      id: 'metrics',
      label: 'Metrics',
      path: '/metrics',
      icon: ChartHistogramIcon,
    },
    {
      id: 'logs',
      label: 'Logs',
      path: '/logs',
      icon: FirePitIcon,
    },
  ]

  const rules: Record<string, (p: string) => boolean> = {
    home: p => p === '/',
    traces: p => p === '/traces' || p.startsWith('/trace/'),
    metrics: p => p === '/metrics' || p.startsWith('/metrics/'),
    logs: p => p === '/logs' || p.startsWith('/logs/'),
  }

  /** True when this top-level nav item should show as active (list or detail under that signal). */
  function isNavItemActive(path: string, itemId: string): boolean {
    return (rules[itemId] ?? (() => false))(path)
  }

  // Current path state
  let currentPath = $state(router.path ?? '/')
  $effect(() => {
    const unsubscribe = router.subscribe(route => {
      currentPath = route.path
    })
    return unsubscribe
  })

  const handleNavClick = (path: string) => router.goto(path)
</script>

<!-- Horizontal Navigation Bar -->
<nav
  class="fixed top-0 left-0 right-0 z-40 flex min-h-0 min-w-0 items-center justify-between gap-2 border-b border-base-300/50 bg-base-100/80 px-4 backdrop-blur-md backdrop-saturate-150 min-[900px]:px-6"
  style="height: var(--nav-height);"
>
  <div
    class="flex min-w-0 flex-1 items-center overflow-x-auto overflow-y-hidden"
  >
    <div class="flex flex-nowrap items-center gap-1 pr-2">
      {#each navItems as item}
        {@const active = isNavItemActive(currentPath, item.id)}
        {@const NavIcon = item.icon}
        <button
          type="button"
          class="nav-button {active
            ? 'nav-button-active'
            : 'nav-button-inactive'}"
          aria-current={active ? 'page' : undefined}
          onclick={() => handleNavClick(item.path)}
        >
          <NavIcon class="h-[17px] w-[17px] shrink-0" aria-hidden="true" />
          <span>{item.label}</span>
        </button>
      {/each}
    </div>
  </div>

  <div class="flex shrink-0 items-center">
    <ThemeToggle />
  </div>
</nav>
