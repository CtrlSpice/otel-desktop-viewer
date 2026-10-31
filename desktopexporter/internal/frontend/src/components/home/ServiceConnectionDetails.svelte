<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    discoverySignalHref,
    type ServiceConnection,
    type ServiceConnectionSpan,
    type DiscoveryOptions,
  } from '@/services/discovery-types'
  import { itemHref, parseRoute } from '@/route'
  import { formatDuration } from '@/utils/time'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import type { ConnectionReading } from './service-map'

  let {
    connection,
    options,
    revision,
  }: {
    connection: ServiceConnection
    options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
    revision: number
  } = $props()
  type EvidenceReading = ConnectionReading & { offset: number }
  type EvidenceWork = {
    kind: 'replacement' | 'page'
    reading: EvidenceReading
    controller: AbortController
    pending: boolean
    error: string
  }
  let spans = $state<ServiceConnectionSpan[]>([])
  let more = $state(false)
  let replacement = $state.raw<EvidenceWork>()
  let pagination = $state.raw<EvidenceWork>()
  const loading = $derived(Boolean(replacement?.pending || pagination?.pending))
  const failures = $derived(
    [replacement, pagination].filter((work): work is EvidenceWork =>
      Boolean(work?.error)
    )
  )
  let displayed = $state.raw<EvidenceReading>()
  const shownConnection = $derived(displayed?.connection ?? connection)
  const evidenceQuery = $derived(
    parseRoute(discoverySignalHref('traces', displayed?.options ?? options, ''))
      .query
  )
  let requestKey = $state(0)
  function publishWork(work: EvidenceWork) {
    if (work.kind === 'replacement') replacement = work
    else pagination = work
  }
  function cancelPagination() {
    pagination?.controller.abort()
    pagination = undefined
  }
  async function load(reading: EvidenceReading, kind: EvidenceWork['kind']) {
    if (kind === 'replacement') replacement?.controller.abort()
    else cancelPagination()
    const controller = new AbortController()
    const work: EvidenceWork = {
      kind,
      reading,
      controller,
      pending: true,
      error: '',
    }
    publishWork(work)
    requestKey++
    try {
      const page = await telemetryAPI.discoverServiceConnectionSpans(
        {
          ...reading.options,
          serviceIdentity: serviceIdentity(reading.connection.source),
          targetIdentity: serviceIdentity(reading.connection.target),
          offset: reading.offset,
          limit: 25,
        },
        controller.signal
      )
      if (controller.signal.aborted) return
      if (kind === 'replacement') {
        // A received replacement supersedes pages of the retained reading.
        cancelPagination()
      }
      spans = page.items
      more = page.truncated
      displayed = reading
    } catch (reason) {
      work.error = String(reason)
    } finally {
      if (!controller.signal.aborted) publishWork({ ...work, pending: false })
    }
  }
  function paginate(step: number) {
    if (displayed)
      void load(
        { ...displayed, offset: Math.max(0, displayed.offset + step) },
        'page'
      )
  }
  $effect(() => {
    const reading = { connection, options, revision, offset: 0 }
    untrack(() => void load(reading, 'replacement'))
  })
  onDestroy(() => {
    replacement?.controller.abort()
    pagination?.controller.abort()
  })
</script>

