<script lang="ts">
  import type { EventMarker } from './WaterfallView.svelte'

  type Props = {
    markers: EventMarker[]
    /** Concrete colour for the marker (palette HCL or `--color-error`). */
    color: string
    /** `dots` renders visible markers under bar labels; `tooltips` renders
     *  hover targets + labels in a higher layer so tooltips paint above text. */
    layer: 'dots' | 'tooltips'
  }

  let { markers, color, layer }: Props = $props()
</script>

{#each markers as marker}
  {#if layer === 'dots'}
    <span
      class="event-marker event-marker--dot"
      style:left="{marker.percent}%"
      style:--marker-color={color}
      aria-hidden="true"
    ></span>
  {:else}
    <span
      class="event-marker event-marker--tooltip-target"
      style:left="{marker.percent}%"
      style:--marker-color={color}
      data-tooltip={marker.name}
      data-side={marker.percent < 50 ? 'right' : 'left'}
      aria-label="Event: {marker.name}"
    ></span>
  {/if}
{/each}

<style lang="postcss">
  @reference "../../../app.css";

  /* Vertical event ticks (not dots) — short, narrow bars centred on the
     row so they read against the wide span pill behind them. */
  .event-marker {
    @apply absolute;
    width: 3px;
    height: 14px;
    top: 50%;
    transform: translate(-50%, -50%);
  }

  /* Tick is the bar's colour mixed toward a per-theme target colour
     (`--waterfall-tick-mix-target`) by `--waterfall-tick-mix-strength`.
     Today all themes lighten toward white; flip the target to black to
     darken instead (saturation dip vs highlight). */
  .event-marker--dot {
    @apply rounded-sm border-0 pointer-events-none;
    background-color: color-mix(
      in srgb,
      var(--marker-color),
      var(--waterfall-tick-mix-target, white)
        var(--waterfall-tick-mix-strength, 40%)
    );
  }

  /* Invisible hit target sized larger than the visible tick so hover is
     forgiving in both axes. Wider (11px) to clear the bar's rounded caps
     for first/last events, taller (22px) so the hover area extends a few
     pixels above and below the 14px bar. Tooltip paints in the z-20
     layer above bar labels. */
  .event-marker--tooltip-target {
    @apply rounded-sm border-0 pointer-events-auto;
    width: 11px;
    height: 22px;
    background: transparent;
  }

  /* Tooltip floats to the side of the tick — right when the event sits
     in the left half of the row, left when it sits in the right half —
     so it never escapes the timeline horizontally and doesn't need to
     fight other rows vertically. Side is set by data-side on the
     target; CSS picks left/right placement via attribute selectors. */
  .event-marker--tooltip-target::before {
    content: attr(data-tooltip);
    @apply absolute top-1/2 -translate-y-1/2 px-1.5 py-0.5 text-[10px] whitespace-nowrap rounded-xl bg-base-100 text-base-content opacity-0 pointer-events-none;
    z-index: 1;
    border: 1px solid var(--marker-color);
    transition: opacity 0.1s;
  }

  .event-marker--tooltip-target[data-side='right']::before {
    left: 100%;
    margin-left: 0.5rem;
  }

  .event-marker--tooltip-target[data-side='left']::before {
    right: 100%;
    margin-right: 0.5rem;
  }

  .event-marker--tooltip-target:hover::before {
    @apply opacity-100;
  }
</style>
