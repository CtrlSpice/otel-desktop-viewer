<script lang="ts">
  import type { EventData } from '@/types/api-types'
  import SpanField from './SpanField.svelte'
  import { formatDuration } from '@/utils/duration'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    events: EventData[]
    spanStartTime: bigint
  }

  let { events, spanStartTime }: Props = $props()

  let timeContext = getTimeContext()

  let openEvents = $state<Set<number>>(new Set([0]))

  function toggle(index: number, e: MouseEvent) {
    e.stopPropagation()
    let next = new Set(openEvents)
    if (next.has(index)) {
      next.delete(index)
    } else {
      next.add(index)
    }
    openEvents = next
  }
</script>

{#each events as event, index}
  {@const open = openEvents.has(index)}
  <tr class="table-row">
    <th scope="row" class="table-cell--field-name">
      <div class="events-panel__field-name-inner">
        <button
          type="button"
          class="group-toggle"
          class:group-toggle--open={open}
          aria-expanded={open}
          aria-label={open ? `Collapse ${event.name}` : `Expand ${event.name}`}
          onclick={e => toggle(index, e)}
        >
          <svg
            class="group-toggle__caret"
            viewBox="0 0 24 24"
            fill="none"
            aria-hidden="true"
          >
            <circle
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="1.5"
            />
            <path
              d="M10 8l4 4-4 4"
              stroke="currentColor"
              stroke-width="1.5"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>
        <span class="events-panel__key"
          >name<span aria-hidden="true">:</span></span
        >
      </div>
      <span class="col-resize-marker" aria-hidden="true"></span>
    </th>
    <td class="table-cell">
      <div class="events-panel__value-cell">
        <span class="events-panel__value-text">{event.name}</span>
        <span class="events-panel__badges">
          <span class="badge-type">string</span>
          <span class="badge-offset"
            >+{formatDuration(event.timestamp - spanStartTime)}</span
          >
        </span>
      </div>
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
  .events-panel__field-name-inner {
    @apply flex min-w-0 items-center gap-1.5;
  }

  .events-panel__key {
    @apply min-w-0 truncate;
  }

  .events-panel__value-cell {
    @apply flex min-w-0 items-center gap-1.5;
  }

  .events-panel__value-text {
    @apply min-w-0 flex-1 truncate text-sm text-base-content;
  }

  .events-panel__badges {
    @apply flex shrink-0 items-center gap-1;
  }
</style>
