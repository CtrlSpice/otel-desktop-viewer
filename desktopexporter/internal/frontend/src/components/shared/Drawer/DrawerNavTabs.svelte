<script module lang="ts">
  import type { Component } from 'svelte'
  import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/svelte'
  import {
    BarChartHorizontalIcon,
    ChartHistogramIcon,
    Home12Icon,
    TestTube01Icon,
  } from '@hugeicons/core-free-icons'
  import { LogIcon } from '@/icons'
  import type { SignalName } from '@/route'

  type NavItemID = 'home' | 'evals' | SignalName

  export const EVALS_NAV: NavItem<'evals'> = {
    id: 'evals',
    label: 'Evals',
    path: '/evals',
    iconType: 'hugeicon',
    icon: TestTube01Icon,
  }

  type NavItemBase<ID extends NavItemID> = {
    id: ID
    label: string
    path: string
  }

  export type NavItem<ID extends NavItemID = NavItemID> = NavItemBase<ID> &
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

  export const NAV_ITEMS: NavItem<SignalName>[] = [
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

  const ACTIVE_RULES = {
    home: (path: string) => path === '/',
    traces: (path: string) => path === '/traces' || path.startsWith('/traces/'),
    metrics: (path: string) =>
      path === '/metrics' || path.startsWith('/metrics/'),
    logs: (path: string) => path === '/logs' || path.startsWith('/logs/'),
    evals: (path: string) => path === '/evals' || path.startsWith('/evals/'),
  } satisfies Record<NavItemID, (path: string) => boolean>

  function isNavItemID(itemID: string): itemID is NavItemID {
    switch (itemID) {
      case 'home':
      case 'traces':
      case 'metrics':
      case 'logs':
      case 'evals':
        return true
      default:
        return false
    }
  }

  export function isNavItemActive(itemID: string, path: string): boolean {
    return isNavItemID(itemID) ? ACTIVE_RULES[itemID](path) : false
  }
</script>

<script lang="ts">
  import {
    isPlainLeftClick,
    navigate,
    navigateToSignal,
    signalHref,
  } from '@/route'
  import { evalsEnabled } from '@/state/features'
  import { getRouteContext } from '@/contexts/route-context.svelte'

  type Props = {
    collapsed?: boolean
  }
  let { collapsed = false }: Props = $props()

  const routeContext = getRouteContext()
  const items = $derived($evalsEnabled ? [...NAV_ITEMS, EVALS_NAV] : NAV_ITEMS)

  // NAV_ITEMS are all signal tabs, so navigate through the helper to carry the
  // active time window across signals.
  // Switching signal is navigational: push so back returns to the prior signal.
  function goto(event: MouseEvent, item: NavItem<SignalName | 'evals'>) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    if (item.id === 'evals') navigate('/evals')
    else navigateToSignal(item.id)
  }
</script>

{#if collapsed}
  <nav class="drawer-nav-tabs drawer-nav-tabs--collapsed" aria-label="Primary">
    {#each items as item (item.id)}
      {@const active = isNavItemActive(item.id, routeContext.route.path)}
      <a
        href={item.id === 'evals'
          ? '/evals'
          : signalHref(item.id, routeContext.route.query)}
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
    {#each items as item (item.id)}
      {@const active = isNavItemActive(item.id, routeContext.route.path)}
      <a
        href={item.id === 'evals'
          ? '/evals'
          : signalHref(item.id, routeContext.route.query)}
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
