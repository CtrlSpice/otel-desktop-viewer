<script module lang="ts">
  export function rejectionLabel(kind: string): string {
    switch (kind) {
      case 'span_already_stored':
        return 'duplicate span id'
      case 'span_refused':
        return 'rejected by the store'
      case 'receiver_recursion_depth':
        return 'receiver protobuf nesting limit'
      default:
        return kind
    }
  }
</script>

<script lang="ts">
  import { onDestroy, onMount, untrack, type Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import { createRefreshCoordinator } from '@/services/refresh-coordinator'
  import { createRead } from '@/services/reads'
  import { createPageReadGroup } from '@/contexts/page-read-group.svelte'
  import { getDataChanges } from '@/contexts/data-changes.svelte'
  import PageLayout from '@/components/shared/Layout/PageLayout.svelte'
  import DetailStack from '@/components/shared/Detail/DetailStack.svelte'
  import DetailBubble from '@/components/shared/Detail/DetailBubble.svelte'
  import DetailProperty from '@/components/shared/Detail/DetailProperty.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import IngestionIssues, {
    INGESTION_ISSUES_ID,
  } from '@/components/shared/IngestionIssues.svelte'
  import HomeSidebar from '@/features/home/HomeSidebar.svelte'
  import GettingStarted from '@/features/home/GettingStarted.svelte'
  import ServiceTelemetry from '@/features/home/ServiceTelemetry.svelte'
  import ServiceMap from '@/features/home/ServiceMap.svelte'
  import ServiceConnectionDetails from '@/features/home/ServiceConnectionDetails.svelte'
  import type { MapSelection } from '@/features/home/service-map'
  import {
    homeFields,
    homeQueries,
    queryText,
  } from '@/features/home/home-search'
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
  import type { FieldDefinition } from '@/search/model'

  let {
    importContent,
    importFailures = [],
  }: { importContent?: Snippet; importFailures?: readonly ImportFailure[] } =
    $props()
  const time = getTimeContext()
  const readGroup = createPageReadGroup(() => void load(0, true))
  const listRead = createRead<
    Awaited<ReturnType<typeof telemetryAPI.discoverServices>>
  >({ formatError: String })
  const pageRead = createRead<
    Awaited<ReturnType<typeof telemetryAPI.discoverServices>>
  >({ formatError: String })
  const statsRead = createRead<Stats>({ group: null, formatError: String })
  const navigation = getNavigationCounts()
  const signals = SIGNAL_ORDER
  let fields = homeFields(signals.map(getStaticFieldsForSearch))
  const fieldRead = createRead<FieldDefinition[]>()
  const submissionRead = createRead<void>()
  let queries = $state(homeQueries(null))
  let pendingQueries = homeQueries(null)
  let totalServices = $state<number>()
  let initialized = false
  let services = $state<ServiceSummary[]>([])
  let mapSelection = $state.raw<MapSelection | null>(null)
  const inspectedService = $derived(
    mapSelection?.kind === 'service' ? mapSelection.service : null
  )
  let stats = $state<Stats | null>(null)
  const statsError = $derived(statsRead.error ?? '')
  const statsLoading = $derived(statsRead.pending)
  const error = $derived(submissionRead.error || listRead.error || '')
  const pageError = $derived(pageRead.error ?? '')
  let draft = $state(readRoute().query.q ?? '')
  let routeQuery = readRoute().query.q ?? ''
  let term = $state('')
  let offset = $state(0)
  let more = $state(false)
  const loaded = $derived(listRead.data !== undefined)
  let revision = $state(0)
  function inspect(selection: MapSelection) {
    mapSelection = selection
    detailPanel.setOpen(true)
  }
  let range = $state<Pick<DiscoveryOptions, 'startTime' | 'endTime'>>({
    startTime: null,
    endTime: null,
  })
  const discoveryScope = $derived({ ...range, queries })
  // A relative time window moves on every Live refresh; the selection doesn't.
  const telemetrySelection = $derived(JSON.stringify([time.selection, term]))
  let displayedListRequest:
    | { range: typeof range; queries: typeof queries; searchActive: boolean }
    | undefined
  const loading = $derived(listRead.pending || pageRead.pending)
  const requestKey = $derived(`${listRead.requestKey}:${pageRead.requestKey}`)
  const rejections = $derived(stats?.rejections ?? [])
  const issueCount = $derived(
    stats
      ? importFailures.length +
          rejections.reduce((sum, entry) => sum + entry.occurrences, 0)
      : undefined
  )

  const statsRefresh = createRefreshCoordinator(() =>
    statsRead.load(
      signal => telemetryAPI.getStats(signal),
      value => {
        stats = value
      }
    )
  )
  const changes = getDataChanges()
  if (changes) onDestroy(changes.subscribe(() => void statsRefresh.request()))

  function refresh() {
    void statsRefresh.request()
    void readGroup.refresh()
  }

  async function load(pageOffset = 0, refresh = false) {
    if (!refresh && (pageRead.pending || !more || !displayedListRequest)) return
    let nextRange = range
    if (refresh) {
      const bounds = selectionToQueryRangeNs(time.selection, Date.now())
      nextRange = {
        startTime: bounds.startTime?.toString() ?? null,
        endTime: bounds.endTime?.toString() ?? null,
      }
    }
    // Pagination belongs to the received list, independently of its replacement.
    const reading =
      !refresh && displayedListRequest
        ? displayedListRequest
        : {
            range: nextRange,
            queries: pendingQueries,
            searchActive: term.trim().length > 0,
          }
    const options = {
      ...reading.range,
      queries: reading.queries,
      offset: pageOffset,
      limit: 100,
    }
    if (refresh) {
      range = nextRange
      queries = options.queries
      revision++
    }
    await (refresh ? listRead : pageRead).load(
      signal => telemetryAPI.discoverServices(options, signal),
      page => {
        if (refresh) {
          // Only a successful replacement invalidates the retained list's page.
          pageRead.clear()
        }
        services =
          pageOffset === 0
            ? page.items
            : [
                ...new Map(
                  [...services, ...page.items].map(service => [
                    serviceIdentity(service),
                    service,
                  ])
                ).values(),
              ]
        displayedListRequest = reading
        totalServices = page.totals?.home
        navigation?.setHomeCounts(
          page.totals ?? {},
          {
            traces: discoverySignalHref(
              'traces',
              reading.range,
              queryText(reading.queries.traces)
            ),
            metrics: discoverySignalHref(
              'metrics',
              reading.range,
              queryText(reading.queries.metrics)
            ),
            logs: discoverySignalHref(
              'logs',
              reading.range,
              queryText(reading.queries.logs)
            ),
          },
          reading.searchActive
        )
        more = page.truncated
        offset = pageOffset
        // Connection inspection follows its own received graph membership.
        const previousService = inspectedService
        if (previousService && refresh) {
          const selected =
            services.find(
              service =>
                serviceIdentity(service) === serviceIdentity(previousService)
            ) ??
            services[0] ??
            null
          if (
            !selected ||
            serviceIdentity(selected) !== serviceIdentity(previousService)
          )
            mapSelection = selected
              ? { kind: 'service', service: selected }
              : null
        } else if (!mapSelection && services[0]) {
          mapSelection = { kind: 'service', service: services[0] }
        }
      }
    )
  }
  function ensureFields(): Promise<void> {
    if (!fieldRead.pending && !fieldRead.data)
      void fieldRead.load(
        async abort => {
          const groups = await Promise.all(
            signals.map(signal => getDynamicAttributes(signal, abort))
          )
          return homeFields([fields, ...groups])
        },
        received => {
          fields = received
        }
      )
    return fieldRead.completion
  }
  async function submit() {
    const text = draft
    await submissionRead.load(async (signal, publish) => {
      if (text.trim()) await ensureFields()
      signal.throwIfAborted()
      if (text !== draft) return
      const parsed = homeQueries(parseQuery(text, fields))
      publish(() => {
        pendingQueries = parsed
        initialized = true
        term = text
        routeQuery = text
        const route = readRoute()
        if ((route.query.q ?? '') !== text)
          navigateCurrentRoute(
            { ...route.query, q: text || undefined },
            'replace'
          )
        void load(0, true)
      })
    })
  }
  $effect(() => {
    void time.selection
    untrack(() => {
      if (!initialized) {
        void submit()
      } else void load(0, true)
    })
  })
  onMount(() => {
    void statsRefresh.request()
    return subscribeToRoute(() => {
      const route = readRoute()
      if (route.path !== '/') return
      const next = route.query.q ?? ''
      if (next === routeQuery) return
      routeQuery = next
      draft = next
      void submit()
    })
  })
  onDestroy(() => {
    statsRefresh.dispose()
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

{#snippet foundOn()}
  <ServiceTelemetry
    service={inspectedService}
    options={discoveryScope}
    selection={telemetrySelection}
    {revision}
  />
{/snippet}
{#snippet connectionDetails()}
  {#if mapSelection?.kind === 'connection'}
    <ServiceConnectionDetails
      connection={mapSelection.connection}
      options={mapSelection.options}
      revision={mapSelection.revision}
    />
  {/if}
{/snippet}
{#snippet storage()}
  <DetailBubble label="Storage" initialOpen={false}>
    <LoadingStatus
      pending={statsLoading}
      requestKey={statsRead.requestKey}
      label="Loading storage information…"
      class="block px-3 text-subtle"
    />
    {#if statsError}<p class="px-3 text-papaya" role="alert">
        {statsError}{stats ? ' · Showing the last successful reading.' : ''}
      </p>{/if}
    {#if stats?.storage}{@const stored = stats.storage}
      <table class="detail-fields w-full" aria-label="Viewer storage">
        <tbody>
          <DetailProperty
            fieldName="Database"
            fieldType="string"
            showType={false}
            fieldValue="DuckDB · On disk"
          />
          <DetailProperty
            fieldName="Database size"
            fieldType="uint64"
            showType={false}
            fieldValue={String(stored.sizeBytes)}
          >
            {#snippet value()}<span
                title={`${stored.sizeBytes.toLocaleString()} bytes`}
              >
                {bytes(stored.sizeBytes)}
              </span>{/snippet}
          </DetailProperty>
          <DetailProperty
            fieldName="Retention limit"
            fieldType="uint64"
            showType={false}
            fieldValue={String(stored.maxSizeBytes)}
          >
            {#snippet value()}<span
                title={`${stored.maxSizeBytes.toLocaleString()} bytes`}
              >
                {stored.maxSizeBytes === 0
                  ? 'No size limit'
                  : bytes(stored.maxSizeBytes)}
              </span>{/snippet}
          </DetailProperty>
        </tbody>
      </table>{:else if !statsLoading && !statsError}<p class="p-3 text-subtle">
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
    <LoadingStatus
      pending={statsLoading}
      requestKey={statsRead.requestKey}
      label="Loading ingestion issues…"
      class="block px-3 text-subtle"
    />
    {#if statsError}<p class="px-3 text-papaya" role="alert">
        {statsError}{stats ? ' · Showing the last successful reading.' : ''}
      </p>{/if}
    <IngestionIssues
      {rejections}
      {importFailures}
      {rejectionLabel}
      formatRelativeTime={relative}
    />
  </DetailBubble>
{/snippet}

{#snippet guide()}
  <DetailBubble label="Getting started" contained headerless>
    <GettingStarted {importContent} />
  </DetailBubble>
{/snippet}

<PageLayout
  mainTitle="Service map"
  items={[]}
  emptyDrawer
  selectedID={null}
  drawerID="signal-drawer"
  drawerLabel="Home"
  revealDrawer={window.location.hash === `#${INGESTION_ISSUES_ID}`}
  mainSurface="base"
  onRefresh={refresh}
  loading={listRead.pending}
  loadingRequestKey={String(requestKey)}
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
        aria-label="Search telemetry"
        placeholder="Search telemetry…"
        bind:value={draft}
        onfocus={() => {
          void ensureFields()
        }}
      />
      <button
        type="button"
        class="btn btn-circle btn-ghost btn-sm shrink-0"
        aria-label="Clear search"
        onclick={() => {
          draft = ''
          submit()
        }}
      >
        <HugeiconsIcon
          icon={Cancel01Icon}
          size={17}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      </button>
    </form>
  {/snippet}
  {#snippet drawerContent()}
    <HomeSidebar
      sections={[
        { id: 'guide', content: guide },
        { id: 'storage', content: storage },
        { id: 'issues', content: issues },
      ]}
    />
  {/snippet}
  {#snippet main()}
    <ServiceMap
      service={inspectedService}
      {services}
      servicesLoaded={loaded}
      {totalServices}
      loadingServices={loading}
      loadingMoreServices={pageRead.pending}
      servicesRequestKey={requestKey}
      servicesError={[error, pageError].filter(Boolean).join(' · ')}
      moreServices={more}
      onMoreServices={() => load(offset + 100)}
      options={discoveryScope}
      {revision}
      selection={mapSelection}
      onSelect={inspect}
      onUpdateSelection={selection => (mapSelection = selection)}
    />
  {/snippet}
  {#snippet detail()}
    <DetailStack
      label="Home details"
      storageKey="home-details"
      sections={mapSelection?.kind === 'connection'
        ? [{ id: 'connection', content: connectionDetails }]
        : [{ id: 'found-on', content: foundOn }]}
    />
  {/snippet}
</PageLayout>
