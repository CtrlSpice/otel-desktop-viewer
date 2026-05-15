<script lang="ts">
  import type { Snippet } from 'svelte'
  import { ArrowDownIcon } from '@/icons'

  type Props = {
    label: string
    count?: number
    open?: boolean
    children: Snippet
  }

  let { label, count, open = $bindable(true), children }: Props = $props()
</script>

<details class="field-group" {open}
  ontoggle={(e) => (open = (e.currentTarget as HTMLDetailsElement).open)}>
  <summary class="field-group__heading">
    <span>{label}</span>
    {#if count !== undefined}
      <span class="badge badge-xs badge-soft badge-neutral">{count}</span>
    {/if}
    <ArrowDownIcon class="field-group__caret" aria-hidden="true" />
  </summary>
  <div class="field-group__content">
    {@render children()}
  </div>
</details>

<style lang="postcss">
  @reference "../app.css";

  .field-group {
    @apply border-b border-base-300/30;
  }

  .field-group:last-child {
    @apply border-b-0;
  }

  .field-group__heading {
    @apply cursor-pointer select-none list-none px-3 py-2 text-sm font-medium flex items-center gap-2;
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
