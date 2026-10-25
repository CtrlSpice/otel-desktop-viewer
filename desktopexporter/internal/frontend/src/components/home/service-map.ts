import { hierarchy, type HierarchyPointNode } from 'd3-hierarchy'
import {
  serviceIdentity,
  type ServiceConnection,
  type ServiceSummary,
} from '@/services/discovery-types'

export type MapSelection =
  | { kind: 'service'; service: ServiceSummary }
  | { kind: 'connection'; connection: ServiceConnection }

export function connectionIdentity(connection: ServiceConnection): string {
  return JSON.stringify([
    serviceIdentity(connection.source),
    serviceIdentity(connection.target),
  ])
}

export type ServiceBranch = {
  service: ServiceSummary
  children: ServiceBranch[]
}

// The spanning tree only positions nodes. Every received directed edge is drawn
// separately, including reverse edges and links between branches.
export function serviceHierarchy(
  root: ServiceSummary,
  connections: readonly ServiceConnection[]
) {
  const rootID = serviceIdentity(root)
  const services = new Map([[rootID, root]])
  const neighbours = new Map<string, Set<string>>()
  for (const edge of connections) {
    const source = serviceIdentity(edge.source)
    const target = serviceIdentity(edge.target)
    services.set(source, edge.source)
    services.set(target, edge.target)
    for (const [a, b] of [
      [source, target],
      [target, source],
    ]) {
      const adjacent = neighbours.get(a!) ?? new Set<string>()
      adjacent.add(b!)
      neighbours.set(a!, adjacent)
    }
  }
  const tree: ServiceBranch = { service: root, children: [] }
  const seen = new Set([rootID])
  const queue = [tree]
  for (let index = 0; index < queue.length; index++) {
    const parent = queue[index]!
    for (const id of [
      ...(neighbours.get(serviceIdentity(parent.service)) ?? []),
    ].sort()) {
      if (seen.has(id)) continue
      seen.add(id)
      const branch: ServiceBranch = { service: services.get(id)!, children: [] }
      parent.children.push(branch)
      queue.push(branch)
    }
  }
  return hierarchy(tree)
}

export function radialPoint(angle: number, radius: number) {
  return { x: Math.sin(angle) * radius, y: -Math.cos(angle) * radius }
}

// Keep labels readable when a small viewport or a crowded ring needs more room.
// LayerChart owns the angles; larger rings remain reachable through pan/zoom.
export function servicePositions(
  nodes: readonly HierarchyPointNode<ServiceBranch>[]
) {
  const rings = new Map<number, number[]>()
  for (const node of nodes) {
    const angles = rings.get(node.depth) ?? []
    angles.push(node.x)
    rings.set(node.depth, angles)
  }
  const radii = new Map([[0, 0]])
  for (const [depth, angles] of [...rings].sort(([a], [b]) => a - b)) {
    if (depth === 0) continue
    angles.sort((a, b) => a - b)
    let radius = (radii.get(depth - 1) ?? 0) + 220
    if (angles.length > 1) {
      const gaps = angles.map(
        (angle, index) =>
          (angles[index + 1] ?? angles[0]! + 2 * Math.PI) - angle
      )
      radius = Math.max(radius, 210 / (2 * Math.sin(Math.min(...gaps) / 2)))
    }
    radii.set(depth, radius)
  }
  return new Map(
    nodes.map(node => [
      serviceIdentity(node.data.service),
      radialPoint(node.x, radii.get(node.depth)!),
    ])
  )
}

export type ServicePositions = Map<string, { x: number; y: number }>

/** Retain the spatial map; place newly discovered nodes in unoccupied space. */
export function retainServicePositions(
  previous: ServicePositions,
  candidates: ServicePositions
): ServicePositions {
  const positions: ServicePositions = new Map(
    [...previous].filter(([identity]) => candidates.has(identity))
  )
  for (const [identity, candidate] of candidates) {
    if (positions.has(identity)) continue
    const angle = Math.atan2(candidate.x, -candidate.y)
    let radius = Math.hypot(candidate.x, candidate.y)
    let point = candidate
    while (
      [...positions.values()].some(
        other => Math.hypot(point.x - other.x, point.y - other.y) < 209.9
      )
    ) {
      radius += 220
      point = radialPoint(angle, radius)
    }
    positions.set(identity, point)
  }
  return positions
}

export function connectionPath(
  source: { x: number; y: number },
  target: { x: number; y: number }
) {
  const dx = target.x - source.x
  const dy = target.y - source.y
  const distance = Math.hypot(dx, dy) || 1
  // Opposite directions bend to opposite sides of the same chord.
  const bend = Math.min(32, distance / 5)
  const control = {
    x: (source.x + target.x) / 2 - (dy / distance) * bend,
    y: (source.y + target.y) / 2 + (dx / distance) * bend,
  }
  const boundary = (point: { x: number; y: number }) => {
    const x = control.x - point.x,
      y = control.y - point.y
    const scale = Math.min(96 / Math.abs(x), 38 / Math.abs(y), 0.9)
    return { x: point.x + x * scale, y: point.y + y * scale }
  }
  const from = boundary(source),
    to = boundary(target)
  return `M${from.x},${from.y} Q${control.x},${control.y} ${to.x},${to.y}`
}
