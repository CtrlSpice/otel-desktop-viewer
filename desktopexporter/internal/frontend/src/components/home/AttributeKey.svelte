<script lang="ts">
  import AttributeValues from './AttributeValues.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import type { CoverageValue } from './attribute-coverage'
  import type {
    AttributeKey,
    DiscoveryOptions,
  } from '@/services/discovery-types'
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
  }: {
    entry: AttributeKey
    options: Omit<DiscoveryOptions, 'kind'>
    sortBy: 'name' | 'frequency'
    sortDirection: 'asc' | 'desc'
    pinnedLeft?: number
    selected: boolean
    onSelect: () => void
    selectedValue?: string
    onSelectValue: (value: CoverageValue) => void
  } = $props()
</script>

<AttributeValues
  {options}
  name={entry.key}
  kind={entry.kind}
  {sortBy}
  {sortDirection}
  {pinnedLeft}
  {selected}
  {selectedValue}
  {onSelectValue}
>
  {#snippet heading()}
    <div class="min-w-0 flex-1">
      <TableValueCell
        label={`${entry.key} · ${entry.kind} · ${options.ownerType}`}
        text={`${entry.key} · ${entry.kind} · ${options.ownerType}`}
        inset={false}
        onInspect={onSelect}
      >
        <span
          class="min-w-0 truncate"
          title={`${entry.key} · ${entry.kind} · ${options.ownerType}`}
          >{entry.key}<span class="text-rp-subtle" aria-hidden="true"
            >{' · '}</span
          ><span class="text-secondary">{entry.kind}</span><span
            class="text-rp-subtle"
            aria-hidden="true">{' · '}</span
          ><span class="text-secondary">{options.ownerType}</span></span
        >
      </TableValueCell>
    </div>
  {/snippet}
</AttributeValues>
