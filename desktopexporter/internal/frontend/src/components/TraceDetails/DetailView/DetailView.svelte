<script lang="ts">
  import type { SpanData } from '@/types/api-types'
  import SpanField from './SpanField.svelte'
  import EventsPanel from './EventsPanel.svelte'
  import LinksPanel from './LinksPanel.svelte'
  import { formatDuration } from '@/utils/duration'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    span: SpanData | undefined
  }

  let { span }: Props = $props()

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
  const tabs: { id: Tab; label: string; count?: () => number }[] = [
    { id: 'fields', label: 'Fields' },
    { id: 'events', label: 'Events', count: () => numEvents },
    { id: 'links', label: 'Links', count: () => numLinks },
  ]
  let activeTab = $state<Tab>('fields')
</script>

{#if span}
  <div class="detail-view">
    <nav class="detail-view__tabs">
      {#each tabs as tab}
        <button
          type="button"
          class="nav-button {activeTab === tab.id ? 'nav-button-active' : 'nav-button-inactive'}"
          onclick={() => { activeTab = tab.id }}
        >
          <span>{tab.label}</span>
          {#if tab.count !== undefined}
            <span class="detail-view__tab-count">{tab.count()}</span>
          {/if}
        </button>
      {/each}
    </nav>

    <div class="detail-view__content">
      {#if activeTab === 'fields'}
        <section class="detail-section">
          <h3 class="detail-section__heading">Span</h3>
          <div class="detail-section__body">
            <SpanField fieldName="name" fieldValue={span.name} fieldType="string" {isRoot} />
            <SpanField fieldName="kind" fieldValue={span.kind} fieldType="string" />
            <SpanField
              fieldName="start time"
              fieldValue={formatTimestamp(span.startTime, timeContext.timezone, 'nanoseconds')}
              fieldType="timestamp"
            />
            <SpanField
              fieldName="end time"
              fieldValue={formatTimestamp(span.endTime, timeContext.timezone, 'nanoseconds')}
              fieldType="timestamp"
            />
            <SpanField fieldName="duration" fieldValue={durationLabel} fieldType="string" />
            <SpanField fieldName="status code" fieldValue={span.statusCode} fieldType="string" />
            {#if span.statusCode !== 'Unset' && span.statusCode !== 'Ok'}
              <SpanField fieldName="status message" fieldValue={span.statusMessage} fieldType="string" />
            {/if}
            <SpanField fieldName="trace id" fieldValue={span.traceID} fieldType="string" />
            {#if !isRoot}
              <SpanField fieldName="parent span id" fieldValue={span.parentSpanID ?? ''} fieldType="string" />
            {/if}
            <SpanField fieldName="span id" fieldValue={span.spanID} fieldType="string" />
            {#each spanAttributes as attr}
              <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} />
            {/each}
            {#if span.droppedAttributesCount > 0}
              <SpanField fieldName="dropped attributes count" fieldValue={span.droppedAttributesCount.toString()} fieldType="uint32" />
            {/if}
            {#if span.droppedEventsCount > 0}
              <SpanField fieldName="dropped events count" fieldValue={span.droppedEventsCount.toString()} fieldType="uint32" />
            {/if}
            {#if span.droppedLinksCount > 0}
              <SpanField fieldName="dropped links count" fieldValue={span.droppedLinksCount.toString()} fieldType="uint32" />
            {/if}
          </div>
        </section>

        {#if resourceAttributes.length > 0 || span.resource.droppedAttributesCount > 0}
          <section class="detail-section">
            <h3 class="detail-section__heading">Resource</h3>
            <div class="detail-section__body">
              {#each resourceAttributes as attr}
                <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} origin="resource" />
              {/each}
              {#if span.resource.droppedAttributesCount > 0}
                <SpanField fieldName="dropped attributes count" fieldValue={span.resource.droppedAttributesCount.toString()} fieldType="uint32" origin="resource" />
              {/if}
            </div>
          </section>
        {/if}

        {#if scopeAttributes.length > 0 || span.scope.name || span.scope.version || span.scope.droppedAttributesCount > 0}
          <section class="detail-section">
            <h3 class="detail-section__heading">Scope</h3>
            <div class="detail-section__body">
              {#if span.scope.name}
                <SpanField fieldName="scope name" fieldValue={span.scope.name} fieldType="string" origin="scope" />
              {/if}
              {#if span.scope.version}
                <SpanField fieldName="scope version" fieldValue={span.scope.version} fieldType="string" origin="scope" />
              {/if}
              {#each scopeAttributes as attr}
                <SpanField fieldName={attr.key} fieldValue={attr.value} fieldType={attr.type} origin="scope" />
              {/each}
              {#if span.scope.droppedAttributesCount > 0}
                <SpanField fieldName="dropped attributes count" fieldValue={span.scope.droppedAttributesCount.toString()} fieldType="uint32" origin="scope" />
              {/if}
            </div>
          </section>
        {/if}

      {:else if activeTab === 'events'}
        {#if numEvents > 0}
          <EventsPanel events={span.events} spanStartTime={span.startTime} />
        {:else}
          <p class="detail-view__empty">No events recorded for this span</p>
        {/if}

      {:else if activeTab === 'links'}
        {#if numLinks > 0}
          <LinksPanel links={span.links} />
        {:else}
          <p class="detail-view__empty">No links recorded for this span</p>
        {/if}
      {/if}
    </div>
  </div>
{:else}
  <div class="detail-view">
    <p class="detail-view__empty">No span selected</p>
  </div>
{/if}

<style lang="postcss">
  .detail-view {
    @apply flex h-full min-h-0 flex-col;
  }

  .detail-view__tabs {
    @apply flex flex-nowrap items-center gap-1 border-b border-base-300/50 px-4 py-2;
  }

  .detail-view__tab-count {
    @apply text-xs opacity-60;
  }

  .detail-view__content {
    @apply flex-1 overflow-y-auto p-2;
  }

  .detail-view__empty {
    @apply text-base-content/60 text-sm py-8 text-center;
  }

  .detail-section {
    @apply border border-base-300 rounded-lg overflow-hidden;
  }

  .detail-section + .detail-section {
    @apply mt-2;
  }

  .detail-section__heading {
    @apply px-4 py-1.5 text-xs font-semibold text-base-content/50 uppercase tracking-wider bg-base-200/30;
  }

  .detail-section__body {
    @apply divide-y divide-base-300/50;
  }
</style>
