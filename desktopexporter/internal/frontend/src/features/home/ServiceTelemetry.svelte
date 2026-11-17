<script lang="ts">
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import { untrack } from 'svelte'
  import { createReadMap, type Read } from '@/services/reads'
  import TraceCard from '@/features/traces/TraceCard.svelte'
  import LogCard from '@/features/logs/LogCard.svelte'
  import MetricCard from '@/features/metrics/MetricCard.svelte'
  import PaneHeader, {
    paneTabID,
  } from '@/components/shared/Layout/PaneHeader.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import PageSizeSelect from '@/components/shared/PageSizeSelect.svelte'
  import { loadPageSize, savePageSize, type PageSize } from '@/utils/page-size'
  import DetailNav from '@/components/shared/Detail/DetailNav.svelte'
  import DetailBubble from '@/components/shared/Detail/DetailBubble.svelte'
  import { itemHref, parseRoute } from '@/route'
  import type {
    DiscoveryOptions,
    DiscoverySignal,
    ServiceSummary,
  } from '@/services/discovery-types'
  import {
    discoverySignalHref,
    serviceIdentity,
    SIGNAL_ORDER,
  } from '@/services/discovery-types'
  import {
    loadServiceTelemetry,
    type ServiceTelemetryCard,
  } from './service-telemetry'

  let {
    service,
    options,
    revision,
  }: {
    service: ServiceSummary | null
    options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
    revision: number
  } = $props()
  type Group = {
    signal: DiscoverySignal
    cards: ServiceTelemetryCard[]
    read: Read<ServiceTelemetryCard[]>
    pageIndex: number
    scrollTop: number
    loaded: boolean
    query: ReturnType<typeof parseRoute>['query']
  }
  const PAGE_SIZE_KEY = 'home-telemetry-page-size'
  let pageSize = $state(loadPageSize(PAGE_SIZE_KEY))
  let groups = $state<Group[]>([])
  const reads = createReadMap<DiscoverySignal, ServiceTelemetryCard[]>({
    formatError: String,
    retainError: true,
  })
  const requestKey = $derived(reads.requestKey)
  const serviceKey = $derived(service ? serviceIdentity(service) : null)
  let selectedSignal = $state<DiscoverySignal>('traces')
  let viewport = $state<HTMLDivElement>()
  const panelID = $props.id()
  const labels = { traces: 'Traces', logs: 'Logs', metrics: 'Metrics' }
  const available = $derived(
    groups.filter(
      group => group.read.pending || group.read.error || group.cards.length
    )
  )
  const active = $derived(
    available.find(group => group.signal === selectedSignal) ?? available[0]
  )
  const loading = $derived(active?.read.pending ?? false)
  const tabs = $derived(
    SIGNAL_ORDER.map(signal => {
      const group = groups.find(group => group.signal === signal)
      return {
        id: signal,
        label: labels[signal],
        countBrackets: true,
        count: group?.loaded ? group.cards.length : undefined,
        disabled: !available.some(group => group.signal === signal),
      }
    })
  )
  const pageCount = $derived(
    active && !active.loaded
      ? undefined
      : Math.ceil((active?.cards.length ?? 0) / pageSize)
  )
  const pageStart = $derived((active?.pageIndex ?? 0) * pageSize)
  const page = $derived(
    active?.cards.slice(pageStart, pageStart + pageSize) ?? []
  )

  function setPageSize(size: PageSize) {
    pageSize = size
    savePageSize(PAGE_SIZE_KEY, size)
    for (const group of groups) {
      group.pageIndex = 0
      group.scrollTop = 0
    }
  }

  function selectSignal(id: string) {
    const group = available.find(group => group.signal === id)
    if (group) selectedSignal = group.signal
  }
  function setPage(index: number) {
    if (!active?.loaded || pageCount === undefined) return
    active.pageIndex = Math.max(0, Math.min(index, pageCount - 1))
    active.scrollTop = 0
  }
  $effect(() => {
    const group = active
    void group?.pageIndex
    void group?.scrollTop
    const element = viewport
    untrack(() => {
      if (element) element.scrollTop = group?.scrollTop ?? 0
    })
  })
  $effect(() => {
    void revision
    void serviceKey
    const selection = {
      service: untrack(() => service),
      options,
    }
    if (!selection.service) {
      groups = []
      return
    }
    const selectedService = selection.service
    const signals = SIGNAL_ORDER.filter(
      signal => options.queries?.[signal] !== false
    )
    untrack(() => {
      groups = signals.map((signal): Group => {
        const previous = groups.find(group => group.signal === signal)
        return {
          signal,
          cards: previous?.cards ?? [],
          read: reads.for(signal),
          pageIndex: previous?.pageIndex ?? 0,
          scrollTop: previous?.scrollTop ?? 0,
          loaded: previous?.loaded ?? false,
          // Native item links keep the displayed range and clear Home search.
          query:
            previous?.query ??
            parseRoute(discoverySignalHref(signal, selection.options, ''))
              .query,
        }
      })
      for (const group of groups) {
        void group.read.load(
          signal =>
            loadServiceTelemetry(
              group.signal,
              selectedService,
              selection.options,
              signal
            ),
          cards => {
            group.cards = cards
            group.query = parseRoute(
              discoverySignalHref(group.signal, selection.options, '')
            ).query
            group.loaded = true
            group.pageIndex = 0
            group.scrollTop = 0
          }
        )
      }
    })
    return () => {
      for (const read of reads.values()) read.cancel()
    }
  })
  function cardID(card: ServiceTelemetryCard) {
    return card.signal === 'traces'
      ? card.trace.traceID
      : card.signal === 'logs'
        ? card.log.logRef
        : card.metric.metricRef
  }
