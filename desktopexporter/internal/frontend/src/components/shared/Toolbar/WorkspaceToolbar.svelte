<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    Home12Icon,
    SidebarLeft01Icon,
    RefreshIcon,
  } from '@hugeicons/core-free-icons'
  import PrimaryNavigation from './PrimaryNavigation.svelte'
  import DateTimeFilter from './DateTimeFilter.svelte'
  import PaneHeader from '../PaneHeader.svelte'
  import ThemeToggle from '../ThemeToggle.svelte'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { isPlainLeftClick, navigate } from '@/route'

  let {
    drawerOpen,
    drawerPanelID,
    hasDrawer = true,
    onToggleDrawer,
    search,
    controls,
    onRefresh,
    refreshPulse = false,
    refreshAsideTip = '',
  }: {
    drawerOpen: boolean
    drawerPanelID: string
    hasDrawer?: boolean
    onToggleDrawer: () => void
    search?: Snippet
    controls?: Snippet
    onRefresh?: () => void
    refreshPulse?: boolean
    refreshAsideTip?: string
  } = $props()
  const route = getRouteContext()
  const refreshLabel = $derived(
    refreshPulse && refreshAsideTip ? `Refresh — ${refreshAsideTip}` : 'Refresh'
  )
  function goHome(event: MouseEvent) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    navigate('/')
  }
</script>

<header class="shrink-0" aria-label="Workspace navigation and search">
  <PaneHeader mode="toolbar" rounded={false} ariaLabel="Workspace controls">
    {#snippet right()}
      <div class="flex w-full min-w-0 flex-wrap items-center gap-2">
        <a
          href="/"
          onclick={goHome}
          aria-label="Home"
          aria-current={route.route.path === '/' ? 'page' : undefined}
          class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom"
          data-tip="Home"
          ><HugeiconsIcon
            icon={Home12Icon}
            size="1em"
            strokeWidth={1.5}
            class="h-[17px] w-[17px] shrink-0"
            aria-hidden="true"
          /></a
        >
        <div class="min-w-0 flex-1 md:w-96 md:flex-none">
          <PrimaryNavigation />
        </div>
        {#if search}<div class="min-w-0 basis-full md:basis-48 md:flex-1">
            {@render search()}
          </div>{:else}<div class="flex-1"></div>{/if}
        <div class="ml-auto flex shrink-0 items-center gap-2">
          {#if controls}{@render controls()}{:else}<DateTimeFilter
              class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom"
            />{/if}
          {#if onRefresh}
            <button
              type="button"
              class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom"
              aria-label={refreshLabel}
              data-tip={refreshLabel}
              onclick={onRefresh}
              ><HugeiconsIcon
                icon={RefreshIcon}
                size="1em"
                strokeWidth={1.5}
                class="h-[17px] w-[17px] shrink-0"
                aria-hidden="true"
              /></button
            >
            {#if refreshPulse}<span class="badge-count" role="status"
                >{refreshAsideTip || 'New data'}</span
              >{/if}
          {/if}
          <ThemeToggle
            class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom"
          />
          {#if hasDrawer}<button
              type="button"
              class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom"
              aria-label={drawerOpen ? 'Collapse sidebar' : 'Open sidebar'}
              data-tip={drawerOpen ? 'Collapse sidebar' : 'Open sidebar'}
              aria-expanded={drawerOpen}
              aria-controls={drawerPanelID}
              onclick={onToggleDrawer}
              ><HugeiconsIcon
                icon={SidebarLeft01Icon}
                size="1em"
                strokeWidth={1.5}
                class="h-[17px] w-[17px] shrink-0"
                aria-hidden="true"
              /></button
            >{/if}
        </div>
      </div>
    {/snippet}
  </PaneHeader>
</header>
