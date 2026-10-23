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
  import { onDestroy, untrack, type Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    Cancel01Icon,
    BarChartHorizontalIcon,
    ChartHistogramIcon,
  } from '@hugeicons/core-free-icons'
  import { LogIcon } from '@/icons'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import PaneHeader, {
    paneTabID,
    type PaneTab,
  } from '@/components/shared/PaneHeader.svelte'
  import DetailStack from '@/components/shared/DetailStack.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  import IngestionIssues, {
    INGESTION_ISSUES_ID,
  } from '@/components/shared/IngestionIssues.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import GettingStarted from '@/components/home/GettingStarted.svelte'
  import ServiceCard from '@/components/home/ServiceCard.svelte'
  import AttributeDiscovery from '@/components/home/AttributeDiscovery.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    type ServiceSummary,
    type DiscoveryOptions,
    type DiscoverySignal,
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
  let services = $state<ServiceSummary[]>([])
  let selected = $state<ServiceSummary | null>(null)
  let stats = $state<Stats | null>(null)
  let statsError = $state('')
  let error = $state('')
  let loading = $state(false)
  let draft = $state('')
  let term = $state('')
  let offset = $state(0)
  let more = $state(false)
  let loaded = $state(false)
  let revision = $state(0)
  let attributeSignal = $state<DiscoverySignal>('traces')
  const attributePanelID = $props.id()
  const attributeTabs: PaneTab[] = [
    { id: 'traces', label: 'Traces', icon: tracesIcon },
    { id: 'metrics', label: 'Metrics', icon: metricsIcon },
    { id: 'logs', label: 'Logs', icon: logsIcon },
  ]
  function selectAttributeSignal(id: string) {
    if (id === 'traces' || id === 'logs' || id === 'metrics')
      attributeSignal = id
  }
  let refreshedAt = $state<bigint>()
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
    if (refresh) {
      const bounds = selectionToQueryRangeNs(time.selection, Date.now())
      range = {
        startTime: bounds.startTime?.toString() ?? null,
        endTime: bounds.endTime?.toString() ?? null,
      }
    }
    const options = { ...range, term, offset: pageOffset, limit: 100 }
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
      more = page.truncated
      offset = pageOffset
      loaded = true
      if (selected)
        selected =
          services.find(
            service => serviceIdentity(service) === serviceIdentity(selected!)
          ) ?? selected
      else selected = services[0] ?? null
      if (refresh) {
        revision++
        refreshedAt = BigInt(Date.now()) * 1_000_000n
      }
    } catch (reason) {
      if (!controller.signal.aborted) error = String(reason)
    } finally {
      if (!controller.signal.aborted) loading = false
    }
  }
  function submit() {
    term = draft
    void load(0, true)
  }
  $effect(() => {
    void time.selection
    untrack(() => {
      void load(0, true)
    })
  })
  onDestroy(() => request?.abort())
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

{#snippet tracesIcon()}<HugeiconsIcon
    icon={BarChartHorizontalIcon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />{/snippet}
{#snippet logsIcon()}<LogIcon
    width={16}
    height={16}
    aria-hidden="true"
  />{/snippet}
{#snippet metricsIcon()}<HugeiconsIcon
    icon={ChartHistogramIcon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />{/snippet}

{#snippet attributes()}
  <DetailBubble label="Attributes" layoutKey="home-attributes">
    {#snippet trailing()}
      <PaneHeader
        mode="tabs"
        bare
        tabs={attributeTabs}
        activeID={attributeSignal}
        onSelect={selectAttributeSignal}
        ariaLabel="Attribute signal"
        tabPanelID={attributePanelID}
      />
    {/snippet}
    {#snippet labelSuffix()}
      {#if selected}<span class="min-w-0 truncate text-base-content"
          >· {selected.serviceName ||
            'Unnamed service'}{#if selected.serviceNamespace}
            · {selected.serviceNamespace}{/if}</span
        >{/if}
    {/snippet}
    <div
      id={attributePanelID}
      role="tabpanel"
      aria-labelledby={paneTabID(attributePanelID, attributeSignal)}
    >
      <AttributeDiscovery
        service={selected}
        {range}
        {revision}
        signal={attributeSignal}
      />
    </div>
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
    initialOpen={window.location.hash === `#${INGESTION_ISSUES_ID}`}
  >
    {#snippet trailing()}<span class="text-rp-subtle"
        >[{issueCount.toLocaleString()}]</span
      >{/snippet}
    {#if issueCount > 0}<IngestionIssues
        {rejections}
        {importFailures}
        {rejectionLabel}
        formatRelativeTime={relative}
      />
    {:else}<p class="p-3 text-rp-subtle">
        {stats
          ? 'No rejected records or failed file imports in this reading.'
          : 'Ingestion status unavailable.'}
      </p>{/if}
  </DetailBubble>
{/snippet}

<PageLayout
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
        aria-label="Search services"
        placeholder="Search services…"
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
  {#snippet drawerContent()}<GettingStarted
      {importContent}
      {issues}
    />{/snippet}
  {#snippet main()}
    <PaneHeader mode="title" pill title="Services" ariaLabel="Services">
      {#snippet right()}<span class="text-rp-subtle"
          >{loaded
            ? `[${offset + services.length}${more ? '+' : ''}]`
            : '[—]'}</span
        >{/snippet}
    </PaneHeader>
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
            {range}
            selected={selected !== null &&
              serviceIdentity(selected) === serviceIdentity(service)}
            onSelect={() => (selected = service)}
          />
        {/each}
      </div>
    </div>
    <SignalFooter label="services">
      {#snippet leading()}
        <div class="min-w-0">
          <p class="text-rp-subtle">
            {range.startTime == null
              ? 'All stored telemetry'
              : 'Selected time range'}
          </p>
          <SignalDate label="Refreshed" timestamp={refreshedAt} />
        </div>
      {/snippet}
      {#snippet trailing()}
        {#if offset > 0 || more}<div class="flex gap-2">
            <button
              class="btn btn-ghost btn-sm"
              disabled={loading || offset === 0}
              onclick={() => load(offset - 100)}>Previous</button
            ><button
              class="btn btn-ghost btn-sm"
              disabled={loading || !more}
              onclick={() => load(offset + 100)}>Next</button
            >
          </div>{/if}
      {/snippet}
    </SignalFooter>
  {/snippet}
  {#snippet detail()}
    <DetailStack
      label="Home details"
      storageKey="home-details"
      sections={[
        { id: 'attributes', content: attributes },
        { id: 'storage', content: storage },
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
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
    gap: var(--header-gap);
    align-items: start;
  }
</style>
