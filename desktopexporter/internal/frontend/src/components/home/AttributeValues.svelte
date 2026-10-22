<script lang="ts">
  import type { Snippet } from 'svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type {
    AttributeFrequency,
    DiscoveryOptions,
  } from '@/services/discovery-types'
  import BreakdownSegment from '@/components/shared/BreakdownSegment.svelte'
  import { traceSpanPalette } from '@/utils/chart-palette'
  import AttributeValue from '@/components/shared/AttributeValue.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { attributeValueCopy } from '@/components/shared/detail-value'
  let {
    options,
    name,
    kind,
    heading: attributeHeading,
  }: {
    options: Omit<DiscoveryOptions, 'kind'>
    name: string
    kind: string
    heading: Snippet
  } = $props()
  let values = $state<AttributeFrequency[]>([])
  let loading = $state(false)
  let error = $state('')
  let offset = $state(0)
  let more = $state(false)
  let open = $state(false)
  let headerValues = $state<AttributeFrequency[]>([])
  let headerMore = $state(false)
  const colors = $derived(traceSpanPalette(offset + values.length))
  const headerColors = $derived(traceSpanPalette(headerValues.length))
  const total = $derived(
    headerValues.reduce((sum, item) => sum + item.count, 0)
  )
  // A stacked bar requires a complete, non-overlapping distribution. Preserve record
  // percentages in the legend for paginated or multi-valued attributes too.
  const showBar = $derived(
    headerValues.length > 0 &&
      !headerMore &&
      total > 0 &&
      total === headerValues[0].denominator
  )
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
        if (request.offset === 0) {
          headerValues = page.items
          headerMore = page.truncated
        }
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

<FieldGroup label={name} detail bare bind:open>
  {#snippet heading()}
    <span class="flex min-w-0 flex-1 flex-col">
      <span class="min-w-0">{@render attributeHeading()}</span>
      {#if showBar}
        <span class="flex min-h-(--signal-card-row-h) w-full gap-1">
          {#each headerValues as item, index (index)}
            <BreakdownSegment
              part={{
                key: String(index),
                label: attributeValueCopy(item.value),
                count: item.count,
                color: headerColors[index],
              }}
              unit="record"
              relativeFrequency={item.relativeFrequency}
            />
          {/each}
        </span>
      {/if}
    </span>
  {/snippet}
  {@render breakdown()}
</FieldGroup>

{#snippet breakdown()}
  {#if loading}<p class="p-3 text-rp-subtle" role="status">Loading values…</p>
  {:else if error}<p class="p-3" role="alert">{error}</p>
  {:else}
    {#if values.length === 0}<p class="p-3 text-rp-subtle">
        No retained values.
      </p>{/if}
    <div role="group" aria-label="Values of {name}" class="flex flex-col gap-2">
      <div class="min-w-0 flex-1">
        {#each values as item, index (index)}
          <div class="item-state flex items-start gap-2 rounded-xl">
            <span
              class="flex h-[1.5em] w-4 shrink-0 items-center justify-center"
              aria-hidden="true"
              ><span
                class="size-2 rounded-full"
                style:background-color={colors[offset + index]}
              ></span></span
            >
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
{/snippet}
