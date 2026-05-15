<script lang="ts">
  /*
   * MetricDetailView is the "detail" pane on the metrics page. A
   * PaneHeader tab strip switches between two views:
   *   - Details: per-metric metadata (Stats + Fields)
   *   - Datapoints: placeholder for now
   *
   * Reads everything through MetricViewContext: this is a near-pure
   * renderer. The only locally-owned state is the active tab and
   * the flattened resource/scope attribute lists for Fields.
   */
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import type { DataPoint } from '@/types/api-types'
  import { timeseriesColor } from '@/utils/timeseries-palette'
  import PaneHeader, { type PaneTab } from '@/components/PaneHeader.svelte'
  import FieldGroup from '@/components/FieldGroup.svelte'
  import { LeftToRightListBulletIcon, HandPointingDownIcon } from '@/icons'
  import MetricField from './MetricField.svelte'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  let activeTab = $state<'fields' | 'datapoints'>('fields')

  let metricOpen = $state(true)
  let resourceOpen = $state(true)
  let scopeOpen = $state(true)

  let metricFieldCount = $derived.by(() => {
    const m = ctx.metric
    if (!m) return 0
    let n = 2 // name + type are always present
    if (m.description) n++
    if (m.unit) n++
    if (ctx.temporality) n++
    if (ctx.isMonotonic !== null) n++
    n += 2 // received + datapoint count
    return n
  })
  type MetadataAttr = {
    key: string
    value: string
    type: string
    scope: 'resource' | 'scope'
  }

  let resourceAttrs = $derived.by((): MetadataAttr[] => {
    const m = ctx.metric
    if (!m) return []
    return m.resource.attributes.map(a => ({
      key: a.key,
      value: a.value,
      type: a.type,
      scope: 'resource' as const,
    }))
  })

  let scopeAttrs = $derived.by((): MetadataAttr[] => {
    const m = ctx.metric
    if (!m) return []
    const out: MetadataAttr[] = []
    if (m.scope.name) {
      out.push({
        key: 'name',
        value: m.scope.name,
        type: 'string',
        scope: 'scope',
      })
    }
    if (m.scope.version) {
      out.push({
        key: 'version',
        value: m.scope.version,
        type: 'string',
        scope: 'scope',
      })
    }
    for (const a of m.scope.attributes) {
      out.push({ key: a.key, value: a.value, type: a.type, scope: 'scope' })
    }
    return out
  })

  type FlatDatapoint = {
    dp: DataPoint
    colorIndex: number
  }

  let flatDatapoints = $derived.by((): FlatDatapoint[] => {
    const ts = ctx.filteredTimeseries
    if (ts.length === 0) return []
    const items: FlatDatapoint[] = []
    for (const series of ts) {
      const colorIdx = ctx.timeseriesColorIndex.get(series.attributesKey) ?? 0
      for (const dp of series.datapoints) {
        items.push({ dp, colorIndex: colorIdx })
      }
    }
    items.sort((a, b) => {
      if (b.dp.timestamp > a.dp.timestamp) return 1
      if (b.dp.timestamp < a.dp.timestamp) return -1
      return 0
    })
    return items
  })

  function datapointValue(dp: DataPoint): string {
    if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
      return String(dp.doubleValue ?? dp.intValue ?? '—')
    }
    if (
      dp.metricType === 'Histogram' ||
      dp.metricType === 'ExponentialHistogram'
    ) {
      return `count: ${dp.count}, sum: ${dp.sum.toFixed(2)}`
    }
    return '—'
  }

  type ValueColumn = {
    key: string
    label: string
    extract: (dp: DataPoint) => string
  }

  let valueColumns = $derived.by((): ValueColumn[] => {
    const t = ctx.metricType
    if (t === 'Gauge' || t === 'Sum') {
      return [
        {
          key: 'value',
          label: 'Value',
          extract: dp => {
            if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
              return String(dp.doubleValue ?? dp.intValue ?? '—')
            }
            return '—'
          },
        },
      ]
    }
    if (t === 'Histogram' || t === 'ExponentialHistogram') {
      return [
        {
          key: 'count',
          label: 'Count',
          extract: dp =>
            dp.metricType === 'Histogram' ||
            dp.metricType === 'ExponentialHistogram'
              ? String(dp.count)
              : '—',
        },
        {
          key: 'sum',
          label: 'Sum',
          extract: dp =>
            dp.metricType === 'Histogram' ||
            dp.metricType === 'ExponentialHistogram'
              ? dp.sum.toFixed(2)
              : '—',
        },
      ]
    }
    return []
  })
</script>

