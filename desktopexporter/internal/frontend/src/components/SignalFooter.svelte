<script lang="ts">
  import DetailNav from './DetailNav.svelte'
  import { TrashIcon } from '@/icons'

  type Props = {
    index: number
    total: number
    label: string
    onFirst: () => void
    onPrev: () => void
    onNext: () => void
    onLast: () => void
    onDelete?: () => void
  }

  let {
    index,
    total,
    label,
    onFirst,
    onPrev,
    onNext,
    onLast,
    onDelete,
  }: Props = $props()
</script>

<div class="signal-footer">
  <span aria-hidden="true"></span>
  <DetailNav
    {index}
    {total}
    {label}
    {onFirst}
    {onPrev}
    {onNext}
    {onLast}
  />
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
        <TrashIcon class="h-3.5 w-3.5" aria-hidden="true" />
      </button>
    </div>
  {:else}
    <span aria-hidden="true"></span>
  {/if}
</div>

<style lang="postcss">
  @reference "../app.css";

  /*
   * Page-level footer strip. Pinned to --app-footer-height (app.css)
   * so it aligns pixel-for-pixel with the drawer footer; both read
   * as one continuous bottom edge across the app shell. Vertical
   * padding is dropped in favor of min-height + items-center so the
   * row size is decoupled from whatever buttons live inside.
   */
  .signal-footer {
    @apply grid shrink-0 items-center gap-2 bg-base-100 px-4;
    grid-template-columns: 1fr auto 1fr;
    min-height: var(--app-footer-height);
  }

  .signal-footer > :global(*:nth-child(2)) {
    @apply justify-self-center;
  }

  .signal-footer > :global(*:last-child) {
    @apply justify-self-end;
  }
</style>
