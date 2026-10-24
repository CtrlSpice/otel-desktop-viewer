<script lang="ts">
  import { untrack } from 'svelte'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import LogCard from '@/components/logs/LogCard.svelte'
  import MetricCard from '@/components/metrics/MetricCard.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import DetailNav from '@/components/shared/DetailNav.svelte'
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
    service: ServiceSummary
    options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
    revision: number
  } = $props()
  type Group = {
    signal: DiscoverySignal
    cards: AttributeTelemetryCard[]
    loading: boolean
    error: string
    pageIndex: number
  }
  const PAGE_SIZE = 25
  let groups = $state<Group[]>([])
  let retry = $state(0)
  const labels = { traces: 'Traces', logs: 'Logs', metrics: 'Metrics' }
  $effect(() => {
    void revision
    void retry
    const controller = new AbortController()
    const selection = { attribute, valueIdentity, service, options }
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
      }))
    untrack(() => {
      for (const group of groups) {
        void loadAttributeTelemetry(
          group.signal,
          selection.attribute,
          selection.valueIdentity,
          selection.service,
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

<div class="p-3" aria-busy={groups.some(group => group.loading)}>
  {#each groups as group (group.signal)}
    {#if group.loading || group.error || group.cards.length}
      <section aria-label={`Matching ${group.signal}`}>
        <FieldGroup label={labels[group.signal]} detail bare>
          {#snippet heading()}
            <h3 class="text-sm font-normal text-rp-subtle">
              {labels[group.signal]}{#if !group.loading && !group.error}
                {' '}[{group.cards.length.toLocaleString()}]{/if}
            </h3>
          {/snippet}
          {#if group.loading}<p role="status" class="text-rp-subtle">
              Loading matching {group.signal}…
            </p>
          {:else if group.error}<p role="alert">{group.error}</p>
            <button class="btn btn-ghost btn-sm" onclick={() => retry++}
              >Retry</button
            >
          {:else}
            {@const pageCount = Math.ceil(group.cards.length / PAGE_SIZE)}
            {@const pageStart = group.pageIndex * PAGE_SIZE}
            {#each group.cards.slice(pageStart, pageStart + PAGE_SIZE) as card (cardID(card))}
              {#if card.signal === 'traces'}
                <TraceCard
                  trace={card.trace}
                  matchingSpanCount={card.count}
                  href={itemHref('traces', card.trace.traceID, {
                    span: card.trace.matchedSpans?.[0]?.spanID,
                  })}
                />
              {:else if card.signal === 'logs'}
                <LogCard
                  log={card.log}
                  href={itemHref('logs', card.log.logRef)}
                />
              {:else}
                <MetricCard
                  metric={card.metric}
                  matchingPointCount={card.count}
                  href={itemHref('metrics', card.metric.metricRef)}
                />
              {/if}
            {/each}
            {#if pageCount > 1}
              <div class="flex justify-center py-2">
                <DetailNav
                  index={group.pageIndex}
                  total={pageCount}
                  label="page"
                  onFirst={() => (group.pageIndex = 0)}
                  onPrev={() => group.pageIndex--}
                  onNext={() => group.pageIndex++}
                  onLast={() => (group.pageIndex = pageCount - 1)}
                />
              </div>
            {/if}
          {/if}
        </FieldGroup>
      </section>
    {/if}
  {/each}
  {#if groups.every(group => !group.loading && !group.error && !group.cards.length)}<p
      class="text-rp-subtle"
    >
      No matching telemetry in this service, search and time range.
    </p>{/if}
</div>
