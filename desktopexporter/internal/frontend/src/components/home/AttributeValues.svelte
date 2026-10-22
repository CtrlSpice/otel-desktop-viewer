<script lang="ts">
  import { telemetryAPI } from '@/services/telemetry-service'
  import type {
    AttributeFrequency,
    DiscoveryOptions,
  } from '@/services/discovery-types'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  let {
    options,
    name,
  }: { options: Omit<DiscoveryOptions, 'kind'>; name: string } = $props()
  let values = $state<AttributeFrequency[]>([])
  let loading = $state(false)
  let error = $state('')
  let offset = $state(0)
  let more = $state(false)
  $effect(() => {
    const controller = new AbortController()
    const request = { ...options, key: name, offset, limit: 25 }
    loading = true
    error = ''
    void telemetryAPI
      .discoverAttributeValues(request, controller.signal)
      .then(page => {
        if (controller.signal.aborted) return
        values = page.items
        more = page.truncated
      })
      .catch(reason => {
        if (!controller.signal.aborted) error = String(reason)
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false
      })
    return () => controller.abort()
  })
</script>

{#if loading}<p class="p-3 text-rp-subtle" role="status">Loading values…</p>
{:else if error}<p class="p-3" role="alert">{error}</p>
{:else}
  {#if values.length === 0}<p class="p-3 text-rp-subtle">
      No retained values.
    </p>{/if}
  <table class="detail-fields w-full" aria-label="Values of {name}">
    <tbody>
      {#each values as item, index (index)}
        <DetailProperty
          fieldName={name}
          fieldType={item.value.kind}
          typedValue={item.value}
          origin={options.ownerType}
          qualifier={`${item.count.toLocaleString()} / ${item.denominator.toLocaleString()} records · ${(item.relativeFrequency * 100).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`}
        />
      {/each}
    </tbody>
  </table>
  {#if offset > 0 || more}<div
      class="flex items-center justify-between gap-2 px-3 py-2"
    >
      <button
        class="btn btn-ghost btn-sm"
        disabled={offset === 0}
        onclick={() => (offset -= 25)}>Previous values</button
      >
      <button
        class="btn btn-ghost btn-sm"
        disabled={!more}
        onclick={() => (offset += 25)}>Next values</button
      >
    </div>{/if}
{/if}
