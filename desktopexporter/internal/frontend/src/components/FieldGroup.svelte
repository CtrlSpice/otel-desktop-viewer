<script lang="ts">
  import type { Snippet } from 'svelte'
  import { ArrowDownIcon } from '@/icons'

  type Props = {
    label: string
    /** Optional snippet replacing the default label text (for icons, rich content). */
    heading?: Snippet
    /** Header row action (e.g. nav link). Uses a separate expand control — not nested in summary. */
    headerAction?: Snippet
    /** Optional badge before the count (e.g. event offset). */
    badge?: string
    count?: number
    open?: boolean
    /** Suppress the trailing separator (last group in a list). */
    last?: boolean
    children: Snippet
  }

  let {
    label,
    heading,
    headerAction,
    badge,
    count,
    open = $bindable(true),
    last = false,
    children,
  }: Props = $props()
</script>

{#if headerAction}
  <div class="field-group" class:field-group--open={open}>
    <div class="field-group__header-row">
      {@render headerAction()}
      <button
        type="button"
        class="field-group__caret-btn"
        aria-expanded={open}
        aria-label="{open ? 'Collapse' : 'Expand'} {label}"
        onclick={() => (open = !open)}
      >
        <ArrowDownIcon class="field-group__caret" aria-hidden="true" />
      </button>
    </div>
    {#if open}
      <div class="field-group__content">
        {@render children()}
      </div>
    {/if}
    {#if !last}
      <div class="separator" aria-hidden="true"></div>
    {/if}
  </div>
{:else}
  <details
    class="field-group"
    {open}
    ontoggle={e => (open = (e.currentTarget as HTMLDetailsElement).open)}
  >
    <summary class="field-group__heading">
      {#if heading}
        {@render heading()}
      {:else}
        <span>{label}</span>
      {/if}
      {#if badge}
        <span class="badge-count">{badge}</span>
      {/if}
      {#if count !== undefined}
        <span class="badge-count">{count}</span>
      {/if}
      <ArrowDownIcon class="field-group__caret" aria-hidden="true" />
    </summary>
    <div class="field-group__content">
      {@render children()}
    </div>
    {#if !last}
      <div class="separator" aria-hidden="true"></div>
    {/if}
  </details>
{/if}

<style lang="postcss">
  @reference "../app.css";

  .field-group {
    @apply border-b-0;
  }

  .field-group__content {
    @apply px-2 pb-2 pt-0;
  }

  .field-group__header-row {
    @apply flex items-center gap-1.5 px-2 py-1.5;
  }

  .field-group__header-row :global(.field-group__caret) {
    @apply h-3.5 w-3.5 shrink-0 transition-transform duration-150;
    color: var(--color-muted);
    transform: rotate(-90deg);
  }

  .field-group--open .field-group__header-row :global(.field-group__caret) {
    transform: rotate(0deg);
  }

  .field-group__caret-btn {
    @apply btn btn-ghost btn-square btn-xs shrink-0 border-transparent shadow-none;
    color: var(--color-muted);
  }

  .field-group__caret-btn:hover {
    @apply bg-base-200/80 text-base-content;
  }

  .field-group__heading {
    @apply cursor-pointer select-none list-none px-3 py-1.5 text-sm font-medium flex items-center gap-2;
    color: var(--color-subtle);
  }

  .field-group__heading::marker,
  .field-group__heading::-webkit-details-marker {
    display: none;
  }

  .field-group__heading :global(.field-group__caret) {
    @apply ml-auto h-3.5 w-3.5 transition-transform duration-150;
    color: var(--color-muted);
    transform: rotate(-90deg);
  }

  .field-group[open] > .field-group__heading :global(.field-group__caret) {
    transform: rotate(0deg);
  }
</style>
