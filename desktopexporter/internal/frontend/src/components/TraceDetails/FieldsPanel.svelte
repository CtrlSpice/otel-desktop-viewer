<script lang="ts">
  import type { SpanData } from '@/types/api-types';
  import SpanField from './SpanField.svelte';
  import { formatDuration } from '@/utils/duration';
  import { formatTimestamp } from '@/utils/time';
  import { getTimeContext } from '@/contexts/time-context.svelte';

  type Props = {
    span: SpanData | undefined;
  };

  let { span }: Props = $props();

  let timeContext = getTimeContext();

  let isRoot = $derived(!span?.parentSpanID);
  let spanDataOpen = $state(true);
  let resourceDataOpen = $state(true);
  let scopeDataOpen = $state(true);
  let durationLabel = $derived(
    span
      ? formatDuration(span.endTime - span.startTime)
      : ''
  );

  let spanAttributes = $derived(span?.attributes ?? []);
  let resourceAttributes = $derived(span?.resource.attributes ?? []);
  let scopeAttributes = $derived(span?.scope.attributes ?? []);
</script>

{#if span}
  <div class="space-y-2">
    <!-- Span Data -->
    <div class="data-table-section">
      <button
        type="button"
        class="data-table-header"
        onclick={() => (spanDataOpen = !spanDataOpen)}
      >
        <div class="section-header">
          <svg
            class="w-4 h-4 transition-transform {spanDataOpen
              ? 'rotate-180'
              : ''}"
            viewBox="0 0 24 24"
          >
            <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
          </svg>
          <span>Span Data</span>
          {#if isRoot}
            <span class="badge badge-secondary badge-outline badge-sm ml-2"
              >root</span
            >
          {/if}
        </div>
      </button>
      {#if spanDataOpen}
        <div class="data-table">
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="name"
                fieldValue={span.name}
                fieldType="string"
              />
            </div>
          </div>
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="kind"
                fieldValue={span.kind}
                fieldType="string"
              />
            </div>
          </div>
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="start time"
                fieldValue={formatTimestamp(span.startTime, timeContext.timezone, 'nanoseconds')}
                fieldType="timestamp"
              />
            </div>
          </div>
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="end time"
                fieldValue={formatTimestamp(span.endTime, timeContext.timezone, 'nanoseconds')}
                fieldType="timestamp"
              />
            </div>
          </div>
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="duration"
                fieldValue={durationLabel}
                fieldType="string"
              />
            </div>
          </div>
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="status code"
                fieldValue={span.statusCode}
                fieldType="string"
              />
            </div>
          </div>
          {#if span.statusCode !== 'Unset' && span.statusCode !== 'Ok'}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="status message"
                  fieldValue={span.statusMessage}
                  fieldType="string"
                />
              </div>
            </div>
          {/if}
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="trace id"
                fieldValue={span.traceID}
                fieldType="string"
              />
            </div>
          </div>
          {#if !isRoot}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="parent span id"
                  fieldValue={span.parentSpanID ?? ''}
                  fieldType="string"
                />
              </div>
            </div>
          {/if}
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="span id"
                fieldValue={span.spanID}
                fieldType="string"
              />
            </div>
          </div>
          {#each spanAttributes as attr}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName={attr.key}
                  fieldValue={attr.value}
                  fieldType={attr.type}
                />
              </div>
            </div>
          {/each}
          {#if span.droppedAttributesCount > 0}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="dropped attributes count"
                  fieldValue={span.droppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              </div>
            </div>
          {/if}
          {#if span.droppedEventsCount > 0}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="dropped events count"
                  fieldValue={span.droppedEventsCount.toString()}
                  fieldType="uint32"
                />
              </div>
            </div>
          {/if}
          {#if span.droppedLinksCount > 0}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="dropped links count"
                  fieldValue={span.droppedLinksCount.toString()}
                  fieldType="uint32"
                />
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Resource Data -->
    <div class="data-table-section">
      <button
        type="button"
        class="data-table-header"
        onclick={() => (resourceDataOpen = !resourceDataOpen)}
      >
        <div class="section-header">
          <svg
            class="w-4 h-4 transition-transform {resourceDataOpen
              ? 'rotate-180'
              : ''}"
            viewBox="0 0 24 24"
          >
            <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
          </svg>
          <span>Resource Data</span>
        </div>
      </button>
      {#if resourceDataOpen}
        <div class="data-table">
          {#each resourceAttributes as attr}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName={attr.key}
                  fieldValue={attr.value}
                  fieldType={attr.type}
                />
              </div>
            </div>
          {/each}
          {#if span.resource.droppedAttributesCount > 0}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="dropped attributes count"
                  fieldValue={span.resource.droppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Scope Data -->
    <div class="data-table-section">
      <button
        type="button"
        class="data-table-header"
        onclick={() => (scopeDataOpen = !scopeDataOpen)}
      >
        <div class="section-header">
          <svg
            class="w-4 h-4 transition-transform {scopeDataOpen
              ? 'rotate-180'
              : ''}"
            viewBox="0 0 24 24"
          >
            <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
          </svg>
          <span>Scope Data</span>
        </div>
      </button>
      {#if scopeDataOpen}
        <div class="data-table">
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="scope name"
                fieldValue={span.scope.name}
                fieldType="string"
              />
            </div>
          </div>
          <div class="data-table-row">
            <div class="data-table-cell">
              <SpanField
                fieldName="scope version"
                fieldValue={span.scope.version}
                fieldType="string"
              />
            </div>
          </div>
          {#each scopeAttributes as attr}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName={attr.key}
                  fieldValue={attr.value}
                  fieldType={attr.type}
                />
              </div>
            </div>
          {/each}
          {#if span.scope.droppedAttributesCount > 0}
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="dropped attributes count"
                  fieldValue={span.scope.droppedAttributesCount.toString()}
                  fieldType="uint32"
                />
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
{:else}
  <p class="text-base-content/60">Nothing here yet.</p>
{/if}

<style lang="postcss">
  .section-header {
    @apply text-sm font-semibold text-base-content py-2 px-4 flex items-center gap-2;
  }

  .data-table-section {
    @apply border border-base-300 rounded-lg overflow-hidden;
  }

  .data-table-header {
    @apply w-full text-left cursor-pointer hover:bg-base-200 transition-colors border-none bg-transparent;
  }

  .data-table {
    display: table;
    width: 100%;
    border-collapse: collapse;
  }

  .data-table-row {
    display: table-row;
    border-top: 1px solid hsl(var(--b3));
  }

  .data-table-row:first-child {
    border-top: none;
  }

  .data-table-cell {
    display: table-cell;
    width: 100%;
    padding: 0;
  }
</style>
