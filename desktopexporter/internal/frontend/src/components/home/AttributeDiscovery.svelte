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
  let offset = $state(0)
  let keys = $state<DiscoveredAttribute[]>([])
  let more = $state(false)
  let loading = $state(false)
  let error = $state('')
  let sortBy = $state<'name' | 'frequency'>('frequency')
  let sortDirection = $state<'asc' | 'desc'>('desc')
  const columns = [
    { id: 'name', label: 'Name', min: 240, width: 400 },
    { id: 'bar', label: 'Frequency', min: 96, width: 240 },
    { id: 'frequency', label: '%', min: 80, width: 88 },
    { id: 'copy', label: 'Copy', min: 64, width: 64, fixed: true },
  ]
  function sort(column: 'name' | 'frequency') {
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
      offset = 0
    }, 200)
    return () => clearTimeout(timer)
  })
  $effect(() => {
    void identity
    void revision
    offset = 0
  })
  $effect(() => {
    const active = service
    const request = { ...options, term, offset, limit: 100 }
    void revision
    const controller = new AbortController()
    if (offset === 0) keys = []
    more = false
    error = ''
    loading = Boolean(active)
    if (active)
      void Promise.all(
        discoverySignals
          .filter(signal => queries?.[signal] !== false)
          .flatMap(signal =>
            attributeOwners[signal].map(async ownerType => {
              const page = await telemetryAPI.discoverAttributeKeys(
                { ...request, signal, ownerType },
                controller.signal
              )
              return {
                ...page,
                items: page.items.map(entry => ({ ...entry, ownerType })),
              }
            })
          )
      )
        .then(pages => {
          if (controller.signal.aborted) return
          keys = mergeAttributeKeys([
            ...keys,
            ...pages.flatMap(page => page.items),
          ])
          more = pages.some(page => page.truncated)
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
      class="p-3 text-rp-subtle"
    >
      Loading attributes…
    </p>
  {:else if keys.length === 0}<p class="p-3 text-rp-subtle">
      No matching attributes in this service and time range.
    </p>
  {:else}
    {#key `${identity}:${revision}:${term}`}
      <ResizableTable
        {columns}
        storageKey="home-attribute-column-widths"
        caption="Attributes"
        fillColumn="bar"
        pinnedCount={1}
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
                label="value"
                inlineDirection
                title="Sort values within each attribute"
                direction={sortBy === 'name' ? sortDirection : undefined}
                onclick={() => sort('name')}
              >
                <TableColumnHeading label="Name" />
              </TableSortButton>
            </th>
            <th class="table-cell" scope="col"
              ><span class="sr-only">Frequency</span></th
            >
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
          {#each keys as entry (attributeIdentity(entry))}<AttributeKey
              {entry}
              options={{ ...options, ownerType: entry.ownerType }}
              {sortBy}
              {sortDirection}
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

  {#if more || (loading && offset > 0)}<div class="px-3 py-2">
      <button
        class="btn btn-ghost btn-sm"
        disabled={loading || !more}
        onclick={() => (offset += 100)}
        >{loading ? 'Loading…' : 'Load more attributes'}</button
      >
    </div>{/if}
{:else}<p class="p-3 text-rp-subtle">
    Select a service to discover its attributes.
  </p>{/if}
