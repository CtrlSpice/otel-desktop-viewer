<script lang="ts">
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import { onDestroy, untrack } from 'svelte'
  import { SvelteMap } from 'svelte/reactivity'
  import { Chart, Svg, type ChartState } from 'layerchart'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    Add01Icon,
    MinusSignIcon,
    RefreshIcon,
  } from '@hugeicons/core-free-icons'
  import WorkspaceHeader from '@/components/shared/WorkspaceHeader.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import ServiceCard from './ServiceCard.svelte'
  import ServiceLabelsPopover from './ServiceLabelsPopover.svelte'
  import ServiceMapLayout from './ServiceMapLayout.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    type ServiceSummary,
    type ServiceConnection,
    type DiscoveryOptions,
  } from '@/services/discovery-types'
  import {
    connectionIdentity,
    graphServices,
    type MapSelection,
    type ConnectionReading,
    SERVICE_CARD_WIDTH,
    SERVICE_CARD_HEIGHT,
    type ServiceSizes,
    type ServicePoint,
  } from './service-map'

  let {
    service,
    options,
    revision,
    selection,
    onSelect,
    onUpdateSelection,
    services,
    servicesLoaded,
    totalServices,
    loadingServices,
    servicesRequestKey,
    servicesError,
    moreServices,
    onMoreServices,
  }: {
    service: ServiceSummary | null
    options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
    revision: number
    selection: MapSelection | null
    onSelect: (selection: MapSelection) => void
    onUpdateSelection?: (selection: MapSelection | null) => void
    services: ServiceSummary[]
    servicesLoaded: boolean
    totalServices?: number
    loadingServices: boolean
    servicesRequestKey?: AbortSignal
    servicesError: string
    moreServices: boolean
    onMoreServices: () => void
  } = $props()
  let received = $state.raw(
    new Map<string, { service: ServiceSummary; items: ServiceConnection[] }>()
  )
  let connectionReadings = $state.raw(new Map<string, ConnectionReading>())
  const connections = $derived(
    [...connectionReadings.values()].map(reading => reading.connection)
  )
  type ConnectionPage = Pick<ConnectionReading, 'options' | 'revision'> & {
    offset: number
    more: boolean
    graphRevision: number
  }
  type ConnectionRequest = Omit<ConnectionPage, 'more'> & {
    service: ServiceSummary
    kind: 'replacement' | 'page'
  }
  let pages = $state<Record<string, ConnectionPage>>({})
  const requests = new SvelteMap<
    string,
    {
      reading: ConnectionRequest
      controller: AbortController
    }
  >()
  const busy = $derived(requests.size > 0)
  let connectionRequestKey = $state(0)
  let failures = $state<
    Record<string, ConnectionRequest & { message: string }>
  >({})
  let chart = $state<ChartState>()
  let layoutController = $state<ServiceMapLayout>()
  function constrainView(view: { scale: number; translate: ServicePoint }) {
    return layoutController?.constrain(view) ?? view
  }
  // LayerChart's wheel-translation path applies processTranslate directly.
  function panView(x: number, y: number, deltaX: number, deltaY: number) {
    return constrainView({
      scale: chart?.transform.scale ?? 1,
      translate: { x: x + deltaX, y: y + deltaY },
    }).translate
  }
  const transform = $derived(chart?.transform)
  let sizes = $state<ServiceSizes>(new Map())
  let graphRevision = $state(0)
  let viewRevision = $state(0)
  let previousScope: typeof options | undefined
  let previousRevision: number | undefined
  const attempted = new Set<string>()
  const markerID = $props.id()
  const nodes = $derived(graphServices(services, connections))
  const inspected = $derived(
    selection?.kind === 'service' ? selection.service : service
  )
  const inspectedPage = $derived(
    inspected ? pages[serviceIdentity(inspected)] : undefined
  )
  const inspectedID = $derived(inspected ? serviceIdentity(inspected) : '')
  const serviceCount = $derived(
    servicesLoaded || nodes.length > 0 ? nodes.length : undefined
  )
  let connectionsKnown = $state(false)
  const connectionCount = $derived(
    connectionsKnown || connections.length > 0 ? connections.length : undefined
  )
  $effect(() => {
    const complete =
      servicesLoaded &&
      !loadingServices &&
      !busy &&
      !Object.keys(failures).length &&
      (services.length > 0 || !moreServices) &&
      [...services, ...(inspected ? [inspected] : [])].every(service => {
        const page = pages[serviceIdentity(service)]
        return page?.graphRevision === graphRevision && !page.more
      })
    // Retain a received count through replacement; initial empty arrays prove nothing.
    if (connections.length > 0 || complete) connectionsKnown = true
  })

  function measureCard(element: HTMLDivElement, identity: string) {
    // Card text and colour controls keep their native interactions. Map dragging
    // starts on the surrounding canvas; an open picker owns its own scrolling.
    const keepPointer = (event: PointerEvent) => {
      if (
        event.pointerType !== 'touch' ||
        (event.target instanceof Element && event.target.closest('[popover]'))
      )
        event.stopPropagation()
    }
    const keepWheel = (event: WheelEvent) => {
      if (event.target instanceof Element && event.target.closest('[popover]'))
        event.stopPropagation()
    }
    const keepDoubleClick = (event: MouseEvent) => event.stopPropagation()
    element.addEventListener('pointerdown', keepPointer)
    element.addEventListener('wheel', keepWheel)
    element.addEventListener('dblclick', keepDoubleClick)
    const observer = new ResizeObserver(() => {
      const height = element.offsetHeight
      if (height > 0 && sizes.get(identity)?.height !== height) {
        sizes = new Map(sizes).set(identity, {
          width: SERVICE_CARD_WIDTH,
          height,
        })
      }
    })
    observer.observe(element)
    return {
      destroy: () => {
        observer.disconnect()
        element.removeEventListener('pointerdown', keepPointer)
        element.removeEventListener('wheel', keepWheel)
        element.removeEventListener('dblclick', keepDoubleClick)
      },
    }
  }

  function panKey(event: KeyboardEvent) {
    if (!transform) return
    let x = 0,
      y = 0
    switch (event.key) {
      case 'ArrowLeft':
        x = -48
        break
      case 'ArrowRight':
        x = 48
        break
      case 'ArrowUp':
        y = -48
        break
      case 'ArrowDown':
        y = 48
        break
      default:
        return
    }
    event.preventDefault()
    transform.translate = {
      x: transform.translate.x + x,
      y: transform.translate.y + y,
    }
  }

  function requestIdentity(identity: string, kind: ConnectionRequest['kind']) {
    return JSON.stringify([identity, kind])
  }
  function expand(service: ServiceSummary) {
    attempted.add(serviceIdentity(service))
    return loadConnections({
      service,
      options,
      revision,
      graphRevision,
      offset: 0,
      kind: 'replacement',
    })
  }
  function moreConnections(service: ServiceSummary) {
    const page = pages[serviceIdentity(service)]
    if (page?.more) return loadConnections({ ...page, service, kind: 'page' })
  }
  async function loadConnections(reading: ConnectionRequest) {
    const {
      service: node,
      options: scope,
      revision: readingRevision,
      offset,
      kind,
    } = reading
    const identity = serviceIdentity(node)
    const key = requestIdentity(identity, kind)
    if (requests.has(key)) return
    const controller = new AbortController()
    requests.set(key, { reading, controller })
    connectionRequestKey++
    const remaining = { ...failures }
    delete remaining[key]
    failures = remaining
    try {
      const page = await telemetryAPI.discoverServiceConnections(
        { ...scope, serviceIdentity: identity, offset, limit: 100 },
        controller.signal
      )
      if (controller.signal.aborted) return
      if (kind === 'replacement') {
        const pageKey = requestIdentity(identity, 'page')
        requests.get(pageKey)?.controller.abort()
        requests.delete(pageKey)
        const remaining = { ...failures }
        delete remaining[pageKey]
        failures = remaining
      }
      const merged = new Map(
        (offset ? (received.get(identity)?.items ?? []) : []).map(edge => [
          connectionIdentity(edge),
          edge,
        ])
      )
      for (const edge of page.items) merged.set(connectionIdentity(edge), edge)
      received = new Map(received).set(identity, {
        service: node,
        items: [...merged.values()],
      })
      const retained = new Set(
        [...received.values()].flatMap(page =>
          page.items.map(connectionIdentity)
        )
      )
      // Only this response's edges replace values; sibling pages retain membership.
      const next = new Map(connectionReadings)
      for (const edge of page.items)
        next.set(connectionIdentity(edge), {
          connection: edge,
          options: scope,
          revision: readingRevision,
        })
      connectionReadings = new Map(
        [...next].filter(([identity]) => retained.has(identity))
      )
      if (selection?.kind === 'connection') {
        const reading = connectionReadings.get(
          connectionIdentity(selection.connection)
        )
        if (!reading) {
          const sourceID = serviceIdentity(selection.connection.source)
          const source =
            nodes.find(node => serviceIdentity(node) === sourceID) ?? nodes[0]
          onUpdateSelection?.(
            source ? { kind: 'service', service: source } : null
          )
        } else if (
          reading.connection !== selection.connection ||
          reading.options !== selection.options ||
          reading.revision !== selection.revision
        )
          onUpdateSelection?.({ kind: 'connection', ...reading })
      }
      pages = {
        ...pages,
        [identity]: {
          offset: offset + page.items.length,
          more: page.truncated,
          options: scope,
          revision: readingRevision,
          graphRevision: reading.graphRevision,
        },
      }
    } catch (reason) {
      if (!controller.signal.aborted) {
        failures = {
          ...failures,
          [key]: { ...reading, message: String(reason) },
        }
      }
    } finally {
      if (requests.get(key)?.controller === controller) {
        requests.delete(key)
      }
    }
  }
  function inspectConnection(edge: ServiceConnection) {
    const reading = connectionReadings.get(connectionIdentity(edge))
    if (reading) onSelect({ kind: 'connection', ...reading })
  }
  $effect(() => {
    const scope = options
    const reading = revision
    const visible = services
    const selected = inspected
    untrack(() => {
      const sameScope = previousScope === scope && previousRevision === reading
      previousScope = scope
      previousRevision = reading
      if (!sameScope) {
        for (const [key, work] of requests) {
          if (work.reading.kind === 'replacement') {
            work.controller.abort()
            requests.delete(key)
          }
        }
        failures = {}
        graphRevision++
        attempted.clear()
        for (const page of received.values()) void expand(page.service)
      }
      for (const node of [...visible, ...(selected ? [selected] : [])]) {
        if (!attempted.has(serviceIdentity(node))) void expand(node)
      }
    })
  })
  onDestroy(() => {
    for (const work of requests.values()) work.controller.abort()
  })
