<script lang="ts">
  import type { Snippet } from 'svelte'
  import DetailNav from './DetailNav.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Delete02Icon } from '@hugeicons/core-free-icons'

  type Props = {
    index?: number
    total?: number
    label: string
    onFirst?: () => void
    onPrev?: () => void
    onNext?: () => void
    onLast?: () => void
    onDelete?: () => void
    leading?: Snippet
    navigation?: Snippet
    trailing?: Snippet
  }

  let {
    index = -1,
    total = 0,
    label,
    onFirst,
    onPrev,
    onNext,
    onLast,
    onDelete,
    leading,
    navigation,
    trailing,
  }: Props = $props()
</script>

<div class="signal-footer">
  {#if leading}<div>{@render leading()}</div>{:else}<span aria-hidden="true"
    ></span>{/if}
  {#if navigation}{@render navigation()}{:else if onFirst && onPrev && onNext && onLast}<DetailNav
      {index}
      {total}
      {label}
      {onFirst}
      {onPrev}
      {onNext}
      {onLast}
    />{:else}<span aria-hidden="true"></span>{/if}
  <div class="signal-footer__actions">
    {@render trailing?.()}
    {#if onDelete}
      <div
        class="tooltip tooltip-left tooltip-error"
        data-tip="Delete this {label}"
      >
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square text-error"
          onclick={onDelete}
          aria-label="Delete this {label}"
        >
          <HugeiconsIcon
            icon={Delete02Icon}
            size="1em"
            strokeWidth={1.5}
            class="h-3.5 w-3.5"
            aria-hidden="true"
          />
        </button>
      </div>
    {/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../app.css";

  /* Share the drawer footer's height and inset for a continuous bottom strip. */
  .signal-footer {
    @apply grid shrink-0 items-center gap-2 bg-base-100 px-4 py-1.5;
    grid-template-columns: 1fr auto 1fr;
    min-height: var(--app-footer-height);
  }

  .signal-footer > :global(*:nth-child(2)) {
    @apply justify-self-center;
  }

  .signal-footer > :global(*:last-child) {
    @apply justify-self-end;
  }
  .signal-footer__actions {
    @apply flex items-center gap-2;
  }
</style>
