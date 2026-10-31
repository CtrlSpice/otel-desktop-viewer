<script lang="ts">
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import { onDestroy, untrack } from 'svelte'
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
    services,
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
    services: ServiceSummary[]
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
  let connections = $state.raw<ServiceConnection[]>([])
  let pages = $state<
    Record<string, { offset: number; more: boolean; revision: number }>
  >({})
  let pending = $state(0)
  const busy = $derived(pending > 0)
  let connectionRequestKey = $state(0)
  let failures = $state<
    Record<string, { service: ServiceSummary; message: string }>
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
  const requests = new Map<string, AbortController>()
  let sizes = $state<ServiceSizes>(new Map())
  let graphRevision = $state(0)
  let viewRevision = $state(0)
  let previousScope: typeof options | undefined
  let previousRevision: number | undefined
  const markerID = $props.id()
  const nodes = $derived(graphServices(services, connections))
  const inspected = $derived(
    selection?.kind === 'service' ? selection.service : service
  )
  const inspectedPage = $derived(
    inspected ? pages[serviceIdentity(inspected)] : undefined
  )
  const inspectedID = $derived(inspected ? serviceIdentity(inspected) : '')

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

  async function expand(node: ServiceSummary) {
    const identity = serviceIdentity(node)
    if (requests.has(identity)) return
    const controller = new AbortController()
    requests.set(identity, controller)
    pending = requests.size
    connectionRequestKey++
    const offset =
      pages[identity]?.revision === graphRevision ? pages[identity].offset : 0
    const remaining = { ...failures }
    delete remaining[identity]
    failures = remaining
    try {
      const page = await telemetryAPI.discoverServiceConnections(
        { ...options, serviceIdentity: identity, offset, limit: 100 },
        controller.signal
      )
      if (controller.signal.aborted) return
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
      const next = new Map(
        connections.map(edge => [connectionIdentity(edge), edge])
      )
      for (const edge of page.items) next.set(connectionIdentity(edge), edge)
      connections = [...next.values()].filter(edge =>
        retained.has(connectionIdentity(edge))
      )
      pages = {
        ...pages,
        [identity]: {
          offset: offset + page.items.length,
          more: page.truncated,
          revision: graphRevision,
        },
      }
    } catch (reason) {
      if (!controller.signal.aborted) {
        failures = {
          ...failures,
          [identity]: { service: node, message: String(reason) },
        }
      }
    } finally {
      if (requests.get(identity) === controller) {
        requests.delete(identity)
        pending = requests.size
      }
    }
  }
  $effect(() => {
    const scope = options
    const reading = revision
    const visible = services
    untrack(() => {
      const sameScope = previousScope === scope && previousRevision === reading
      previousScope = scope
      previousRevision = reading
      if (!sameScope) {
        for (const controller of requests.values()) controller.abort()
        requests.clear()
        failures = {}
        pending = 0
        graphRevision++
        for (const page of received.values()) void expand(page.service)
      }
      for (const node of visible) {
        if (pages[serviceIdentity(node)]?.revision !== graphRevision)
          void expand(node)
      }
    })
  })
  $effect(() => {
    const node = inspected
    untrack(() => {
      if (node && pages[serviceIdentity(node)]?.revision !== graphRevision)
        void expand(node)
    })
  })
  onDestroy(() => {
    for (const controller of requests.values()) controller.abort()
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
  {#snippet metaLeft()}<span class="text-sm text-subtle"
      >{nodes.length} services · {connections.length}
      {connections.length === 1 ? 'connection' : 'connections'}</span
    >{/snippet}
  {#snippet metaRight()}
    {#if inspected}<ServiceLabelsPopover service={inspected} />{/if}
    {#if inspectedPage?.more}<button
        class="btn btn-ghost btn-sm"
        disabled={requests.has(inspectedID) && busy}
        onclick={() => inspected && expand(inspected)}>More connections</button
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
  {#each Object.values(failures) as failure (serviceIdentity(failure.service))}<div
      class="flex items-center gap-2 p-3"
    >
      <p role="alert" class="text-papaya">
        {failure.service.serviceName || 'Unnamed service'}: {failure.message}
      </p>
      <button
        class="btn btn-ghost btn-sm"
        onclick={() => expand(failure.service)}>Retry</button
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
                    onclick={() =>
                      onSelect({ kind: 'connection', connection: edge })}
                    onkeydown={event => {
                      panKey(event)
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault()
                        onSelect({ kind: 'connection', connection: edge })
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
  {:else if !loadingServices && !busy && !servicesError && !Object.keys(failures).length}<p
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
