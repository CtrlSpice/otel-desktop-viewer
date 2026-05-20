<script lang="ts">
  import type { EventMarker } from './WaterfallView.svelte'

  type Props = {
    markers: EventMarker[]
    /** Concrete colour for the marker (palette HCL or `--color-error`).
     *  Threaded into `--marker-color` for both the dot's fill/border mix
     *  and the tooltip border. */
    color: string
  }

  let { markers, color }: Props = $props()
</script>

{#each markers as marker}
  <span
    class="event-marker"
    style:left="{marker.percent}%"
    style:--marker-color={color}
    data-tooltip={marker.name}
    aria-label="Event: {marker.name}"
  ></span>
{/each}

<style lang="postcss">
  @reference "../../../app.css";
  /* `--marker-color` is set inline per marker from the row's colour
     (palette HCL or `--color-error`); the fill mix, border, and tooltip
     border all read from it so the dot stays themed without a per-token
     CSS variant. */
  .event-marker {
    @apply absolute z-0 rounded-full;
    width: 11px;
    height: 11px;
    top: 50%;
    transform: translate(-50%, -50%);
    pointer-events: auto;
    border: 1px solid;
    background-color: color-mix(in srgb, var(--marker-color) 40%, white);
    border-color: color-mix(in srgb, var(--marker-color) 80%, black);
  }

  .event-marker::before {
    content: attr(data-tooltip);
    @apply absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-1.5 py-0.5 text-[10px] whitespace-nowrap rounded-xl bg-base-100 text-base-content opacity-0 pointer-events-none z-50;
    border: 1px solid var(--marker-color);
    transition: opacity 0.1s;
  }

  .event-marker:hover::before {
    @apply opacity-100;
  }
</style>
