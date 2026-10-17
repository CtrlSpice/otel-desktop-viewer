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
  import PaneHeader from '../PaneHeader.svelte'

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

{#snippet homeIcon()}<HugeiconsIcon
    icon={Home12Icon}
    size="1em"
    strokeWidth={1.5}
    class="h-[15px] w-[15px] shrink-0"
  />{/snippet}
{#snippet tracesIcon()}<HugeiconsIcon
    icon={BarChartHorizontalIcon}
    size="1em"
    strokeWidth={1.5}
    class="h-[15px] w-[15px] shrink-0"
  />{/snippet}
{#snippet metricsIcon()}<HugeiconsIcon
    icon={ChartHistogramIcon}
    size="1em"
    strokeWidth={1.5}
    class="h-[15px] w-[15px] shrink-0"
  />{/snippet}
{#snippet logsIcon()}<LogIcon class="h-[15px] w-[15px] shrink-0" />{/snippet}
{#snippet evalsIcon()}<HugeiconsIcon
    icon={TestTube01Icon}
    size="1em"
    strokeWidth={1.5}
    class="h-[15px] w-[15px] shrink-0"
  />{/snippet}

<PaneHeader
  mode="tabs"
  navigation
  rounded={false}
  ariaLabel="Primary"
  activeID={items.find(item =>
    isNavItemActive(item.id, routeContext.route.path)
  )?.id ?? ''}
  onSelect={goto}
  tabs={items.map(item => ({
    id: item.id,
    label: item.label,
    href:
      item.id === 'home' || item.id === 'evals'
        ? item.path
        : signalHref(item.id, routeContext.route.query),
    icon:
      item.id === 'home'
        ? homeIcon
        : item.id === 'traces'
          ? tracesIcon
          : item.id === 'metrics'
            ? metricsIcon
            : item.id === 'logs'
              ? logsIcon
              : evalsIcon,
  }))}
/>
