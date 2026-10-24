<script lang="ts">
  import FilterInput from '@/components/shared/Search/FilterInput.svelte'
  import AttributeKey from './AttributeKey.svelte'
  import ResizableTable from '@/components/shared/ResizableTable.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import TableSortButton from '@/components/shared/TableSortButton.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    type ServiceSummary,
    type DiscoveryOptions,
  } from '@/services/discovery-types'
  import {
    attributeIdentity,
    attributeOwners,
    discoverySignals,
    mergeAttributeKeys,
    type DiscoveredAttribute,
    type CoverageValue,
  } from './attribute-coverage'
  let {
    service,
    range,
    revision,
    queries,
    selectedAttribute,
    onSelectAttribute,
    selectedValue,
  }: {
    service: ServiceSummary | null
    range: Pick<DiscoveryOptions, 'startTime' | 'endTime'>
    revision: number
    queries?: DiscoveryOptions['queries']
    selectedAttribute: DiscoveredAttribute | null
    onSelectAttribute: (
      entry: DiscoveredAttribute,
      value?: CoverageValue
    ) => void
    selectedValue?: string
  } = $props()
  let filter = $state('')
  let term = $state('')
  let keys = $state<DiscoveredAttribute[]>([])
  let counts = $state<Record<string, number>>({})
  let loading = $state(false)
  let error = $state('')
  type SortColumn = 'name' | 'count' | 'frequency'
  let sortBy = $state<SortColumn>('name')
  let sortDirection = $state<'asc' | 'desc'>('asc')
  const sortedKeys = $derived.by(() => {
    const sign = sortDirection === 'asc' ? 1 : -1
    return [...keys].sort((a, b) => {
      let comparison = 0
      if (sortBy === 'count') {
        const left = counts[attributeIdentity(a)]
        const right = counts[attributeIdentity(b)]
        if (left === undefined || right === undefined) {
          if (left !== right) return left === undefined ? 1 : -1
        } else comparison = left - right
      } else if (sortBy === 'name') {
        comparison = a.key.localeCompare(b.key)
      }
      return (
        sign * comparison ||
        a.key.localeCompare(b.key) ||
        a.kind.localeCompare(b.kind) ||
        a.ownerType.localeCompare(b.ownerType)
      )
    })
  })
  const columns = [
    { id: 'name', label: 'Name', min: 240, width: 400 },
    { id: 'count', label: 'Count', min: 80, width: 88, fixed: true },
    { id: 'frequency', label: '%', min: 80, width: 88, fixed: true },
    { id: 'copy', label: 'Copy', min: 64, width: 64, fixed: true },
  ]
  function sort(column: SortColumn) {
    sortDirection =
      sortBy === column
        ? sortDirection === 'asc'
          ? 'desc'
          : 'asc'
        : column === 'name'
          ? 'asc'
          : 'desc'
    sortBy = column
  }
  const options = $derived({
    ...range,
    queries,
    ...(service?.serviceIdentity !== undefined
      ? { serviceIdentity: service.serviceIdentity }
      : {
          serviceName: service?.serviceName,
          namespace: service?.serviceNamespace,
        }),
  })
  const identity = $derived(service ? serviceIdentity(service) : '')
  $effect(() => {
    const text = filter
    const timer = setTimeout(() => {
      term = text
    }, 200)
    return () => clearTimeout(timer)
  })
  $effect(() => {
    const active = service
    const request = { ...options, term, limit: 100 }
    void revision
    const controller = new AbortController()
    keys = []
    counts = {}
    error = ''
    loading = Boolean(active)
    if (active)
      void Promise.all(
        discoverySignals
          .filter(signal => queries?.[signal] !== false)
          .flatMap(signal =>
            attributeOwners[signal].map(async ownerType => {
              const items: DiscoveredAttribute[] = []
              let more = true
              while (more) {
                controller.signal.throwIfAborted()
                const page = await telemetryAPI.discoverAttributeKeys(
                  { ...request, signal, ownerType, offset: items.length },
                  controller.signal
                )
                items.push(
                  ...page.items.map(entry => ({ ...entry, ownerType }))
                )
                more = page.truncated
              }
              return items
            })
          )
      )
        .then(pages => {
          if (controller.signal.aborted) return
          keys = mergeAttributeKeys(pages.flat())
        })
        .catch(reason => {
          if (!controller.signal.aborted) error = String(reason)
        })
        .finally(() => {
          if (!controller.signal.aborted) loading = false
        })
    return () => controller.abort()
  })
