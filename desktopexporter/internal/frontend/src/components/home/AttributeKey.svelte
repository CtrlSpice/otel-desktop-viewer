<script lang="ts">
  import AttributeValues from './AttributeValues.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import type {
    AttributeValueSort,
    CoverageValue,
    DiscoveredAttribute,
  } from './attribute-coverage'
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
    expanded,
    onExpandedChange,
    hidden = false,
  }: {
    entry: DiscoveredAttribute
    options: Omit<DiscoveryOptions, 'kind'>
    sortBy: AttributeValueSort
    sortDirection: 'asc' | 'desc'
    pinnedLeft?: number
    selected: boolean
    onSelect: () => void
    selectedValue?: string
    onSelectValue: (value: CoverageValue) => void
    onCount: (count: number, source: Omit<DiscoveryOptions, 'kind'>) => void
    expanded: boolean
    onExpandedChange: (
      expanded: boolean,
      source: Omit<DiscoveryOptions, 'kind'>
    ) => void
    hidden?: boolean
  } = $props()
  const ownerType = $derived(entry.ownerType)
  const name = $derived(entry.key)
  const kind = $derived(entry.kind)
  const valueOptions = $derived({ ...options, ownerType })
</script>

<AttributeValues
  options={valueOptions}
  {name}
  {kind}
  {sortBy}
  {sortDirection}
  {pinnedLeft}
  {selected}
  {selectedValue}
  {onSelectValue}
  {onCount}
  {expanded}
  {onExpandedChange}
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
          >{entry.key}<span class="text-subtle" aria-hidden="true">{' · '}</span
          ><span class="text-secondary">{entry.kind}</span></span
        >
      </TableValueCell>
    </div>
  {/snippet}
</AttributeValues>
