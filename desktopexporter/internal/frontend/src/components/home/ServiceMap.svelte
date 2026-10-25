<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import { Chart, Svg, type ChartState } from 'layerchart'
  import { Tree } from 'layerchart/hierarchy'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    Add01Icon,
    MinusSignIcon,
    RefreshIcon,
  } from '@hugeicons/core-free-icons'
  import WorkspaceHeader from '@/components/shared/WorkspaceHeader.svelte'
  import ServiceMapLayout from './ServiceMapLayout.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    serviceIdentity,
    type ServiceSummary,
    type ServiceConnection,
    type DiscoveryOptions,
  } from '@/services/discovery-types'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import {
    connectionIdentity,
    serviceHierarchy,
    connectionPath,
    type MapSelection,
  } from './service-map'

  let {
    service,
    options,
    revision,
    selection,
    onSelect,
  }: {
    service: ServiceSummary | null
    options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
    revision: number
    selection: MapSelection | null
    onSelect: (selection: MapSelection) => void
  } = $props()
  let connections = $state<ServiceConnection[]>([])
  let pages = $state<Record<string, { offset: number; more: boolean }>>({})
  let busy = $state(false)
  let error = $state('')
  let failedNode = $state<ServiceSummary | null>(null)
  let chart = $state<ChartState>()
  const transform = $derived(chart?.transform)
  let request: AbortController | undefined
  let layoutRoot = $state<ServiceSummary | null>(null)
  let graphRevision = $state(0)
  let viewRevision = $state(0)
  let previousScope: typeof options | undefined
  let previousRevision: number | undefined
  const markerID = $props.id()
  const tree = $derived(
    layoutRoot ? serviceHierarchy(layoutRoot, connections) : null
  )
  const inspected = $derived(
    selection?.kind === 'service' ? selection.service : service
  )
  const inspectedPage = $derived(
    inspected ? pages[serviceIdentity(inspected)] : undefined
  )
  const rootID = $derived(service ? serviceIdentity(service) : '')

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

  async function expand(node: ServiceSummary, reset = false) {
    request?.abort()
    const controller = new AbortController()
    request = controller
    const identity = serviceIdentity(node)
    const offset = reset ? 0 : (pages[identity]?.offset ?? 0)
    busy = true
    error = ''
    failedNode = null
    try {
      const page = await telemetryAPI.discoverServiceConnections(
        { ...options, serviceIdentity: identity, offset, limit: 100 },
        controller.signal
      )
      if (controller.signal.aborted) return
      const merged = new Map(
        (reset ? [] : connections).map(edge => [connectionIdentity(edge), edge])
      )
      for (const edge of page.items) merged.set(connectionIdentity(edge), edge)
      connections = [...merged.values()]
      if (reset) pages = {}
      pages = {
        ...pages,
        [identity]: {
          offset: offset + page.items.length,
          more: page.truncated,
        },
      }
    } catch (reason) {
      if (!controller.signal.aborted) {
        error = String(reason)
        failedNode = node
      }
    } finally {
      if (!controller.signal.aborted) busy = false
    }
  }
  $effect(() => {
    const root = service
    const scope = options
    const reading = revision
    untrack(() => {
      const sameScope = previousScope === scope && previousRevision === reading
      previousScope = scope
      previousRevision = reading
      const known =
        root &&
        tree
          ?.descendants()
          .some(
            node => serviceIdentity(node.data.service) === serviceIdentity(root)
          )
      if (sameScope && known) {
        if (!pages[serviceIdentity(root)]) void expand(root)
        return
      }
      request?.abort()
      connections = []
      pages = {}
      error = ''
      failedNode = null
      busy = false
      layoutRoot = root
      graphRevision++
      if (root) void expand(root, true)
    })
  })
  onDestroy(() => request?.abort())
</script>

<WorkspaceHeader
  mode="title"
  title="Service map"
  subtitle={service
    ? `${service.serviceName || 'Unnamed service'}${service.serviceNamespace ? ` · ${service.serviceNamespace}` : ''}`
    : undefined}
  subtitleService={service?.serviceName}
  ariaLabel="Service map"
  pill
