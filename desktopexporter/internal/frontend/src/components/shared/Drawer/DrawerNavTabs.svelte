<script module lang="ts">
  import type { Component } from 'svelte'
  import {
    HomeIcon,
    BarChartHorizontalIcon,
    ChartHistogramIcon,
    LogIcon,
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
    { id: 'logs', label: 'Logs', path: '/logs', icon: LogIcon },
  ]

  const ACTIVE_RULES: Record<string, (p: string) => boolean> = {
    home: p => p === '/',
    traces: p => p === '/traces' || p.startsWith('/traces/'),
    metrics: p => p === '/metrics' || p.startsWith('/metrics/'),
    logs: p => p === '/logs' || p.startsWith('/logs/'),
  }

  export function isNavItemActive(itemId: string, path: string): boolean {
    return (ACTIVE_RULES[itemId] ?? (() => false))(path)
  }
</script>

<script lang="ts">
  import { navigateToSignal, type SignalName } from '@/route'
  import { getRouteContext } from '@/contexts/route-context.svelte'

  type Props = {
    collapsed?: boolean
  }
  let { collapsed = false }: Props = $props()

  const routeContext = getRouteContext()

  // NAV_ITEMS are all signal tabs, so navigate through the helper to carry the
  // active time window across signals.
  // Switching signal is navigational: push so back returns to the prior signal.
  const goto = (item: NavItem) =>
    navigateToSignal(item.id as SignalName, { replace: false })
</script>

{#if collapsed}
  <nav class="drawer-nav-tabs drawer-nav-tabs--collapsed" aria-label="Primary">
    {#each NAV_ITEMS as item (item.id)}
      {@const active = isNavItemActive(item.id, routeContext.route.path)}
      {@const Icon = item.icon}
      <button
        type="button"
        class="drawer-header-btn tooltip tooltip-right {active
          ? 'drawer-header-btn--active'
          : 'drawer-header-btn--inactive'}"
        data-tip={item.label}
        aria-current={active ? 'page' : undefined}
        aria-label={item.label}
        onclick={() => goto(item)}
      >
        <Icon class="h-[17px] w-[17px] shrink-0" aria-hidden="true" />
      </button>
    {/each}
  </nav>
{:else}
  <nav class="drawer-nav-tabs drawer-nav-tabs--expanded" aria-label="Primary">
    {#each NAV_ITEMS as item (item.id)}
      {@const active = isNavItemActive(item.id, routeContext.route.path)}
      {@const Icon = item.icon}
      <button
        type="button"
        class="drawer-tab {active
          ? 'drawer-tab--active'
          : 'drawer-tab--inactive'}"
        aria-current={active ? 'page' : undefined}
        onclick={() => goto(item)}
      >
        <Icon class="h-[15px] w-[15px] shrink-0" aria-hidden="true" />
        <span class="truncate">{item.label}</span>
      </button>
    {/each}
  </nav>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .drawer-nav-tabs--collapsed {
    @apply flex flex-col items-center gap-2;
  }

  .drawer-nav-tabs--expanded {
    @apply flex items-center gap-1;
  }

  .drawer-nav-tabs--expanded :global(.drawer-tab:not(.drawer-tab--icon-only)) {
    @apply px-3 gap-1 text-xs;
  }
</style>
