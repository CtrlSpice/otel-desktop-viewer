<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { LiveStreaming02Icon, RefreshIcon } from '@hugeicons/core-free-icons'
  import PrimaryNavigation from './PrimaryNavigation.svelte'
  import DateTimeFilter from './DateTimeFilter.svelte'
  import SearchBar from '../Search/SearchBar.svelte'
  import PaneHeader from '../PaneHeader.svelte'
  import ThemeToggle from '../ThemeToggle.svelte'
  import IconButton from '../IconButton.svelte'
  import { getNavigationCounts } from '@/contexts/navigation-counts.svelte'
  import { getDataChanges } from '@/contexts/data-changes.svelte'

  const totals = getNavigationCounts()
  const changes = getDataChanges()

  let {
    search,
    controls,
    navigationLeading,
    navigationTrailing,
    onRefresh,
    refreshPulse = false,
    refreshAsideTip = '',
  }: {
    search?: Snippet
    controls?: Snippet
    navigationLeading?: Snippet
    navigationTrailing?: Snippet
    onRefresh?: () => void
    refreshPulse?: boolean
    refreshAsideTip?: string
  } = $props()

  const refreshLabel = $derived(
    refreshPulse && refreshAsideTip ? `Refresh: ${refreshAsideTip}` : 'Refresh'
  )
</script>

<header
  class="relative z-30 shrink-0 bg-base-100 text-sm"
  style="--pane-header-bg: var(--color-base-100)"
  aria-label="Workspace navigation and search"
>
  <PaneHeader mode="toolbar" rounded={false} ariaLabel="Workspace controls">
    {#snippet right()}
      <div class="workspace-toolbar">
        <div class="workspace-toolbar__identity">
          <div class="workspace-toolbar__leading">
            {@render navigationLeading?.()}
          </div>
          <div class="workspace-toolbar__navigation">
            <PrimaryNavigation counts={totals?.counts} />
          </div>
        </div>
        <div
          class="workspace-toolbar__data-controls"
          role="group"
          aria-label="Data controls"
        >
          {#if controls}{@render controls()}{:else}<DateTimeFilter />{/if}
          {#if changes && onRefresh}
            <IconButton
              label="Live"
              class="shrink-0"
              aria-pressed={changes.live}
              tooltip={changes.live
                ? changes.connected
                  ? 'Pause Live updates'
                  : 'Live · reconnecting'
                : 'Start Live updates'}
              onclick={() => changes.setLive(!changes.live)}
            >
              <HugeiconsIcon
                icon={LiveStreaming02Icon}
                size={17}
                strokeWidth={1.5}
                aria-hidden="true"
              />
            </IconButton>
          {/if}
          {#if onRefresh || search}
            <IconButton
              label={refreshLabel}
              data-selected={refreshPulse || undefined}
              disabled={!onRefresh}
              onclick={onRefresh}
            >
              <HugeiconsIcon
                icon={RefreshIcon}
                size={17}
                strokeWidth={1.5}
                aria-hidden="true"
              />
            </IconButton>
            {#if refreshPulse}
              <span class="sr-only" role="status"
                >{refreshAsideTip || 'New data'}</span
              >
            {/if}
          {/if}
          <div class="workspace-toolbar__search">
            <SearchBar {search} />
          </div>
        </div>
        <div class="workspace-toolbar__actions">
          <ThemeToggle
            class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-left"
          />
          {@render navigationTrailing?.()}
        </div>
      </div>
    {/snippet}
  </PaneHeader>
</header>

<style lang="postcss">
  @reference "../../../app.css";

  .workspace-toolbar {
    @apply grid w-full min-w-0 items-center;
    grid-template-columns: minmax(0, 1fr) max-content max-content;
    column-gap: var(--layout-gap);
    row-gap: var(--header-gap);
  }

  .workspace-toolbar__data-controls {
    @apply flex min-w-0 items-center;
    gap: var(--header-gap);
  }

  .workspace-toolbar__identity {
    @apply flex min-w-0 items-center gap-2;
  }

  .workspace-toolbar__leading,
  .workspace-toolbar__actions {
    @apply flex items-center;
  }

  .workspace-toolbar__leading {
    flex-shrink: 0;
  }

  .workspace-toolbar__leading:empty {
    display: none;
  }

  .workspace-toolbar__actions {
    @apply flex-wrap justify-end gap-2;
  }

  .workspace-toolbar__navigation {
    @apply flex min-w-0 justify-start;
  }

  .workspace-toolbar__search {
    @apply flex min-w-0;
    width: clamp(16rem, 24vw, 28rem);
  }

  @media (max-width: 79.999rem) {
    .workspace-toolbar {
      grid-template-columns: minmax(0, 1fr) max-content;
    }
    .workspace-toolbar__identity {
      grid-column: 1 / -1;
    }
    .workspace-toolbar__search {
      flex: 1;
      width: 0;
    }
  }

  @media (max-width: 47.999rem) {
    .workspace-toolbar {
      grid-template-columns: minmax(0, 1fr);
    }
    .workspace-toolbar__data-controls {
      flex-wrap: wrap;
    }
    .workspace-toolbar__search {
      flex-basis: 100%;
    }
    .workspace-toolbar__actions {
      grid-column: 1 / -1;
    }
  }
</style>
