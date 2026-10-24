<script lang="ts">
  import AttributeValues from './AttributeValues.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import type {
    AttributeValueSort,
    CoverageValue,
    DiscoveredAttribute,
  } from './attribute-coverage'
  import type { AttributeColumn, AttributeSummary } from './attribute-columns'
  import type { DiscoveryOptions } from '@/services/discovery-types'
  let {
    entry,
    options,
    sortBy,
    sortDirection,
    columns,
    pinned,
    selected,
    onSelect,
    selectedValue,
    onSelectValue,
    onSummary,
    hidden = false,
  }: {
    entry: DiscoveredAttribute
    options: Omit<DiscoveryOptions, 'kind'>
    sortBy: AttributeValueSort
    sortDirection: 'asc' | 'desc'
    columns: AttributeColumn[]
    pinned: Record<string, number | undefined>
    selected: boolean
    onSelect: () => void
    selectedValue?: string
    onSelectValue: (value: CoverageValue) => void
    onSummary: (summary: AttributeSummary) => void
    hidden?: boolean
  } = $props()
  const valueOptions = $derived({ ...options, ownerType: entry.ownerType })
</script>

<AttributeValues
  options={valueOptions}
  name={entry.key}
  kind={entry.kind}
  {sortBy}
  {sortDirection}
  {columns}
  {pinned}
  foundOn={entry.foundOn}
  {selected}
  {selectedValue}
  {onSelectValue}
  {onSummary}
  {onSelect}
  {hidden}
>
  {#snippet heading()}
    <div class="min-w-0 flex-1">
      <TableValueCell
        label={`${entry.key} · ${entry.kind} · ${entry.ownerType}`}
        text={`${entry.key} · ${entry.kind} · ${entry.ownerType}`}
        inset={false}
        onInspect={onSelect}
      >
        <span
          class="min-w-0 truncate"
          title={`${entry.key} · ${entry.kind} · ${entry.ownerType}`}
          >{entry.key}{#if !columns.some(column => column.id === 'type')}<span
              class="text-subtle"
              aria-hidden="true">{' · '}</span
            ><span class="text-secondary">{entry.kind}</span>{/if}</span
        >
      </TableValueCell>
    </div>
  {/snippet}
</AttributeValues>
