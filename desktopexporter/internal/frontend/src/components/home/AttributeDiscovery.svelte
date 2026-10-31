<script lang="ts">
  import { untrack } from 'svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { UnfoldLessIcon, UnfoldMoreIcon } from '@hugeicons/core-free-icons'
  import AttributeKey from './AttributeKey.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import ListFilter from '@/components/shared/Search/ListFilter.svelte'
  import ResizableTable from '@/components/shared/ResizableTable.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import TableSortButton from '@/components/shared/TableSortButton.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import PageSizeSelect from '@/components/shared/PageSizeSelect.svelte'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import { loadPageSize, savePageSize, type PageSize } from '@/utils/page-size'
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
    filter = $bindable(''),
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
  } = $props()
  let keys = $state<DiscoveredAttribute[]>([])
  const received = new Map<string, DiscoveredAttribute[]>()
  let counts = $state<Record<string, number>>({})
  let loading = $state(false)
  let error = $state('')
  type SortColumn = 'name' | 'count' | 'frequency'
  let sortBy = $state<SortColumn>('name')
  let sortDirection = $state<'asc' | 'desc'>('asc')
  let allExpanded = $state(false)
  let expansionOverrides = $state<
    Record<string, { scope: string; expanded: boolean }>
  >({})
  function setAll(expanded: boolean) {
    allExpanded = expanded
    expansionOverrides = {}
  }
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
      if (sortBy === 'count') {
        const left = counts[attributeIdentity(a)]
        const right = counts[attributeIdentity(b)]
        if (left === undefined || right === undefined) {
          if (left !== right) return left === undefined ? 1 : -1
        } else comparison = left - right
      } else if (sortBy === 'name') comparison = a.key.localeCompare(b.key)
      return (
        sign * comparison ||
        a.key.localeCompare(b.key) ||
        a.kind.localeCompare(b.kind) ||
        a.ownerType.localeCompare(b.ownerType)
      )
    })
  })
  const pageCount = $derived(Math.ceil(sortedKeys.length / pageSize))
  const columns = [
    { id: 'name', label: 'Name', min: 240, width: 400 },
    { id: 'count', label: 'Records', min: 112, width: 112, fixed: true },
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
    setPage(0)
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
  let displayedOptions = $state.raw<Omit<DiscoveryOptions, 'kind'>>({})
  let disclosureScope = ''
  let requestKey = $state(0)
  $effect(() => {
    const active = service
    const scope = identity
    const selectedOptions = options
    const request = { ...options, term: filter, limit: 100 }
    void revision
    const controller = new AbortController()
    requestKey = untrack(() => requestKey) + 1
    error = ''
    loading = Boolean(active)
    if (!active) {
      keys = []
      received.clear()
      counts = {}
      disclosureScope = ''
      pageIndex = 0
      setAll(false)
    }
    if (active) {
      if (disclosureScope !== scope) {
        // Keep each retained row's disclosure until its own values are replaced.
        expansionOverrides = untrack(() =>
          Object.fromEntries(
            keys.map(entry => {
              const id = attributeIdentity(entry)
              return [
                id,
                expansionOverrides[id] ?? {
                  scope: disclosureScope,
                  expanded: allExpanded,
                },
              ]
            })
          )
        )
        allExpanded = false
        disclosureScope = scope
      }
      displayedOptions = { ...selectedOptions }
      const owners = discoverySignals
        .filter(signal => queries?.[signal] !== false)
        .flatMap(signal =>
          attributeOwners[signal].map(ownerType => ({ signal, ownerType }))
        )
      const enabled = new Set(
        owners.map(({ signal, ownerType }) => `${signal}/${ownerType}`)
      )
      for (const owner of received.keys())
        if (!enabled.has(owner)) received.delete(owner)
      let pending = owners.length
      loading = pending > 0
      for (const { signal, ownerType } of owners) {
        void (async () => {
          try {
            const items: DiscoveredAttribute[] = []
            let more = true
            while (more) {
              controller.signal.throwIfAborted()
              const page = await telemetryAPI.discoverAttributeKeys(
                { ...request, signal, ownerType, offset: items.length },
                controller.signal
              )
              if (controller.signal.aborted) return
              items.push(...page.items.map(entry => ({ ...entry, ownerType })))
              more = page.truncated
              const owner = `${signal}/${ownerType}`
              received.set(
                owner,
                more
                  ? mergeAttributeKeys([
                      ...(received.get(owner) ?? []),
                      ...items,
                    ])
                  : items
              )
              keys = mergeAttributeKeys([...received.values()].flat())
            }
            if (controller.signal.aborted) return
            received.set(`${signal}/${ownerType}`, items)
            keys = mergeAttributeKeys([...received.values()].flat())
            pageIndex = 0
          } catch (reason) {
            if (!controller.signal.aborted) error = String(reason)
          } finally {
            if (!controller.signal.aborted) loading = --pending > 0
          }
        })()
      }
      if (!owners.length) keys = []
    }
    return () => controller.abort()
  })
</script>

<ListFilter label="Filter attribute keys" bind:value={filter}>
  {#snippet children(filterToggle, filterField)}
    <PaneHeader mode="toolbar" ariaLabel="Attribute controls">
      {#snippet right()}
        <button
          type="button"
          class="btn btn-xs chart-header-control tooltip tooltip-left"
          onclick={() => setAll(false)}
          disabled={keys.length === 0}
          aria-label="Collapse all attributes"
          data-tip="Collapse all attributes"
        >
          <HugeiconsIcon
            icon={UnfoldLessIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-4 w-4"
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="btn btn-xs chart-header-control tooltip tooltip-left"
          onclick={() => setAll(true)}
          disabled={keys.length === 0}
          aria-label="Expand all attributes"
          data-tip="Expand all attributes"
        >
          <HugeiconsIcon
            icon={UnfoldMoreIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-4 w-4"
            aria-hidden="true"
          />
        </button>
        {#if service}{@render filterToggle()}{/if}
      {/snippet}
    </PaneHeader>
    <div class="attribute-discovery">
      {#if service}
        {@render filterField()}
        {#if error}<p role="alert" class="p-3 text-papaya">{error}</p>
        {/if}
        <LoadingStatus
          pending={loading}
          {requestKey}
          label="Loading attributes…"
          class="block p-3 text-subtle"
        />
        {#if !loading && !error && keys.length === 0}<p class="p-3 text-subtle">
            No matching attributes in this service and time range.
          </p>
        {/if}
        <div class="attribute-discovery__table" bind:this={viewport}>
          <ResizableTable
            {columns}
            storageKey="home-attribute-column-widths"
            caption="Attributes"
            fillColumn="name"
            pinnedCount={1}
            constrainToViewport
            selectable
          >
            {#snippet header(layout)}
              <tr>
                <th
                  class="table-cell text-left"
                  class:table-cell--pinned={layout.pinned.name !== undefined}
                  style:left={layout.pinned.name === undefined
                    ? undefined
                    : `${layout.pinned.name}px`}
                  data-attribute-column="name"
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
                  data-attribute-column="count"
                  scope="col"
                  aria-sort={sortBy === 'count'
                    ? sortDirection === 'asc'
                      ? 'ascending'
                      : 'descending'
                    : 'none'}
                >
                  <TableSortButton
                    label="records"
                    inlineDirection
                    align="right"
                    title="Records containing this attribute and type; value rows count records containing that exact value"
                    direction={sortBy === 'count' ? sortDirection : undefined}
                    onclick={() => sort('count')}
                  >
                    <TableColumnHeading label="Records" align="right" />
                  </TableSortButton>
                </th>
                <th
                  class="table-cell text-right"
                  data-attribute-column="frequency"
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
                    align="right"
                    title="Sort values within each attribute"
                    direction={sortBy === 'frequency'
                      ? sortDirection
                      : undefined}
                    onclick={() => sort('frequency')}
                  >
                    <TableColumnHeading label="%" align="right" />
                  </TableSortButton>
                </th>
                <th class="table-cell" data-attribute-column="copy" scope="col"
                  ><span class="sr-only">Copy</span></th
                >
              </tr>
            {/snippet}
            {#snippet children(layout)}
              {#each sortedKeys as entry, index (attributeIdentity(entry))}
                {@const id = attributeIdentity(entry)}
                <AttributeKey
                  {entry}
                  options={displayedOptions}
                  sortBy={sortBy === 'name' ? 'name' : 'frequency'}
                  {sortDirection}
                  onCount={count => {
                    counts[id] = count
                    const retained = expansionOverrides[id]
                    if (retained && retained.scope !== identity)
                      delete expansionOverrides[id]
                  }}
                  pinnedLeft={layout.pinned.name}
                  expanded={expansionOverrides[id]?.expanded ?? allExpanded}
                  onExpandedChange={expanded =>
                    (expansionOverrides[id] = { scope: identity, expanded })}
                  selected={selectedAttribute !== null &&
                    attributeIdentity(selectedAttribute) === id}
                  onSelect={() => onSelectAttribute(entry)}
                  {selectedValue}
                  onSelectValue={value => onSelectAttribute(entry, value)}
                  hidden={index < pageIndex * pageSize ||
                    index >= (pageIndex + 1) * pageSize}
                />
              {/each}
            {/snippet}
          </ResizableTable>
        </div>
        <SignalFooter label="page" compact>
          {#snippet trailing()}<PageSizeSelect
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
  {/snippet}
</ListFilter>

<style lang="postcss">
  @reference '../../app.css';
  .attribute-discovery {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }
  .attribute-discovery__table {
    @apply flex min-h-0 flex-1 flex-col overflow-hidden;
  }
</style>
