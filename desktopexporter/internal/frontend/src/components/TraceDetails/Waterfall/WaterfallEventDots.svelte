<script lang="ts">
  import type { EventMarker, CategoricalToken } from './WaterfallView.svelte'

  type Props = {
    markers: EventMarker[]
    colorToken: CategoricalToken | 'error'
  }

  let { markers, colorToken }: Props = $props()
</script>

{#each markers as marker}
  <span
    class="event-marker event-marker--{colorToken}"
    style:left="{marker.percent}%"
    data-tooltip={marker.name}
    aria-label="Event: {marker.name}"
  ></span>
{/each}

<style lang="postcss">
  @reference "../../../app.css";
  .event-marker {
    @apply absolute z-[3] rounded-full;
    width: 11px;
    height: 11px;
    top: 50%;
    transform: translate(-50%, -50%);
    pointer-events: auto;
    border: 1px solid;
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

  .event-marker--iris {
    --marker-color: var(--color-primary);
    background-color: color-mix(in srgb, var(--color-primary) 40%, white);
    border-color: color-mix(in srgb, var(--color-primary) 80%, black);
  }
  .event-marker--pine {
    --marker-color: var(--color-secondary);
    background-color: color-mix(in srgb, var(--color-secondary) 40%, white);
    border-color: color-mix(in srgb, var(--color-secondary) 80%, black);
  }
  .event-marker--gold {
    --marker-color: var(--color-warning);
    background-color: color-mix(in srgb, var(--color-warning) 40%, white);
    border-color: color-mix(in srgb, var(--color-warning) 80%, black);
  }
  .event-marker--rose {
    --marker-color: var(--color-rose);
    background-color: color-mix(in srgb, var(--color-rose) 40%, white);
    border-color: color-mix(in srgb, var(--color-rose) 80%, black);
  }
  .event-marker--foam {
    --marker-color: var(--color-accent);
    background-color: color-mix(in srgb, var(--color-accent) 40%, white);
    border-color: color-mix(in srgb, var(--color-accent) 80%, black);
  }
  .event-marker--error {
    --marker-color: var(--color-error);
    background-color: color-mix(in srgb, var(--color-error) 40%, white);
    border-color: color-mix(in srgb, var(--color-error) 80%, black);
  }
</style>
