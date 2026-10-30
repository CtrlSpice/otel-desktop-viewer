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
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  import IngestionIssues, {
    INGESTION_ISSUES_ID,
  } from '@/components/shared/IngestionIssues.svelte'
  import HomeSidebar from '@/components/home/HomeSidebar.svelte'
  import GettingStarted from '@/components/home/GettingStarted.svelte'
  import AttributeTelemetry from '@/components/home/AttributeTelemetry.svelte'
  import ServiceMap from '@/components/home/ServiceMap.svelte'
  import ServiceConnectionDetails from '@/components/home/ServiceConnectionDetails.svelte'
  import type { MapSelection } from '@/components/home/service-map'
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
  const signals = SIGNAL_ORDER
  let fields = homeFields(signals.map(getStaticFieldsForSearch))
  let fieldsReady = false
  let fieldRequest: Promise<void> | undefined
  let submission = 0
  let queries = $state(homeQueries(null))
  let pendingQueries = homeQueries(null)
  let totalServices = $state<number>()
  let alive = true
  let initialized = false
  let services = $state<ServiceSummary[]>([])
  let mapSelection = $state<MapSelection | null>(null)
  const inspectedService = $derived(
    mapSelection?.kind === 'service' ? mapSelection.service : null
  )
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
  function inspect(selection: MapSelection) {
    mapSelection = selection
    detailPanel.setOpen(true)
  }
  let range = $state<Pick<DiscoveryOptions, 'startTime' | 'endTime'>>({
    startTime: null,
    endTime: null,
  })
  const discoveryScope = $derived({ ...range, queries })
  let request = $state.raw<AbortController>()
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
      if (refresh || !loaded) {
        range = nextRange
        queries = options.queries
      }
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
      // Connection totals belong to the old discovery scope. Invalidate that
      // inspection on successful refresh, retaining its source-service context.
      const previousService =
        inspectedService ??
        (mapSelection?.kind === 'connection'
          ? mapSelection.connection.source
          : null)
      if (previousService && refresh) {
        const selected =
          services.find(
            service =>
              serviceIdentity(service) === serviceIdentity(previousService)
          ) ??
          services[0] ??
          null
        mapSelection = selected ? { kind: 'service', service: selected } : null
      } else if (!mapSelection && services[0]) {
        mapSelection = { kind: 'service', service: services[0] }
      }
      if (refresh) {
        revision++
      }
    } catch (reason) {
      if (!controller.signal.aborted) error = String(reason)
    } finally {
      if (!controller.signal.aborted) loading = false
    }
  }
  function ensureFields(): Promise<void> {
    if (fieldsReady) return Promise.resolve()
    fieldRequest ??= Promise.all(signals.map(getDynamicAttributes)).then(
      groups => {
        if (!alive) return
        fields = homeFields([fields, ...groups])
        fieldsReady = true
      }
    )
    return fieldRequest
  }
  async function submit() {
    const text = draft
    const current = ++submission
    try {
      if (text.trim()) await ensureFields()
      if (!alive || current !== submission || text !== draft) return
      pendingQueries = homeQueries(parseQuery(text, fields))
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
    } catch (reason) {
      if (alive && current === submission)
        error = reason instanceof Error ? reason.message : String(reason)
    }
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

{#snippet foundOn()}
  <AttributeTelemetry
    attribute={null}
    service={inspectedService}
    options={discoveryScope}
    {revision}
  />
{/snippet}
{#snippet connectionDetails()}
  {#if mapSelection?.kind === 'connection'}
    <ServiceConnectionDetails
      connection={mapSelection.connection}
      options={discoveryScope}
      {revision}
    />
  {/if}
{/snippet}
{#snippet storage()}
  <DetailBubble label="Storage" initialOpen={false}>
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
            fieldValue={stored.inMemory === undefined
              ? 'DuckDB'
              : `DuckDB · ${stored.inMemory ? 'In memory' : 'On disk'}`}
          />
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
      </table>{:else}<p class="p-3 text-subtle">
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
      {totalServices}
      loadingServices={loading}
      servicesRequestKey={request?.signal}
      servicesError={error}
      moreServices={more}
      onMoreServices={() => load(offset + 100)}
      options={discoveryScope}
      {revision}
      selection={mapSelection}
      onSelect={inspect}
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
