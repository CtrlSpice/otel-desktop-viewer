<script lang="ts">
  import {
    ArrowLeftDoubleIcon,
    ArrowLeftIcon,
    ArrowRightDoubleIcon,
    ArrowRightIcon,
  } from '@/icons'

  type Props = {
    /** 0-based index of current selection in the list, or -1 if nothing is selected */
    index: number
    total: number
    /** Singular noun used for aria-labels and the counter (e.g. "trace", "log", "metric") */
    label: string
    onFirst: () => void
    onPrev: () => void
    onNext: () => void
    onLast: () => void
  }

  let { index, total, label, onFirst, onPrev, onNext, onLast }: Props = $props()

  let canGoPrev = $derived(index > 0)
  let canGoNext = $derived(index >= 0 && index < total - 1)
  let displayPosition = $derived(total === 0 ? 0 : index + 1)
  let plural = $derived(total === 1 ? label : `${label}s`)
</script>

<div class="detail-nav" role="group" aria-label="{label} navigation">
  <button
    type="button"
    class="btn btn-ghost btn-sm btn-square"
    onclick={onFirst}
    disabled={!canGoPrev}
    aria-label="First {label}"
  >
    <ArrowLeftDoubleIcon class="h-3.5 w-3.5" aria-hidden="true" />
  </button>
  <button
    type="button"
    class="btn btn-ghost btn-sm btn-square"
    onclick={onPrev}
    disabled={!canGoPrev}
    aria-label="Previous {label}"
  >
    <ArrowLeftIcon class="h-3.5 w-3.5" aria-hidden="true" />
  </button>
  <span class="detail-nav__counter tabular-nums" aria-live="polite">
    {displayPosition} / {total} {plural}
  </span>
  <button
    type="button"
    class="btn btn-ghost btn-sm btn-square"
    onclick={onNext}
    disabled={!canGoNext}
    aria-label="Next {label}"
  >
    <ArrowRightIcon class="h-3.5 w-3.5" aria-hidden="true" />
  </button>
  <button
    type="button"
    class="btn btn-ghost btn-sm btn-square"
    onclick={onLast}
    disabled={!canGoNext}
    aria-label="Last {label}"
  >
    <ArrowRightDoubleIcon class="h-3.5 w-3.5" aria-hidden="true" />
  </button>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .detail-nav {
    @apply flex items-center gap-1;
  }

  .detail-nav__counter {
    @apply px-2 text-xs text-base-content/60 select-none;
  }
</style>
