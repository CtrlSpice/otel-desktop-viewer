<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { LogData } from '@/types/api-types'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import ExportButton from '@/components/shared/ExportButton.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import LogField from '@/components/shared/DetailProperty.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import { severityLabel as buildSeverityLabel } from '@/components/logs/log-severity'
  import { getServiceName } from '@/utils/resource'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { itemHref, navigateToItem } from '@/route'
  import { SPAN_PARAM } from '@/route/query-params'
  import { detailPanel } from '@/state/detail-panel.svelte'

  type Props = {
    log: LogData | undefined
    footer?: Snippet
  }

  let { log, footer }: Props = $props()

  let timeContext = getTimeContext()

  let service = $derived(log ? (getServiceName(log.resource) ?? '') : '')

  let severityLabel = $derived(
    log ? buildSeverityLabel(log.severityText, log.severityNumber) : ''
  )

  let traceSpanPatch = $derived(
    log?.traceID && log?.spanID
      ? ({ [SPAN_PARAM]: log.spanID } as const)
      : undefined
  )

  function goToTrace(e: MouseEvent) {
    if (!log?.traceID) return
    detailPanel.reveal()
    e.preventDefault()
    navigateToItem('traces', log.traceID, 'push', traceSpanPatch)
  }
</script>

{#if log}
  <div class="log-detail-panel">
    <PaneHeader
      mode="title"
      title={service || '—'}
      timestampMs={Number(log.timestamp / 1_000_000n)}
      ariaLabel="Log record"
    >
      {#snippet badge()}
        <SignalBadges
          signal="log"
          severityNumber={log.severityNumber}
          severityText={log.severityText}
        />
      {/snippet}
      {#snippet leading()}
        {#key log.logRef}
          <ExportButton signal="log" id={log.logRef} />
        {/key}
      {/snippet}
    </PaneHeader>

    <div class="log-detail-panel__scroll">
      <table class="detail-fields w-full" aria-label="Log fields">
        <tbody>
          <LogField
            fieldName="timeUnixNano"
            fieldType="fixed64"
            fieldValue={log.timestamp.toString()}
            timestampLabel={formatTimestamp(
              log.timestamp,
              timeContext.tz,
              'milliseconds'
            )}
          />
          <LogField
            fieldName="observedTimeUnixNano"
            fieldType="fixed64"
            fieldValue={log.observedTimestamp.toString()}
            timestampLabel={formatTimestamp(
              log.observedTimestamp,
              timeContext.tz,
              'milliseconds'
            )}
          />
          <LogField
            fieldName="severityText"
            fieldType="string"
            fieldValue={log.severityText}
          />
          <LogField
            fieldName="severityNumber"
            fieldType="enum"
            fieldValue={String(log.severityNumber)}
            valueLabel={severityLabel}
          />
          {#if log.traceID}
            {@const traceID = log.traceID}
            <LogField
              fieldName="traceId"
              fieldType="bytes"
              fieldValue={traceID}
            >
              {#snippet value()}
                <a
                  class="detail-cell__value link link-primary font-mono"
                  href={itemHref('traces', traceID, traceSpanPatch)}
                  onclick={goToTrace}>{traceID}</a
                >
              {/snippet}
            </LogField>
          {/if}
          {#if log.spanID && log.traceID}
            {@const traceID = log.traceID}
            <LogField
              fieldName="spanId"
              fieldType="bytes"
              fieldValue={log.spanID}
            >
              {#snippet value()}
                <a
                  class="detail-cell__value link link-primary font-mono"
                  href={itemHref('traces', traceID, traceSpanPatch)}
                  onclick={goToTrace}>{log.spanID}</a
                >
              {/snippet}
            </LogField>
          {:else if log.spanID}
            <LogField
              fieldName="spanId"
              fieldType="bytes"
              fieldValue={log.spanID}
            />
          {/if}
          {#if log.body}
            <LogField
              fieldName="body"
              fieldType={log.body.kind}
              typedValue={log.body}
            />
          {/if}
          {#if log.eventName}
            <LogField
              fieldName="eventName"
              fieldType="string"
              fieldValue={log.eventName}
            />
          {/if}
          <LogField
            fieldName="flags"
            fieldType="uint32"
            fieldValue={String(log.flags)}
          />
          {#if log.droppedAttributesCount > 0}
            <LogField
              fieldName="droppedAttributesCount"
              fieldType="uint32"
              fieldValue={String(log.droppedAttributesCount)}
            />
          {/if}
          <AttributeRows attributes={log.attributes} owner="log" />
          <AttributeRows
            attributes={log.resource.attributes}
            owner="resource"
            origin="resource"
          />
          {#if log.resource.droppedAttributesCount > 0}
            <LogField
              fieldName="droppedAttributesCount"
              origin="resource"
              fieldType="uint32"
              fieldValue={String(log.resource.droppedAttributesCount)}
            />
          {/if}
          {#if log.scope.name}
            <LogField
              fieldName="name"
              origin="scope"
              fieldType="string"
              fieldValue={log.scope.name}
            />
          {/if}
          {#if log.scope.version}
            <LogField
              fieldName="version"
              origin="scope"
              fieldType="string"
              fieldValue={log.scope.version}
            />
          {/if}
          <AttributeRows
            attributes={log.scope.attributes}
            owner="scope"
            origin="scope"
          />
          {#if log.scope.droppedAttributesCount > 0}
            <LogField
              fieldName="droppedAttributesCount"
              origin="scope"
              fieldType="uint32"
              fieldValue={String(log.scope.droppedAttributesCount)}
            />
          {/if}
        </tbody>
      </table>
    </div>

    {#if footer}
      {@render footer()}
    {/if}
  </div>
{:else}
  <div class="log-detail-panel log-detail-panel--empty">
    <p class="text-rp-muted text-sm">Select a log to view details</p>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .log-detail-panel {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden;
  }

  .log-detail-panel--empty {
    @apply items-center justify-center;
  }

  .log-detail-panel__scroll {
    @apply flex-1 min-h-0 overflow-y-auto;
    scrollbar-width: thin;
  }
</style>
