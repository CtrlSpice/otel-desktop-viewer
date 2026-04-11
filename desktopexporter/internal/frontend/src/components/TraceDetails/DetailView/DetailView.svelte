<script lang="ts">
  import type { SpanData } from '@/types/api-types'
  import type { FieldDefinition } from '@/constants/fields'
  import {
    detailSearchFieldVisible,
    detailAttributeVisible,
    detailDurationVisible,
  } from '@/utils/detail-column-filter'
  import SpanField from './SpanField.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import LinksPanel from './LinksPanel.svelte'
  import { formatDuration } from '@/utils/duration'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    span: SpanData | undefined
    /** Empty: show all Fields rows. Non-empty: only selected search fields / attributes. */
    columnFilter?: FieldDefinition[]
  }

  let { span, columnFilter = [] }: Props = $props()

  let timeContext = getTimeContext()

  const DETAIL_FIELDS_COL_KEY = 'detail-fields-col-widths'
  const DEFAULT_FIELD_COL_PX = 176

  function loadFieldColWidth(): number {
    if (typeof localStorage === 'undefined') return DEFAULT_FIELD_COL_PX
    try {
      const raw = localStorage.getItem(DETAIL_FIELDS_COL_KEY)
      if (!raw) return DEFAULT_FIELD_COL_PX
      const o = JSON.parse(raw) as { field?: number; fieldRem?: number }
      if (typeof o.field === 'number' && o.field > 0) {
        return Math.round(o.field)
      }
      // Brief rem-based save: ~16px per rem at default root
      if (typeof o.fieldRem === 'number' && o.fieldRem > 0) {
        return Math.round(o.fieldRem * 16)
      }
      return DEFAULT_FIELD_COL_PX
    } catch {
      return DEFAULT_FIELD_COL_PX
    }
  }

  let fieldColWidthPx = $state(loadFieldColWidth())
  let detailTableEl = $state<HTMLTableElement | null>(null)
  let detailDividerDrag = $state(false)

  function saveFieldColWidth() {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(
      DETAIL_FIELDS_COL_KEY,
      JSON.stringify({ field: fieldColWidthPx }),
    )
  }

  function clampFieldCol(w: number): number {
    const tableW = detailTableEl?.getBoundingClientRect().width ?? 600
    return Math.round(Math.max(96, Math.min(Math.max(160, tableW * 0.55), w)))
  }

  function startResizeFieldCol(e: PointerEvent) {
    e.preventDefault()
    const startX = e.clientX
    const startW = fieldColWidthPx
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)
    detailDividerDrag = true

    function onMove(ev: PointerEvent) {
      fieldColWidthPx = clampFieldCol(startW + (ev.clientX - startX))
    }

    function end() {
      detailDividerDrag = false
      target.removeEventListener('pointermove', onMove)
      target.removeEventListener('pointerup', end)
      target.removeEventListener('pointercancel', end)
      saveFieldColWidth()
    }

    target.addEventListener('pointermove', onMove)
    target.addEventListener('pointerup', end)
    target.addEventListener('pointercancel', end)
  }

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
      <div class="col-resize-context">
        <table
          bind:this={detailTableEl}
          class="split-table detail-fields w-full"
          aria-label="Span details"
          style:--detail-field-col-w="{fieldColWidthPx}px"
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
        <div
          class="col-resize-bar col-resize-bar--below-header"
          class:col-resize-bar--active={detailDividerDrag}
          style:left="{fieldColWidthPx}px"
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize field and value columns"
          onpointerdown={startResizeFieldCol}
        >
          <div class="col-resize-bar__line"></div>
        </div>
      </div>
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
    @apply flex min-h-0 flex-1 flex-col;
  }

  .detail-view__scroll > :global(.col-resize-context) {
    @apply flex min-h-0 flex-1 flex-col;
  }

  :global(.detail-fields) {
    @apply min-h-0 flex-1;
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