</script>

<WorkspaceHeader
  mode="title"
  title="Service map"
  subtitle={inspected
    ? `${inspected.serviceName || 'Unnamed service'}${inspected.serviceNamespace ? ` · ${inspected.serviceNamespace}` : ''}`
    : undefined}
  subtitleService={inspected?.serviceName}
  ariaLabel="Service map"
  pill
>
  {#snippet metaLeft()}<span class="text-sm text-subtle">
      {#if serviceCount !== undefined}{serviceCount} services{/if}{#if serviceCount !== undefined && connectionCount !== undefined}{' · '}{/if}{#if connectionCount !== undefined}{connectionCount}
        {connectionCount === 1 ? 'connection' : 'connections'}{/if}
    </span>{/snippet}
  {#snippet metaRight()}
    {#if inspected}<ServiceLabelsPopover service={inspected} />{/if}
    {#if inspectedPage?.more}<button
        class="btn btn-ghost btn-sm"
        disabled={requests.has(requestIdentity(inspectedID, 'page'))}
        onclick={() => inspected && moreConnections(inspected)}
        >More connections</button
      >{/if}
    <div
      class="header-icon-controls"
      role="group"
      aria-label="Service map view"
    >
      <button
        class="header-icon-button tooltip tooltip-left"
        aria-label="Zoom out service map"
        data-tip="Zoom out"
        disabled={nodes.length === 0}
        onclick={() => transform?.zoomOut()}
        ><HugeiconsIcon icon={MinusSignIcon} size={16} /></button
      >
      <button
        class="header-icon-button tooltip tooltip-left"
        aria-label="Zoom in service map"
        data-tip="Zoom in"
        disabled={nodes.length === 0}
        onclick={() => transform?.zoomIn()}
        ><HugeiconsIcon icon={Add01Icon} size={16} /></button
      >
      <button
        class="header-icon-button tooltip tooltip-left"
        aria-label="Reset service map view"
        data-tip="Fit graph"
        disabled={nodes.length === 0}
        onclick={() => viewRevision++}
        ><HugeiconsIcon icon={RefreshIcon} size={16} /></button
      >
    </div>
  {/snippet}
</WorkspaceHeader>
<div class="service-map" aria-busy={busy}>
  {#if servicesError}<p role="alert" class="p-3 text-papaya">
      {servicesError}
    </p>{/if}
  {#each Object.entries(failures) as [key, failure] (key)}<div
      class="flex items-center gap-2 p-3"
    >
      <p role="alert" class="text-papaya">
        {failure.service.serviceName || 'Unnamed service'}: {failure.message}
      </p>
      <button
        class="btn btn-ghost btn-sm"
        onclick={() => loadConnections(failure)}>Retry</button
      >
    </div>{/each}
  <LoadingStatus
    pending={loadingServices || busy}
    label="Loading service map…"
    requestKey={loadingServices ? servicesRequestKey : connectionRequestKey}
    class="block p-3 text-subtle"
  />
  {#if nodes.length}
    <p class="px-[var(--pane-inline-inset)] py-3 text-subtle">
      Scroll to move. Pinch to zoom.
      <span class="sr-only"
        >You can also drag empty space to move the map, or use arrow keys with a
        service or connection focused. Control and scroll zooms.</span
      >
    </p>
    <div class="service-map__canvas">
      <Chart
        padding={0}
        bind:context={chart}
        transform={{
          mode: 'canvas',
          scrollMode: 'translate',
          motion: 'none',
          constrain: constrainView,
          processTranslate: panView,
        }}
      >
        <Svg title="Service connections">
          {#snippet defs()}<marker
              id={markerID}
              viewBox="0 0 10 10"
              refX="9"
              refY="5"
              markerWidth="6"
              markerHeight="6"
              orient="auto-start-reverse"
              ><path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" /></marker
            >{/snippet}
          <ServiceMapLayout
            bind:this={layoutController}
            services={nodes}
            {connections}
            {viewRevision}
            {sizes}
          >
            {#snippet children({ positions, paths })}
              {#each connections as edge (connectionIdentity(edge))}
                {@const chosen =
                  selection?.kind === 'connection' &&
                  connectionIdentity(selection.connection) ===
                    connectionIdentity(edge)}
                {@const path = paths.get(connectionIdentity(edge)) ?? ''}
                <g
                  class="service-map__edge"
                  class:service-map__edge--selected={chosen}
                >
                  <path
                    d={path}
                    fill="none"
                    stroke="currentColor"
                    stroke-width={chosen ? 2 : 1}
                    marker-end={`url(#${markerID})`}
                  />
                  <path
                    class="service-map__edge-target"
                    d={path}
                    fill="none"
                    stroke="transparent"
                    stroke-width="16"
                    role="button"
                    tabindex="0"
                    aria-pressed={chosen}
                    aria-label={`${edge.source.serviceName || 'Unnamed service'} → ${edge.target.serviceName || 'Unnamed service'}: ${edge.spanCount} parent-child connections in ${edge.traceCount} traces`}
                    onclick={() => inspectConnection(edge)}
                    onkeydown={event => {
                      panKey(event)
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault()
                        inspectConnection(edge)
                      }
                    }}
                  >
                    <title
                      >{edge.spanCount} parent-child connections · {edge.traceCount}
                      traces</title
                    >
                  </path>
                </g>
              {/each}
              {#each nodes as node (serviceIdentity(node))}
                {@const identity = serviceIdentity(node)}
                {@const point = positions.get(identity)!}
                {@const chosen =
                  selection?.kind === 'service' &&
                  serviceIdentity(selection.service) === identity}
                <foreignObject
                  x={point.x - SERVICE_CARD_WIDTH / 2}
                  y={point.y -
                    (sizes.get(identity)?.height ?? SERVICE_CARD_HEIGHT) / 2}
                  width={SERVICE_CARD_WIDTH}
                  height={sizes.get(identity)?.height ?? SERVICE_CARD_HEIGHT}
                  overflow="visible"
                >
                  <div
                    class="service-map__node"
                    data-service-identity={identity}
                    use:measureCard={identity}
                  >
                    <ServiceCard
                      service={node}
                      selected={chosen}
                      onSelect={() =>
                        onSelect({
                          kind: 'service',
                          service: node,
                        })}
                      onkeydown={panKey}
                    />
                  </div>
                </foreignObject>
              {/each}
            {/snippet}
          </ServiceMapLayout>
        </Svg>
      </Chart>
    </div>
  {:else if servicesLoaded && !loadingServices && !busy && !servicesError && !Object.keys(failures).length}<p
      class="p-3 text-subtle"
    >
      No services match this search and time range.
    </p>{/if}
  {#if moreServices}<SignalFooter label="services" compact>
      {#snippet leading()}<span class="text-subtle"
          >{services.length}{totalServices == null
            ? ''
            : ` of ${totalServices}`} services loaded</span
        >{/snippet}
      {#snippet trailing()}<button
          class="btn btn-ghost btn-sm"
          disabled={loadingServices}
          onclick={onMoreServices}>More services</button
        >{/snippet}
    </SignalFooter>{/if}
</div>

<style lang="postcss">
  @reference '../../app.css';
  .service-map {
    @apply flex min-h-0 flex-1 flex-col;
  }
  .service-map__canvas {
    @apply relative min-h-0 flex-1 overflow-hidden;
  }
  .service-map__node {
    @apply w-full rounded-xl bg-base-200;
  }
  .service-map__edge {
    color: var(--color-muted);
  }
  .service-map__edge--selected,
  .service-map__edge:hover,
  .service-map__edge:focus-within {
    color: var(--color-primary);
  }
  .service-map__edge-target {
    cursor: pointer;
  }
</style>
