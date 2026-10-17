<script lang="ts">
  import type { Snippet } from 'svelte'
  import PrimaryNavigation from './PrimaryNavigation.svelte'
  import SearchBar from '../Search/SearchBar.svelte'
  import PaneHeader from '../PaneHeader.svelte'
  import ThemeToggle from '../ThemeToggle.svelte'

  let {
    search,
    controls,
    onRefresh,
    refreshPulse = false,
    refreshAsideTip = '',
  }: {
    search?: Snippet
    controls?: Snippet
    onRefresh?: () => void
    refreshPulse?: boolean
    refreshAsideTip?: string
  } = $props()
</script>

<header
  class="shrink-0 bg-base-100 text-sm"
  style="--pane-header-bg: var(--color-base-100)"
  aria-label="Workspace navigation and search"
>
  <PaneHeader mode="toolbar" rounded={false} ariaLabel="Workspace controls">
    {#snippet right()}
      <div class="flex w-full min-w-0 flex-wrap items-center gap-x-4 gap-y-2">
        <div class="min-w-0 max-w-full flex-1 md:mr-4 md:flex-none">
          <PrimaryNavigation />
        </div>
        <div
          class="flex min-w-0 flex-1 items-center justify-end {search
            ? 'basis-full md:basis-48'
            : ''}"
        >
          <SearchBar
            {search}
            {controls}
            {onRefresh}
            {refreshPulse}
            {refreshAsideTip}
          />
        </div>
        <div class="ml-auto flex shrink-0 items-center gap-2">
          <ThemeToggle
            class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
          />
        </div>
      </div>
    {/snippet}
  </PaneHeader>
</header>