</script>

{#snippet signalTabs()}
  <div class="service-telemetry__tabs">
    <PaneHeader
      mode="tabs"
      bare
      tabLayout="equal"
      {tabs}
      activeID={active?.signal ?? ''}
      onSelect={selectSignal}
      tabPanelID={panelID}
      ariaLabel="Telemetry signals"
    />
  </div>
{/snippet}

<DetailBubble
  label="Telemetry"
  layoutKey="attribute-telemetry"
  hideLabel
  disabled={!service}
  contained
>
  {#snippet action()}{@render signalTabs()}{/snippet}
  <div
    id={panelID}
    role="tabpanel"
    aria-labelledby={active ? paneTabID(panelID, active.signal) : undefined}
    aria-label={active ? undefined : 'Matching telemetry'}
    class="service-telemetry"
  >
    <!-- Keyboard users can focus and scroll the card list independently. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div
      bind:this={viewport}
      class="service-telemetry__cards"
      role="region"
      aria-label={`Matching ${active?.signal ?? 'telemetry'}`}
      aria-busy={loading}
      tabindex="0"
      onscroll={event => {
        if (active) active.scrollTop = event.currentTarget.scrollTop
      }}
    >
      <LoadingStatus
        pending={loading}
        label={`Loading matching ${active?.signal ?? 'telemetry'}…`}
        requestKey={`${requestKey}:${active?.signal ?? ''}`}
      />
      {#if active?.read.error}<p role="alert" class="text-papaya">
          {active.read.error}
        </p>
      {/if}
      {#if active}
        {#each page as card (cardID(card))}
          {#if card.signal === 'traces'}
            <TraceCard
              trace={card.trace}
              href={itemHref(
                'traces',
                card.trace.traceID,
                {
                  span: card.trace.matchedSpans?.[0]?.spanID,
                },
                active.query
              )}
            />
          {:else if card.signal === 'logs'}
            <LogCard
              log={card.log}
              href={itemHref('logs', card.log.logRef, {}, active.query)}
            />
          {:else}
            <MetricCard
              metric={card.metric}
              href={itemHref(
                'metrics',
                card.metric.metricRef,
                {},
                active.query
              )}
            />
          {/if}
        {/each}
      {:else if !loading}<p class="text-subtle">
          No matching telemetry in this service, search and time range.
        </p>{/if}
    </div>
    <SignalFooter label="page" compact>
      {#snippet trailing()}
        <PageSizeSelect
          label="Telemetry per page"
          value={pageSize}
          onchange={setPageSize}
        />
      {/snippet}
      {#snippet navigation()}
        <div
          class:service-telemetry__navigation--pending={active &&
            !active.loaded}
        >
          <DetailNav
            index={active?.pageIndex ?? -1}
            total={pageCount}
            busy={!!active && !active.loaded}
            label="page"
            onFirst={() => setPage(0)}
            onPrev={() => setPage((active?.pageIndex ?? 0) - 1)}
            onNext={() => setPage((active?.pageIndex ?? 0) + 1)}
            onLast={() => setPage((pageCount ?? 0) - 1)}
          />
        </div>
      {/snippet}
    </SignalFooter>
  </div>
</DetailBubble>

<style lang="postcss">
  @reference '../../app.css';

  .service-telemetry {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }
  .service-telemetry__tabs {
    @apply flex w-full min-w-0 overflow-x-auto;
  }
  .service-telemetry__cards {
    @apply min-h-0 flex-1 overflow-auto px-3 pb-3;
  }
  .service-telemetry__navigation--pending :global(.detail-nav__counter) {
    visibility: hidden;
  }
</style>
