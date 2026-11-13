<script lang="ts">
  import type { Snippet } from 'svelte'

  let { search }: { search?: Snippet } = $props()
</script>

<div
  class="search-bar flex w-full min-w-0 items-center rounded-full text-sm text-base-content {search
    ? 'search-bar--editable input-surface'
    : ''}"
  role={search ? 'search' : undefined}
  aria-label={search ? 'Workspace search' : undefined}
>
  <div class="search-bar__query flex min-w-0 flex-1 items-center rounded-full">
    <div
      class="search-bar__field min-w-0 flex-1"
      style="--typed-field-bg: transparent"
    >
      {#if search}{@render search()}{/if}
    </div>
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .search-bar--editable {
    padding-inline: 0.25rem;
  }

  .search-bar__query {
    height: 2rem;
  }

  .search-bar__field {
    --search-editor-row-height: 2rem;
    display: flex;
    align-items: center;
    width: 0;
    height: 2rem;
    overflow: visible;
  }

  .search-bar__query .search-bar__field :global(.typed-field) {
    border: 0;
    height: 2rem;
    min-height: 2rem;
    padding-block: 0;
    padding-left: 0.5rem;
  }

  .search-bar__field :global(.typed-field::placeholder),
  .search-bar__field :global(.cm-placeholder) {
    color: var(--color-subtle);
  }

  .search-bar__field :global(.typed-field:focus-visible),
  .search-bar__field :global(.search-editor-container:focus-within) {
    outline: none;
  }
</style>
