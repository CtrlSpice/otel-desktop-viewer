<script module lang="ts">
  import type { AttributeScope, FieldDefinition } from '@/search/model'

  /** When empty, show all detail rows; otherwise only matching search fields / attributes. */
  export function detailSearchFieldVisible(
    selected: FieldDefinition[],
    searchFieldName: string
  ): boolean {
    if (selected.length === 0) return true
    return selected.some(
      f =>
        f.searchScope === 'field' && 'name' in f && f.name === searchFieldName
    )
  }

  export function detailAttributeVisible(
    selected: FieldDefinition[],
    key: string,
    attributeScope: AttributeScope
  ): boolean {
    if (selected.length === 0) return true
    return selected.some(
      f =>
        f.searchScope === 'attribute' &&
        'name' in f &&
        'attributeScope' in f &&
        f.name === key &&
        f.attributeScope === attributeScope
    )
  }

  /** Duration is not a search field; tie visibility to start/end time columns. */
  export function detailDurationVisible(selected: FieldDefinition[]): boolean {
    if (selected.length === 0) return true
    return (
      detailSearchFieldVisible(selected, 'startTime') ||
      detailSearchFieldVisible(selected, 'endTime')
    )
  }

  /**
   * Flags is not a search field either; tie visibility to the span identity
   * columns it qualifies. Making it searchable means adding it to the field
   * registry, which is a query-grammar change and a separate decision.
   */
  export function detailFlagsVisible(selected: FieldDefinition[]): boolean {
    if (selected.length === 0) return true
    return (
      detailSearchFieldVisible(selected, 'spanID') ||
      detailSearchFieldVisible(selected, 'parentSpanID')
    )
  }
</script>

