<script lang="ts">
  /*
   * MetricDetailView is the "detail" pane on the metrics page.
   * Stacked collapsible sections contain:
   *   - Metric: metric, resource and instrumentation scope fields
   *   - Series: per-timeseries rows with nested datapoints
   *
   * Reads everything through MetricViewContext: this is a near-pure
   * renderer. Disclosure state stays local to the panel.
   */
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    ChartLineData02Icon,
    ChartHistogramIcon,
  } from '@hugeicons/core-free-icons'
  import MetricField from './MetricField.svelte'
  import TimeseriesPanel from './TimeseriesPanel.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  let seriesCount = $derived.by(() => {
    if (!ctx.metric) return 0
    return ctx.isHistogramKind
      ? ctx.histogramTimeseriesCount
      : ctx.gaugeSumLegendTimeseries.length
  })
</script>

{#if !ctx.metric}
  <div class="detail-view detail-view--empty">
    <p class="text-rp-muted text-sm">Select a metric to view details</p>
  </div>
{:else}
  {@const metric = ctx.metric}

  {#snippet metricIcon()}<HugeiconsIcon
      icon={ChartHistogramIcon}
      size={16}
      class="shrink-0 text-rp-muted"
      strokeWidth={1.5}
    />{/snippet}
  {#snippet seriesIcon()}<HugeiconsIcon
      icon={ChartLineData02Icon}
      size={16}
      class="shrink-0 text-rp-muted"
      strokeWidth={1.5}
    />{/snippet}

  <DetailStack label="Metric details">
    <DetailBubble label="Series" icon={seriesIcon} count={seriesCount}>
      <TimeseriesPanel />
    </DetailBubble>
    <DetailBubble label="Metric" icon={metricIcon}>
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
          <AttributeRows attributes={metric.metadata} owner="metric metadata" />
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
          {#if metric.lastSeenNs !== null}
            <!-- Metric-wide value; narrowed responses may omit its source series. -->
            <MetricField
              fieldName="last seen"
              fieldValue={formatTimestamp(
                metric.lastSeenNs,
                timeContext.tz,
                'milliseconds'
              )}
              fieldType="timestamp"
            />
          {/if}
          <MetricField
            fieldName="datapoint count"
            fieldValue={ctx.totalDatapointCount.toString()}
            fieldType="uint32"
          />
          {#if metric.resourceSchemaUrl}<MetricField
              fieldName="resource.schemaUrl"
              fieldValue={metric.resourceSchemaUrl}
              fieldType="string"
            />{/if}
          {#if metric.resourceDroppedAttributesCount > 0}
            <MetricField
              fieldName="resource.droppedAttributesCount"
              fieldValue={metric.resourceDroppedAttributesCount.toString()}
              fieldType="uint32"
            />
          {/if}
          <AttributeRows
            attributes={metric.resource.attributes}
            owner="resource"
            prefix="resource."
          />
          {#if metric.scope.name}<MetricField
              fieldName="scope.name"
              fieldValue={metric.scope.name}
              fieldType="string"
            />{/if}
          {#if metric.scope.version}<MetricField
              fieldName="scope.version"
              fieldValue={metric.scope.version}
              fieldType="string"
            />{/if}
          {#if metric.scopeSchemaUrl}<MetricField
              fieldName="scope.schemaUrl"
              fieldValue={metric.scopeSchemaUrl}
              fieldType="string"
            />{/if}
          {#if metric.scopeDroppedAttributesCount > 0}
            <MetricField
              fieldName="scope.droppedAttributesCount"
              fieldValue={metric.scopeDroppedAttributesCount.toString()}
              fieldType="uint32"
            />
          {/if}
          <AttributeRows
            attributes={metric.scope.attributes}
            owner="scope"
            prefix="scope."
          />
        </tbody>
      </table>
    </DetailBubble>
  </DetailStack>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .detail-view {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden;
  }

  .detail-view--empty {
    @apply items-center justify-center;
  }
</style>
