<script lang="ts">
  import AttributeValues from './AttributeValues.svelte'
  import type {
    AttributeKey,
    DiscoveryOptions,
  } from '@/services/discovery-types'
  let {
    entry,
    options,
  }: { entry: AttributeKey; options: Omit<DiscoveryOptions, 'kind'> } = $props()
  let requested = $state(false)
</script>

<details
  class="attribute-key"
  ontoggle={event => {
    if (event.currentTarget.open) requested = true
  }}
>
  <summary
    ><span>{entry.key}</span><span class="text-secondary">
      · {entry.kind} · {options.ownerType}</span
    ></summary
  >
  {#if requested}<AttributeValues {options} name={entry.key} />{/if}
</details>

<style lang="postcss">
  @reference '../../app.css';
  .attribute-key {
    @apply rounded-xl;
  }
  summary {
    @apply cursor-pointer rounded-xl px-3 py-2 text-sm;
    color: var(--color-subtle);
    overflow-wrap: anywhere;
  }
  summary:hover {
    background: var(--color-highlight-low);
  }
</style>