{#if !ctx.metric}
  <div class="detail-view detail-view--empty">
    <p class="text-rp-muted text-sm">Select a metric to view details</p>
  </div>
{:else}
  {@const metric = ctx.metric}

  {#snippet fieldsIcon()}<LeftToRightListBulletIcon />{/snippet}
  {#snippet datapointsIcon()}<HandPointingDownIcon />{/snippet}

  {@const tabs: PaneTab[] = [
    { id: 'fields', label: 'Fields', icon: fieldsIcon },
    { id: 'datapoints', label: 'Datapoints', icon: datapointsIcon },
  ]}

  <div class="detail-view">
    <PaneHeader
      mode="tabs"
      {tabs}
      activeId={activeTab}
      onSelect={id => (activeTab = id as 'fields' | 'datapoints')}
      ariaLabel="Metric detail tabs"
    />

    <div class="detail-view__scroll">
      {#if activeTab === 'fields'}
        <FieldGroup
          label="Metric"
          count={metricFieldCount}
          bind:open={metricOpen}
        >
          <table class="detail-fields w-full" aria-label="Metric fields">
            <tbody>
              <MetricField
                fieldName="name"
                fieldValue={metric.name}
                fieldType="string"
              />
              {#if metric.description}
                <MetricField
                  fieldName="description"
                  fieldValue={metric.description}
                  fieldType="string"
                />
              {/if}
              <MetricField
                fieldName="type"
                fieldValue={ctx.metricType}
                fieldType="string"
              />
              {#if metric.unit}
                <MetricField
                  fieldName="unit"
                  fieldValue={metric.unit}
                  fieldType="string"
                />
              {/if}
              {#if ctx.temporality}
                <MetricField
                  fieldName="aggregation temporality"
                  fieldValue={ctx.temporality}
                  fieldType="string"
                />
              {/if}
              {#if ctx.isMonotonic !== null}
                <MetricField
                  fieldName="is monotonic"
                  fieldValue={String(ctx.isMonotonic)}
                  fieldType="bool"
                />
              {/if}
              <MetricField
                fieldName="received"
                fieldValue={formatTimestamp(
                  metric.received,
                  timeContext.timezone,
                  'milliseconds'
                )}
                fieldType="timestamp"
              />
              <MetricField
                fieldName="datapoint count"
                fieldValue={ctx.totalDatapointCount.toString()}
                fieldType="uint32"
              />
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup
          label="Resource"
          count={resourceAttrs.length}
          bind:open={resourceOpen}
        >
          <table class="detail-fields w-full" aria-label="Resource attributes">
            <tbody>
              {#if metric.resourceDroppedAttributesCount > 0}
                <MetricField
                  fieldName="dropped attributes"
                  fieldValue={metric.resourceDroppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              {/if}
              {#each resourceAttrs as attr (`resource:${attr.key}`)}
                <MetricField
                  fieldName={attr.key}
                  fieldValue={attr.value}
                  fieldType={attr.type}
                />
              {/each}
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup
          label="Scope"
          count={scopeAttrs.length}
          bind:open={scopeOpen}
        >
          <table class="detail-fields w-full" aria-label="Scope attributes">
            <tbody>
              {#each scopeAttrs as attr (`scope:${attr.key}`)}
                <MetricField
                  fieldName={attr.key}
                  fieldValue={attr.value}
                  fieldType={attr.type}
                />
              {/each}
            </tbody>
          </table>
        </FieldGroup>
      {:else if flatDatapoints.length === 0}
        <p class="dp-list__empty">No datapoints</p>
      {:else}
        <table class="dp-list" aria-label="Datapoints">
          <tbody>
            {#each flatDatapoints as { dp, colorIndex } (dp.id)}
              {@const selected = ctx.selectedDatapointId === dp.id}
              {@const hasExtra = dp.flags > 0 || dp.exemplars.length > 0}
              {@const expanded = hasExtra && ctx.expandedDatapoints.has(dp.id)}
              <tr
                class="dp-list__row"
                class:dp-list__row--selected={selected}
                class:dp-list__row--expandable={hasExtra}
                data-dp-id={dp.id}
                onclick={() => ctx.onDatapointClick(dp)}
              >
                <td class="dp-list__td dp-list__td--swatch">
                  <span
                    class="dp-list__swatch"
                    style:background-color={timeseriesColor(colorIndex)}
                    aria-hidden="true"
                  ></span>
                </td>
                <td class="dp-list__td dp-list__td--content">
                  <span class="dp-list__time tabular-nums">{formatTimestamp(dp.timestamp, timeContext.timezone, 'milliseconds')}</span>
                  {#each valueColumns as col (col.key)}
                    <span class="dp-list__field-label">{col.label}:</span>
                    <span class="dp-list__field-value tabular-nums">{col.extract(dp)}</span>
                  {/each}
                  {#if hasExtra}
                    {#if dp.exemplars.length > 0}
                      <span class="badge badge-xs badge-soft badge-neutral">{dp.exemplars.length} ex</span>
                    {/if}
                    {#if dp.flags > 0}
                      <span class="badge badge-xs badge-soft badge-warning">flags</span>
                    {/if}
                  {/if}
                </td>
              </tr>
              {#if expanded}
                <tr class="dp-list__expansion-row">
                  <td colspan="2" class="dp-list__expansion-cell">
                    <div class="dp-list__expansion">
                      {#if dp.flags > 0}
                        <div class="dp-list__detail">
                          <span class="dp-list__detail-label">flags</span>
                          <span class="dp-list__detail-value">{dp.flags}</span>
                        </div>
                      {/if}
                      {#each dp.exemplars as ex, i}
                        <div class="dp-list__exemplar">
                          <span class="dp-list__detail-label">exemplar {i + 1}</span>
                          <div class="dp-list__exemplar-fields">
                            <span class="dp-list__detail-value">value: {ex.value}</span>
                            <span class="dp-list__detail-value tabular-nums">
                              time: {formatTimestamp(ex.timestamp, timeContext.timezone, 'milliseconds')}
                            </span>
                            {#if ex.traceID}
                              <span class="dp-list__detail-value">trace: {ex.traceID}</span>
                            {/if}
                            {#if ex.spanID}
                              <span class="dp-list__detail-value">span: {ex.spanID}</span>
                            {/if}
                            {#each ex.filteredAttributes as attr (attr.key)}
                              <span class="dp-list__detail-value">{attr.key}: {attr.value}</span>
                            {/each}
                          </div>
                        </div>
                      {/each}
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .detail-view {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden;
  }

  .detail-view--empty {
    @apply items-center justify-center;
  }

  /* Single vertical scroll viewport for both sections. min-h-0 lets
     the flex parent shrink past content size so overflow-y-auto
     actually engages instead of pushing the page footer down. */
  .detail-view__scroll {
    @apply flex-1 min-h-0 overflow-y-auto;
    scrollbar-width: thin;
  }

  /* Section block: heading + content. gap above the content gives
     the heading room to breathe without manual margin discipline. */
  .detail-view__section {
    @apply flex flex-col gap-2 px-3 py-3;
  }

  .detail-view__section + .detail-view__section {
    border-top: 1px solid
      color-mix(in oklab, var(--color-base-300) 30%, transparent);
  }

  .detail-view__section-heading {
    @apply m-0 text-sm font-medium;
    color: var(--color-subtle);
  }

  .detail-view__section-empty {
    @apply m-0 text-sm italic;
    color: var(--color-muted);
  }

  /* --- Datapoints tab ------------------------------------------- */

  .dp-list__empty {
    @apply m-0 px-3 py-6 text-center text-sm italic;
    color: var(--color-muted);
  }

  .dp-list {
    @apply w-full text-sm;
    border-collapse: collapse;
  }

  .dp-list__row {
    @apply cursor-pointer transition-colors hover:bg-base-300/30;
  }

  .dp-list__row--selected {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 18%,
      transparent
    );
  }

  .dp-list__td {
    @apply py-1 align-middle;
  }

  .dp-list__td--swatch {
    @apply pl-3 pr-1;
    width: 1.25rem;
  }

  .dp-list__swatch {
    @apply inline-block rounded-full;
    width: 6px;
    height: 6px;
  }

  .dp-list__td--content {
    @apply pl-1 pr-4 whitespace-nowrap;
  }

  .dp-list__time {
    color: var(--color-subtle);
    @apply mr-3;
  }

  .dp-list__field-label {
    color: var(--color-subtle);
    @apply mr-0.5;
  }

  .dp-list__field-value {
    @apply font-mono mr-3;
    color: var(--color-base-content);
  }

  .dp-list__expansion-row {
    @apply bg-base-200/50;
  }

  .dp-list__expansion-cell {
    @apply p-0;
  }

  .dp-list__expansion {
    @apply flex flex-col gap-2 px-4 py-2;
    border-bottom: 1px solid
      color-mix(in oklab, var(--color-base-300) 30%, transparent);
  }

  .dp-list__detail {
    @apply flex items-baseline gap-2 text-xs;
  }

  .dp-list__detail-label {
    @apply shrink-0 text-sm font-medium;
    color: var(--color-subtle);
  }

  .dp-list__detail-value {
    @apply font-mono text-xs;
    color: var(--color-base-content);
  }

  .dp-list__exemplar {
    @apply flex flex-col gap-0.5;
  }

  .dp-list__exemplar-fields {
    @apply flex flex-col gap-0.5 pl-3;
  }
</style>
