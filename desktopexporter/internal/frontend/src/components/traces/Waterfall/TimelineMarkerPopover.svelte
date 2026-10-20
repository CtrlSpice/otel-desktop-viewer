<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { CircleIcon, PlusSignIcon } from '@hugeicons/core-free-icons'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'
  import LogSeverity from '@/components/logs/LogSeverity.svelte'
  import { formatSignedDuration } from '@/utils/time'
  import {
    MARKER_GLYPH_REM,
    MARKER_HIT_AREA_REM,
    markerColor,
    markerLabel,
    type TimelineMarker,
    type TimelineRecord,
  } from './timeline-markers'

  type Props = {
    marker: TimelineMarker
    spanColor: string
    spanStartTime: bigint
    onSelectRecord: (record: TimelineRecord) => void
  }

  let { marker, spanColor, spanStartTime, onSelectRecord }: Props = $props()
  const popoverID = createPopoverID('activity-popover')
  let trigger = $state<HTMLButtonElement | null>(null)
  let popover = $state<HTMLDivElement | null>(null)
  let open = $state(false)
  let pinned = $state(false)
  let pointerInside = $state(false)
  let restoringTriggerFocus = false
  let closeTimer: ReturnType<typeof setTimeout> | null = null

  let singleton = $derived(marker.members.length === 1)
  let placement = $derived(marker.position.placement)
  let markerLeft = $derived(
    marker.position.placement === 'inside'
      ? `${marker.position.percent}%`
      : undefined
  )

  function offsetLabel(record: TimelineRecord): string {
    return formatSignedDuration(record.timestamp - spanStartTime)
  }

  function recordName(record: TimelineRecord): string {
    return record.kind === 'event' ? record.event.name : record.log.eventName
  }

  function buttonTitle(): string {
    const placement = marker.position.placement
    const suffix =
      placement === 'before'
        ? ' before trace'
        : placement === 'after'
          ? ' after trace'
          : placement === 'outside'
            ? ' outside trace axis'
            : ''
    return `${markerLabel(marker)}${suffix}`
  }

  function showPreview() {
    if (restoringTriggerFocus || !popover?.showPopover) return
    cancelClose()
    closeOtherActivityPopovers()
    popover.showPopover()
  }

  function closePopover(restoreFocus = false) {
    restoringTriggerFocus = true
    try {
      popover?.hidePopover()
      if (restoreFocus) trigger?.focus()
    } finally {
      restoringTriggerFocus = false
    }
  }

  function closeOtherActivityPopovers() {
    for (const candidate of document.querySelectorAll<HTMLElement>(
      '.activity-popover'
    )) {
      if (candidate !== popover) candidate.hidePopover()
    }
  }

  function scheduleClose() {
    if (pinned) return
    cancelClose()
    closeTimer = setTimeout(() => {
      closeTimer = null
      hidePreviewIfUnowned()
    }, 80)
  }

  function cancelClose() {
    if (closeTimer !== null) clearTimeout(closeTimer)
    closeTimer = null
  }

  function handlePointerEnter() {
    pointerInside = true
    showPreview()
  }

  function handlePointerLeave() {
    pointerInside = false
    scheduleClose()
  }

  function previewHasOwner(): boolean {
    const focused = document.activeElement
    return (
      pointerInside ||
      (focused instanceof Node &&
        (trigger?.contains(focused) === true ||
          popover?.contains(focused) === true))
    )
  }

  function hidePreviewIfUnowned() {
    if (pinned || previewHasOwner()) return
    closePopover()
  }

  function handleFocusOut() {
    if (pinned) return
    setTimeout(hidePreviewIfUnowned)
  }

  function select(record: TimelineRecord, event?: MouseEvent) {
    const restoreFocus =
      event?.detail === 0 &&
      document.activeElement instanceof Node &&
      popover?.contains(document.activeElement)
    closePopover(restoreFocus === true)
    onSelectRecord(record)
  }

  function activateMarker(event: MouseEvent) {
    event.stopPropagation()
    if (singleton) {
      const record = marker.members[0]
      if (record) select(record)
      return
    }
    if (pinned) {
      closePopover(true)
    } else {
      pinned = true
      cancelClose()
      closeOtherActivityPopovers()
      popover?.showPopover()
    }
  }

  $effect(() => {
    const popoverElement = popover
    const triggerElement = trigger
    if (!popoverElement || !triggerElement) return
    return setupAnchorPopover({
      popover: popoverElement,
      trigger: triggerElement,
      anchor: 'below-end',
      onOpenChange: nextOpen => {
        open = nextOpen
        if (!nextOpen) pinned = false
      },
    })
  })

  $effect(() => () => cancelClose())

  $effect(() => {
    if (!open) return
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      closePopover(true)
    }
    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  })
</script>

