<script module lang="ts">
  import type { Component } from 'svelte'
  import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/svelte'
  import BarChartHorizontalIcon from '@hugeicons/core-free-icons/BarChartHorizontalIcon'
  import ChartHistogramIcon from '@hugeicons/core-free-icons/ChartHistogramIcon'
  import Home12Icon from '@hugeicons/core-free-icons/Home12Icon'
  import { LogIcon } from '@/icons'

  type NavItemBase = {
    id: string
    label: string
    path: string
  }

  export type NavItem = NavItemBase &
    (
      | { iconType: 'hugeicon'; icon: IconSvgElement }
      | { iconType: 'component'; icon: Component }
    )

  export const HOME_NAV: NavItem = {
    id: 'home',
    label: 'Home',
    path: '/',
    iconType: 'hugeicon',
    icon: Home12Icon,
  }

  export const NAV_ITEMS: NavItem[] = [
    {
      id: 'traces',
      label: 'Traces',
      path: '/traces',
      iconType: 'hugeicon',
      icon: BarChartHorizontalIcon,
    },
    {
      id: 'metrics',
      label: 'Metrics',
      path: '/metrics',
      iconType: 'hugeicon',
      icon: ChartHistogramIcon,
    },
    {
      id: 'logs',
      label: 'Logs',
      path: '/logs',
      iconType: 'component',
      icon: LogIcon,
    },
  ]

  const ACTIVE_RULES: Record<string, (p: string) => boolean> = {
    home: p => p === '/',
    traces: p => p === '/traces' || p.startsWith('/traces/'),
    metrics: p => p === '/metrics' || p.startsWith('/metrics/'),
    logs: p => p === '/logs' || p.startsWith('/logs/'),
  }

  export function isNavItemActive(itemID: string, path: string): boolean {
    return (ACTIVE_RULES[itemID] ?? (() => false))(path)
  }
</script>

<script lang="ts">
  import {
    isPlainLeftClick,
    navigateToSignal,
    signalHref,
    type SignalName,
  } from '@/route'
  import { getRouteContext } from '@/contexts/route-context.svelte'

  type Props = {
    collapsed?: boolean
  }
  let { collapsed = false }: Props = $props()

  const routeContext = getRouteContext()

  // NAV_ITEMS are all signal tabs, so navigate through the helper to carry the
  // active time window across signals.
  // Switching signal is navigational: push so back returns to the prior signal.
  function goto(event: MouseEvent, item: NavItem) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    navigateToSignal(item.id as SignalName)
  }
</script>

{#if collapsed}
  <nav class="drawer-nav-tabs drawer-nav-tabs--collapsed" aria-label="Primary">
    {#each NAV_ITEMS as item (item.id)}
      {@const active = isNavItemActive(item.id, routeContext.route.path)}
      <a
        href={signalHref(item.id as SignalName, routeContext.route.query)}
        class="drawer-header-btn tooltip tooltip-right {active
          ? 'drawer-header-btn--active'
          : 'drawer-header-btn--inactive'}"
        data-tip={item.label}
        aria-current={active ? 'page' : undefined}
        aria-label={item.label}
        onclick={event => goto(event, item)}
      >
        {#if item.iconType === 'component'}
          {@const Icon = item.icon}
          <Icon class="h-[17px] w-[17px] shrink-0" aria-hidden="true" />
        {:else}
          <HugeiconsIcon
            icon={item.icon}
            size="1em"
            strokeWidth={1.5}
            class="h-[17px] w-[17px] shrink-0"
            aria-hidden="true"
          />
        {/if}
      </a>
    {/each}
  </nav>
{:else}
  <nav class="drawer-nav-tabs drawer-nav-tabs--expanded" aria-label="Primary">
    {#each NAV_ITEMS as item (item.id)}
      {@const active = isNavItemActive(item.id, routeContext.route.path)}
      <a
        href={signalHref(item.id as SignalName, routeContext.route.query)}
        class="drawer-tab {active
          ? 'drawer-tab--active'
          : 'drawer-tab--inactive'}"
        aria-current={active ? 'page' : undefined}
        onclick={event => goto(event, item)}
      >
        {#if item.iconType === 'component'}
          {@const Icon = item.icon}
          <Icon class="h-[15px] w-[15px] shrink-0" aria-hidden="true" />
        {:else}
          <HugeiconsIcon
            icon={item.icon}
            size="1em"
            strokeWidth={1.5}
            class="h-[15px] w-[15px] shrink-0"
            aria-hidden="true"
          />
        {/if}
        <span class="truncate">{item.label}</span>
      </a>
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
