<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ChevronLeftIcon, ChevronRightIcon } from '@hugeicons/core-free-icons'

  let {
    count,
    index = null,
    previous,
    next,
    onPrevious,
    onNext,
    busy = false,
  }: {
    count: number
    /** One-based position when the selection is an error. */
    index?: number | null
    previous: boolean
    next: boolean
    onPrevious: () => void
    onNext: () => void
    busy?: boolean
  } = $props()
</script>

{#if count > 0}
  <span
    class="error-nav"
    role="group"
    aria-label="Error navigation"
  >
    <button
      type="button"
      class="error-nav__button"
      disabled={busy || !previous}
      onclick={onPrevious}
      aria-label="Previous error"
      title="Previous error"
      ><HugeiconsIcon
        icon={ChevronLeftIcon}
        size="1em"
        strokeWidth={1.5}
        class="h-3 w-3"
        aria-hidden="true"
      /></button
    >
    <span class="error-count" aria-live="polite"
      >{index !== null ? `${index}/${count} err` : `${count} err`}</span
    >
    <button
      type="button"
      class="error-nav__button"
      disabled={busy || !next}
      onclick={onNext}
      aria-label="Next error"
      title="Next error"
      ><HugeiconsIcon
        icon={ChevronRightIcon}
        size="1em"
        strokeWidth={1.5}
        class="h-3 w-3"
        aria-hidden="true"
      /></button
    >
  </span>
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .error-nav {
    @apply inline-flex min-h-5 shrink-0 items-center gap-1 text-xs text-base-content;
  }
  .error-nav__button {
    @apply -my-1 inline-flex cursor-pointer items-center justify-center px-0.5 py-1 transition-colors duration-100;
    color: color-mix(in oklab, currentColor 60%, transparent);
  }
  .error-nav__button:hover:not(:disabled),
  .error-nav__button:focus-visible {
    color: inherit;
  }
  .error-nav__button:disabled {
    @apply cursor-default;
    color: color-mix(in oklab, currentColor 30%, transparent);
  }
</style>
