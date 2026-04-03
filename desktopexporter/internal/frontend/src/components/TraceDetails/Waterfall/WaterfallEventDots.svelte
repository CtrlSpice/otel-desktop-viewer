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
  .event-marker {
    @apply absolute z-[4] rounded-full;
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
    --marker-color: var(--rp-iris);
    background-color: color-mix(in srgb, var(--rp-iris) 40%, white);
    border-color: color-mix(in srgb, var(--rp-iris) 80%, black);
  }
  .event-marker--pine {
    --marker-color: var(--rp-pine);
    background-color: color-mix(in srgb, var(--rp-pine) 40%, white);
    border-color: color-mix(in srgb, var(--rp-pine) 80%, black);
  }
  .event-marker--gold {
    --marker-color: var(--rp-gold);
    background-color: color-mix(in srgb, var(--rp-gold) 40%, white);
    border-color: color-mix(in srgb, var(--rp-gold) 80%, black);
  }
  .event-marker--rose {
    --marker-color: var(--rp-rose);
    background-color: color-mix(in srgb, var(--rp-rose) 40%, white);
    border-color: color-mix(in srgb, var(--rp-rose) 80%, black);
  }
  .event-marker--foam {
    --marker-color: var(--rp-foam);
    background-color: color-mix(in srgb, var(--rp-foam) 40%, white);
    border-color: color-mix(in srgb, var(--rp-foam) 80%, black);
  }
  .event-marker--error {
    --marker-color: var(--rp-love);
    background-color: color-mix(in srgb, var(--rp-love) 40%, white);
    border-color: color-mix(in srgb, var(--rp-love) 80%, black);
  }
</style>
