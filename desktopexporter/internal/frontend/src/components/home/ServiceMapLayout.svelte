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
    type ServiceLayout,
    type ServiceSizes,
    SERVICE_CARD_WIDTH,
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
  let camera: { width: number; height: number; revision: number } | undefined

  $effect.pre(() => {
    const items = services,
      edges = connections,
      dimensions = sizes,
      width = chart.width
    untrack(() => {
      layout = serviceGraphLayout(
        items,
        edges,
        dimensions,
        width,
        layout.positions
      )
    })
  })
  $effect(() => {
    const positions = layout.positions
    const transform = chart.transform
    const width = chart.width,
      height = chart.height
    const revision = viewRevision
    if (
      !positions.size ||
      width <= 0 ||
      height <= 0 ||
      transform.mode !== 'canvas'
    )
      return
    if (
      camera &&
      camera.width === width &&
      camera.height === height &&
      camera.revision === revision
    )
      return
    untrack(() => {
      if (camera && camera.revision === revision) {
        transform.translate = {
          x: transform.translate.x + (width - camera.width) / 2,
          y: transform.translate.y + (height - camera.height) / 2,
        }
        return
      }
      transform.scale = 1
      const points = [...positions].map(([identity, position]) => ({
        ...position,
        width: sizes.get(identity)?.width ?? SERVICE_CARD_WIDTH,
        height: sizes.get(identity)?.height ?? SERVICE_CARD_HEIGHT,
      }))
      const centre = {
        x:
          (Math.min(...points.map(p => p.x - p.width / 2)) +
            Math.max(...points.map(p => p.x + p.width / 2))) /
          2,
        y: Math.min(...points.map(p => p.y - p.height / 2)) + height / 2 - 32,
      }
      transform.translate = {
        x: width / 2 - centre.x * transform.scale,
        y: height / 2 - centre.y * transform.scale,
      }
    })
    camera = { width, height, revision }
  })
</script>

{#if services.every(service => layout.positions.has(serviceIdentity(service)))}
  {@render children(layout)}
{/if}
