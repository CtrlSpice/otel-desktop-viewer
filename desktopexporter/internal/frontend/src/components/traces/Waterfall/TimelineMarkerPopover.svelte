<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { CircleIcon } from '@hugeicons/core-free-icons'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'
  import { severityBadgeClass } from '@/components/logs/log-severity'
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
    if (restoringTriggerFocus || !singleton || !popover?.showPopover) return
    cancelClose()
    closeOtherActivityPopovers()
    popover.showPopover()
  }

  function closeAndRestoreTriggerFocus() {
    restoringTriggerFocus = true
    try {
      popover?.hidePopover()
      trigger?.focus()
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
    if (!singleton || pinned) return
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
    if (!singleton || pinned || previewHasOwner()) return
    popover?.hidePopover()
  }

  function handleFocusOut() {
    if (!singleton || pinned) return
    setTimeout(hidePreviewIfUnowned)
  }

  function select(record: TimelineRecord, event?: MouseEvent) {
    const restoreFocus =
      event?.detail === 0 &&
      document.activeElement instanceof Node &&
      popover?.contains(document.activeElement)
    if (restoreFocus) closeAndRestoreTriggerFocus()
    else popover?.hidePopover()
    onSelectRecord(record)
  }

  function activateMarker(event: MouseEvent) {
    event.stopPropagation()
    if (singleton) {
      const record = marker.members[0]
      if (record) select(record)
      return
    }
    pinned = true
    closeOtherActivityPopovers()
    popover?.togglePopover()
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
      closeAndRestoreTriggerFocus()
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
    <span class="timeline-marker__count">{marker.members.length}</span>
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
          class="activity-popover__row"
          onclick={event => {
            event.stopPropagation()
            select(record, event)
          }}
        >
          {#if record.kind === 'event'}
            <span
              class="badge badge-xs badge-soft activity-popover__event-badge"
              style:--activity-color={spanColor}>Event</span
            >
          {:else}
            <span class={severityBadgeClass(record.log.severityNumber)}
              >Log</span
            >
          {/if}
          <span class="activity-popover__offset">{offsetLabel(record)}</span>
          {#if name}
            <span class="activity-popover__name">{name}</span>
          {/if}
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

  .timeline-marker__count {
    @apply pointer-events-none absolute inset-0 flex items-center justify-center text-[9px] font-bold leading-none;
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
    @apply grid w-full cursor-pointer grid-cols-[auto_auto_minmax(0,1fr)] items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-base-content/10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary;
  }

  .activity-popover__event-badge {
    color: var(--activity-color);
    border-color: color-mix(in oklab, var(--activity-color) 45%, transparent);
    background: color-mix(in oklab, var(--activity-color) 14%, transparent);
  }

  .activity-popover__offset {
    @apply whitespace-nowrap font-mono tabular-nums text-base-content/70;
  }

  .activity-popover__name {
    @apply truncate text-base-content;
  }
</style>
