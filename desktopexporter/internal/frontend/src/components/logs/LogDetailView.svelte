<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { LogData } from '@/types/api-types'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import ServiceDot from '@/components/shared/ServiceDot.svelte'
  import ExportButton from '@/components/shared/ExportButton.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { readOTLPText } from '@/services/export-service'
  import DetailViews from '@/components/shared/DetailViews.svelte'
  import DetailViewTabs, {
    type DetailViewMode,
  } from '@/components/shared/DetailViewTabs.svelte'
  import ScrollToTopButton from '@/components/shared/ScrollToTopButton.svelte'
  import LogField from '@/components/shared/DetailProperty.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import AssociatedTrace from './AssociatedTrace.svelte'
  import { severityLabel as buildSeverityLabel } from '@/components/logs/log-severity'
  import LogSeverity from './LogSeverity.svelte'
  import { getServiceName } from '@/utils/resource'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import {
    revealDetailField,
    type DetailFieldTarget,
  } from '@/components/shared/utils/reveal-detail-field'

  type Props = {
    log: LogData | undefined
    footer?: Snippet
    fieldTarget?: DetailFieldTarget
    detailView?: DetailViewMode
    loading?: boolean
    error?: string
  }

  let {
    log,
    footer,
    fieldTarget,
    detailView = $bindable<DetailViewMode>('fields'),
    loading = false,
    error = '',
  }: Props = $props()
  const detailViewID = $props.id()
  const exportURL = $derived(
    log ? `/export/logs/${encodeURIComponent(log.logRef)}` : ''
  )
  const loadLogJSON = $derived.by(() => {
    const id = log?.logRef
    return id
      ? (signal: AbortSignal) => readOTLPText('log', id, signal)
      : undefined
  })
  let resolvedTarget = $derived(
    fieldTarget?.source === 'log' &&
      fieldTarget.kind === 'field' &&
      fieldTarget.key === 'timestamp' &&
      log
      ? {
          ...fieldTarget,
          key: log.timestamp === 0n ? 'observedTimeUnixNano' : 'timeUnixNano',
        }
      : fieldTarget
  )

  let timeContext = getTimeContext()
  let scrollEl = $state<HTMLDivElement | null>(null)
  let scrolled = $state(false)

  function scrollToTop() {
    scrollEl?.scrollTo({
      top: 0,
      behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches
        ? 'auto'
        : 'smooth',
    })
  }

  $effect(() => {
    const scroll = scrollEl
    if (!scroll) {
      scrolled = false
      return
    }
    const update = () => {
      scrolled = scroll.scrollTop > 1
    }
    update()
    scroll.addEventListener('scroll', update)
    return () => scroll.removeEventListener('scroll', update)
  })

  let service = $derived(log ? (getServiceName(log.resource) ?? '') : '')

  let severityLabel = $derived(
    log ? buildSeverityLabel(log.severityText, log.severityNumber) : ''
  )
</script>

<div
  class="log-detail-panel"
  data-field-source="log"
  use:revealDetailField={resolvedTarget}
>
  <PaneHeader
    pill
    mode="title"
    title={service || 'Log record'}
    ariaLabel="Log record"
  >
    {#snippet leading()}{#if log}<ServiceDot {service} />{/if}{/snippet}
    {#snippet right()}
      <DetailViewTabs id={detailViewID} bind:view={detailView} />
      {#if log}{#key log.logRef}
          <ValueCopyButton
            label="log"
            tooltip="Copy log"
            loadText={loadLogJSON}
          />
          <ExportButton signal="log" id={log.logRef} />
        {/key}{/if}
    {/snippet}
  </PaneHeader>

  {#if log}
    {#if log.traceID}
      <div class="log-detail-panel__trace">
        <AssociatedTrace traceID={log.traceID} spanID={log.spanID} />
      </div>
    {/if}

    <div class="log-detail-panel__body">
      <div class="log-detail-panel__scroll" bind:this={scrollEl}>
        <DetailViews
          id={detailViewID}
          showTabs={false}
          bind:view={detailView}
          wholeURL={exportURL}
          scopes={[
            {
              id: 'log',
              label: 'Log',
              url: exportURL,
            },
          ]}
          reveal={resolvedTarget}
        >
          <table
            class="detail-fields w-full"
            aria-label="Log fields"
            data-field-source="log"
          >
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
              >
                {#snippet value()}<LogSeverity
                    number={log.severityNumber}
                    text={log.severityText}
                    >{log.severityText === ''
                      ? '""'
                      : log.severityText}</LogSeverity
                  >{/snippet}
              </LogField>
              <LogField
                fieldName="severityNumber"
                fieldType="enum"
                fieldValue={String(log.severityNumber)}
              >
                {#snippet value()}<LogSeverity
                    number={log.severityNumber}
                    text={log.severityText}
                    >{log.severityNumber} ({severityLabel})</LogSeverity
                  >{/snippet}
              </LogField>
              {#if log.spanID && !log.traceID}
                <LogField
                  identifier
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
              <LogField
                fieldName="eventName"
                fieldType="string"
                fieldValue={log.eventName}
              />
              <LogField
                fieldName="flags"
                fieldType="uint32"
                fieldValue={String(log.flags)}
              />
              <LogField
                fieldName="droppedAttributesCount"
                fieldType="uint32"
                fieldValue={String(log.droppedAttributesCount)}
              />
              <AttributeRows attributes={log.attributes} owner="log" />
              <AttributeRows
                attributes={log.resource.attributes}
                owner="resource"
                origin="resource"
              />
              <LogField
                fieldName="droppedAttributesCount"
                origin="resource"
                fieldType="uint32"
                fieldValue={String(log.resource.droppedAttributesCount)}
              />
              {#if log.resourceSchemaURL !== undefined}
                <LogField
                  fieldName="schemaUrl"
                  origin="resource"
                  fieldType="string"
                  fieldValue={log.resourceSchemaURL}
                />
              {/if}
              <LogField
                fieldName="name"
                origin="scope"
                fieldType="string"
                fieldValue={log.scope.name}
              />
              <LogField
                fieldName="version"
                origin="scope"
                fieldType="string"
                fieldValue={log.scope.version}
              />
              {#if log.scopeSchemaURL !== undefined}
                <LogField
                  fieldName="schemaUrl"
                  origin="scope"
                  fieldType="string"
                  fieldValue={log.scopeSchemaURL}
                />
              {/if}
              <AttributeRows
                attributes={log.scope.attributes}
                owner="scope"
                origin="scope"
              />
              <LogField
                fieldName="droppedAttributesCount"
                origin="scope"
                fieldType="uint32"
                fieldValue={String(log.scope.droppedAttributesCount)}
              />
            </tbody>
          </table>
        </DetailViews>
      </div>
      <ScrollToTopButton
        visible={scrolled}
        label="Scroll log to top"
        onclick={scrollToTop}
      />
    </div>

    {#if footer}
      {@render footer()}
    {/if}
  {:else if error}<p class="p-3" role="alert">Error: {error}</p>
  {:else}<p class="p-3 text-subtle" role={loading ? 'status' : undefined}>
      {loading ? 'Loading log details…' : 'Select a log to view details'}
    </p>{/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .log-detail-panel {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden;
  }

  .log-detail-panel__scroll {
    @apply flex-1 min-h-0 overflow-y-auto;
    scrollbar-width: thin;
  }

  .log-detail-panel__trace {
    @apply shrink-0;
  }

  .log-detail-panel__body {
    @apply relative flex min-h-0 flex-1 flex-col;
  }
</style>