<DetailBubble label="Connection" layoutKey="connection" contained>
  <div
    class="flex min-h-0 flex-1 flex-col overflow-auto p-3"
    aria-busy={loading}
  >
    <table class="detail-fields w-full" aria-label="Connection details">
      <tbody>
        <DetailProperty
          fieldName="Parent service"
          fieldType={shownConnection.source.serviceNameKind ?? 'string'}
          fieldValue={shownConnection.source.serviceName}
          sourceKey={shownConnection}
          loadText={async signal =>
            (
              await telemetryAPI.getServiceLabels(
                serviceIdentity(shownConnection.source),
                signal
              )
            ).serviceName}
        />
        <DetailProperty
          fieldName="Parent namespace"
          fieldType={shownConnection.source.serviceNamespaceKind ?? 'string'}
          fieldValue={shownConnection.source.serviceNamespace}
          sourceKey={shownConnection}
          loadText={async signal =>
            (
              await telemetryAPI.getServiceLabels(
                serviceIdentity(shownConnection.source),
                signal
              )
            ).serviceNamespace}
        />
        <DetailProperty
          fieldName="Child service"
          fieldType={shownConnection.target.serviceNameKind ?? 'string'}
          fieldValue={shownConnection.target.serviceName}
          sourceKey={shownConnection}
          loadText={async signal =>
            (
              await telemetryAPI.getServiceLabels(
                serviceIdentity(shownConnection.target),
                signal
              )
            ).serviceName}
        />
        <DetailProperty
          fieldName="Child namespace"
          fieldType={shownConnection.target.serviceNamespaceKind ?? 'string'}
          fieldValue={shownConnection.target.serviceNamespace}
          sourceKey={shownConnection}
          loadText={async signal =>
            (
              await telemetryAPI.getServiceLabels(
                serviceIdentity(shownConnection.target),
                signal
              )
            ).serviceNamespace}
        />
        <DetailProperty
          fieldName="Parent-child connections"
          fieldType="number"
          fieldValue={String(shownConnection.spanCount)}
        />
        <DetailProperty
          fieldName="Traces"
          fieldType="number"
          fieldValue={String(shownConnection.traceCount)}
        />
        <DetailProperty
          fieldName="Error child spans"
          fieldType="number"
          fieldValue={String(shownConnection.errorSpanCount)}
        />
      </tbody>
    </table>
    <p class="py-3 text-subtle">
      Recorded child spans connecting these services. Both parent and child
      match the trace search and time range.
    </p>
    <LoadingStatus
      pending={loading}
      label="Loading connected spans…"
      {requestKey}
    />
    {#each failures as failure (failure.kind)}<p
        role="alert"
        class="text-papaya"
      >
        {failure.error}
      </p>
      <button
        class="btn btn-ghost btn-sm"
        onclick={() => load(failure.reading, failure.kind)}>Retry</button
      >
    {/each}
    {#each spans as span (`${span.traceID}:${span.spanID}`)}
      <SignalCard
        id={`${span.traceID}:${span.spanID}`}
        title={span.name}
        href={itemHref(
          'traces',
          span.traceID,
          { span: span.spanID },
          evidenceQuery
        )}
      >
        {#snippet titleContent()}<span class="flex min-w-0 items-center gap-2"
            ><span
              class="size-2 shrink-0 rounded-full"
              style:background-color={traceServiceColor(
                shownConnection.target.serviceName,
                themeSignal.value,
                shownConnection.target.serviceColorKey
              )}
              aria-hidden="true"
            ></span><span class="truncate">{span.name}</span></span
          >{/snippet}
        {#snippet badge()}{#if span.statusCode === 2}<span
              class="flex items-center gap-2"
              ><span class="size-2 rounded-full bg-error" aria-hidden="true"
              ></span><span class="text-subtle">Error</span></span
            >{/if}{/snippet}
        {#snippet date()}<SignalDate
            label="Start"
            timestamp={BigInt(span.startTime)}
          />{/snippet}
        {#snippet dateEnd()}<span
            >Duration: <span class="text-base-content"
              >{formatDuration(
                BigInt(span.endTime) - BigInt(span.startTime)
              )}</span
            ></span
          >{/snippet}
      </SignalCard>
    {:else}{#if !loading && !failures.length}<p class="text-subtle">
          No connected spans match this search and time range.
        </p>{/if}{/each}
  </div>
  <SignalFooter compact label="page">
    {#snippet navigation()}<div class="flex items-center gap-2">
        <button
          class="btn btn-ghost btn-sm"
          disabled={!displayed || pagination?.pending || displayed.offset === 0}
          onclick={() => paginate(-25)}>Previous</button
        ><span class="text-subtle"
          >Page {(displayed?.offset ?? 0) / 25 + 1}</span
        ><button
          class="btn btn-ghost btn-sm"
          disabled={!displayed || pagination?.pending || !more}
          onclick={() => paginate(25)}>Next</button
        >
      </div>{/snippet}
  </SignalFooter>
</DetailBubble>