>
  {#snippet metaLeft()}<span class="text-sm text-subtle"
      >{tree?.descendants().length ?? 0} services · {connections.length} connections</span
    >{/snippet}
  {#snippet metaRight()}
    <button
      class="btn btn-ghost btn-sm"
      disabled={!inspected || busy || (!!inspectedPage && !inspectedPage.more)}
      onclick={() => inspected && expand(inspected)}
      >{inspectedPage?.more ? 'More connections' : 'Expand connections'}</button
    >
    <button
      class="header-icon-button tooltip tooltip-left"
      aria-label="Zoom out service map"
      data-tip="Zoom out"
      disabled={!tree}
      onclick={() => transform?.zoomOut()}
      ><HugeiconsIcon icon={MinusSignIcon} size={16} /></button
    >
    <button
      class="header-icon-button tooltip tooltip-left"
      aria-label="Zoom in service map"
      data-tip="Zoom in"
      disabled={!tree}
      onclick={() => transform?.zoomIn()}
      ><HugeiconsIcon icon={Add01Icon} size={16} /></button
    >
    <button
      class="header-icon-button tooltip tooltip-left"
      aria-label="Reset service map view"
      data-tip="Reset view"
      disabled={!tree}
      onclick={() => viewRevision++}
      ><HugeiconsIcon icon={RefreshIcon} size={16} /></button
    >
  {/snippet}
</WorkspaceHeader>
<div class="service-map" aria-busy={busy}>
  {#if error}<div class="flex items-center gap-2 p-3">
      <p role="alert" class="text-papaya">{error}</p>
      <button
        class="btn btn-ghost btn-sm"
        onclick={() => failedNode && expand(failedNode)}>Retry</button
      >
    </div>{/if}
  {#if busy}<p class="px-3 text-subtle" role="status">
      Loading connections…
    </p>{/if}
  {#if tree}
    <div class="service-map__canvas">
      {#key graphRevision}
        <Chart
          radial
          padding={0}
          bind:context={chart}
          transform={{
            mode: 'canvas',
            motion: 'none',
            scaleExtent: [0.1, 6],
          }}
        >
          <Svg center title="Service connections">
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
            <Tree hierarchy={tree}>
              {#snippet children({ nodes })}
                <ServiceMapLayout {nodes} centreID={rootID} {viewRevision}>
                  {#snippet children(positions)}
                    {#each connections as edge (connectionIdentity(edge))}
                      {@const source = positions.get(
                        serviceIdentity(edge.source)
                      )!}
                      {@const target = positions.get(
                        serviceIdentity(edge.target)
                      )!}
                      {@const chosen =
                        selection?.kind === 'connection' &&
                        connectionIdentity(selection.connection) ===
                          connectionIdentity(edge)}
                      <g
                        class="service-map__edge"
                        class:service-map__edge--selected={chosen}
                      >
                        <path
                          d={connectionPath(source, target)}
                          fill="none"
                          stroke="currentColor"
                          stroke-width={chosen ? 2 : 1}
                          marker-end={`url(#${markerID})`}
                        />
                        <path
                          class="service-map__edge-target"
                          d={connectionPath(source, target)}
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
                    {#each nodes as node (serviceIdentity(node.data.service))}
                      {@const identity = serviceIdentity(node.data.service)}
                      {@const point = positions.get(identity)!}
                      {@const chosen =
                        selection?.kind === 'service' &&
                        serviceIdentity(selection.service) === identity}
                      <foreignObject
                        x={point.x - 90}
                        y={point.y - 32}
                        width="180"
                        height="64"
                        overflow="visible"
                      >
                        <button
                          type="button"
                          class="service-map__node item-state"
                          class:service-map__node--root={identity === rootID}
                          aria-pressed={chosen}
                          data-selected={chosen || undefined}
                          data-service-identity={identity}
                          title={`${node.data.service.serviceName || 'Unnamed service'}${node.data.service.serviceNamespace ? ` (${node.data.service.serviceNamespace})` : ''}`}
                          aria-label={`Inspect ${node.data.service.serviceName || 'Unnamed service'}${node.data.service.serviceNamespace ? ` (${node.data.service.serviceNamespace})` : ''}`}
                          onclick={() =>
                            onSelect({
                              kind: 'service',
                              service: node.data.service,
                            })}
                          onkeydown={panKey}
                        >
                          <span
                            class="flex min-w-0 items-center justify-center gap-2"
                            ><span
                              class="size-2 shrink-0 rounded-full"
                              style:background-color={traceServiceColor(
                                node.data.service.serviceName,
                                themeSignal.value
                              )}
                              aria-hidden="true"
                            ></span><span class="truncate"
                              >{node.data.service.serviceName ||
                                'Unnamed service'}</span
                            ></span
                          >
                          {#if node.data.service.serviceNamespace}<span
                              class="block truncate text-subtle"
                              >{node.data.service.serviceNamespace}</span
                            >{/if}
                        </button>
                      </foreignObject>
                    {/each}
                  {/snippet}
                </ServiceMapLayout>
              {/snippet}
            </Tree>
          </Svg>
        </Chart>
      {/key}
    </div>
    {#if !busy && !error && connections.length === 0}<p class="p-3 text-subtle">
        No recorded cross-service parent-child connections match this search and
        time range.
      </p>{/if}
    {#if Object.values(pages).some(page => page.more)}<p
        class="px-3 text-subtle"
      >
        More connections are available. Select a service and choose More
        connections.
      </p>{/if}
    <p class="p-3 text-subtle">
      Arrows follow recorded parent → child spans. Drag or use arrow keys on
      focused items to pan; scroll to zoom.
    </p>
  {:else}<p class="p-3 text-subtle">
      Select a service to explore its connections.
    </p>{/if}
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
    @apply h-full w-full cursor-pointer rounded-xl px-3 py-2 text-sm text-base-content;
    --item-idle-bg: var(--color-base-200);
  }
  .service-map__node--root {
    @apply text-primary;
  }
  .service-map__node:focus-visible {
    outline: 2px solid var(--color-primary);
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
