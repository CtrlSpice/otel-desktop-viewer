<script lang="ts">
  import { tick } from 'svelte'
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
  let controls = $state<HTMLElement>()
  $effect.pre(() => {
    if (!busy) return
    const button = document.activeElement
    if (!(button instanceof HTMLButtonElement) || !controls?.contains(button))
      return
    // Disabling a focused navigation button blurs it. Preserve focus unless
    // the user moves elsewhere while the next error is loading.
    let moved = false
    const onFocus = () => (moved = true)
    document.addEventListener('focusin', onFocus)
    return () => {
      document.removeEventListener('focusin', onFocus)
      void tick().then(() => {
        if (
          !moved &&
          button.isConnected &&
          !button.disabled &&
          document.activeElement === document.body
        )
          button.focus({ preventScroll: true })
      })
    }
  })
</script>

{#if count > 0}
  <span
    bind:this={controls}
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
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      /></button
    >
    <span class="error-nav__dot" aria-hidden="true"></span>
    <span aria-live="polite" class="error-nav__position"
      >[{index === null ? count : `${index}/${count}`} err]</span
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
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      /></button
    >
  </span>
{/if}

<style lang="postcss">
  @reference "../../app.css";
  .error-nav {
    @apply inline-flex shrink-0 items-center gap-2 text-sm font-normal whitespace-nowrap tabular-nums text-base-content;
  }
  .error-nav__dot {
    @apply size-2 shrink-0 rounded-full bg-error;
  }
  .error-nav__position {
    color: var(--color-subtle);
  }
  .error-nav__button {
    @apply inline-flex size-8 cursor-pointer items-center justify-center border-0 bg-transparent p-0;
    color: inherit;
  }
  .error-nav__button:disabled {
    @apply cursor-default;
    color: color-mix(in oklab, currentColor 30%, transparent);
  }
</style>
