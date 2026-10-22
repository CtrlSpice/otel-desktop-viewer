<script lang="ts">
  import AttributeValues from './AttributeValues.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import type {
    AttributeKey,
    DiscoveryOptions,
  } from '@/services/discovery-types'
  let {
    entry,
    options,
  }: { entry: AttributeKey; options: Omit<DiscoveryOptions, 'kind'> } = $props()
  let requested = $state(false)
  let open = $state(false)
</script>

<FieldGroup
  label={entry.key}
  detail
  {open}
  onOpenChange={next => {
    open = next
    if (next) requested = true
  }}
>
  {#snippet heading()}
    <span class="min-w-0 wrap-anywhere"
      >{entry.key}<span class="text-rp-subtle" aria-hidden="true">{' · '}</span
      ><span class="text-secondary">{entry.kind}</span><span
        class="text-rp-subtle"
        aria-hidden="true">{' · '}</span
      ><span class="text-secondary">{options.ownerType}</span></span
    >
  {/snippet}
  {#if requested}<AttributeValues
      {options}
      name={entry.key}
      kind={entry.kind}
    />{/if}
</FieldGroup>
