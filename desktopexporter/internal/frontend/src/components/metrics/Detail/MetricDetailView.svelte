<script lang="ts">
  /*
   * MetricDetailView is the "detail" pane on the metrics page.
   * Stacked collapsible sections contain:
   *   - Metric: metric, resource and instrumentation scope fields
   *   - Series: independently selectable series and chart visibility
   *   - Datapoints: received rows for the selected series
   *
   * Uses MetricViewContext and the received-series response for rendering.
   * Disclosure state stays local to the panel.
   */
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailViews from '@/components/shared/DetailViews.svelte'
  import DetailViewTabs, {
    type DetailViewMode,
  } from '@/components/shared/DetailViewTabs.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    ChartLineData02Icon,
    ChartHistogramIcon,
  } from '@hugeicons/core-free-icons'
  import MetricField from '@/components/shared/DetailProperty.svelte'
  import TimeseriesPanel from './TimeseriesPanel.svelte'
  import DatapointsPanel from './DatapointsPanel.svelte'
  import type { DatapointSeries } from './SeriesDatapointList.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { readOTLPText } from '@/services/export-service'

  const ctx = getMetricViewContext()
  let {
    datapointSeries,
    datapointsError,
    retryDatapoints,
    selectionCurrent = true,
  }: {
    datapointSeries?: DatapointSeries
    datapointsError?: string
    retryDatapoints?: () => void
    selectionCurrent?: boolean
  } = $props()
  const timeContext = getTimeContext()
  const detailViewID = $props.id()
  let detailView = $state<DetailViewMode>('fields')
  let rawMetric = $state.raw<typeof ctx.metric>()
  const actionMetric = $derived(
    detailView === 'raw' ? (rawMetric ?? ctx.metric) : ctx.metric
  )

  let seriesCount = $derived.by(() => {
    if (!ctx.metric) return 0
    return ctx.isHistogramKind
      ? ctx.histogramTimeseriesCount
      : ctx.gaugeSumLegendTimeseries.length
  })
  let metricURL = $derived(
    ctx.metric ? `/export/metrics/${ctx.metric.metricRef}` : ''
  )
  let rawScopes = $derived([
    {
      id: 'metric',
      label: 'Metric',
      url: metricURL,
      previewURL: metricURL ? `${metricURL}?includeDatapoints=false` : '',
    },
  ])
</script>

{#snippet metricIcon()}<HugeiconsIcon
    icon={ChartHistogramIcon}
    size={16}
    class="shrink-0 text-subtle"
    strokeWidth={1.5}
  />{/snippet}
{#snippet seriesIcon()}<HugeiconsIcon
    icon={ChartLineData02Icon}
    size={16}
    class="shrink-0 text-subtle"
    strokeWidth={1.5}
  />{/snippet}

{#snippet seriesSection()}
  <DetailBubble
    label="Series"
    contained
    icon={seriesIcon}
    count={ctx.metric ? seriesCount : undefined}
  >
    {#if ctx.metric}<TimeseriesPanel />{:else}<p class="p-3 text-subtle">
        Select a metric to view details
      </p>{/if}
  </DetailBubble>
{/snippet}
{#snippet datapointsSection()}
  <DatapointsPanel
    icon={seriesIcon}
    response={datapointSeries}
    {selectionCurrent}
    error={datapointsError}
    onretry={retryDatapoints}
  />
{/snippet}
{#snippet metricSection()}
  <DetailBubble label="Metric" icon={metricIcon}>
    {#snippet action()}
      <DetailViewTabs id={detailViewID} bind:view={detailView}>
        {#snippet action()}
          {#if actionMetric}{@const metric = actionMetric}
            <ValueCopyButton
              grouped
              loadText={signal =>
                readOTLPText('metric', metric.metricRef, signal)}
              label="metric"
              tooltip="Copy metric as OTLP JSON"
            />
          {/if}
        {/snippet}
      </DetailViewTabs>
    {/snippet}
    {#if ctx.metric}{@const metric = ctx.metric}<DetailViews
        id={detailViewID}
        showTabs={false}
        bind:view={detailView}
        record={metric}
        bind:displayedRecord={rawMetric}
        scopes={rawScopes}
        wholeURL={metricURL}
        refreshKey={metric}
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
            <AttributeRows
              attributes={metric.metadata}
              owner="metric metadata"
              source="metadata"
            />
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
                fieldName="aggregationTemporality"
                fieldValue={ctx.temporality}
                fieldType="string"
              />
            {/if}
            {#if ctx.isMonotonic !== null}
              <MetricField
                fieldName="isMonotonic"
                fieldValue={String(ctx.isMonotonic)}
                fieldType="bool"
              />
            {/if}
            {#if metric.lastSeenNs !== null}
              <!-- Metric-wide value; narrowed responses may omit its source series. -->
              <MetricField
                fieldName="lastSeen"
                fieldValue={metric.lastSeenNs.toString()}
                timestampLabel={formatTimestamp(
                  metric.lastSeenNs,
                  timeContext.tz,
                  'milliseconds'
                )}
                fieldType="timestamp"
              />
            {/if}
            <MetricField
              fieldName="dataPointCount"
              fieldValue={ctx.totalDatapointCount.toString()}
              fieldType="uint32"
            />
            {#if metric.resourceSchemaUrl}<MetricField
                fieldName="schemaUrl"
                origin="resource"
                fieldValue={metric.resourceSchemaUrl}
                fieldType="string"
              />{/if}
            {#if metric.resourceDroppedAttributesCount > 0}
              <MetricField
                fieldName="droppedAttributesCount"
                origin="resource"
                fieldValue={metric.resourceDroppedAttributesCount.toString()}
                fieldType="uint32"
              />
            {/if}
            <AttributeRows
              attributes={metric.resource.attributes}
              owner="resource"
              origin="resource"
            />
            {#if metric.scope.name}<MetricField
                fieldName="name"
                origin="scope"
                fieldValue={metric.scope.name}
                fieldType="string"
              />{/if}
            {#if metric.scope.version}<MetricField
                fieldName="version"
                origin="scope"
                fieldValue={metric.scope.version}
                fieldType="string"
              />{/if}
            {#if metric.scopeSchemaUrl}<MetricField
                fieldName="schemaUrl"
                origin="scope"
                fieldValue={metric.scopeSchemaUrl}
                fieldType="string"
              />{/if}
            {#if metric.scopeDroppedAttributesCount > 0}
              <MetricField
                fieldName="droppedAttributesCount"
                origin="scope"
                fieldValue={metric.scopeDroppedAttributesCount.toString()}
                fieldType="uint32"
              />
            {/if}
            <AttributeRows
              attributes={metric.scope.attributes}
              owner="scope"
              origin="scope"
            />
          </tbody>
        </table>
      </DetailViews>{/if}
  </DetailBubble>
{/snippet}
<DetailStack
  label="Metric details"
  storageKey="metric-detail-section-sizes"
  sections={[
    { id: 'metric', content: metricSection },
    { id: 'series', content: seriesSection },
    { id: 'datapoints', content: datapointsSection },
  ]}
/>
