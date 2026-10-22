<script lang="ts">
  import PaneHeader, {
    type PaneTab,
  } from '@/components/shared/PaneHeader.svelte'
  import FilterInput from '@/components/shared/Search/FilterInput.svelte'
  import AttributeKey from './AttributeKey.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    type ServiceSummary,
    type DiscoveryOptions,
    type DiscoverySignal,
    type DiscoveryOwner,
    type AttributeKey as Key,
  } from '@/services/discovery-types'
  let {
    service,
    range,
    revision,
  }: {
    service: ServiceSummary | null
    range: Pick<DiscoveryOptions, 'startTime' | 'endTime'>
    revision: number
  } = $props()
  let signal = $state<DiscoverySignal>('traces')
  let owner = $state<DiscoveryOwner>('resource')
  let filter = $state('')
  let term = $state('')
  let offset = $state(0)
  let keys = $state<Key[]>([])
  let more = $state(false)
  let loading = $state(false)
  let error = $state('')
  const signals: PaneTab[] = [
    { id: 'traces', label: 'Traces' },
    { id: 'logs', label: 'Logs' },
    { id: 'metrics', label: 'Metrics' },
  ]
  const owners = $derived<PaneTab[]>([
    { id: 'resource', label: 'Resource' },
    { id: 'scope', label: 'Scope' },
    ...(signal === 'traces'
      ? [
          { id: 'span', label: 'Span' },
          { id: 'event', label: 'Event' },
          { id: 'link', label: 'Link' },
        ]
      : signal === 'logs'
        ? [{ id: 'log', label: 'Log' }]
        : [
            { id: 'datapoint', label: 'Datapoint' },
            { id: 'exemplar', label: 'Exemplar' },
            { id: 'metadata', label: 'Metadata' },
          ]),
  ])
  const options = $derived({
    ...range,
    signal,
    ownerType: owner,
    serviceName: service?.serviceName,
    namespace: service?.serviceNamespace,
  })
  const identity = $derived(service ? serviceIdentity(service) : '')
  $effect(() => {
    const text = filter
    const timer = setTimeout(() => {
      term = text
      offset = 0
    }, 200)
    return () => clearTimeout(timer)
  })
  $effect(() => {
    void identity
    void signal
    void owner
    void revision
    offset = 0
  })
  $effect(() => {
    const active = service
    const request = { ...options, term, offset, limit: 100 }
    void revision
    const controller = new AbortController()
    keys = []
    more = false
    error = ''
    loading = Boolean(active)
    if (active)
      void telemetryAPI
        .discoverAttributeKeys(request, controller.signal)
        .then(page => {
          if (controller.signal.aborted) return
          keys = page.items
          more = page.truncated
        })
        .catch(reason => {
          if (!controller.signal.aborted) error = String(reason)
        })
        .finally(() => {
          if (!controller.signal.aborted) loading = false
        })
    return () => controller.abort()
  })
  function selectSignal(id: string) {
    const selected = signals.find(item => item.id === id)
    if (!selected) return
    signal = selected.id as DiscoverySignal
    owner = 'resource'
  }
  function selectOwner(id: string) {
    if (owners.some(item => item.id === id)) owner = id as DiscoveryOwner
  }
</script>

{#if service}
  <div class="flex flex-wrap items-center gap-2 px-3 py-2">
    <PaneHeader
      mode="tabs"
      bare
      tabs={signals}
      activeID={signal}
      onSelect={selectSignal}
      ariaLabel="Attribute signal"
    />
    <select
      class="select select-ghost select-sm w-auto min-w-0 text-sm"
      aria-label="Attribute owner"
      value={owner}
      onchange={event => selectOwner(event.currentTarget.value)}
    >
      {#each owners as item (item.id)}<option value={item.id}
          >{item.label}</option
        >{/each}
    </select>
  </div>
  <div class="p-3">
    <FilterInput
      label="Filter attribute keys"
      placeholder="Find an attribute…"
      bind:value={filter}
    />
  </div>
  {#if error}<p role="alert" class="p-3">{error}</p>
  {:else if loading}<p role="status" class="p-3 text-rp-subtle">
      Loading attributes…
    </p>
  {:else if keys.length === 0}<p class="p-3 text-rp-subtle">
      No matching attributes in this service and time range.
    </p>
  {:else}
    {#key `${identity}:${signal}:${owner}:${revision}:${offset}:${term}`}
      {#each keys as entry (JSON.stringify( [entry.key, entry.kind] ))}<AttributeKey
          {entry}
          {options}
        />{/each}
    {/key}
  {/if}
  {#if offset > 0 || more}<div class="flex justify-between gap-2 px-3 py-2">
      <button
        class="btn btn-ghost btn-sm"
        disabled={loading || offset === 0}
        onclick={() => (offset -= 100)}>Previous keys</button
      >
      <button
        class="btn btn-ghost btn-sm"
        disabled={loading || !more}
        onclick={() => (offset += 100)}>Next keys</button
      >
    </div>{/if}
{:else}<p class="p-3 text-rp-subtle">
    Select a service to discover its attributes.
  </p>{/if}
