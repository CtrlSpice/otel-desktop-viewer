<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { LogData } from '@/types/api-types'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import LogField from './LogField.svelte'
  import { severityLabel as buildSeverityLabel } from '@/components/logs/log-severity'
  import { getServiceName } from '@/utils/resource'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { navigateToItem } from '@/route'

  type Props = {
    log: LogData | undefined
    footer?: Snippet
  }

  let { log, footer }: Props = $props()

  let timeContext = getTimeContext()

  let service = $derived(log ? (getServiceName(log.resource) ?? '') : '')

  let severityLabel = $derived(
    log
      ? `${buildSeverityLabel(log.severityText, log.severityNumber)} (${log.severityNumber})`
      : '',
  )

  let logOpen = $state(true)
  let resourceOpen = $state(true)
  let scopeOpen = $state(true)

  let logFieldCount = $derived.by(() => {
    if (!log) return 0
    let n = 4
    if (log.traceID) n++
    if (log.spanID) n++
    if (log.body) n++
    if (log.eventName) n++
    if (log.droppedAttributesCount > 0) n++
    n += log.attributes.length
    return n
  })

  let resourceFieldCount = $derived.by(() => {
    if (!log) return 0
    let n = log.resource.attributes.length
    if (log.resource.droppedAttributesCount > 0) n++
    return n
  })

  let scopeFieldCount = $derived.by(() => {
    if (!log) return 0
    let n = 0
    if (log.scope.name) n++
    if (log.scope.version) n++
    n += log.scope.attributes.length
    if (log.scope.droppedAttributesCount > 0) n++
    return n
  })
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
    </PaneHeader>

    <div class="log-detail-panel__scroll">
      <FieldGroup label="Log" count={logFieldCount} bind:open={logOpen}>
        <table class="detail-fields w-full" aria-label="Log fields">
          <tbody>
            <LogField
              fieldName="timestamp"
              fieldType="timestamp"
              fieldValue={formatTimestamp(
                log.timestamp,
                timeContext.tz,
                'nanoseconds'
              )}
            />
            <LogField
              fieldName="observed timestamp"
              fieldType="timestamp"
              fieldValue={formatTimestamp(
                log.observedTimestamp,
                timeContext.tz,
                'nanoseconds'
              )}
            />
            <LogField
              fieldName="severity"
              fieldType="enum"
              fieldValue={severityLabel}
            />
            {#if log.traceID}
              <LogField fieldName="trace id" fieldType="string">
                {#snippet value()}
                  <a
                    class="detail-cell__value link link-primary font-mono"
                    href="/traces/{log.traceID}"
                    onclick={e => {
                      e.preventDefault()
                      navigateToItem('traces', log.traceID)
                    }}
                  >{log.traceID}</a>
                {/snippet}
              </LogField>
            {/if}
            {#if log.spanID}
              <LogField
                fieldName="span id"
                fieldType="string"
                fieldValue={log.spanID}
              />
            {/if}
            {#if log.body}
              <LogField
                fieldName="body"
                fieldType={log.bodyType}
                fieldValue={log.body}
                multiline
              />
            {/if}
            {#if log.eventName}
              <LogField
                fieldName="event name"
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
                fieldName="dropped attributes count"
                fieldType="uint32"
                fieldValue={String(log.droppedAttributesCount)}
              />
            {/if}
            {#each log.attributes as attr (attr.key)}
              <LogField
                fieldName={attr.key}
                fieldValue={attr.value}
                fieldType={attr.type}
              />
            {/each}
          </tbody>
        </table>
      </FieldGroup>

      <FieldGroup
        label="Resource"
        count={resourceFieldCount}
        bind:open={resourceOpen}
      >
        <table class="detail-fields w-full" aria-label="Resource attributes">
          <tbody>
            {#each log.resource.attributes as attr (attr.key)}
              <LogField
                fieldName={attr.key}
                fieldValue={attr.value}
                fieldType={attr.type}
              />
            {/each}
            {#if log.resource.droppedAttributesCount > 0}
              <LogField
                fieldName="dropped attributes count"
                fieldType="uint32"
                fieldValue={String(log.resource.droppedAttributesCount)}
              />
            {/if}
          </tbody>
        </table>
      </FieldGroup>

      <FieldGroup label="Scope" count={scopeFieldCount} bind:open={scopeOpen}>
        <table class="detail-fields w-full" aria-label="Scope attributes">
          <tbody>
            {#if log.scope.name}
              <LogField
                fieldName="scope name"
                fieldType="string"
                fieldValue={log.scope.name}
              />
            {/if}
            {#if log.scope.version}
              <LogField
                fieldName="scope version"
                fieldType="string"
                fieldValue={log.scope.version}
              />
            {/if}
            {#each log.scope.attributes as attr (attr.key)}
              <LogField
                fieldName={attr.key}
                fieldValue={attr.value}
                fieldType={attr.type}
              />
            {/each}
            {#if log.scope.droppedAttributesCount > 0}
              <LogField
                fieldName="dropped attributes count"
                fieldType="uint32"
                fieldValue={String(log.scope.droppedAttributesCount)}
              />
            {/if}
          </tbody>
        </table>
      </FieldGroup>
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
