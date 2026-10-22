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
</script>

<section class="px-3 py-2" aria-label={`${entry.key} · ${options.ownerType}`}>
  <h3 class="text-sm font-normal text-rp-subtle">
    <span class="min-w-0 wrap-anywhere"
      >{entry.key}<span class="text-rp-subtle" aria-hidden="true">{' · '}</span
      ><span class="text-secondary">{entry.kind}</span><span
        class="text-rp-subtle"
        aria-hidden="true">{' · '}</span
      ><span class="text-secondary">{options.ownerType}</span></span
    >
  </h3>
  <AttributeValues {options} name={entry.key} kind={entry.kind} />
</section>
