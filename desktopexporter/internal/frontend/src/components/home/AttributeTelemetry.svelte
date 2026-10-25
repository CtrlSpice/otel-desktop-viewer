<script lang="ts">
  import { untrack } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import LogCard from '@/components/logs/LogCard.svelte'
  import MetricCard from '@/components/metrics/MetricCard.svelte'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
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
  let retry = $state(0)
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
  const signalChoices = $derived(
    discoverySignals.map(signal => {
      const group = groups.find(group => group.signal === signal)
      return {
        id: signal,
        label: labels[signal],
        count:
          group && !group.loading && !group.error
            ? group.cards.length
            : undefined,
        disabled: !available.some(group => group.signal === signal),
      }
    })
  )
  const activeChoice = $derived(
    signalChoices.find(choice => choice.id === active?.signal)
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
  function navigateSignal(event: KeyboardEvent, signal: DiscoverySignal) {
    const choices = signalChoices.filter(choice => !choice.disabled)
    const index = choices.findIndex(choice => choice.id === signal)
    let next: number
    if (event.key === 'ArrowDown' || event.key === 'ArrowRight')
      next = (index + 1) % choices.length
    else if (event.key === 'ArrowUp' || event.key === 'ArrowLeft')
      next = (index - 1 + choices.length) % choices.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = choices.length - 1
    else return
    event.preventDefault()
    const choice = choices[next]
    if (!choice) return
    selectSignal(choice.id)
    if (event.currentTarget instanceof HTMLElement)
      event.currentTarget.parentElement
        ?.querySelector<HTMLButtonElement>(`[data-signal="${choice.id}"]`)
        ?.focus()
  }
  function setPage(index: number) {
    if (!active || active.loading || active.error) return
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
    void retry
    const controller = new AbortController()
    const selection = { attribute, valueIdentity, service, options }
    if (!selection.service) {
      groups = []
      return
    }
    const selectedService = selection.service
    groups = discoverySignals
      .filter(
        signal =>
          (!attribute ||
            attributeOwners[signal].includes(attribute.ownerType)) &&
          options.queries?.[signal] !== false
      )
      .map(signal => ({
        signal,
        cards: [],
        loading: true,
        error: '',
        pageIndex: 0,
        scrollTop: 0,
      }))
    untrack(() => {
      for (const group of groups) {
        void loadAttributeTelemetry(
          group.signal,
          selection.attribute,
          selection.valueIdentity,
          selectedService,
          selection.options,
          controller.signal
        )
          .then(cards => {
            if (!controller.signal.aborted) group.cards = cards
          })
          .catch(error => {
            if (!controller.signal.aborted) group.error = String(error)
          })
          .finally(() => {
            if (!controller.signal.aborted) group.loading = false
          })
      }
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

{#snippet signalFilter()}
  <FilterPopover
    label={`Filter telemetry${activeChoice ? `: ${activeChoice.label}${activeChoice.count !== undefined ? ` [${activeChoice.count}]` : ''}` : ''}`}
    tooltip="Filter telemetry"
    compact
    showHeader={false}
    focusSelected
    popoverClass="telemetry-signal-filter"
  >
    {#snippet icon()}
      {#if activeChoice}<span
          >{activeChoice.label}{#if activeChoice.count !== undefined}<span
              class="text-subtle tabular-nums"
            >
              [{activeChoice.count}]</span
            >{/if}</span
        >{/if}
      <HugeiconsIcon
        icon={FilterIcon}
        size={16}
        strokeWidth={1.5}
        aria-hidden="true"
      />
    {/snippet}
    {#snippet children(close)}
      <div role="radiogroup" aria-label="Telemetry signals">
        {#each signalChoices as choice (choice.id)}
          <button
            type="button"
            role="radio"
            class="item-state item-state--menu flex w-full items-center justify-between gap-2 px-3 py-2 text-left disabled:text-muted"
            aria-checked={active?.signal === choice.id}
            aria-controls={panelID}
            tabindex={active?.signal === choice.id ? 0 : -1}
            disabled={choice.disabled}
            data-signal={choice.id}
            onkeydown={event => navigateSignal(event, choice.id)}
            onclick={() => {
              selectSignal(choice.id)
              close()
            }}
          >
            <span>{choice.label}</span>
            {#if choice.count !== undefined}<span
                class="tabular-nums"
                class:text-subtle={!choice.disabled}>[{choice.count}]</span
              >{/if}
          </button>
        {/each}
      </div>
    {/snippet}
  </FilterPopover>
{/snippet}

<DetailBubble
  label={attribute?.key ?? 'Telemetry'}
  layoutKey="attribute-telemetry"
  disabled={!service}
  contained
>
  {#snippet labelSuffix()}
    {#if attribute}
      <span class="text-subtle" aria-hidden="true">·</span>
      <span class="text-secondary">{attribute.kind}</span>
      <span class="text-subtle" aria-hidden="true">·</span>
      <span class="text-base-content">{attribute.ownerType}</span>
    {/if}
  {/snippet}
  {#snippet action()}{@render signalFilter()}{/snippet}
  <div id={panelID} class="attribute-telemetry">
    <!-- Keyboard users can focus and scroll the card list independently. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div
      bind:this={viewport}
      class="attribute-telemetry__cards"
      role="region"
      aria-label={`Matching ${active?.signal ?? 'telemetry'}`}
      aria-busy={active?.loading ?? false}
      tabindex="0"
      onscroll={event => {
        if (active) active.scrollTop = event.currentTarget.scrollTop
      }}
    >
      {#if active?.loading}<p role="status" class="text-subtle">
          Loading matching {active.signal}…
        </p>
      {:else if active?.error}<p role="alert" class="text-papaya">
          {active.error}
        </p>
        <button class="btn btn-ghost btn-sm" onclick={() => retry++}
          >Retry</button
        >
      {:else if active}
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
      {:else}<p class="text-subtle">
          No matching telemetry in this service, search and time range.
        </p>{/if}
    </div>
    <SignalFooter label="page" compact>
      {#snippet leading()}
        <PageSizeSelect
          label="Telemetry per page"
          value={pageSize}
          onchange={setPageSize}
        />
      {/snippet}
      {#snippet navigation()}
        <div class:invisible={active?.loading || !!active?.error}>
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
  :global(.filter-popover.telemetry-signal-filter) {
    @apply w-48 p-2;
  }
  .attribute-telemetry__cards {
    @apply min-h-0 flex-1 overflow-auto px-3 pb-3;
  }
</style>
