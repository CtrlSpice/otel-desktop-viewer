<script module lang="ts">
  import type { AttributeScope, FieldDefinition } from '@/constants/fields'

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
  import type { SpanData } from '@/types/api-types'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import BiohazardIcon from '@hugeicons/core-free-icons/BiohazardIcon'
  import LeftToRightListBulletIcon from '@hugeicons/core-free-icons/LeftToRightListBulletIcon'
  import Link01Icon from '@hugeicons/core-free-icons/Link01Icon'
  import StopWatchIcon from '@hugeicons/core-free-icons/StopWatchIcon'
  import PaneHeader, {
    paneTabID,
    type PaneTab,
  } from '@/components/shared/PaneHeader.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import SpanField from './SpanField.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import LinksPanel from './LinksPanel.svelte'
  import { formatDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { setSpanInQuery } from '@/route'

  const SPAN_DETAIL_PANEL_ID = 'span-detail-tabpanel'

  type Props = {
    span: SpanData | undefined
    /** Node-level flags from the salvage walk; absent on healthy traces. */
    salvaged?: boolean
    cyclePoint?: boolean
    selectedEventIndex?: number | null
    /** Empty: show all Fields rows. Non-empty: only selected search fields / attributes. */
    columnFilter?: FieldDefinition[]
  }

  let {
    span,
    salvaged = false,
    cyclePoint = false,
    selectedEventIndex = null,
    columnFilter = [],
  }: Props = $props()

  let timeContext = getTimeContext()

  // --- Derived span data ---

  let isRoot = $derived(!span?.parentSpanID)
  let durationLabel = $derived(
    span ? formatDuration(span.endTime - span.startTime) : ''
  )
  let spanAttributes = $derived(span?.attributes ?? [])
  let resourceAttributes = $derived(span?.resource.attributes ?? [])
  let scopeAttributes = $derived(span?.scope.attributes ?? [])
  let numEvents = $derived(span?.events.length ?? 0)
  let numLinks = $derived(span?.links.length ?? 0)

  // --- Tab state ---

  type Tab = 'fields' | 'events' | 'links'
  let activeTab = $state<Tab>('fields')

  $effect(() => {
    if (selectedEventIndex !== null) activeTab = 'events'
  })

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
    if (span.flags > 0 && detailFlagsVisible(f)) n++
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
  {#snippet fieldsIcon()}<HugeiconsIcon
      icon={LeftToRightListBulletIcon}
      size="1em"
      strokeWidth={1.5}
    />{/snippet}

  {#snippet eventsIcon()}<HugeiconsIcon
      icon={StopWatchIcon}
      size="1em"
      strokeWidth={1.5}
    />{/snippet}
  {#snippet linksIcon()}<HugeiconsIcon
      icon={Link01Icon}
      size="1em"
      strokeWidth={1.5}
    />{/snippet}

  {@const tabs: PaneTab[] = [
    { id: 'fields', label: 'Fields', icon: fieldsIcon },
    { id: 'events', label: 'Events', icon: eventsIcon, count: numEvents },
    { id: 'links', label: 'Links', icon: linksIcon, count: numLinks },
  ]}

  <div class="detail-view">
    <PaneHeader
      mode="tabs"
      {tabs}
      activeID={activeTab}
      onSelect={id => (activeTab = id as Tab)}
      ariaLabel="Span detail tabs"
      tabLayout="equal"
      tabPanelID={SPAN_DETAIL_PANEL_ID}
    />

    <div
      class="detail-view__scroll"
      id={SPAN_DETAIL_PANEL_ID}
      role="tabpanel"
      aria-labelledby={paneTabID(SPAN_DETAIL_PANEL_ID, activeTab)}
    >
      {#if activeTab === 'fields'}
        {#if cyclePoint}
          <div
            class="detail-view__paradox detail-view__paradox--offender"
            role="alert"
          >
            <HugeiconsIcon
              icon={BiohazardIcon}
              size="1em"
              strokeWidth={1.5}
              aria-hidden="true"
            /> This span causes a cycle: its parent span id points into its own subtree,
            so nothing here can be reached from the trace root. Likely an instrumentation
            bug in the emitting service.
          </div>
        {:else if salvaged}
          <div class="detail-view__paradox" role="alert">
            {'\u26A0'} Recovered from a broken part of this trace: a parent link forms
            a loop, so this span has no place under the root.
          </div>
        {/if}
        <FieldGroup label="Span" count={spanFieldCount} bind:open={spanOpen}>
          <table class="detail-fields w-full" aria-label="Span fields">
            <tbody>
              {#if detailSearchFieldVisible(columnFilter, 'name')}
                <SpanField
                  fieldName="name"
                  fieldValue={span.name}
                  fieldType="string"
                  {isRoot}
                />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'kind')}
                <SpanField
                  fieldName="kind"
                  fieldValue={span.kind}
                  fieldType="string"
                />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'startTime')}
                <SpanField
                  fieldName="start time"
                  fieldValue={formatTimestamp(
                    span.startTime,
                    timeContext.tz,
                    'nanoseconds'
                  )}
                  fieldType="timestamp"
                />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'endTime')}
                <SpanField
                  fieldName="end time"
                  fieldValue={formatTimestamp(
                    span.endTime,
                    timeContext.tz,
                    'nanoseconds'
                  )}
                  fieldType="timestamp"
                />
              {/if}
              {#if detailDurationVisible(columnFilter)}
                <SpanField
                  fieldName="duration"
                  fieldValue={durationLabel}
                  fieldType="string"
                />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'statusCode')}
                <SpanField
                  fieldName="status code"
                  fieldValue={span.statusCode}
                  fieldType="string"
                />
              {/if}
              {#if span.statusCode !== 'Unset' && span.statusCode !== 'Ok' && detailSearchFieldVisible(columnFilter, 'statusMessage')}
                <SpanField
                  fieldName="status message"
                  fieldValue={span.statusMessage}
                  fieldType="string"
                />
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'traceID')}
                <SpanField
                  fieldName="trace id"
                  fieldValue={span.traceID}
                  fieldType="string"
                />
              {/if}
              {#if !isRoot && detailSearchFieldVisible(columnFilter, 'parentSpanID')}
                <tr class="table-row">
                  <td class="detail-cell">
                    <span class="detail-cell__key">
                      parent span id <span class="detail-cell__type"
                        >(string)</span
                      >:
                    </span>
                    <button
                      type="button"
                      class="detail-cell__value link link-primary font-mono"
                      onclick={() => setSpanInQuery(span.parentSpanID!, 'push')}
                      >{span.parentSpanID}</button
                    >
                  </td>
                </tr>
              {/if}
              {#if detailSearchFieldVisible(columnFilter, 'spanID')}
                <SpanField
                  fieldName="span id"
                  fieldValue={span.spanID}
                  fieldType="string"
                />
              {/if}
              {#if span.flags > 0 && detailFlagsVisible(columnFilter)}
                <SpanField
                  fieldName="flags"
                  fieldValue={span.flags.toString()}
                  fieldType="uint32"
                />
              {/if}
              {#each spanAttributes as attr (attr.key)}
                {#if detailAttributeVisible(columnFilter, attr.key, 'span')}
                  <SpanField
                    fieldName={attr.key}
                    fieldValue={attr.value}
                    fieldType={attr.type}
                  />
                {/if}
              {/each}
              {#if span.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedAttributesCount')}
                <SpanField
                  fieldName="dropped attributes count"
                  fieldValue={span.droppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              {/if}
              {#if span.droppedEventsCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedEventsCount')}
                <SpanField
                  fieldName="dropped events count"
                  fieldValue={span.droppedEventsCount.toString()}
                  fieldType="uint32"
                />
              {/if}
              {#if span.droppedLinksCount > 0 && detailSearchFieldVisible(columnFilter, 'droppedLinksCount')}
                <SpanField
                  fieldName="dropped links count"
                  fieldValue={span.droppedLinksCount.toString()}
                  fieldType="uint32"
                />
              {/if}
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
              {#each resourceAttributes as attr (attr.key)}
                {#if detailAttributeVisible(columnFilter, attr.key, 'resource')}
                  <SpanField
                    fieldName={attr.key}
                    fieldValue={attr.value}
                    fieldType={attr.type}
                  />
                {/if}
              {/each}
              {#if span.resource.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'resource.droppedAttributesCount')}
                <SpanField
                  fieldName="dropped attributes count"
                  fieldValue={span.resource.droppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              {/if}
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup label="Scope" count={scopeFieldCount} bind:open={scopeOpen}>
          <table class="detail-fields w-full" aria-label="Scope attributes">
            <tbody>
              {#if span.scope.name && detailSearchFieldVisible(columnFilter, 'scope.name')}
                <SpanField
                  fieldName="scope name"
                  fieldValue={span.scope.name}
                  fieldType="string"
                />
              {/if}
              {#if span.scope.version && detailSearchFieldVisible(columnFilter, 'scope.version')}
                <SpanField
                  fieldName="scope version"
                  fieldValue={span.scope.version}
                  fieldType="string"
                />
              {/if}
              {#each scopeAttributes as attr (attr.key)}
                {#if detailAttributeVisible(columnFilter, attr.key, 'scope')}
                  <SpanField
                    fieldName={attr.key}
                    fieldValue={attr.value}
                    fieldType={attr.type}
                  />
                {/if}
              {/each}
              {#if span.scope.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'scope.droppedAttributesCount')}
                <SpanField
                  fieldName="dropped attributes count"
                  fieldValue={span.scope.droppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              {/if}
            </tbody>
          </table>
        </FieldGroup>
      {:else if activeTab === 'events'}
        {#if numEvents === 0}
          <p class="detail-view__tab-empty">No events recorded for this span</p>
        {:else}
          <EventsPanel
            events={span.events}
            spanStartTime={span.startTime}
            {selectedEventIndex}
          />
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

  /* Repeats the waterfall's cycle marks where the reader is actually
     looking. Same split as the rows: warning gold for every stranded span,
     error red for the one whose parent link caused it. */
  .detail-view__paradox {
    @apply m-2 rounded px-3 py-2 text-xs text-warning;
    background: color-mix(in srgb, var(--color-warning) 12%, transparent);
  }

  .detail-view__paradox--offender {
    @apply text-error;
    background: color-mix(in srgb, var(--color-error) 12%, transparent);
  }

  /* Preflight makes svg block-level, which broke the line after the icon.
     :global because the svg is rendered by the icon component, out of reach
     of scoped selectors. Baseline nudge matches how the glyph banner sits. */
  .detail-view__paradox :global(svg) {
    display: inline-block;
    vertical-align: -0.125em;
  }

  .detail-view__empty {
    @apply py-8 text-center text-sm text-base-content/60;
  }

  .detail-view__tab-empty {
    @apply m-0 px-3 py-6 text-center text-sm italic;
    color: var(--color-muted);
  }

  .detail-cell__type {
    color: var(--color-subtle);
    @apply font-normal;
  }
</style>
