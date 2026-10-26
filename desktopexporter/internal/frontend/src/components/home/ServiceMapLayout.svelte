<script lang="ts">
  import { untrack, type Snippet } from 'svelte'
  import { getChartContext } from 'layerchart'
  import {
    serviceIdentity,
    type ServiceSummary,
    type ServiceConnection,
  } from '@/services/discovery-types'
  import {
    serviceGraphLayout,
    serviceGraphBounds,
    fitServiceGraph,
    constrainServiceGraph,
    type ServiceLayout,
    type ServiceSizes,
    SERVICE_CARD_HEIGHT,
  } from './service-map'

  let {
    services,
    connections,
    viewRevision,
    sizes,
    children,
  }: {
    services: ServiceSummary[]
    connections: ServiceConnection[]
    viewRevision: number
    sizes: ServiceSizes
    children: Snippet<[ServiceLayout]>
  } = $props()
  const chart = getChartContext()
  let layout = $state<ServiceLayout>({ positions: new Map(), paths: new Map() })
  let previousViewport:
    { width: number; height: number; sizes: ServiceSizes } | undefined
  let layoutRevision = $state(0)
  let retainView = $state(false)
  let camera:
    | {
        width: number
        height: number
        revision: number
        layoutRevision: number
        fitted: ReturnType<typeof fitServiceGraph>
      }
    | undefined
  const bounds = $derived(serviceGraphBounds(layout.positions, sizes))

  $effect.pre(() => {
    const items = services,
      edges = connections,
      dimensions = sizes,
      width = chart.width,
      height = chart.height
    if (width <= 0 || height <= 0) return
    untrack(() => {
      const retain =
        previousViewport?.width === width &&
        Math.abs(previousViewport.height - height) < SERVICE_CARD_HEIGHT &&
        [...previousViewport.sizes].every(
          ([id, size]) => dimensions.get(id)?.height === size.height
        )
      retainView = retain
      layout = serviceGraphLayout(
        items,
        edges,
        dimensions,
        { width, height },
        retain ? layout.positions : new Map()
      )
      // Loading the final page removes the footer; preserve the existing cards.
      previousViewport = {
        width,
        height: retain ? previousViewport!.height : height,
        sizes: dimensions,
      }
      layoutRevision++
    })
  })
  $effect(() => {
    const content = bounds
    const transform = chart.transform
    const width = chart.width,
      height = chart.height
    const revision = viewRevision
    const currentLayoutRevision = layoutRevision
    const preserve = retainView
    if (
      !content ||
      !services.every(service => sizes.has(serviceIdentity(service))) ||
      width <= 0 ||
      height <= 0 ||
      transform.mode !== 'canvas'
    )
      return
    const viewport = { width, height }
    const fitted = fitServiceGraph(content, viewport)
    transform.scaleExtent = [Math.min(0.1, fitted.scale), 6]
    transform.constrain = ({ scale, translate }) => ({
      scale,
      translate: constrainServiceGraph(content, viewport, scale, translate),
    })
    if (
      camera &&
      camera.width === width &&
      camera.height === height &&
      camera.revision === revision &&
      camera.layoutRevision === currentLayoutRevision
    )
      return
    untrack(() => {
      if (
        camera &&
        camera.revision === revision &&
        (preserve ||
          Math.abs(transform.scale - camera.fitted.scale) > 0.0001 ||
          Math.abs(transform.translate.x - camera.fitted.translate.x) > 0.5 ||
          Math.abs(transform.translate.y - camera.fitted.translate.y) > 0.5)
      ) {
        transform.translate = {
          x: transform.translate.x + (width - camera.width) / 2,
          y: transform.translate.y + (height - camera.height) / 2,
        }
        return
      }
      transform.scale = fitted.scale
      transform.translate = fitted.translate
    })
    camera = {
      width,
      height,
      revision,
      fitted,
      layoutRevision: currentLayoutRevision,
    }
  })
</script>

{#if services.every(service => layout.positions.has(serviceIdentity(service)))}
  {@render children(layout)}
{/if}
