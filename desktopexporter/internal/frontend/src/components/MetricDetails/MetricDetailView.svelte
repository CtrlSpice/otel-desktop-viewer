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
  import PaneHeader, { type PaneTab } from '@/components/PaneHeader.svelte'
  import FieldGroup from '@/components/FieldGroup.svelte'
  import { LeftToRightListBulletIcon, FocusPointIcon } from '@/icons'
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
    return m.resource.attributes.map((a) => ({
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
</script>

{#if !ctx.metric}
  <div class="detail-view detail-view--empty">
    <p class="text-rp-muted text-sm">Select a metric to view details</p>
  </div>
{:else}
  {@const metric = ctx.metric}

  {#snippet fieldsIcon()}<LeftToRightListBulletIcon />{/snippet}
  {#snippet datapointsIcon()}<FocusPointIcon />{/snippet}

  {@const tabs: PaneTab[] = [
    { id: 'fields', label: 'Fields', icon: fieldsIcon },
    { id: 'datapoints', label: 'Datapoints', icon: datapointsIcon },
  ]}

  <div class="detail-view">
    <PaneHeader
      mode="tabs"
      {tabs}
      activeId={activeTab}
      onSelect={(id) => (activeTab = id as 'fields' | 'datapoints')}
      ariaLabel="Metric detail tabs"
    />

    <div class="detail-view__scroll">
      {#if activeTab === 'fields'}
        <FieldGroup label="Metric" count={metricFieldCount} bind:open={metricOpen}>
          <table class="detail-fields w-full" aria-label="Metric fields">
            <tbody>
              <MetricField fieldName="name" fieldValue={metric.name} fieldType="string" />
              {#if metric.description}
                <MetricField fieldName="description" fieldValue={metric.description} fieldType="string" />
              {/if}
              <MetricField fieldName="type" fieldValue={ctx.metricType} fieldType="string" />
              {#if metric.unit}
                <MetricField fieldName="unit" fieldValue={metric.unit} fieldType="string" />
              {/if}
              {#if ctx.temporality}
                <MetricField fieldName="aggregation temporality" fieldValue={ctx.temporality} fieldType="string" />
              {/if}
              {#if ctx.isMonotonic !== null}
                <MetricField fieldName="is monotonic" fieldValue={String(ctx.isMonotonic)} fieldType="bool" />
              {/if}
              <MetricField
                fieldName="received"
                fieldValue={formatTimestamp(metric.received, timeContext.timezone, 'milliseconds')}
                fieldType="timestamp"
              />
              <MetricField fieldName="datapoint count" fieldValue={ctx.totalDatapointCount.toString()} fieldType="uint32" />
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup label="Resource" count={resourceAttrs.length} bind:open={resourceOpen}>
          <table class="detail-fields w-full" aria-label="Resource attributes">
            <tbody>
              {#if metric.resourceDroppedAttributesCount > 0}
                <MetricField fieldName="dropped attributes" fieldValue={metric.resourceDroppedAttributesCount.toString()} fieldType="uint32" />
              {/if}
              {#each resourceAttrs as attr (`resource:${attr.key}`)}
                <MetricField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} />
              {/each}
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup label="Scope" count={scopeAttrs.length} bind:open={scopeOpen}>
          <table class="detail-fields w-full" aria-label="Scope attributes">
            <tbody>
              {#each scopeAttrs as attr (`scope:${attr.key}`)}
                <MetricField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} />
              {/each}
            </tbody>
          </table>
        </FieldGroup>
      {:else}
        <section class="detail-view__section" aria-labelledby="metric-datapoints-heading">
          <h2 id="metric-datapoints-heading" class="detail-view__section-heading">
            Datapoints
          </h2>
          <p class="detail-view__section-empty">
            No datapoints yet
          </p>
        </section>
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
    @apply m-0 text-xs font-semibold uppercase tracking-wide;
    color: var(--color-subtle);
  }

  .detail-view__section-empty {
    @apply m-0 text-sm italic;
    color: var(--color-muted);
  }
</style>
