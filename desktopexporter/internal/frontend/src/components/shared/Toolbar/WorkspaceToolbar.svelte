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
  class="relative z-30 shrink-0 bg-base-100 text-sm"
  style="--pane-header-bg: var(--color-base-100)"
  aria-label="Workspace navigation and search"
>
  <PaneHeader mode="toolbar" rounded={false} ariaLabel="Workspace controls">
    {#snippet right()}
      <div
        class="workspace-toolbar"
        class:workspace-toolbar--details={!!navigationTrailing}
      >
        <div class="workspace-toolbar__leading">
          {@render navigationLeading?.()}
        </div>
        <div class="workspace-toolbar__centre">
          <div class="workspace-toolbar__navigation">
            <PrimaryNavigation counts={totals?.counts} />
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
    --workspace-toolbar-edge-width: 9.5rem;
    @apply grid w-full min-w-0 items-center gap-x-4 gap-y-2;
    grid-template-columns:
      var(--workspace-toolbar-edge-width) minmax(0, 1fr)
      var(--workspace-toolbar-edge-width);
  }

  .workspace-toolbar--details {
    --workspace-toolbar-edge-width: 12rem;
  }

  .workspace-toolbar__leading,
  .workspace-toolbar__actions {
    @apply flex items-center;
    grid-row: 1;
  }

  .workspace-toolbar__leading {
    justify-content: flex-start;
    grid-column: 1;
  }

  .workspace-toolbar__actions {
    @apply justify-end gap-2;
    grid-column: 3;
  }

  .workspace-toolbar__centre {
    display: contents;
  }

  .workspace-toolbar__navigation {
    @apply flex min-w-0 justify-center;
    grid-column: 2;
    grid-row: 1;
  }

  .workspace-toolbar__search {
    @apply flex min-w-0 w-full justify-self-end;
    max-width: 28rem;
    grid-column: 1 / -1;
    grid-row: 2;
  }

  @media (min-width: 96rem) {
    .workspace-toolbar {
      grid-template-columns:
        var(--workspace-toolbar-edge-width) minmax(0, 1fr)
        max-content minmax(0, 1fr) var(--workspace-toolbar-edge-width);
    }
    .workspace-toolbar__navigation {
      grid-column: 3;
    }
    .workspace-toolbar__search {
      grid-column: 4;
      grid-row: 1;
    }
    .workspace-toolbar__actions {
      grid-column: 5;
    }
  }
</style>