<script lang="ts">
  import { tick, untrack } from 'svelte'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import type { SpanData, TraceLogSummary } from '@/types/api-types'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    BiohazardIcon,
    BarChartHorizontalIcon,
    Link01Icon,
  } from '@hugeicons/core-free-icons'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailViews from '@/components/shared/DetailViews.svelte'
  import DetailViewTabs, {
    type DetailViewMode,
  } from '@/components/shared/DetailViewTabs.svelte'
  import { LogIcon } from '@/icons'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import SpanField from '@/components/shared/DetailProperty.svelte'
  import ActivityPanel from './ActivityPanel.svelte'
  import LinksPanel from './LinksPanel.svelte'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
  import { formatDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import {
    isPlainLeftClick,
    itemHref,
    setSpanInQuery,
    SPAN_PARAM,
  } from '@/route'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import { spanCopyJSON, spanExportURL } from './span-copy'
  import {
    revealDetailField,
    type DetailFieldTarget,
  } from '@/components/shared/utils/reveal-detail-field'

  type Props = {
    span: SpanData | undefined
    rootSpanID?: string
    fieldTarget?: DetailFieldTarget
    /** Node-level flags from the salvage walk; absent on healthy traces. */
    salvaged?: boolean
    cyclePoint?: boolean
    selectedEventIndex?: number | null
    selectedLogRef?: string | null
    logs?: TraceLogSummary[]
    /** Empty: show all Fields rows. Non-empty: only selected search fields / attributes. */
    columnFilter?: FieldDefinition[]
  }

  let {
    span,
    rootSpanID,
    fieldTarget,
    salvaged = false,
    cyclePoint = false,
    selectedEventIndex = null,
    selectedLogRef = null,
    logs = [],
    columnFilter = [],
  }: Props = $props()

  let timeContext = getTimeContext()
  $effect(() => {
    const eventIndex = selectedEventIndex
    const logRef = selectedLogRef
    void detailPanel.revealRequest
    if (!span || (eventIndex === null && logRef === null)) return

    const target = document.getElementById(
      logRef !== null ? `span-log-${logRef}` : `span-event-${eventIndex}`
    )
    untrack(() => {
      const disclosure = target?.closest('details')
      if (disclosure) disclosure.open = true
    })
    let cancelled = false
    void tick().then(() => {
      if (!cancelled) target?.scrollIntoView({ block: 'start' })
    })
    return () => {
      cancelled = true
    }
  })
  const detailViewID = $props.id()
  let detailView = $state<DetailViewMode>('fields')

  function requestedField(key: string, source = 'span') {
    return (
      fieldTarget?.kind === 'field' &&
      fieldTarget.key === key &&
      fieldTarget.source === source
    )
  }

  let isRoot = $derived(!span?.parentSpanID)
  const loadSpanJSON = $derived.by(() => {
    const selected = span
    return selected
      ? (signal: AbortSignal) => spanCopyJSON(selected, signal)
      : undefined
  })
  let durationLabel = $derived(
    span ? formatDuration(span.endTime - span.startTime) : ''
  )
  let spanAttributes = $derived(span?.attributes ?? [])
  let resourceAttributes = $derived(span?.resource.attributes ?? [])
  let scopeAttributes = $derived(span?.scope.attributes ?? [])
  let numEvents = $derived(span?.events.length ?? 0)
  let numCombinedEvents = $derived(numEvents + logs.length)
  let numLinks = $derived(span?.links.length ?? 0)
  let hasUnrecognisedStatusCode = $derived(
    span !== undefined &&
      span.statusCodeValue !== 0 &&
      span.statusCodeValue !== 1 &&
      span.statusCodeValue !== 2
  )
</script>

{#snippet spanIcon()}<HugeiconsIcon
    icon={BarChartHorizontalIcon}
    size={16}
    class="shrink-0 text-subtle"
    strokeWidth={1.5}
  />{/snippet}

{#snippet eventsIcon()}<LogIcon
    class="size-4 shrink-0 text-subtle"
    aria-hidden="true"
  />{/snippet}
{#snippet linksIcon()}<HugeiconsIcon
    icon={Link01Icon}
    size={16}
    class="shrink-0 text-subtle"
    strokeWidth={1.5}
  />{/snippet}

{#snippet spanSection()}
  <DetailBubble label="Span" icon={spanIcon}>
    {#snippet labelSuffix()}{#if span && isRoot}<span class="text-secondary"
          >· root</span
        >{/if}{/snippet}
    {#snippet action()}
      <DetailViewTabs id={detailViewID} bind:view={detailView}>
        {#snippet action()}{#if span}<ValueCopyButton
              grouped
              loadText={loadSpanJSON}
              label="span"
              tooltip="Copy span as JSON"
            />{/if}{/snippet}
      </DetailViewTabs>
    {/snippet}
    {#if span}<DetailViews
        id={detailViewID}
        showTabs={false}
        bind:view={detailView}
        wholeURL={`/export/traces/${span.traceID}`}
        scopes={[
          {
            id: 'span',
            label: 'Span',
            url: spanExportURL(span),
          },
        ]}
        reveal={fieldTarget}
      >
        {#if cyclePoint}
          <div
            class="detail-view__paradox detail-view__paradox--cycle-point"
            role="alert"
          >
            <HugeiconsIcon
              icon={BiohazardIcon}
              size="1em"
              strokeWidth={1.5}
              aria-hidden="true"
            /> Cycle detected: this span's reported parent is at or below it in the
            recovered branch, so following parent IDs would loop instead of reaching
            the trace root. Check the parent assignments in the emitting service.
          </div>
        {:else if salvaged}
          <div class="detail-view__paradox" role="alert">
            {'\u26A0'} Recovered from a broken part of this trace: a parent link forms
            a loop, so this span has no place under the root.
          </div>
        {/if}
        <table
          class="detail-fields span-fields w-full"
          aria-label="Span fields"
          data-field-source="span"
          use:revealDetailField={fieldTarget}
        >
          <tbody>
            {#if detailSearchFieldVisible(columnFilter, 'name')}
              <SpanField
                fieldName="name"
                fieldValue={span.name}
                fieldType="string"
              />
            {/if}
            {#if detailSearchFieldVisible(columnFilter, 'kind') || detailSearchFieldVisible(columnFilter, 'kindCode')}
              <SpanField
                fieldName="kind"
                fieldValue={span.kindCode.toString()}
                valueLabel={detailSearchFieldVisible(columnFilter, 'kind')
                  ? span.kind
                  : undefined}
                fieldType="enum"
              />
            {/if}
            {#if detailSearchFieldVisible(columnFilter, 'startTime')}
              <SpanField
                fieldName="startTimeUnixNano"
                fieldValue={span.startTime.toString()}
                timestampLabel={formatTimestamp(
                  span.startTime,
                  timeContext.tz,
                  'milliseconds'
                )}
                fieldType="fixed64"
              />
            {/if}
            {#if detailSearchFieldVisible(columnFilter, 'endTime')}
              <SpanField
                fieldName="endTimeUnixNano"
                fieldValue={span.endTime.toString()}
                timestampLabel={formatTimestamp(
                  span.endTime,
                  timeContext.tz,
                  'milliseconds'
                )}
                fieldType="fixed64"
              />
            {/if}
            {#if detailDurationVisible(columnFilter)}
              <SpanField
                fieldName="duration"
                fieldValue={durationLabel}
                fieldType="string"
              />
            {/if}
            {#if detailSearchFieldVisible(columnFilter, 'statusCode') || detailSearchFieldVisible(columnFilter, 'statusCodeValue')}
              <SpanField
                fieldName="status.code"
                valueTone={span.statusCodeValue === 2 ? 'error' : undefined}
                fieldValue={span.statusCodeValue.toString()}
                valueLabel={detailSearchFieldVisible(columnFilter, 'statusCode')
                  ? span.statusCode
                  : undefined}
                fieldType="enum"
                defectLabel={hasUnrecognisedStatusCode
                  ? 'Unrecognised status code'
                  : undefined}
              />
            {/if}
            {#if requestedField('status.message') || (span.statusCodeValue !== 0 && span.statusCodeValue !== 1 && detailSearchFieldVisible(columnFilter, 'statusMessage'))}
              <SpanField
                fieldName="status.message"
                valueTone={span.statusCodeValue === 2 ? 'error' : undefined}
                fieldValue={span.statusMessage}
                fieldType="string"
              />
            {/if}
            {#if requestedField('traceState') || (span.traceState && detailSearchFieldVisible(columnFilter, 'traceState'))}
              <SpanField
                fieldName="traceState"
                fieldValue={span.traceState}
                fieldType="string"
              />
            {/if}
            {#if detailSearchFieldVisible(columnFilter, 'traceID')}
              <SpanField
                fieldName="traceId"
                identifier
                fieldValue={span.traceID}
                fieldType="bytes"
              >
                {#snippet value()}
                  {#if rootSpanID}
                    <a
                      class="detail-cell__value link link-primary"
                      href={itemHref('traces', span.traceID, {
                        [SPAN_PARAM]: rootSpanID,
                      })}
                      aria-label={`Open root span in trace ${span.traceID}`}
                      onclick={event => {
                        if (!isPlainLeftClick(event)) return
                        event.preventDefault()
                        setSpanInQuery(rootSpanID, 'push')
                      }}><Identifier value={span.traceID} /></a
                    >
                  {:else}<Identifier value={span.traceID} />{/if}
                {/snippet}
              </SpanField>
            {/if}
            {#if !isRoot && detailSearchFieldVisible(columnFilter, 'parentSpanID')}
              <SpanField
                fieldName="parentSpanId"
                fieldType="bytes"
                fieldValue={span.parentSpanID ?? ''}
              >
                {#snippet value()}
                  <button
                    type="button"
                    class="detail-cell__value link link-primary"
                    onclick={() => setSpanInQuery(span.parentSpanID!, 'push')}
                    ><Identifier value={span.parentSpanID ?? ''} /></button
                  >
                {/snippet}
              </SpanField>
            {/if}
            {#if detailSearchFieldVisible(columnFilter, 'spanID')}
              <SpanField
                fieldName="spanId"
                identifier
                fieldValue={span.spanID}
                fieldType="bytes"
              />
            {/if}
            {#if requestedField('flags') || (span.flags > 0 && detailFlagsVisible(columnFilter))}
              <SpanField
                fieldName="flags"
                fieldValue={span.flags.toString()}
                fieldType="uint32"
              />
            {/if}
            <AttributeRows
              attributes={spanAttributes.filter(attr =>
                detailAttributeVisible(columnFilter, attr.key, 'span')
              )}
              owner="span"
            />
            {#if requestedField('droppedAttributesCount') || (span.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedAttributesCount'))}
              <SpanField
                fieldName="droppedAttributesCount"
                fieldValue={span.droppedAttributesCount.toString()}
                fieldType="uint32"
              />
            {/if}
            {#if requestedField('droppedEventsCount') || (span.droppedEventsCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedEventsCount'))}
              <SpanField
                fieldName="droppedEventsCount"
                fieldValue={span.droppedEventsCount.toString()}
                fieldType="uint32"
              />
            {/if}
            {#if requestedField('droppedLinksCount') || (span.droppedLinksCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedLinksCount'))}
              <SpanField
                fieldName="droppedLinksCount"
                fieldValue={span.droppedLinksCount.toString()}
                fieldType="uint32"
              />
            {/if}
            <AttributeRows
              attributes={resourceAttributes.filter(attr =>
                detailAttributeVisible(columnFilter, attr.key, 'resource')
              )}
              owner="resource"
              origin="resource"
            />
            {#if requestedField('droppedAttributesCount', 'resource') || (span.resource.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'resource.droppedAttributesCount'))}
              <SpanField
                fieldName="droppedAttributesCount"
                origin="resource"
                fieldValue={span.resource.droppedAttributesCount.toString()}
                fieldType="uint32"
              />
            {/if}
            {#if requestedField('name', 'scope') || (span.scope.name && detailSearchFieldVisible(columnFilter, 'scope.name'))}
              <SpanField
                fieldName="name"
                origin="scope"
                fieldValue={span.scope.name}
                fieldType="string"
              />
            {/if}
            {#if requestedField('version', 'scope') || (span.scope.version && detailSearchFieldVisible(columnFilter, 'scope.version'))}
              <SpanField
                fieldName="version"
                origin="scope"
                fieldValue={span.scope.version}
                fieldType="string"
              />
            {/if}
            <AttributeRows
              attributes={scopeAttributes.filter(attr =>
                detailAttributeVisible(columnFilter, attr.key, 'scope')
              )}
              owner="scope"
              origin="scope"
            />
            {#if requestedField('droppedAttributesCount', 'scope') || (span.scope.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'scope.droppedAttributesCount'))}
              <SpanField
                fieldName="droppedAttributesCount"
                origin="scope"
                fieldValue={span.scope.droppedAttributesCount.toString()}
                fieldType="uint32"
              />
            {/if}
          </tbody>
        </table>
      </DetailViews>{:else}<p class="p-3 text-subtle">No span selected</p>{/if}
  </DetailBubble>
{/snippet}
{#snippet eventsSection()}
  <DetailBubble label="Events" icon={eventsIcon} count={numCombinedEvents}>
    {#if numCombinedEvents === 0}
      <p class="detail-view__section-empty">No events recorded for this span</p>
    {:else if span}
      <ActivityPanel
        spanKey={`${span.traceID}/${span.spanID}`}
        events={span.events}
        {logs}
        spanStartTime={span.startTime}
        {selectedEventIndex}
        {selectedLogRef}
      />
    {/if}
  </DetailBubble>
{/snippet}
{#snippet linksSection()}
  <DetailBubble label="Links" icon={linksIcon} count={numLinks}>
    {#if numLinks === 0}
      <p class="detail-view__section-empty">No links recorded for this span</p>
    {:else if span}
      <LinksPanel
        links={span.links}
        spanKey={`${span.traceID}/${span.spanID}`}
      />
    {/if}
  </DetailBubble>
{/snippet}
<DetailStack
  label="Span details"
  storageKey="trace-detail-section-sizes"
  sections={[
    { id: 'span', content: spanSection },
    { id: 'events', content: eventsSection },
    { id: 'links', content: linksSection },
  ]}
/>

<style lang="postcss">
  @reference "../../../app.css";

  /* Repeats the waterfall's cycle marks where the reader is actually
     looking. Same split as the rows: warning gold for every stranded span,
     error red for the retained cycle point. */
  .detail-view__paradox {
    @apply m-2 rounded px-3 py-2 text-xs text-base-content;
    background: color-mix(in srgb, var(--color-warning) 12%, transparent);
  }

  .detail-view__paradox--cycle-point {
    @apply text-error;
    background: color-mix(in srgb, var(--color-error) 12%, transparent);
  }

  /* The icon component's SVG is outside scoped selectors. */
  .detail-view__paradox :global(svg) {
    display: inline-block;
    vertical-align: -0.125em;
  }

  .detail-view__section-empty {
    @apply m-0 px-3 py-6 text-center text-sm italic;
    color: var(--color-muted);
  }
</style>
