<script lang="ts">
  import type { LinkData } from '@/types/api-types';
  import SpanField from './SpanField.svelte';

  type Props = {
    links: LinkData[];
  };

  let { links }: Props = $props();

  let openLinks = $state<Set<number>>(new Set());
</script>

{#if links && links.length > 0}
  <div class="space-y-2">
    {#each links as link, index}
      <div class="data-table-section">
        <button
          type="button"
          class="data-table-header"
          onclick={() => {
            let newSet = new Set(openLinks);
            if (newSet.has(index)) {
              newSet.delete(index);
            } else {
              newSet.add(index);
            }
            openLinks = newSet;
          }}
        >
          <div class="section-header">
            <svg
              class="w-4 h-4 transition-transform {openLinks.has(index)
                ? 'rotate-180'
                : ''}"
              viewBox="0 0 24 24"
            >
              <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
            </svg>
            <div class="text-sm">
              <div>Trace ID: <strong>{link.traceID}</strong></div>
              <div>Span ID: <strong>{link.spanID}</strong></div>
            </div>
          </div>
        </button>
        {#if openLinks.has(index)}
          <div class="data-table">
            <div class="data-table-row">
              <div class="data-table-cell">
                <SpanField
                  fieldName="trace state"
                  fieldValue={link.traceState}
                  fieldType="string"
                />
              </div>
            </div>
            {#each Object.entries(link.attributes) as [key, value]}
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
            {#if link.droppedAttributesCount > 0}
              <div class="data-table-row">
                <div class="data-table-cell">
                  <SpanField
                    fieldName="dropped attributes count"
                    fieldValue={link.droppedAttributesCount.toString()}
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
  <p class="text-base-content/60">No links</p>
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
