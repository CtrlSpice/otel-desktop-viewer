<script lang="ts">
  import type { Snippet } from 'svelte'
  import { ArrowDownIcon } from '@/icons'

  type Props = {
    label: string
    /** Optional snippet replacing the default label text (for icons, rich content). */
    heading?: Snippet
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
    badge,
    count,
    open = $bindable(true),
    last = false,
    children,
  }: Props = $props()
</script>

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

<style lang="postcss">
  @reference "../app.css";

  .field-group {
    @apply border-b-0;
  }

  .field-group__content {
    @apply px-2 pb-2 pt-0;
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
