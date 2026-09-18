<script lang="ts">
  /*
   * MetricDetailView is the "detail" pane on the metrics page. A
   * PaneHeader tab strip switches between two views:
   *   - Fields: per-metric metadata (Metric / Resource / Scope)
   *   - Series: per-timeseries rows with nested datapoints
   *
   * Reads everything through MetricViewContext: this is a near-pure
   * renderer. The only locally-owned state is the active tab and
   * the flattened resource/scope attribute lists for Fields.
   */
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import PaneHeader, {
    paneTabID,
    type PaneTab,
  } from '@/components/shared/PaneHeader.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import BarChartHorizontalIcon from '@hugeicons/core-free-icons/BarChartHorizontalIcon'
  import LeftToRightListBulletIcon from '@hugeicons/core-free-icons/LeftToRightListBulletIcon'
  import MetricField from './MetricField.svelte'
  import TimeseriesPanel from './TimeseriesPanel.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'

  const METRIC_DETAIL_PANEL_ID = 'metric-detail-tabpanel'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  let activeTab = $state<'fields' | 'series'>('series')

  let seriesCount = $derived.by(() => {
    if (!ctx.metric) return 0
    return ctx.isHistogramKind
      ? ctx.histogramTimeseriesCount
      : ctx.gaugeSumLegendTimeseries.length
  })

  let metricOpen = $state(true)
  let resourceOpen = $state(true)
  let scopeOpen = $state(true)

  let metricFieldCount = $derived.by(() => {
    const m = ctx.metric
    if (!m) return 0
    let n = 2 // name + type are always present
    if (m.description) n++
    // OTLP Metric.metadata: one row per entry, like span attributes.
    n += m.metadata.length
    if (m.unit) n++
    if (ctx.temporality) n++
    if (ctx.isMonotonic !== null) n++
    if (m.lastSeenNs !== null) n++ // last seen
    n++ // datapoint count
    return n
  })
</script>

{#if !ctx.metric}
  <div class="detail-view detail-view--empty">
    <p class="text-rp-muted text-sm">Select a metric to view details</p>
  </div>
{:else}
  {@const metric = ctx.metric}

  {#snippet fieldsIcon()}<HugeiconsIcon
      icon={LeftToRightListBulletIcon}
      size="1em"
      strokeWidth={1.5}
    />{/snippet}
  {#snippet seriesIcon()}<HugeiconsIcon
      icon={BarChartHorizontalIcon}
      size="1em"
      strokeWidth={1.5}
    />{/snippet}

  {@const tabs: PaneTab[] = [
    { id: 'fields', label: 'Fields', icon: fieldsIcon },
    { id: 'series', label: 'Series', icon: seriesIcon, count: seriesCount },
  ]}

  <div class="detail-view">
    <PaneHeader
      mode="tabs"
      {tabs}
      activeID={activeTab}
      onSelect={id => (activeTab = id as 'fields' | 'series')}
      ariaLabel="Metric detail tabs"
      tabLayout="equal"
      tabPanelID={METRIC_DETAIL_PANEL_ID}
    />

    <div
      class="detail-view__scroll"
      id={METRIC_DETAIL_PANEL_ID}
      role="tabpanel"
      aria-labelledby={paneTabID(METRIC_DETAIL_PANEL_ID, activeTab)}
    >
      {#if activeTab === 'fields'}
        <FieldGroup
          label="Metric"
          count={metricFieldCount}
          detail
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
              <!--
                Metric.metadata describes the instrument, so it sits with
                description rather than with the resource and scope panels
                below -- those group attributes of the *emitter*. Rendered one
                row per entry, the way span attributes are.
              -->
              <AttributeRows
                attributes={metric.metadata}
                owner="metric metadata"
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
                <!-- The window's most recent datapoint, from the
                     store. Reading timeseries[0].datapoints[0]
                     relied on that series having shipped its
                     datapoints, which narrowing no longer
                     guarantees: name a persisted selection that
                     excludes the most recent series and the
                     field simply disappeared. -->
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
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup
          label="Resource"
          count={metric.resource.attributes.length +
            (metric.resourceDroppedAttributesCount > 0 ? 1 : 0)}
          detail
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
              <AttributeRows
                attributes={metric.resource.attributes}
                owner="resource"
              />
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup
          label="Scope"
          count={metric.scope.attributes.length +
            (metric.scope.name ? 1 : 0) +
            (metric.scope.version ? 1 : 0) +
            (metric.scopeDroppedAttributesCount > 0 ? 1 : 0)}
          detail
          bind:open={scopeOpen}
        >
          <table class="detail-fields w-full" aria-label="Scope attributes">
            <tbody>
              {#if metric.scope.name}<MetricField
                  fieldName="name"
                  fieldValue={metric.scope.name}
                  fieldType="string"
                />{/if}
              {#if metric.scope.version}<MetricField
                  fieldName="version"
                  fieldValue={metric.scope.version}
                  fieldType="string"
                />{/if}
              {#if metric.scopeDroppedAttributesCount > 0}
                <MetricField
                  fieldName="dropped attributes"
                  fieldValue={metric.scopeDroppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              {/if}
              <AttributeRows
                attributes={metric.scope.attributes}
                owner="scope"
              />
            </tbody>
          </table>
        </FieldGroup>
      {:else}
        <TimeseriesPanel />
      {/if}
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

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
</style>
