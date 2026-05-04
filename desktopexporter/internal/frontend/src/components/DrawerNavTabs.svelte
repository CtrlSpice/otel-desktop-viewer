<script module lang="ts">
  import type { Component } from 'svelte'
  import {
    HomeIcon,
    BarChartHorizontalIcon,
    ChartHistogramIcon,
    FirePitIcon,
  } from '@/icons'

  export type NavItem = {
    id: string
    label: string
    path: string
    icon: Component
  }

  export const HOME_NAV: NavItem = {
    id: 'home',
    label: 'Home',
    path: '/',
    icon: HomeIcon,
  }

  export const NAV_ITEMS: NavItem[] = [
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
    { id: 'logs', label: 'Logs', path: '/logs', icon: FirePitIcon },
  ]

  const ACTIVE_RULES: Record<string, (p: string) => boolean> = {
    home: p => p === '/',
    traces: p => p === '/traces' || p.startsWith('/trace/'),
    metrics: p => p === '/metrics' || p.startsWith('/metrics/'),
    logs: p => p === '/logs' || p.startsWith('/logs/'),
  }

  export function isNavItemActive(itemId: string, path: string): boolean {
    return (ACTIVE_RULES[itemId] ?? (() => false))(path)
  }
</script>

<script lang="ts">
  import { router } from 'tinro5'

  type Props = {
    collapsed?: boolean
  }
  let { collapsed = false }: Props = $props()

  let currentPath = $state(router.path ?? '/')
  $effect(() => {
    const unsubscribe = router.subscribe(route => {
      currentPath = route.path
    })
    return unsubscribe
  })

  const goto = (path: string) => router.goto(path)
</script>

{#if collapsed}
  <nav class="drawer-nav-tabs drawer-nav-tabs--collapsed" aria-label="Primary">
    {#each NAV_ITEMS as item (item.id)}
      {@const active = isNavItemActive(item.id, currentPath)}
      {@const Icon = item.icon}
      <button
        type="button"
        class="drawer-tab drawer-tab--icon-only tooltip tooltip-right {active
          ? 'drawer-tab--active'
          : 'drawer-tab--inactive'}"
        data-tip={item.label}
        aria-current={active ? 'page' : undefined}
        aria-label={item.label}
        onclick={() => goto(item.path)}
      >
        <Icon class="h-[17px] w-[17px] shrink-0" aria-hidden="true" />
      </button>
    {/each}
  </nav>
{:else}
  <nav class="drawer-nav-tabs drawer-nav-tabs--expanded" aria-label="Primary">
    {#each NAV_ITEMS as item (item.id)}
      {@const active = isNavItemActive(item.id, currentPath)}
      {@const Icon = item.icon}
      <button
        type="button"
        class="drawer-tab {active
          ? 'drawer-tab--active'
          : 'drawer-tab--inactive'}"
        aria-current={active ? 'page' : undefined}
        onclick={() => goto(item.path)}
      >
        <Icon class="h-[15px] w-[15px] shrink-0" aria-hidden="true" />
        <span class="truncate">{item.label}</span>
      </button>
    {/each}
  </nav>
{/if}

<style lang="postcss">
  @reference "../app.css";

  .drawer-nav-tabs--collapsed {
    @apply flex flex-col items-center gap-1.5;
  }

  .drawer-nav-tabs--expanded {
    @apply flex items-center gap-1;
  }

  .drawer-nav-tabs--expanded :global(.drawer-tab:not(.drawer-tab--icon-only)) {
    @apply px-3 gap-1 text-xs;
  }
</style>
