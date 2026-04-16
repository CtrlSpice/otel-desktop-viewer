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
  import SpanField from './SpanField.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import LinksPanel from './LinksPanel.svelte'
  import { formatDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

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
</script>

{#if span}
  <div class="detail-view">
    <div class="detail-view__scroll">
        <table
          class="detail-fields w-full"
          aria-label="Span details"
        >
          <thead class="detail-view__thead table-header-surface">
            <tr class="detail-view__header-row">
              <th class="detail-view__th-tabs" colspan="2" scope="colgroup">
                <nav class="detail-view__tabs">
                  <button
                    type="button"
                    class="nav-button {activeTab === 'fields' ? 'nav-button-active' : 'nav-button-inactive'}"
                    onclick={() => { activeTab = 'fields' }}
                  >
                    <svg viewBox="0 0 24 24" class="w-4 h-4">
                      <g fill="none" stroke="currentColor">
                        <path d="M2.5 12c0-4.478 0-6.718 1.391-8.109S7.521 2.5 12 2.5c4.478 0 6.718 0 8.109 1.391S21.5 7.521 21.5 12c0 4.478 0 6.718-1.391 8.109S16.479 21.5 12 21.5c-4.478 0-6.718 0-8.109-1.391S2.5 16.479 2.5 12Z" />
                        <path stroke-linecap="round" stroke-linejoin="round" d="M2.5 12h19M13 7h4" />
                        <circle cx="8.25" cy="7" r="1.25" stroke-linecap="round" stroke-linejoin="round" />
                        <circle cx="8.25" cy="17" r="1.25" stroke-linecap="round" stroke-linejoin="round" />
                        <path stroke-linecap="round" stroke-linejoin="round" d="M13 17h4" />
                      </g>
                    </svg>
                    <span>Fields</span>
                  </button>
                  <button
                    type="button"
                    class="nav-button {activeTab === 'events' ? 'nav-button-active' : 'nav-button-inactive'}"
                    onclick={() => { activeTab = 'events' }}
                  >
                    <svg viewBox="0 0 24 24" class="w-4 h-4">
                      <path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"
                        d="M18.01 7.49L19.5 6m1 7.5a8.5 8.5 0 1 1-17 0a8.5 8.5 0 0 1 17 0M14.5 2h-5M12 13.5l3.5-3.5" />
                    </svg>
                    <span>Events</span>
                    <span class="detail-view__tab-count">{numEvents}</span>
                  </button>
                  <button
                    type="button"
                    class="nav-button {activeTab === 'links' ? 'nav-button-active' : 'nav-button-inactive'}"
                    onclick={() => { activeTab = 'links' }}
                  >
                    <svg viewBox="0 0 24 24" class="w-4 h-4">
                      <g fill="none" stroke="currentColor" stroke-linecap="round">
                        <path d="M10 13.229q.213.349.504.654a3.56 3.56 0 0 0 4.454.59q.391-.24.73-.59l3.239-3.372c1.43-1.49 1.43-3.904 0-5.394a3.564 3.564 0 0 0-5.183 0l-.714.743" />
                        <path d="m10.97 18.14l-.713.743a3.564 3.564 0 0 1-5.184 0c-1.43-1.49-1.43-3.905 0-5.394l3.24-3.372a3.564 3.564 0 0 1 5.183 0q.291.305.504.654" />
                      </g>
                    </svg>
                    <span>Links</span>
                    <span class="detail-view__tab-count">{numLinks}</span>
                  </button>
                </nav>
              </th>
            </tr>
          </thead>
        <tbody class="table-body-surface">
          {#if activeTab === 'fields'}
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
            {#each spanAttributes as attr}
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
            {#each resourceAttributes as attr}
              {#if detailAttributeVisible(columnFilter, attr.key, 'resource')}
                <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} origin="resource" />
              {/if}
            {/each}
            {#if span.resource.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'resource.droppedAttributesCount')}
              <SpanField fieldName="dropped attributes count" fieldValue={span.resource.droppedAttributesCount.toString()} fieldType="uint32" origin="resource" />
            {/if}
            {#if span.scope.name && detailSearchFieldVisible(columnFilter, 'scope.name')}
              <SpanField fieldName="scope name" fieldValue={span.scope.name} fieldType="string" origin="scope" />
            {/if}
            {#if span.scope.version && detailSearchFieldVisible(columnFilter, 'scope.version')}
              <SpanField fieldName="scope version" fieldValue={span.scope.version} fieldType="string" origin="scope" />
            {/if}
            {#each scopeAttributes as attr}
              {#if detailAttributeVisible(columnFilter, attr.key, 'scope')}
                <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} origin="scope" />
              {/if}
            {/each}
            {#if span.scope.droppedAttributesCount > 0 && detailSearchFieldVisible(columnFilter, 'scope.droppedAttributesCount')}
              <SpanField fieldName="dropped attributes count" fieldValue={span.scope.droppedAttributesCount.toString()} fieldType="uint32" origin="scope" />
            {/if}

          {:else if activeTab === 'events'}
            {#if numEvents > 0}
              <EventsPanel events={span.events} spanStartTime={span.startTime} />
            {:else}
              <tr class="table-row"><td colspan="2" class="detail-view__empty">No events recorded for this span</td></tr>
            {/if}

          {:else if activeTab === 'links'}
            {#if numLinks > 0}
              <LinksPanel links={span.links} />
            {:else}
              <tr class="table-row"><td colspan="2" class="detail-view__empty">No links recorded for this span</td></tr>
            {/if}
          {/if}
        </tbody>
        </table>
    </div>
  </div>
{:else}
  <div class="detail-view">
    <p class="detail-view__empty">No span selected</p>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";
  .detail-view {
    @apply flex h-full min-h-0 flex-col;
  }

  .detail-view__scroll {
    @apply flex min-h-0 flex-1 flex-col overflow-auto;
  }

  .detail-view__scroll > :global(.detail-fields) {
    @apply min-h-0;
  }

  .detail-view__header-row {
    height: var(--table-header-h);
  }

  .detail-view__th-tabs {
    @apply relative align-middle;
  }

  .detail-view__tabs {
    @apply flex w-full flex-nowrap items-center gap-1 px-1;

    & > :global(button) {
      flex: 1 1 0%;
      justify-content: center;
      padding-top: 0.25rem;
      padding-bottom: 0.25rem;
      font-size: 0.75rem;
    }
  }

  .detail-view__tab-count {
    @apply text-xs opacity-60;
  }

  .detail-view__empty {
    @apply text-base-content/60 text-sm py-8 text-center;
  }
</style>
