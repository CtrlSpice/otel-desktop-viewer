<script lang="ts">
  import AttributeValues from './AttributeValues.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import type { CoverageValue, DiscoveredAttribute } from './attribute-coverage'
  import type { DiscoveryOptions } from '@/services/discovery-types'
  let {
    entry,
    options,
    sortBy,
    sortDirection,
    pinnedLeft,
    selected,
    onSelect,
    selectedValue,
    onSelectValue,
    onCount,
  }: {
    entry: DiscoveredAttribute
    options: Omit<DiscoveryOptions, 'kind'>
    sortBy: 'name' | 'frequency'
    sortDirection: 'asc' | 'desc'
    pinnedLeft?: number
    selected: boolean
    onSelect: () => void
    selectedValue?: string
    onSelectValue: (value: CoverageValue) => void
    onCount: (count: number) => void
  } = $props()
  const valueOptions = $derived({ ...options, ownerType: entry.ownerType })
</script>

<AttributeValues
  options={valueOptions}
  name={entry.key}
  kind={entry.kind}
  {sortBy}
  {sortDirection}
  {pinnedLeft}
  {selected}
  {selectedValue}
  {onSelectValue}
  {onCount}
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
          >{entry.key}<span class="text-subtle" aria-hidden="true">{' · '}</span
          ><span class="text-secondary">{entry.kind}</span></span
        >
      </TableValueCell>
    </div>
  {/snippet}
</AttributeValues>
