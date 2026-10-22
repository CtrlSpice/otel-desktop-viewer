<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'

  type Props = {
    label: string
    /** Shared names make native details mutually exclusive. */
    name?: string
    /** Optional icon rendered before the heading. */
    icon?: Snippet
    /** Optional snippet replacing the default label text with rich content. */
    heading?: Snippet
    /** Custom header content, including existing links or controls. */
    headerAction?: Snippet
    /** Optional badge before the count (e.g. event offset). */
    badge?: string
    count?: number
    /** Visible unit inside the count badge; defaults to the group label. */
    countLabel?: string
    /** Detail headings use the same inset as the field cards. */
    detail?: boolean
    /** Use the same rounded, interactive surface as signal summary cards. */
    card?: boolean
    open?: boolean
    /** When set, parent owns `open` (e.g. a Set membership). */
    onOpenChange?: (open: boolean) => void
    children: Snippet
  }

  let {
    label,
    name,
    icon,
    heading,
    headerAction,
    badge,
    count,
    countLabel,
    detail = false,
    card = false,
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
  {#if badge || count !== undefined}
    <span class="field-group__badges">
      {#if badge}<span class="badge-count">{badge}</span>{/if}
      {#if count !== undefined}
        <span class="badge-count shrink-0"
          >{count} {countLabel ?? label.toLowerCase()}</span
        >
      {/if}
    </span>
  {/if}
{/snippet}

<details
  class="field-group"
  {name}
  class:field-group--detail={detail}
  class:field-group--card={card}
  {open}
  ontoggle={e => setOpen(e.currentTarget.open)}
>
  <summary class="field-group__heading" class:item-state={card || detail}>
    {#if detail}
      <span class="field-group__caret" aria-hidden="true">
        <HugeiconsIcon icon={ArrowRight01Icon} size={16} strokeWidth={1.5} />
      </span>
    {/if}
    {#if headerAction}
      {@render headerAction()}
    {:else if icon}
      {@render icon()}
      <span class="field-group__heading-content">
        {@render headingBody()}
      </span>
    {:else}
      {@render headingBody()}
    {/if}
  </summary>
  <div class="field-group__content">
    {@render children()}
  </div>
</details>

<style lang="postcss">
  @reference "../../app.css";

  .field-group {
    @apply border-b-0;
    --fg-inline: var(--field-group-inline, 0.75rem);
  }

  .field-group--detail {
    --field-group-inline: var(--pane-inline-inset);
  }
  .field-group--detail > .field-group__heading {
    --item-idle-bg: var(--color-base-300);
    @apply mb-1 rounded-xl font-normal;
  }

  /* Content aligns with heading inset (icon or label). */
  .field-group__content {
    padding-inline: var(--fg-inline);
    @apply pb-2 pt-0;
  }
  .field-group--detail > .field-group__content {
    padding-inline: 0;
  }

  .field-group__heading {
    padding-inline: var(--fg-inline);
    @apply cursor-pointer select-none list-none py-1.5 text-sm font-medium flex items-center gap-2;
    color: var(--color-subtle);
  }

  .field-group__heading-content {
    @apply flex min-w-0 flex-1 items-center gap-2;
  }
  .field-group__caret {
    @apply flex shrink-0 text-base-content;
  }
  .field-group[open] > .field-group__heading > .field-group__caret {
    transform: rotate(90deg);
  }

  .field-group__badges {
    @apply ml-auto flex shrink-0 items-center justify-end gap-2;
  }

  .field-group--card > .field-group__heading {
    --item-idle-bg: var(--color-base-100);
    @apply rounded-xl px-3 py-2.5 font-normal text-base-content;
  }

  .field-group__heading::marker,
  .field-group__heading::-webkit-details-marker {
    display: none;
  }
</style>