</script>

{#if service}
  <div class="p-3">
    <FilterInput
      label="Filter attribute keys"
      placeholder="Filter"
      bind:value={filter}
    />
  </div>
  {#if error}<p role="alert" class="p-3">{error}</p>
  {:else if loading && keys.length === 0}<p
      role="status"
      class="p-3 text-subtle"
    >
      Loading attributes…
    </p>
  {:else if keys.length === 0}<p class="p-3 text-subtle">
      No matching attributes in this service and time range.
    </p>
  {:else}
    {#key `${identity}:${revision}:${term}`}
      <ResizableTable
        {columns}
        storageKey="home-attribute-column-widths"
        caption="Attributes"
        fillColumn="name"
        pinnedCount={1}
        constrainToViewport
      >
        {#snippet header(layout)}
          <tr>
            <th
              class="table-cell text-left"
              class:table-cell--pinned={layout.pinned.name !== undefined}
              style:left={layout.pinned.name === undefined
                ? undefined
                : `${layout.pinned.name}px`}
              scope="col"
              aria-sort={sortBy === 'name'
                ? sortDirection === 'asc'
                  ? 'ascending'
                  : 'descending'
                : 'none'}
            >
              <TableSortButton
                label="name"
                inlineDirection
                title="Sort attributes and their values by name"
                direction={sortBy === 'name' ? sortDirection : undefined}
                onclick={() => sort('name')}
              >
                <TableColumnHeading label="Name" />
              </TableSortButton>
            </th>
            <th
              class="table-cell text-right"
              scope="col"
              aria-sort={sortBy === 'count'
                ? sortDirection === 'asc'
                  ? 'ascending'
                  : 'descending'
                : 'none'}
            >
              <TableSortButton
                label="count"
                inlineDirection
                align="right"
                direction={sortBy === 'count' ? sortDirection : undefined}
                onclick={() => sort('count')}
              >
                <TableColumnHeading label="Count" align="right" />
              </TableSortButton>
            </th>
            <th
              class="table-cell text-right"
              scope="col"
              aria-sort={sortBy === 'frequency'
                ? sortDirection === 'asc'
                  ? 'ascending'
                  : 'descending'
                : 'none'}
            >
              <TableSortButton
                label="percentage"
                inlineDirection
                title="Sort values within each attribute"
                align="right"
                direction={sortBy === 'frequency' ? sortDirection : undefined}
                onclick={() => sort('frequency')}
              >
                <TableColumnHeading label="%" align="right" />
              </TableSortButton>
            </th>
            <th class="table-cell" scope="col"
              ><span class="sr-only">Copy</span></th
            >
          </tr>
        {/snippet}
        {#snippet children(layout)}
          {#each sortedKeys as entry (attributeIdentity(entry))}<AttributeKey
              {entry}
              {options}
              sortBy={sortBy === 'name' ? 'name' : 'frequency'}
              {sortDirection}
              onCount={count => (counts[attributeIdentity(entry)] = count)}
              pinnedLeft={layout.pinned.name}
              selected={selectedAttribute !== null &&
                attributeIdentity(selectedAttribute) ===
                  attributeIdentity(entry)}
              onSelect={() => onSelectAttribute(entry)}
              {selectedValue}
              onSelectValue={value => onSelectAttribute(entry, value)}
            />{/each}
        {/snippet}
      </ResizableTable>
    {/key}
  {/if}
{:else}<p class="p-3 text-subtle">
    Select a service to discover its attributes.
  </p>{/if}
