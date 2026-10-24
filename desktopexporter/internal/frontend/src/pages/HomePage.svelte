<script module lang="ts">
  export function rejectionLabel(kind: string): string {
    switch (kind) {
      case 'span_already_stored':
        return 'duplicate span id'
      case 'span_refused':
        return 'rejected by the store'
      default:
        return kind
    }
  }
</script>

<script lang="ts">
  import { onDestroy, onMount, untrack, type Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import WorkspaceHeader from '@/components/shared/WorkspaceHeader.svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  import IngestionIssues, {
    INGESTION_ISSUES_ID,
  } from '@/components/shared/IngestionIssues.svelte'
  import GettingStarted from '@/components/home/GettingStarted.svelte'
  import ServiceCard from '@/components/home/ServiceCard.svelte'
  import AttributeDiscovery from '@/components/home/AttributeDiscovery.svelte'
  import AttributeTelemetry from '@/components/home/AttributeTelemetry.svelte'
  import type {
    CoverageValue,
    DiscoveredAttribute,
  } from '@/components/home/attribute-coverage'
  import {
    homeFields,
    homeQueries,
    queryText,
  } from '@/components/home/home-search'
  import { getStaticFieldsForSearch } from '@/constants/fields'
  import { getDynamicAttributes } from '@/services/search-field-service'
  import { parseQuery } from '@/components/shared/Search/queryParser'
  import { getNavigationCounts } from '@/contexts/navigation-counts.svelte'
  import { readRoute, navigateCurrentRoute, subscribeToRoute } from '@/route'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import {
    SIGNAL_ORDER,
    serviceIdentity,
    discoverySignalHref,
    type ServiceSummary,
    type DiscoveryOptions,
  } from '@/services/discovery-types'
  import {
    getTimeContext,
    selectionToQueryRangeNs,
  } from '@/contexts/time-context.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import type { Stats } from '@/types/api-types'

  let {
    importContent,
    importFailures = [],
  }: { importContent?: Snippet; importFailures?: readonly ImportFailure[] } =
    $props()
  const time = getTimeContext()
  const navigation = getNavigationCounts()
  if (window.location.hash === `#${INGESTION_ISSUES_ID}`)
    detailPanel.setOpen(true)
  const signals = SIGNAL_ORDER
  let fields = homeFields(signals.map(getStaticFieldsForSearch))
  let fieldsReady = $state(false)
  let queries = $state(homeQueries(null))
  let pendingQueries = homeQueries(null)
  let totalServices = $state<number>()
  let alive = true
  let initialized = false
  let services = $state<ServiceSummary[]>([])
  let selected = $state<ServiceSummary | null>(null)
  let stats = $state<Stats | null>(null)
  let statsError = $state('')
  let error = $state('')
  let loading = $state(false)
  let draft = $state(readRoute().query.q ?? '')
  let routeQuery = readRoute().query.q ?? ''
  let term = $state('')
  let offset = $state(0)
  let more = $state(false)
  let loaded = $state(false)
  let revision = $state(0)
  let selectedAttribute = $state<DiscoveredAttribute | null>(null)
  let selectedValue = $state<CoverageValue>()
  const selectedServiceIdentity = $derived(
    selected ? serviceIdentity(selected) : null
  )
  $effect(() => {
    void selectedServiceIdentity
    selectedAttribute = null
    selectedValue = undefined
  })
  function selectAttribute(
    attribute: DiscoveredAttribute,
    value?: CoverageValue
  ) {
    selectedAttribute = attribute
    selectedValue = value
    detailPanel.setOpen(true)
  }
  let range = $state<Pick<DiscoveryOptions, 'startTime' | 'endTime'>>({
    startTime: null,
    endTime: null,
  })
  let request: AbortController | undefined
  const rejections = $derived(stats?.rejections ?? [])
  const issueCount = $derived(
    importFailures.length +
      rejections.reduce((sum, entry) => sum + entry.occurrences, 0)
  )

  async function load(pageOffset = 0, refresh = false) {
    request?.abort()
    const controller = new AbortController()
    request = controller
    let nextRange = range
    if (refresh) {
      const bounds = selectionToQueryRangeNs(time.selection, Date.now())
      nextRange = {
        startTime: bounds.startTime?.toString() ?? null,
        endTime: bounds.endTime?.toString() ?? null,
      }
    }
    const options = {
      ...nextRange,
      queries: pendingQueries,
      offset: pageOffset,
      limit: 100,
    }
    loading = true
    error = ''
    if (refresh) {
      statsError = ''
      void telemetryAPI
        .getStats()
        .then(value => {
          if (!controller.signal.aborted) stats = value
        })
        .catch(reason => {
          if (!controller.signal.aborted) statsError = String(reason)
        })
    }
    try {
      const page = await telemetryAPI.discoverServices(
        options,
        controller.signal
      )
      if (controller.signal.aborted) return
      services = page.items
      range = nextRange
      queries = options.queries
      totalServices = page.totals?.home
      navigation?.setHomeCounts(
        page.totals ?? {},
        {
          traces: discoverySignalHref(
            'traces',
            range,
            queryText(queries.traces)
          ),
          metrics: discoverySignalHref(
            'metrics',
            range,
            queryText(queries.metrics)
          ),
          logs: discoverySignalHref('logs', range, queryText(queries.logs)),
        },
        term.trim().length > 0
      )
      more = page.truncated
      offset = pageOffset
      loaded = true
      if (selected)
        selected =
          services.find(
            service => serviceIdentity(service) === serviceIdentity(selected!)
          ) ??
          services[0] ??
          null
      else selected = services[0] ?? null
      if (refresh) {
        revision++
      }
    } catch (reason) {
      if (!controller.signal.aborted) error = String(reason)
    } finally {
      if (!controller.signal.aborted) loading = false
    }
  }
  function submit() {
    try {
      pendingQueries = homeQueries(parseQuery(draft, fields))
      term = draft
      routeQuery = draft
      const route = readRoute()
      if ((route.query.q ?? '') !== draft)
        navigateCurrentRoute(
          { ...route.query, q: draft || undefined },
          'replace'
        )
      void load(0, true)
    } catch (reason) {
      error = reason instanceof Error ? reason.message : String(reason)
    }
  }
  $effect(() => {
    void time.selection
    if (!fieldsReady) return
    untrack(() => {
      if (!initialized) {
        initialized = true
        submit()
      } else void load(0, true)
    })
  })
  onMount(() => {
    void Promise.all(signals.map(getDynamicAttributes)).then(groups => {
      if (!alive) return
      fields = homeFields([fields, ...groups])
      fieldsReady = true
    })
    return subscribeToRoute(() => {
      const route = readRoute()
      if (route.path !== '/') return
      const next = route.query.q ?? ''
      if (next === routeQuery) return
      routeQuery = next
      draft = next
      if (fieldsReady) submit()
    })
  })
  onDestroy(() => {
    alive = false
    request?.abort()
    navigation?.setHomeCounts(null)
  })
  function relative(timestamp: bigint | null | undefined) {
    if (timestamp == null) return 'Unavailable'
    const minutes = Math.max(
      0,
      Math.floor((Date.now() - Number(timestamp / 1_000_000n)) / 60_000)
    )
    return minutes === 0 ? 'just now' : `${minutes}m ago`
  }
  function bytes(value: number) {
    const units = ['bytes', 'KB', 'MB', 'GB', 'TB']
    let unit = 0
    while (value >= 1000 && unit < units.length - 1) {
      value /= 1000
      unit++
    }
    return `${value.toLocaleString(undefined, { maximumFractionDigits: 1 })} ${units[unit]}`
  }
