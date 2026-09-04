<script lang="ts">
  import type { Snippet } from 'svelte'
  import { ArrowDownIcon } from '@/icons'

  type Props = {
    label: string
    /** Optional icon rendered before the heading. */
    icon?: Snippet
    /** Optional snippet replacing the default label text with rich content. */
    heading?: Snippet
    /** Header row action (e.g. nav link). Uses a separate expand control — not nested in summary. */
    headerAction?: Snippet
    /** Optional badge before the count (e.g. event offset). */
    badge?: string
    count?: number
    open?: boolean
    /** When set, parent owns `open` (e.g. a Set membership). */
    onOpenChange?: (open: boolean) => void
    children: Snippet
  }

  let {
    label,
    icon,
    heading,
    headerAction,
    badge,
    count,
    open = $bindable(true),
    onOpenChange,
    children,
  }: Props = $props()

  function setOpen(next: boolean) {
    if (onOpenChange) onOpenChange(next)
    else open = next
  }
</script>

{#snippet headingBody()}
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
{/snippet}

{#if headerAction}
  <div class="field-group" class:field-group--open={open}>
    <div class="field-group__header-row">
      {@render headerAction()}
      <button
        type="button"
        class="field-group__caret-btn tooltip tooltip-left"
        aria-expanded={open}
        aria-label="{open ? 'Collapse' : 'Expand'} {label}"
        data-tip="{open ? 'Collapse' : 'Expand'} {label}"
        onclick={() => setOpen(!open)}
      >
        <ArrowDownIcon class="field-group__caret" aria-hidden="true" />
      </button>
    </div>
    {#if open}
      <div class="field-group__content">
        {@render children()}
      </div>
    {/if}
  </div>
{:else}
  <details
    class="field-group"
    {open}
    ontoggle={e => setOpen((e.currentTarget as HTMLDetailsElement).open)}
  >
    <summary class="field-group__heading">
      {#if icon}
        {@render icon()}
        <span class="field-group__heading-content">
          {@render headingBody()}
        </span>
      {:else}
        {@render headingBody()}
      {/if}
      <ArrowDownIcon class="field-group__caret" aria-hidden="true" />
    </summary>
    <div class="field-group__content">
      {@render children()}
    </div>
  </details>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .field-group {
    @apply border-b-0;
    --fg-inline: var(--field-group-inline, 0.75rem);
    --fg-caret-size: 0.875rem;
  }

  /* Content aligns with heading inset (icon or label). */
  .field-group__content {
    padding-inline: var(--fg-inline);
    @apply pb-2 pt-0;
  }

  .field-group__header-row {
    padding-inline: var(--fg-inline);
    @apply flex items-center gap-1.5 py-1.5;
  }

  .field-group__header-row :global(.field-group__caret) {
    width: var(--fg-caret-size);
    height: var(--fg-caret-size);
    @apply shrink-0 transition-transform duration-150;
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
    padding-inline: var(--fg-inline);
    @apply cursor-pointer select-none list-none py-1.5 text-sm font-medium flex items-center gap-2;
    color: var(--color-subtle);
  }

  .field-group__heading-content {
    @apply flex min-w-0 flex-1 items-center gap-2;
  }

  .field-group__heading::marker,
  .field-group__heading::-webkit-details-marker {
    display: none;
  }

  .field-group__heading :global(.field-group__caret) {
    width: var(--fg-caret-size);
    height: var(--fg-caret-size);
    @apply ml-auto transition-transform duration-150;
    color: var(--color-muted);
    transform: rotate(-90deg);
  }

  .field-group[open] > .field-group__heading :global(.field-group__caret) {
    transform: rotate(0deg);
  }
</style>
