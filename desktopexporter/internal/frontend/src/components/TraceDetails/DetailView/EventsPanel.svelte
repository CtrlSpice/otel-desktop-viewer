<script lang="ts">
  import { tick } from 'svelte'
  import type { EventData } from '@/types/api-types'
  import SpanField from './SpanField.svelte'
  import { formatDuration, formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { ArrowDownIcon } from '@/icons'

  type Props = {
    events: EventData[]
    spanStartTime: bigint
  }

  let { events, spanStartTime }: Props = $props()

  let timeContext = getTimeContext()

  let expandedIndex = $state<number | null>(0)
  let headerRows: HTMLTableRowElement[] = []

  function toggle(index: number) {
    const opening = expandedIndex !== index
    expandedIndex = opening ? index : null
    if (opening) {
      tick().then(() => {
        headerRows[index]?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
      })
    }
  }
</script>

{#each events as event, index}
  {@const open = expandedIndex === index}
  <tr
    bind:this={headerRows[index]}
    class="table-row events-panel__header-row cursor-pointer {open ? 'events-panel__header-row--open' : ''}"
    onclick={() => toggle(index)}
    role="button"
    tabindex="0"
    onkeydown={e => (e.key === 'Enter' || e.key === ' ') && toggle(index)}
  >
    <td class="detail-cell">
      <span class="events-panel__indicator {open ? 'events-panel__indicator--open' : ''}">
        <ArrowDownIcon />
      </span>
      <span class="detail-cell__key">name:</span>
      <span class="detail-cell__value">{event.name}</span>
    </td>
    <td class="detail-cell--badges">
      <span class="badge-type">string</span>
      <span class="badge-offset">+{formatDuration(event.timestamp - spanStartTime)}</span>
    </td>
  </tr>
  {#if open}
    <SpanField
      nested
      fieldName="timestamp"
      fieldValue={formatTimestamp(
        event.timestamp,
        timeContext.timezone,
        'nanoseconds'
      )}
      fieldType="timestamp"
    />
    {#each event.attributes as attr}
      <SpanField
        nested
        fieldName={attr.key}
        fieldValue={attr.value}
        fieldType={attr.type}
      />
    {/each}
    {#if event.droppedAttributesCount > 0}
      <SpanField
        nested
        fieldName="dropped attributes count"
        fieldValue={event.droppedAttributesCount.toString()}
        fieldType="uint32"
      />
    {/if}
  {/if}
{/each}

<style lang="postcss">
  @reference "../../../app.css";

  .events-panel__header-row--open,
  .events-panel__header-row--open ~ :global(.table-row--nested) {
    @apply bg-base-200/40;
  }

  .events-panel__indicator {
    @apply inline-flex align-middle text-base-content/35 transition-all duration-150 mr-1;
    font-size: 14px;
    transform: rotate(-90deg);
  }

  .events-panel__indicator--open {
    @apply text-base-content/70;
    transform: rotate(0deg);
  }

  .events-panel__header-row:hover .events-panel__indicator {
    @apply text-base-content/60;
  }
</style>