</script>

{#snippet attributes()}
  <DetailBubble label="Attributes" disabled={!selected}>
    <AttributeDiscovery
      service={selected}
      {range}
      {queries}
      {revision}
      {selectedAttribute}
      selectedValue={selectedValue?.identity}
      onSelectAttribute={selectAttribute}
    />
  </DetailBubble>
{/snippet}
{#snippet foundOn()}
  <DetailBubble
    label={selectedAttribute?.key ?? 'Telemetry'}
    layoutKey="attribute-telemetry"
    disabled={!selected}
    contained
  >
    {#snippet labelSuffix()}
      {#if selectedAttribute}
        <span class="text-rp-subtle" aria-hidden="true">·</span>
        <span class="text-secondary">{selectedAttribute.kind}</span>
        <span class="text-rp-subtle" aria-hidden="true">·</span>
        <span class="text-base-content">{selectedAttribute.ownerType}</span>
      {/if}
    {/snippet}
    {#if selected}
      <AttributeTelemetry
        attribute={selectedAttribute}
        valueIdentity={selectedValue?.identity}
        service={selected}
        options={{ ...range, queries }}
        {revision}
      />
    {/if}
  </DetailBubble>
{/snippet}
{#snippet storage()}
  <DetailBubble label="Storage" initialOpen={false}>
    {#if statsError}<p class="px-3" role="alert">
        {statsError}{stats ? ' · Showing the last successful reading.' : ''}
      </p>{/if}
    {#if stats?.storage}{@const stored = stats.storage}
      <table class="detail-fields w-full" aria-label="Viewer storage">
        <tbody>
          <DetailProperty
            fieldName="Database size"
            fieldType="uint64"
            showType={false}
            fieldValue={String(stored.sizeBytes)}
          >
            {#snippet value()}<span
                title={`${stored.sizeBytes.toLocaleString()} bytes`}
                >{bytes(stored.sizeBytes)}</span
              >{/snippet}
          </DetailProperty>
          <DetailProperty
            fieldName="Retention limit"
            fieldType="uint64"
            showType={false}
            fieldValue={String(stored.maxSizeBytes)}
          >
            {#snippet value()}<span
                title={`${stored.maxSizeBytes.toLocaleString()} bytes`}
                >{stored.maxSizeBytes === 0
                  ? 'No size limit'
                  : bytes(stored.maxSizeBytes)}</span
              >{/snippet}
          </DetailProperty>
        </tbody>
      </table>{:else}<p class="p-3 text-rp-subtle">
        Storage information unavailable.
      </p>{/if}
  </DetailBubble>
{/snippet}
{#snippet issues()}
  <DetailBubble
    label="Ingestion issues"
    count={issueCount}
    initialOpen={window.location.hash === `#${INGESTION_ISSUES_ID}`}
  >
    <IngestionIssues
      {rejections}
      {importFailures}
      {rejectionLabel}
      formatRelativeTime={relative}
    />
  </DetailBubble>
{/snippet}

<PageLayout
  mainTitle="Services"
  items={[]}
  emptyDrawer
  selectedID={null}
  drawerID="signal-drawer"
  drawerLabel="Getting started"
  mainSurface="base"
  onRefresh={submit}
  {loading}
>
  {#snippet drawerSearch()}
    <form
      class="flex w-full min-w-0 items-center gap-1"
      onsubmit={event => {
        event.preventDefault()
        submit()
      }}
    >
      <input
        class="input typed-field typed-field--pill min-w-0 flex-1"
        type="search"
        aria-label="Search all telemetry"
        placeholder="Search all telemetry…"
        bind:value={draft}
      />
      <button
        type="button"
        class="btn btn-circle btn-ghost btn-sm shrink-0"
        aria-label="Clear search"
        onclick={() => {
          draft = ''
          submit()
        }}
        ><HugeiconsIcon
          icon={Cancel01Icon}
          size={17}
          strokeWidth={1.5}
          aria-hidden="true"
        /></button
      >
    </form>
  {/snippet}
  {#snippet drawerContent()}
    <GettingStarted {importContent} />
  {/snippet}
  {#snippet main()}
    <WorkspaceHeader mode="title" pill title="Services" ariaLabel="Services">
      {#snippet right()}<span class="text-rp-subtle"
          >{loaded
            ? `[${totalServices ?? `${offset + services.length}${more ? '+' : ''}`}]`
            : '[—]'}</span
        >{/snippet}
    </WorkspaceHeader>
    <div class="service-scroll" aria-busy={loading}>
      {#if error}<p role="alert" class="p-3">
          {error}{loaded ? ' · Showing the last successful reading.' : ''}
        </p>{/if}
      {#if loading && !loaded}<p role="status" class="p-3 text-rp-subtle">
          Loading services…
        </p>
      {:else if loaded && services.length === 0}<p class="p-3 text-rp-subtle">
          {term
            ? 'No services match this search.'
            : 'No services in this time range. Import a file or connect an exporter to get started.'}
        </p>{/if}
      <div class="service-grid">
        {#each services as service (serviceIdentity(service))}
          <ServiceCard
            {service}
            selected={selected !== null &&
              serviceIdentity(selected) === serviceIdentity(service)}
            onSelect={() => {
              selected = service
              selectedAttribute = null
              selectedValue = undefined
              detailPanel.setOpen(true)
            }}
          />
        {/each}
      </div>
      {#if offset > 0 || more}
        <div class="flex gap-2 py-2">
          <button
            class="btn btn-ghost btn-sm"
            disabled={loading || offset === 0}
            onclick={() => load(offset - 100)}>Previous</button
          ><button
            class="btn btn-ghost btn-sm"
            disabled={loading || !more}
            onclick={() => load(offset + 100)}>Next</button
          >
        </div>
      {/if}
    </div>
  {/snippet}
  {#snippet detail()}
    <DetailStack
      label="Home details"
      storageKey="home-details"
      sections={[
        { id: 'found-on', content: foundOn },
        { id: 'attributes', content: attributes },
        { id: 'storage', content: storage },
        { id: 'issues', content: issues },
      ]}
    />
  {/snippet}
</PageLayout>

<style lang="postcss">
  @reference '../app.css';
  .service-scroll {
    @apply min-h-0 flex-1 overflow-y-auto p-3;
  }
  .service-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, min(100%, 24rem));
    gap: var(--header-gap);
    align-items: start;
  }
</style>
