<script module lang="ts">
  import type { AttributeScope, FieldDefinition } from '@/constants/fields'

  /** When empty, show all detail rows; otherwise only matching search fields / attributes. */
  export function detailSearchFieldVisible(
    selected: FieldDefinition[],
    searchFieldName: string
  ): boolean {
    if (selected.length === 0) return true;
    return selected.some(
      f =>
        f.searchScope === 'field' &&
        'name' in f &&
        f.name === searchFieldName
    );
  }

  export function detailAttributeVisible(
    selected: FieldDefinition[],
    key: string,
    attributeScope: AttributeScope
  ): boolean {
    if (selected.length === 0) return true;
    return selected.some(
      f =>
        f.searchScope === 'attribute' &&
        'name' in f &&
        'attributeScope' in f &&
        f.name === key &&
        f.attributeScope === attributeScope
    );
  }

  /** Duration is not a search field; tie visibility to start/end time columns. */
  export function detailDurationVisible(selected: FieldDefinition[]): boolean {
    if (selected.length === 0) return true;
    return (
      detailSearchFieldVisible(selected, 'startTime') ||
      detailSearchFieldVisible(selected, 'endTime')
    );
  }
</script>

<script lang="ts">
  import type { SpanData } from '@/types/api-types'
  import PaneHeader, { type PaneTab } from '@/components/PaneHeader.svelte'
  import FieldGroup from '@/components/FieldGroup.svelte'
  import SpanField from './SpanField.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import LinksPanel from './LinksPanel.svelte'
  import { formatDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import {
    LeftToRightListBulletIcon,
    LinkIcon,
    StopWatchIcon,
  } from '@/icons'

  type Props = {
    span: SpanData | undefined
    /** Empty: show all Fields rows. Non-empty: only selected search fields / attributes. */
    columnFilter?: FieldDefinition[]
  }

  let { span, columnFilter = [] }: Props = $props()

  let timeContext = getTimeContext()

  // --- Derived span data ---

  let isRoot = $derived(!span?.parentSpanID)
  let durationLabel = $derived(
    span ? formatDuration(span.endTime - span.startTime) : '',
  )
  let spanAttributes = $derived(span?.attributes ?? [])
  let resourceAttributes = $derived(span?.resource.attributes ?? [])
  let scopeAttributes = $derived(span?.scope.attributes ?? [])
  let numEvents = $derived(span?.events.length ?? 0)
  let numLinks = $derived(span?.links.length ?? 0)

  // --- Tab state ---

  type Tab = 'fields' | 'events' | 'links'
  let activeTab = $state<Tab>('fields')

  let spanOpen = $state(true)
  let resourceOpen = $state(true)
  let scopeOpen = $state(true)

  let spanFieldCount = $derived.by(() => {
    if (!span) return 0
    const f = columnFilter
    let n = 0
    if (detailSearchFieldVisible(f, 'name')) n++
    if (detailSearchFieldVisible(f, 'kind')) n++
    if (detailSearchFieldVisible(f, 'startTime')) n++
    if (detailSearchFieldVisible(f, 'endTime')) n++
    if (detailDurationVisible(f)) n++
    if (detailSearchFieldVisible(f, 'statusCode')) n++
    if (
      span.statusCode !== 'Unset' &&
      span.statusCode !== 'Ok' &&
      detailSearchFieldVisible(f, 'statusMessage')
    ) {
      n++
    }
    if (detailSearchFieldVisible(f, 'traceID')) n++
    if (!isRoot && detailSearchFieldVisible(f, 'parentSpanID')) n++
    if (detailSearchFieldVisible(f, 'spanID')) n++
    for (const attr of span.attributes) {
      if (detailAttributeVisible(f, attr.key, 'span')) n++
    }
    if (
      span.droppedAttributesCount > 0 &&
      detailSearchFieldVisible(f, 'droppedAttributesCount')
    ) {
      n++
    }
    if (
      span.droppedEventsCount > 0 &&
      detailSearchFieldVisible(f, 'droppedEventsCount')
    ) {
      n++
    }
    if (
      span.droppedLinksCount > 0 &&
      detailSearchFieldVisible(f, 'droppedLinksCount')
    ) {
      n++
    }
    return n
  })

  let resourceFieldCount = $derived.by(() => {
    if (!span) return 0
    const f = columnFilter
    let n = 0
    for (const attr of span.resource.attributes) {
      if (detailAttributeVisible(f, attr.key, 'resource')) n++
    }
    if (
      span.resource.droppedAttributesCount > 0 &&
      detailSearchFieldVisible(f, 'resource.droppedAttributesCount')
    ) {
      n++
    }
    return n
  })

  let scopeFieldCount = $derived.by(() => {
    if (!span) return 0
    const f = columnFilter
    let n = 0
    if (span.scope.name && detailSearchFieldVisible(f, 'scope.name')) n++
    if (span.scope.version && detailSearchFieldVisible(f, 'scope.version')) n++
    for (const attr of span.scope.attributes) {
      if (detailAttributeVisible(f, attr.key, 'scope')) n++
    }
    if (
      span.scope.droppedAttributesCount > 0 &&
      detailSearchFieldVisible(f, 'scope.droppedAttributesCount')
    ) {
      n++
    }
    return n
  })
</script>

{#if span}
  {#snippet fieldsIcon()}<LeftToRightListBulletIcon />{/snippet}

  {#snippet eventsIcon()}<StopWatchIcon />{/snippet}
  {#snippet linksIcon()}<LinkIcon />{/snippet}

  {@const tabs: PaneTab[] = [
    { id: 'fields', label: 'Fields', icon: fieldsIcon },
    { id: 'events', label: 'Events', icon: eventsIcon, count: numEvents },
    { id: 'links', label: 'Links', icon: linksIcon, count: numLinks },
  ]}

  <div class="detail-view">
    <PaneHeader
      mode="tabs"
      {tabs}
      activeId={activeTab}
      onSelect={id => (activeTab = id as Tab)}
      ariaLabel="Span detail tabs"
    />

    <div class="detail-view__scroll">
      {#if activeTab === 'fields'}
        <FieldGroup label="Span" count={spanFieldCount} bind:open={spanOpen}>
          <table class="detail-fields w-full" aria-label="Span fields">
            <tbody>
              {#if detailSearchFieldVisible(columnFilter, 'name')}
                <SpanField fieldName="name" fieldValue={span.name} fieldType="string" {isRoot} />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'kind')}
                <SpanField fieldName="kind" fieldValue={span.kind} fieldType="string" />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'startTime')}
                <SpanField
                  fieldName="start time"
                  fieldValue={formatTimestamp(span.startTime, timeContext.timezone, 'nanoseconds')}
                  fieldType="timestamp"
                />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'endTime')}
                <SpanField
                  fieldName="end time"
                  fieldValue={formatTimestamp(span.endTime, timeContext.timezone, 'nanoseconds')}
                  fieldType="timestamp"
                />
              {/if}
              {#if detailDurationVisible(columnFilter)}
                <SpanField fieldName="duration" fieldValue={durationLabel} fieldType="string" />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'statusCode')}
                <SpanField fieldName="status code" fieldValue={span.statusCode} fieldType="string" />
              {/if}
              {#if span.statusCode !== 'Unset' && span.statusCode !== 'Ok' && detailSearchFieldVisible(columnFilter, 'statusMessage')}
                <SpanField fieldName="status message" fieldValue={span.statusMessage} fieldType="string" />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'traceID')}
                <SpanField fieldName="trace id" fieldValue={span.traceID} fieldType="string" />
              {/if}
              {#if !isRoot && detailSearchFieldVisible(columnFilter, 'parentSpanID')}
                <SpanField fieldName="parent span id" fieldValue={span.parentSpanID ?? ''} fieldType="string" />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'spanID')}
                <SpanField fieldName="span id" fieldValue={span.spanID} fieldType="string" />
              {/if}
              {#each spanAttributes as attr (attr.key)}
                {#if detailAttributeVisible(columnFilter, attr.key, 'span')}
                  <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} />
                {/if}
              {/each}
              {#if span.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedAttributesCount')}
                <SpanField fieldName="dropped attributes count" fieldValue={span.droppedAttributesCount.toString()} fieldType="uint32" />
              {/if}
              {#if span.droppedEventsCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedEventsCount')}
                <SpanField fieldName="dropped events count" fieldValue={span.droppedEventsCount.toString()} fieldType="uint32" />
              {/if}
              {#if span.droppedLinksCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedLinksCount')}
                <SpanField fieldName="dropped links count" fieldValue={span.droppedLinksCount.toString()} fieldType="uint32" />
              {/if}
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup label="Resource" count={resourceFieldCount} bind:open={resourceOpen}>
          <table class="detail-fields w-full" aria-label="Resource attributes">
            <tbody>
              {#each resourceAttributes as attr (attr.key)}
                {#if detailAttributeVisible(columnFilter, attr.key, 'resource')}
                  <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} />
                {/if}
              {/each}
              {#if span.resource.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'resource.droppedAttributesCount')}
                <SpanField fieldName="dropped attributes count" fieldValue={span.resource.droppedAttributesCount.toString()} fieldType="uint32" />
              {/if}
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup label="Scope" count={scopeFieldCount} bind:open={scopeOpen}>
          <table class="detail-fields w-full" aria-label="Scope attributes">
            <tbody>
              {#if span.scope.name && detailSearchFieldVisible(columnFilter, 'scope.name')}
                <SpanField fieldName="scope name" fieldValue={span.scope.name} fieldType="string" />
              {/if}
              {#if span.scope.version && detailSearchFieldVisible(columnFilter, 'scope.version')}
                <SpanField fieldName="scope version" fieldValue={span.scope.version} fieldType="string" />
              {/if}
              {#each scopeAttributes as attr (attr.key)}
                {#if detailAttributeVisible(columnFilter, attr.key, 'scope')}
                  <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} />
                {/if}
              {/each}
              {#if span.scope.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'scope.droppedAttributesCount')}
                <SpanField fieldName="dropped attributes count" fieldValue={span.scope.droppedAttributesCount.toString()} fieldType="uint32" />
              {/if}
            </tbody>
          </table>
        </FieldGroup>
      {:else if activeTab === 'events'}
        {#if numEvents === 0}
          <p class="detail-view__tab-empty">No events recorded for this span</p>
        {:else}
          <EventsPanel events={span.events} spanStartTime={span.startTime} />
        {/if}
      {:else if activeTab === 'links'}
        {#if numLinks === 0}
          <p class="detail-view__tab-empty">No links recorded for this span</p>
        {:else}
          <LinksPanel links={span.links} />
        {/if}
      {/if}
    </div>
  </div>
{:else}
  <div class="detail-view detail-view--empty">
    <p class="detail-view__empty">No span selected</p>
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

  .detail-view__scroll {
    @apply flex-1 min-h-0 overflow-y-auto;
    scrollbar-width: thin;
  }

  .detail-view__empty {
    @apply py-8 text-center text-sm text-base-content/60;
  }

  .detail-view__tab-empty {
    @apply m-0 px-3 py-6 text-center text-sm italic;
    color: var(--color-muted);
  }
</style>