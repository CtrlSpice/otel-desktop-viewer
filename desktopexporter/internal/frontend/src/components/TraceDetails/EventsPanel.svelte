<script lang="ts">
  import type { EventData } from '@/types/api-types';
  import { PreciseTimestamp } from '@/types/precise-timestamp';
  import SpanField from './SpanField.svelte';
  import { formatDuration } from '@/utils/duration';

  type Props = {
    events: EventData[];
    spanStartTime: PreciseTimestamp;
  };

  let { events, spanStartTime }: Props = $props();
  let openEvents = $state<Set<number>>(new Set());
</script>

{#if events && events.length > 0}
  <div class="space-y-2">
    {#each events as event, index}
      <div class="data-table-section">
        <button
          type="button"
          class="data-table-header"
          onclick={() => {
            let newSet = new Set(openEvents);
            if (newSet.has(index)) {
              newSet.delete(index);
            } else {
              newSet.add(index);
            }
            openEvents = newSet;
          }}
        >
          <div class="section-header">
            <svg
              class="w-4 h-4 transition-transform {openEvents.has(index)
                ? 'rotate-180'
                : ''}"
              viewBox="0 0 24 24"
            >
              <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
            </svg>
            <div>
              <div>{event.name}</div>
              <div class="text-xs text-base-content/60 font-normal">
                {formatDuration(
                  event.timestamp.nanoseconds - spanStartTime.nanoseconds
                )}{' '}
                since span start
              </div>
            </div>
          </div>
        </button>
        {#if openEvents.has(index)}
          <div class="data-table">
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="timestamp"
                  fieldValue={event.timestamp.toString()}
                  fieldType="timestamp"
                />
              </div>
            </div>
            {#each Object.entries(event.attributes) as [key, value]}
              <div class="data-table-row">
                <div class="data-table-cell">
                  <SpanField
                    fieldName={key}
                    fieldValue={value.toString()}
                    fieldType="type"
                  />
                </div>
              </div>
            {/each}
            {#if event.droppedAttributesCount > 0}
              <div class="data-table-row">
                <div class="data-table-cell">
                  <SpanField
                    fieldName="dropped attributes count"
                    fieldValue={event.droppedAttributesCount.toString()}
                    fieldType="uint32"
                  />
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>
{:else}
  <p class="text-base-content/60">No events</p>
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
