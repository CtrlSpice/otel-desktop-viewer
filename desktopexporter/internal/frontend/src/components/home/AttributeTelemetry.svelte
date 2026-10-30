<script lang="ts">
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import { untrack } from 'svelte'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import LogCard from '@/components/logs/LogCard.svelte'
  import MetricCard from '@/components/metrics/MetricCard.svelte'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import PageSizeSelect from '@/components/shared/PageSizeSelect.svelte'
  import { loadPageSize, savePageSize, type PageSize } from '@/utils/page-size'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import { itemHref } from '@/route'
  import type {
    DiscoveryOptions,
    DiscoverySignal,
    ServiceSummary,
  } from '@/services/discovery-types'
  import {
    attributeOwners,
    discoverySignals,
    type DiscoveredAttribute,
  } from './attribute-coverage'
  import {
    loadAttributeTelemetry,
    type AttributeTelemetryCard,
  } from './attribute-telemetry'

  let {
    attribute,
    valueIdentity,
    service,
    options,
    revision,
  }: {
    attribute: DiscoveredAttribute | null
    valueIdentity?: string
    service: ServiceSummary | null
    options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
    revision: number
  } = $props()
  type Group = {
    signal: DiscoverySignal
    cards: AttributeTelemetryCard[]
    loading: boolean
    error: string
    pageIndex: number
    scrollTop: number
  }
  const PAGE_SIZE_KEY = 'home-telemetry-page-size'
  let pageSize = $state(loadPageSize(PAGE_SIZE_KEY))
  let groups = $state<Group[]>([])
  let loading = $state(false)
  let requestKey = $state(0)
  let displayedAttribute = $state<DiscoveredAttribute | null>(null)
  let selectedSignal = $state<DiscoverySignal>('traces')
  let viewport = $state<HTMLDivElement>()
  const panelID = $props.id()
  const labels = { traces: 'Traces', logs: 'Logs', metrics: 'Metrics' }
  const available = $derived(
    groups.filter(group => group.loading || group.error || group.cards.length)
  )
  const active = $derived(
    available.find(group => group.signal === selectedSignal) ?? available[0]
  )
  const tabs = $derived(
    discoverySignals.map(signal => {
      const group = groups.find(group => group.signal === signal)
      return {
        id: signal,
        label: labels[signal],
        countBrackets: true,
        count:
          group && !group.loading && !group.error
            ? group.cards.length
            : undefined,
        disabled: !available.some(group => group.signal === signal),
      }
    })
  )
  const pageCount = $derived(Math.ceil((active?.cards.length ?? 0) / pageSize))
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
    if (!active || loading || active.error) return
    active.pageIndex = Math.max(0, Math.min(index, pageCount - 1))
    active.scrollTop = 0
  }
  $effect(() => {
    const group = active
    void group?.pageIndex
    const element = viewport
    untrack(() => {
      if (element) element.scrollTop = group?.scrollTop ?? 0
    })
  })
  $effect(() => {
    void revision
    const controller = new AbortController()
    const selection = { attribute, valueIdentity, service, options }
    requestKey = untrack(() => requestKey) + 1
    if (!selection.service) {
      groups = []
      displayedAttribute = null
      loading = false
      return
    }
    loading = true
    const selectedService = selection.service
    const signals = discoverySignals.filter(
      signal =>
        (!attribute || attributeOwners[signal].includes(attribute.ownerType)) &&
        options.queries?.[signal] !== false
    )
    const pending = signals.map((signal): Group => ({
      signal,
      cards: [],
      loading: true,
      error: '',
      pageIndex: 0,
      scrollTop: 0,
    }))
    if (untrack(() => groups.length === 0)) {
      groups = pending
      displayedAttribute = selection.attribute
    }
    void Promise.all(
      pending.map(async group => {
        try {
          group.cards = await loadAttributeTelemetry(
            group.signal,
            selection.attribute,
            selection.valueIdentity,
            selectedService,
            selection.options,
            controller.signal
          )
        } catch (error) {
          group.error = String(error)
        }
        group.loading = false
        return group
      })
    ).then(result => {
      if (controller.signal.aborted) return
      groups = result
      displayedAttribute = selection.attribute
      loading = false
    })
    return () => controller.abort()
  })
  function cardID(card: AttributeTelemetryCard) {
    return card.signal === 'traces'
      ? card.trace.traceID
      : card.signal === 'logs'
        ? card.log.logRef
        : card.metric.metricRef
  }
</script>

{#snippet signalTabs()}
  <div class="attribute-telemetry__tabs">
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
  label={displayedAttribute?.key ?? 'Telemetry'}
  layoutKey="attribute-telemetry"
  hideLabel={!displayedAttribute}
  disabled={!service}
  contained
>
  {#snippet labelSuffix()}
    {#if displayedAttribute}
      <span class="text-subtle" aria-hidden="true">·</span>
      <span class="text-secondary">{displayedAttribute.kind}</span>
      <span class="text-subtle" aria-hidden="true">·</span>
      <span class="text-base-content">{displayedAttribute.ownerType}</span>
    {/if}
  {/snippet}
  {#snippet action()}{@render signalTabs()}{/snippet}
  <div
    id={panelID}
    role="tabpanel"
    aria-labelledby={active ? paneTabID(panelID, active.signal) : undefined}
    aria-label={active ? undefined : 'Matching telemetry'}
    class="attribute-telemetry"
  >
    <!-- Keyboard users can focus and scroll the card list independently. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div
      bind:this={viewport}
      class="attribute-telemetry__cards"
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
        {requestKey}
      />
      {#if active?.error}<p role="alert" class="text-papaya">
          {active.error}
        </p>
      {/if}
      {#if active}
        {#each page as card (cardID(card))}
          {#if card.signal === 'traces'}
            <TraceCard
              trace={card.trace}
              matchingSpanCount={card.count}
              href={itemHref('traces', card.trace.traceID, {
                span: card.trace.matchedSpans?.[0]?.spanID,
              })}
            />
          {:else if card.signal === 'logs'}
            <LogCard log={card.log} href={itemHref('logs', card.log.logRef)} />
          {:else}
            <MetricCard
              metric={card.metric}
              matchingPointCount={card.count}
              href={itemHref('metrics', card.metric.metricRef)}
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
        <div>
          <DetailNav
            index={active?.pageIndex ?? -1}
            total={pageCount}
            label="page"
            onFirst={() => setPage(0)}
            onPrev={() => setPage((active?.pageIndex ?? 0) - 1)}
            onNext={() => setPage((active?.pageIndex ?? 0) + 1)}
            onLast={() => setPage(pageCount - 1)}
          />
        </div>
      {/snippet}
    </SignalFooter>
  </div>
</DetailBubble>

<style lang="postcss">
  @reference '../../app.css';

  .attribute-telemetry {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }
  .attribute-telemetry__tabs {
    @apply flex w-full min-w-0 overflow-x-auto;
  }
  .attribute-telemetry__cards {
    @apply min-h-0 flex-1 overflow-auto px-3 pb-3;
  }
</style>
