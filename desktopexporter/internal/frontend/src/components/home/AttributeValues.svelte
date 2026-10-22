<script lang="ts">
  import { telemetryAPI } from '@/services/telemetry-service'
  import type {
    AttributeFrequency,
    DiscoveryOptions,
  } from '@/services/discovery-types'
  import { BarChart } from 'layerchart'
  import AttributeValue from '@/components/shared/AttributeValue.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { attributeValueCopy } from '@/components/shared/detail-value'
  let {
    options,
    name,
    kind,
  }: { options: Omit<DiscoveryOptions, 'kind'>; name: string; kind: string } =
    $props()
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
  <div role="group" aria-label="Values of {name}" class="flex flex-col gap-2">
    {#each values as item, index (index)}
      <div class="item-state rounded-xl px-3 py-2">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0 flex-1">
            <AttributeValue value={item.value} path={name} />
          </div>
          {#if item.value.kind !== kind}<span class="text-secondary"
              >{item.value.kind}</span
            >{/if}
          <ValueCopyButton
            text={attributeValueCopy(item.value)}
            label={`${name} value`}
            tooltip="Copy"
          />
        </div>
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <div class="min-w-16 flex-1" aria-hidden="true">
            <BarChart
              data={[{ key: 'frequency', frequency: item.relativeFrequency }]}
              orientation="horizontal"
              x="frequency"
              y="key"
              xDomain={[0, 1]}
              height={8}
              padding={{ top: 0, right: 0, bottom: 0, left: 0 }}
              bandPadding={0}
              axis={false}
              grid={false}
              legend={false}
              rule={false}
              highlight={false}
              tooltipContext={false}
              motion="none"
              props={{ bars: { strokeWidth: 0 } }}
            />
          </div>
          <span class="text-base-content"
            >{item.count.toLocaleString()} / {item.denominator.toLocaleString()} records
            · {(item.relativeFrequency * 100).toLocaleString(undefined, {
              maximumFractionDigits: 1,
            })}%</span
          >
        </div>
      </div>
    {/each}
  </div>
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
