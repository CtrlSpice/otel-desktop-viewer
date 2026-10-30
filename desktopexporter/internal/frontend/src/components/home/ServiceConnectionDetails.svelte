<script lang="ts">
  import { untrack } from 'svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import DetailBubble from '@/components/shared/DetailBubble.svelte'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    type ServiceConnection,
    type ServiceConnectionSpan,
    type DiscoveryOptions,
  } from '@/services/discovery-types'
  import { itemHref } from '@/route'
  import { formatDuration } from '@/utils/time'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'

  let {
    connection,
    options,
    revision,
  }: {
    connection: ServiceConnection
    options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
    revision: number
  } = $props()
  let spans = $state<ServiceConnectionSpan[]>([])
  let offset = $state(0)
  let more = $state(false)
  let loading = $state(false)
  let error = $state('')
  let retry = $state(0)
  let displayed = $state.raw<ServiceConnection>()
  const shownConnection = $derived(displayed ?? connection)
  let displayedOffset = $state(0)
  let requestKey = $state(0)
  $effect(() => {
    void connection
    void options
    void revision
    offset = 0
  })
  $effect(() => {
    const controller = new AbortController()
    void revision
    void retry
    const selected = connection
    const selectedOffset = offset
    requestKey = untrack(() => requestKey) + 1
    loading = true
    error = ''
    void telemetryAPI
      .discoverServiceConnectionSpans(
        {
          ...options,
          serviceIdentity: serviceIdentity(connection.source),
          targetIdentity: serviceIdentity(connection.target),
          offset,
          limit: 25,
        },
        controller.signal
      )
      .then(page => {
        if (!controller.signal.aborted) {
          spans = page.items
          more = page.truncated
          displayed = selected
          displayedOffset = selectedOffset
        }
      })
      .catch(reason => {
        if (!controller.signal.aborted) error = String(reason)
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false
      })
    return () => controller.abort()
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
    {#if error}<p role="alert" class="text-papaya">{error}</p>
      <button class="btn btn-ghost btn-sm" onclick={() => retry++}>Retry</button
      >
    {/if}
    {#each spans as span (`${span.traceID}:${span.spanID}`)}
      <SignalCard
        id={`${span.traceID}:${span.spanID}`}
        title={span.name}
        href={itemHref('traces', span.traceID, { span: span.spanID })}
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
    {:else}{#if !loading && !error}<p class="text-subtle">
          No connected spans match this search and time range.
        </p>{/if}{/each}
  </div>
  <SignalFooter compact label="page">
    {#snippet navigation()}<div class="flex items-center gap-2">
        <button
          class="btn btn-ghost btn-sm"
          disabled={loading || offset === 0}
          onclick={() => (offset = Math.max(0, offset - 25))}>Previous</button
        ><span class="text-subtle">Page {displayedOffset / 25 + 1}</span><button
          class="btn btn-ghost btn-sm"
          disabled={loading || !more}
          onclick={() => (offset += 25)}>Next</button
        >
      </div>{/snippet}
  </SignalFooter>
</DetailBubble>
