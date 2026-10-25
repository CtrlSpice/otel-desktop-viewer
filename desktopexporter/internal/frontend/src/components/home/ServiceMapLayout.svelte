<script lang="ts">
  import { untrack, type Snippet } from 'svelte'
  import { getChartContext } from 'layerchart'
  import type { HierarchyPointNode } from 'd3-hierarchy'
  import { serviceIdentity } from '@/services/discovery-types'
  import {
    retainServicePositions,
    servicePositions,
    type ServiceBranch,
    type ServicePositions,
  } from './service-map'

  let {
    nodes,
    centreID,
    viewRevision,
    children,
  }: {
    nodes: HierarchyPointNode<ServiceBranch>[]
    centreID: string
    viewRevision: number
    children: Snippet<[ServicePositions]>
  } = $props()
  const chart = getChartContext()
  let positions = $state<ServicePositions>(new Map())
  let camera:
    | { identity: string; width: number; height: number; revision: number }
    | undefined

  $effect.pre(() => {
    const candidates = servicePositions(nodes)
    untrack(() => {
      positions = retainServicePositions(positions, candidates)
    })
  })
  $effect(() => {
    const point = positions.get(centreID)
    const transform = chart.transform
    const width = chart.width,
      height = chart.height
    const revision = viewRevision
    if (!point || transform.mode !== 'canvas') return
    if (
      camera?.identity === centreID &&
      camera.width === width &&
      camera.height === height &&
      camera.revision === revision
    )
      return
    untrack(() => {
      if (!camera || camera.revision !== revision) transform.scale = 1
      transform.translate = {
        x: width / 2 - point.x * transform.scale,
        y: height / 2 - point.y * transform.scale,
      }
    })
    camera = { identity: centreID, width, height, revision }
  })
</script>

{#if nodes.every(node => positions.has(serviceIdentity(node.data.service)))}
  {@render children(positions)}
{/if}
