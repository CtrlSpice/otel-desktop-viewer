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

  export const HOME_NAV: NavItem = { id: 'home', label: 'Home', path: '/', icon: HomeIcon }

  export const NAV_ITEMS: NavItem[] = [
    { id: 'traces', label: 'Traces', path: '/traces', icon: BarChartHorizontalIcon },
    { id: 'metrics', label: 'Metrics', path: '/metrics', icon: ChartHistogramIcon },
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
        class="nav-button nav-button-icon-only tooltip tooltip-right {active
          ? 'nav-button-active'
          : 'nav-button-inactive'}"
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
  <div class="drawer-nav-tabs">
    <div
      role="tablist"
      class="tabs tabs-lift tabs-sm w-full"
      aria-label="Primary"
    >
      {#each NAV_ITEMS as item (item.id)}
        {@const active = isNavItemActive(item.id, currentPath)}
        {@const Icon = item.icon}
        <a
          role="tab"
          class="tab flex-1 gap-1 no-underline {active ? 'tab-active text-primary' : ''}"
          href={item.path}
          aria-current={active ? 'page' : undefined}
          onclick={(e) => {
            e.preventDefault()
            goto(item.path)
          }}
        >
          <Icon class="h-[13px] w-[13px] shrink-0" aria-hidden="true" />
          <span class="truncate">{item.label}</span>
        </a>
      {/each}
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../app.css";

  .drawer-nav-tabs--collapsed {
    @apply flex flex-col items-center gap-1.5;
  }

  .drawer-nav-tabs:not(.drawer-nav-tabs--collapsed) {
    @apply flex w-full min-w-0 flex-col;
  }
</style>
