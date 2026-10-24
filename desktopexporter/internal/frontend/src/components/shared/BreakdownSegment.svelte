<script lang="ts">
  import type { BubbleBreakdownPart } from './BubbleBreakdown.svelte'
  import { setupAnchorPopover } from './utils/anchor-popover'

  let {
    part,
    unit,
  }: {
    part: BubbleBreakdownPart
    unit: string
  } = $props()
  const tooltipID = $props.id()
  let trigger = $state<HTMLSpanElement>()
  let tooltip = $state<HTMLDivElement>()
  let open = $state(false)
  let closeTimer: ReturnType<typeof setTimeout> | undefined

  function show() {
    clearTimeout(closeTimer)
    if (!tooltip?.matches(':popover-open')) tooltip?.showPopover()
  }

  function scheduleClose() {
    clearTimeout(closeTimer)
    closeTimer = setTimeout(() => {
      if (
        trigger?.matches(':hover') ||
        tooltip?.matches(':hover, :focus-within')
      )
        return
      tooltip?.hidePopover()
    }, 80)
  }

  $effect(() => {
    if (!trigger || !tooltip) return
    return setupAnchorPopover({
      trigger,
      popover: tooltip,
      anchor: 'below-end',
      onOpenChange: value => {
        open = value
      },
    })
  })
  $effect(() => () => clearTimeout(closeTimer))
</script>

<span
  bind:this={trigger}
  class="breakdown-segment"
  style:flex={part.count}
  style:--part-color={part.color}
  role="img"
  aria-label={`${part.label}: ${part.count.toLocaleString()} ${unit}${part.count === 1 ? '' : 's'}`}
  aria-describedby={open ? tooltipID : undefined}
  onpointerenter={show}
  onpointerleave={scheduleClose}
>
  <span class="breakdown-segment__fill" aria-hidden="true"></span>
</span>
<div
  bind:this={tooltip}
  id={tooltipID}
  popover="auto"
  role="tooltip"
  class="anchor-popover anchor-popover--anchored breakdown-tooltip"
  onpointerenter={show}
  onpointerleave={scheduleClose}
  onfocusout={scheduleClose}
>
  <span class="flex min-w-0 items-center gap-2">
    <span
      class="size-2 shrink-0 rounded-full"
      style:background-color={part.color}
      aria-hidden="true"
    ></span>
    <span class="breakdown-tooltip__label">{part.label}</span>
    <span class="shrink-0 text-subtle tabular-nums"
      >[{part.count.toLocaleString()}]</span
    >
  </span>
</div>

<style lang="postcss">
  @reference "../../app.css";

  .breakdown-segment {
    @apply flex min-w-2 items-center rounded-full border-0 bg-transparent p-0;
  }
  .breakdown-segment__fill {
    @apply pointer-events-none block h-2 w-full rounded-full;
    background-color: var(--part-color);
  }
  .breakdown-tooltip {
    @apply overflow-y-auto px-3 py-2 font-normal;
    max-width: calc(100vw - 1rem);
    pointer-events: auto;
    overscroll-behavior: contain;
  }
  .breakdown-tooltip__label {
    min-width: 0;
    overflow-wrap: anywhere;
  }
</style>
