<script lang="ts">
  import type { Snippet } from 'svelte'
  import AttributeKey from './AttributeKey.svelte'
  import ResizableTable from '@/components/shared/ResizableTable.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import TableSortButton from '@/components/shared/TableSortButton.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import PageSizeSelect from '@/components/shared/PageSizeSelect.svelte'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import { loadPageSize, savePageSize, type PageSize } from '@/utils/page-size'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    loadAttributeColumns,
    loadPinnedAttributeColumns,
    visibleAttributeColumns,
    type AttributeColumn,
    type AttributeColumnID,
    type AttributeSortColumn,
    type AttributeSummary,
  } from './attribute-columns'
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
    filter = '',
    filterControl,
    columns = visibleAttributeColumns(
      loadAttributeColumns(),
      loadPinnedAttributeColumns()
    ),
    pinnedCount = 1,
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
    filter?: string
    filterControl?: Snippet
    columns?: AttributeColumn[]
    pinnedCount?: number
  } = $props()
  let term = $state('')
  let keys = $state<DiscoveredAttribute[]>([])
  let summaries = $state<Record<string, AttributeSummary>>({})
  let loading = $state(false)
  let error = $state('')
  let sortBy = $state<AttributeSortColumn>('name')
  let sortDirection = $state<'asc' | 'desc'>('asc')
  const PAGE_SIZE_KEY = 'home-attributes-page-size'
  let pageSize = $state(loadPageSize(PAGE_SIZE_KEY))
  let pageIndex = $state(0)
  let viewport = $state<HTMLDivElement>()
  function setPage(index: number) {
    pageIndex = Math.max(0, Math.min(index, pageCount - 1))
    const scroll = viewport?.querySelector<HTMLElement>(
      '.resizable-table__scroll'
    )
    if (scroll) scroll.scrollTop = 0
  }
  function setPageSize(size: PageSize) {
    pageSize = size
    savePageSize(PAGE_SIZE_KEY, size)
    setPage(0)
  }
  const sortedKeys = $derived.by(() => {
    const sign = sortDirection === 'asc' ? 1 : -1
    return [...keys].sort((a, b) => {
      let comparison = 0
      if (['count', 'values', 'traces', 'metrics', 'logs'].includes(sortBy)) {
        const numeric = (entry: DiscoveredAttribute) => {
          const summary = summaries[attributeIdentity(entry)]
          if (!summary) return undefined
          switch (sortBy) {
            case 'count':
            case 'values':
              return summary[sortBy]
            case 'traces':
            case 'metrics':
            case 'logs':
              return summary.counts[sortBy]
            default:
              return undefined
          }
        }
        const left = numeric(a)
        const right = numeric(b)
        if (left === undefined || right === undefined) {
          if (left !== right) return left === undefined ? 1 : -1
        } else comparison = left - right
      } else if (sortBy === 'name') {
        comparison = a.key.localeCompare(b.key)
      } else if (sortBy === 'type') {
        comparison = a.kind.localeCompare(b.kind)
      } else if (sortBy === 'source') {
        comparison = a.ownerType.localeCompare(b.ownerType)
      } else if (sortBy === 'signals') {
        const signals = (entry: DiscoveredAttribute) =>
          discoverySignals
            .filter(signal =>
              entry.foundOn.some(source => source.signal === signal)
            )
            .join(' ')
        comparison = signals(a).localeCompare(signals(b))
      }
      return (
        sign * comparison ||
        a.key.localeCompare(b.key) ||
        a.kind.localeCompare(b.kind) ||
        a.ownerType.localeCompare(b.ownerType)
      )
    })
  })
  const pageCount = $derived(Math.ceil(sortedKeys.length / pageSize))
  function sort(column: AttributeColumnID) {
    if (column === 'copy') return
    sortDirection =
      sortBy === column
        ? sortDirection === 'asc'
          ? 'desc'
          : 'asc'
        : ['name', 'type', 'source', 'signals'].includes(column)
          ? 'asc'
          : 'desc'
    sortBy = column
    setPage(0)
  }
  $effect(() => {
    if (!columns.some(column => column.id === sortBy)) {
      sortBy = 'name'
      sortDirection = 'asc'
      setPage(0)
    }
  })
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
    summaries = {}
    error = ''
    loading = Boolean(active)
    pageIndex = 0
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

