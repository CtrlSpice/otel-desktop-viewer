<script lang="ts">
  import { telemetryAPI } from '@/services/telemetry-service'
  import type {
    AttributeFrequency,
    DiscoveryOptions,
  } from '@/services/discovery-types'
  import { BarChart } from 'layerchart'
  import { traceSpanPalette } from '@/utils/chart-palette'
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
  const colors = $derived(traceSpanPalette(offset + values.length))
  const total = $derived(values.reduce((sum, item) => sum + item.count, 0))
  // A stacked bar requires a complete, non-overlapping distribution. Preserve record
  // percentages in the legend for paginated or multi-valued attributes too.
  const showBar = $derived(
    values.length > 0 &&
      offset === 0 &&
      !more &&
      total > 0 &&
      total === values[0].denominator
  )
  const series = $derived(
    values.map((_, index) => ({
      key: String(index),
      color: colors[offset + index],
    }))
  )
  const distribution = $derived([
    {
      key: 'frequency',
      ...Object.fromEntries(
        values.map((item, index) => [String(index), item.relativeFrequency])
      ),
    },
  ])
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
    {#if showBar}
      <div class="w-full" aria-hidden="true">
        <BarChart
          data={distribution}
          {series}
          seriesLayout="stack"
          orientation="horizontal"
          y="key"
          xDomain={[0, 1]}
          height={8}
          bandPadding={0}
          padding={{ top: 0, right: 0, bottom: 0, left: 0 }}
          axis={false}
          grid={false}
          rule={false}
          highlight={false}
          legend={false}
          tooltipContext={false}
          motion="none"
          props={{ bars: { strokeWidth: 0 } }}
        />
      </div>
    {/if}
    <div class="min-w-0 flex-1">
      {#each values as item, index (index)}
        <div class="item-state flex items-start gap-2 rounded-xl">
          <span
            class="mt-2 size-2 shrink-0 rounded-full"
            style:background-color={colors[offset + index]}
            aria-hidden="true"
          ></span>
          <div class="min-w-0">
            <AttributeValue value={item.value} path={name} />
          </div>
          {#if item.value.kind !== kind}<span class="text-secondary"
              >{item.value.kind}</span
            >{/if}
          <span
            class="shrink-0 text-sm text-rp-subtle"
            title={`${item.count.toLocaleString()} / ${item.denominator.toLocaleString()} records`}
            aria-label={`${item.count.toLocaleString()} / ${item.denominator.toLocaleString()} records · ${(item.relativeFrequency * 100).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`}
            >[{(item.relativeFrequency * 100).toLocaleString(undefined, {
              maximumFractionDigits: 1,
            })}%]</span
          >
          <div class="ml-auto shrink-0">
            <ValueCopyButton
              text={attributeValueCopy(item.value)}
              label={`${name} value`}
              tooltip="Copy"
            />
          </div>
        </div>
      {/each}
    </div>
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
