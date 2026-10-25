<script lang="ts">
  import type { Snippet } from 'svelte'
  import PrimaryNavigation from './PrimaryNavigation.svelte'
  import SearchBar from '../Search/SearchBar.svelte'
  import PaneHeader from '../PaneHeader.svelte'
  import ThemeToggle from '../ThemeToggle.svelte'
  import { getNavigationCounts } from '@/contexts/navigation-counts.svelte'

  const totals = getNavigationCounts()

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
</script>

<header
  class="relative z-30 shrink-0 bg-base-300 text-sm"
  style="--pane-header-bg: var(--color-base-300)"
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
        <div class="workspace-toolbar__search">
          <SearchBar
            {search}
            {controls}
            onRefresh={onRefresh
              ? () => {
                  onRefresh()
                  void totals?.refresh()
                }
              : undefined}
            {refreshPulse}
            {refreshAsideTip}
          />
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
    @apply grid w-full min-w-0 items-center gap-x-4 gap-y-2;
    grid-template-columns:
      minmax(0, max-content) minmax(16rem, 1fr)
      max-content;
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
    @apply flex min-w-0 w-full;
  }

  @media (max-width: 79.999rem) {
    .workspace-toolbar {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .workspace-toolbar__identity {
      grid-column: 1 / -1;
    }
  }

  @media (max-width: 47.999rem) {
    .workspace-toolbar__search,
    .workspace-toolbar__actions {
      grid-column: 1 / -1;
    }
  }
</style>
