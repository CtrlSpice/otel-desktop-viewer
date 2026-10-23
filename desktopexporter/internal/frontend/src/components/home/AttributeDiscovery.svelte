<script lang="ts">
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
    signal,
    queries,
  }: {
    service: ServiceSummary | null
    range: Pick<DiscoveryOptions, 'startTime' | 'endTime'>
    revision: number
    signal: DiscoverySignal
    queries?: DiscoveryOptions['queries']
  } = $props()
  let filter = $state('')
  let term = $state('')
  let offset = $state(0)
  let keys = $state<(Key & { ownerType: DiscoveryOwner })[]>([])
  let more = $state(false)
  let loading = $state(false)
  let error = $state('')
  const owners: Record<DiscoverySignal, DiscoveryOwner[]> = {
    traces: ['resource', 'scope', 'span', 'event', 'link'],
    logs: ['resource', 'scope', 'log'],
    metrics: ['resource', 'scope', 'datapoint', 'exemplar', 'metadata'],
  }
  const options = $derived({
    ...range,
    queries,
    signal,
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
    void revision
    offset = 0
  })
  $effect(() => {
    const active = service
    const request = { ...options, term, offset, limit: 100 }
    void revision
    const controller = new AbortController()
    if (offset === 0) keys = []
    more = false
    error = ''
    loading = Boolean(active)
    if (active)
      void Promise.all(
        owners[signal].map(async ownerType => {
          const page = await telemetryAPI.discoverAttributeKeys(
            { ...request, ownerType },
            controller.signal
          )
          return {
            ...page,
            items: page.items.map(entry => ({ ...entry, ownerType })),
          }
        })
      )
        .then(pages => {
          if (controller.signal.aborted) return
          keys = [...keys, ...pages.flatMap(page => page.items)].sort(
            (a, b) =>
              a.key.localeCompare(b.key) ||
              a.kind.localeCompare(b.kind) ||
              a.ownerType.localeCompare(b.ownerType)
          )
          more = pages.some(page => page.truncated)
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

{#if service}
  <div class="p-3">
    <FilterInput
      label="Filter attribute keys"
      placeholder="Find an attribute…"
      bind:value={filter}
    />
  </div>
  {#if error}<p role="alert" class="p-3">{error}</p>
  {:else if loading && keys.length === 0}<p
      role="status"
      class="p-3 text-rp-subtle"
    >
      Loading attributes…
    </p>
  {:else if keys.length === 0}<p class="p-3 text-rp-subtle">
      No matching attributes in this service and time range.
    </p>
  {:else}
    {#key `${identity}:${signal}:${revision}:${term}`}
      {#each keys as entry (JSON.stringify( [entry.key, entry.kind, entry.ownerType] ))}<AttributeKey
          {entry}
          options={{ ...options, ownerType: entry.ownerType }}
        />{/each}
    {/key}
  {/if}
  {#if more || (loading && offset > 0)}<div class="px-3 py-2">
      <button
        class="btn btn-ghost btn-sm"
        disabled={loading || !more}
        onclick={() => (offset += 100)}
        >{loading ? 'Loading…' : 'Load more attributes'}</button
      >
    </div>{/if}
{:else}<p class="p-3 text-rp-subtle">
    Select a service to discover its attributes.
  </p>{/if}