<button
  bind:this={trigger}
  type="button"
  class="timeline-marker"
  class:timeline-marker--outside={placement !== 'inside'}
  style:left={markerLeft}
  style:--marker-color={markerColor(marker, spanColor)}
  style:--marker-glyph="{MARKER_GLYPH_REM}rem"
  style:--marker-hit-area="{MARKER_HIT_AREA_REM}rem"
  aria-controls={popoverID}
  aria-expanded={open}
  aria-haspopup="dialog"
  aria-label={buttonTitle()}
  title={buttonTitle()}
  onmouseenter={handlePointerEnter}
  onmouseleave={handlePointerLeave}
  onfocus={showPreview}
  onfocusout={handleFocusOut}
  onclick={activateMarker}
>
  <HugeiconsIcon
    icon={CircleIcon}
    size={`${MARKER_GLYPH_REM}rem`}
    strokeWidth={1.5}
    class="timeline-marker__circle"
    aria-hidden="true"
  />
  {#if marker.members.length > 1}
    <HugeiconsIcon
      icon={PlusSignIcon}
      size={8}
      strokeWidth={2}
      class="timeline-marker__plus"
      aria-hidden="true"
    />
  {/if}
</button>

<div
  bind:this={popover}
  id={popoverID}
  popover="auto"
  class="anchor-popover anchor-popover--anchored activity-popover"
  role="dialog"
  tabindex="-1"
  aria-label="Span activity"
  onmouseenter={handlePointerEnter}
  onmouseleave={handlePointerLeave}
  onfocusout={handleFocusOut}
>
  <div class="activity-popover__list" role="list">
    {#each marker.members as record (record.id)}
      {@const name = recordName(record)}
      <div role="listitem">
        <button
          type="button"
          class="activity-popover__row item-state item-state--menu"
          onclick={event => {
            event.stopPropagation()
            select(record, event)
          }}
        >
          <span class="activity-popover__heading">
            {#if record.kind === 'event'}
              <span class="activity-popover__name">
                {#if name === 'exception'}
                  <span class="size-2 shrink-0 rounded-full bg-error" aria-hidden="true"
                  ></span>
                {/if}
                <span class="min-w-0 truncate" title={name}>{name}</span>
              </span>
            {:else}
              <LogSeverity
                number={record.log.severityNumber}
                text={record.log.severityText}
              />
            {/if}
            <span class="activity-popover__offset">{offsetLabel(record)}</span>
          </span>
          <span
            class="activity-popover__metadata text-rp-subtle"
            class:activity-popover__metadata--with-dot={record.kind === 'log' || name === 'exception'}
          >
            {#if record.kind === 'event'}
              <span>Span event</span>
            {:else}
              <span>Log</span>
              {#if name}
                <span aria-hidden="true">·</span>
                <span class="min-w-0 truncate" title={name}>{name}</span>
              {/if}
            {/if}
          </span>
        </button>
      </div>
    {/each}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .timeline-marker {
    @apply pointer-events-auto absolute top-1/2 z-20 flex -translate-x-1/2 -translate-y-1/2 cursor-pointer items-center justify-center border-0 bg-transparent p-0;
    width: var(--marker-hit-area);
    height: var(--marker-hit-area);
    color: var(--marker-color);
  }

  .timeline-marker--outside {
    left: calc(100% + var(--duration-gutter) + var(--outside-record-slot) / 2);
  }

  .timeline-marker :global(.timeline-marker__circle) {
    width: var(--marker-glyph);
    height: var(--marker-glyph);
    fill: color-mix(
      in oklab,
      var(--marker-color),
      var(--waterfall-marker-mix-target) var(--waterfall-marker-mix-strength)
    );
    color: var(--color-base-200);
  }

  .timeline-marker :global(.timeline-marker__plus) {
    @apply pointer-events-none absolute;
    color: var(--color-base-200);
  }

  .activity-popover {
    @apply pointer-events-auto w-auto min-w-48 max-w-[min(24rem,calc(100vw-1rem))] overflow-y-auto p-1;
    max-height: min(18rem, calc(100vh - 1rem));
  }

  .activity-popover__list {
    @apply flex flex-col;
  }

  .activity-popover__row {
    @apply flex w-full cursor-pointer flex-col gap-0 px-3 py-2 text-left text-sm font-normal;
  }

  .activity-popover__heading {
    @apply flex w-full min-w-0 items-center gap-2;
  }

  .activity-popover__offset {
    @apply ml-auto shrink-0 whitespace-nowrap text-right font-sans tabular-nums;
    color: var(--color-subtle);
  }

  .activity-popover__name {
    @apply flex min-w-0 items-center gap-2 text-base-content;
  }

  .activity-popover__metadata {
    @apply flex w-full min-w-0 items-center gap-2 whitespace-nowrap;
  }

  .activity-popover__metadata--with-dot {
    padding-left: 1rem;
  }
</style>