<div class="attribute-discovery">
  {#if service}
    {@render filterControl?.()}
    {#if error}<p role="alert" class="p-3 text-papaya">{error}</p>
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
      <div class="attribute-discovery__table" bind:this={viewport}>
        {#key `${identity}:${revision}:${term}`}
          <ResizableTable
            {columns}
            storageKey="home-attribute-column-widths"
            caption="Attributes"
            fillColumn="name"
            {pinnedCount}
            stableColumns={['name']}
            selectable
          >
            {#snippet header(layout)}
              <tr>
                {#each columns as column (column.id)}
                  {@const align = [
                    'name',
                    'type',
                    'source',
                    'signals',
                  ].includes(column.id)
                    ? 'left'
                    : 'right'}
                  <th
                    class="table-cell"
                    class:text-left={align === 'left'}
                    class:text-right={align === 'right'}
                    class:table-cell--pinned={layout.pinned[column.id] !==
                      undefined}
                    style:left={layout.pinned[column.id] === undefined
                      ? undefined
                      : `${layout.pinned[column.id]}px`}
                    data-attribute-column={column.id}
                    scope="col"
                    aria-sort={column.id === 'copy'
                      ? undefined
                      : sortBy === column.id
                        ? sortDirection === 'asc'
                          ? 'ascending'
                          : 'descending'
                        : 'none'}
                  >
                    {#if column.id === 'copy'}<span class="sr-only">Copy</span>
                    {:else}
                      <TableSortButton
                        label={column.id === 'frequency'
                          ? 'percentage'
                          : column.label.toLowerCase()}
                        inlineDirection
                        {align}
                        title={column.description}
                        direction={sortBy === column.id
                          ? sortDirection
                          : undefined}
                        onclick={() => sort(column.id)}
                      >
                        <TableColumnHeading label={column.label} {align} />
                      </TableSortButton>
                    {/if}
                  </th>
                {/each}
              </tr>
            {/snippet}
            {#snippet children(layout)}
              {#each sortedKeys as entry, index (attributeIdentity(entry))}<AttributeKey
                  {entry}
                  {options}
                  sortBy={sortBy === 'name' ||
                  sortBy === 'traces' ||
                  sortBy === 'metrics' ||
                  sortBy === 'logs'
                    ? sortBy
                    : 'frequency'}
                  {sortDirection}
                  onSummary={summary =>
                    (summaries[attributeIdentity(entry)] = summary)}
                  {columns}
                  pinned={layout.pinned}
                  selected={selectedAttribute !== null &&
                    attributeIdentity(selectedAttribute) ===
                      attributeIdentity(entry)}
                  onSelect={() => onSelectAttribute(entry)}
                  {selectedValue}
                  onSelectValue={value => onSelectAttribute(entry, value)}
                  hidden={index < pageIndex * pageSize ||
                    index >= (pageIndex + 1) * pageSize}
                />{/each}
            {/snippet}
          </ResizableTable>
        {/key}
      </div>
    {/if}
    <SignalFooter label="page" compact>
      {#snippet leading()}<PageSizeSelect
          label="Attributes per page"
          value={pageSize}
          onchange={setPageSize}
        />{/snippet}
      {#snippet navigation()}
        <DetailNav
          index={keys.length ? pageIndex : -1}
          total={pageCount}
          label="page"
          onFirst={() => setPage(0)}
          onPrev={() => setPage(pageIndex - 1)}
          onNext={() => setPage(pageIndex + 1)}
          onLast={() => setPage(pageCount - 1)}
        />
      {/snippet}
    </SignalFooter>
  {:else}<p class="p-3 text-subtle">
      Select a service to discover its attributes.
    </p>{/if}
</div>

<style lang="postcss">
  @reference '../../app.css';
  .attribute-discovery {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }
  .attribute-discovery__table {
    @apply flex min-h-0 flex-1 flex-col overflow-hidden;
  }
</style>
