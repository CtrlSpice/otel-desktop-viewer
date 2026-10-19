<script lang="ts">
  import type { Snippet } from 'svelte'
  import PrimaryNavigation from './PrimaryNavigation.svelte'
  import SearchBar from '../Search/SearchBar.svelte'
  import PaneHeader from '../PaneHeader.svelte'
  import ThemeToggle from '../ThemeToggle.svelte'
  import PalettePicker from '../PalettePicker.svelte'

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
  class="shrink-0 bg-base-100 text-sm"
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
            <PrimaryNavigation />
          </div>
          <div class="workspace-toolbar__search">
            <SearchBar
              {search}
              {controls}
              {onRefresh}
              {refreshPulse}
              {refreshAsideTip}
            />
          </div>
        </div>
        <div class="workspace-toolbar__actions">
          <PalettePicker />
          <ThemeToggle
            class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content tooltip tooltip-bottom"
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
    grid-template-columns: 5.5rem minmax(0, 1fr) 5.5rem;
  }

  .workspace-toolbar--details {
    grid-template-columns: 8rem minmax(0, 1fr) 8rem;
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
    @apply flex min-w-0;
    grid-column: 1 / -1;
    grid-row: 2;
  }

  @media (min-width: 64rem) {
    .workspace-toolbar__centre {
      @apply grid w-full min-w-0 items-center gap-8;
      grid-column: 2;
      grid-row: 1;
      grid-template-columns: max-content minmax(0, 1fr);
      max-width: 76rem;
      justify-self: center;
    }

    .workspace-toolbar__navigation,
    .workspace-toolbar__search {
      grid-column: auto;
      grid-row: auto;
    }
  }
</style>
