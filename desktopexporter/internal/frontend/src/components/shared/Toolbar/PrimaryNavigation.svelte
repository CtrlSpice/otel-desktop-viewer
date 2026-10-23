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

  export const HOME_NAV: NavItem<'home'> = {
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
  import type { NavigationCounts } from '@/contexts/navigation-counts.svelte'

  let { counts = {} }: { counts?: NavigationCounts } = $props()
  const countID = $props.id()
  const routeContext = getRouteContext()
  const items = $derived(
    $evalsEnabled
      ? [HOME_NAV, ...NAV_ITEMS, EVALS_NAV]
      : [HOME_NAV, ...NAV_ITEMS]
  )

  // NAV_ITEMS are all signal tabs, so navigate through the helper to carry the
  // active time window across signals.
  // Switching signal is navigational: push so back returns to the prior signal.
  function goto(id: string, event?: MouseEvent) {
    if (!event || !isPlainLeftClick(event)) return
    const item = items.find(item => item.id === id)
    if (!item) return
    event.preventDefault()
    if (item.id === 'home' || item.id === 'evals') navigate(item.path)
    else navigateToSignal(item.id)
  }
</script>

<nav
  aria-label="Primary"
  class="primary-navigation tabs tabs-border flex-nowrap overflow-x-auto"
>
  {#each items as item (item.id)}
    {@const active = isNavItemActive(item.id, routeContext.route.path)}
    {@const itemCount = counts[item.id]}
    <a
      href={item.id === 'home' || item.id === 'evals'
        ? item.path
        : signalHref(item.id, routeContext.route.query)}
      class="tab shrink-0 flex-nowrap items-center gap-2 px-4 text-sm"
      class:tab-active={active}
      aria-current={active ? 'page' : undefined}
      aria-label={item.id === 'evals' ? 'Experimental Evals' : item.label}
      title={item.id === 'evals' ? 'Test Feature Please Ignore' : undefined}
      aria-describedby={itemCount !== undefined
        ? `${countID}-${item.id}`
        : undefined}
      onclick={event => goto(item.id, event)}
    >
      {#if item.iconType === 'hugeicon'}
        <HugeiconsIcon
          icon={item.icon}
          size={16}
          strokeWidth={1.5}
          class="shrink-0 {active ? 'text-primary' : ''}"
        />
      {:else}
        <item.icon class="size-4 shrink-0 {active ? 'text-primary' : ''}" />
      {/if}
      <span>{item.label}</span>
      {#if item.id === 'evals'}<span>[experimental]</span>{/if}
      <span
        id={`${countID}-${item.id}`}
        class="primary-navigation__count shrink-0 text-sm"
        class:invisible={itemCount === undefined}
        aria-hidden={itemCount === undefined}
        ><span class="sr-only"
          >{item.id === 'evals'
            ? 'Stored experiments:'
            : item.id === 'home'
              ? 'Stored services:'
              : `Stored ${item.id}:`}</span
        >{' '}{itemCount === undefined ? '' : `[${itemCount}]`}</span
      >
    </a>
  {/each}
</nav>

<style>
  .primary-navigation {
    gap: var(--header-gap);
  }

  .primary-navigation__count {
    min-width: var(--header-counter-min-width);
    color: var(--color-base-content);
    font-weight: 400;
    font-variant-numeric: tabular-nums;
  }

  .tab {
    color: var(--color-base-content);
    background: transparent;
    transition: background-color var(--motion-feedback-duration);
  }

  .tab::before {
    display: none;
  }

  .tab[aria-current='page'] {
    background: var(--item-selected-bg);
  }

  @media (hover: hover) {
    .tab:hover {
      background: var(--item-hover-bg);
    }
  }
</style>
