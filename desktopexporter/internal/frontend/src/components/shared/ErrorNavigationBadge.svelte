<script lang="ts">
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
      ><svg
        viewBox="0 0 12 12"
        class="error-nav__caret"
        aria-hidden="true"
      ><path d="m8 2-5 4 5 4Z" /></svg></button
    >
    <span aria-live="polite"
      >{index !== null ? `err (${index}/${count})` : 'err'}</span
    >
    <button
      type="button"
      class="error-nav__button"
      disabled={busy || !next}
      onclick={onNext}
      aria-label="Next error"
      title="Next error"
      ><svg
        viewBox="0 0 12 12"
        class="error-nav__caret"
        aria-hidden="true"
      ><path d="m4 2 5 4-5 4Z" /></svg></button
    >
  </span>
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .error-nav {
    @apply inline-flex shrink-0 items-center gap-2 text-sm font-normal whitespace-nowrap tabular-nums text-error;
  }
  .error-nav__button {
    @apply inline-flex size-8 cursor-pointer items-center justify-center border-0 bg-transparent p-0;
    color: inherit;
  }
  .error-nav__caret {
    @apply size-3;
    fill: currentColor;
    stroke: none;
  }
  .error-nav__button:disabled {
    @apply cursor-default;
    color: color-mix(in oklab, currentColor 30%, transparent);
  }
</style>
