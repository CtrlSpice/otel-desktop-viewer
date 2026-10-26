import { dagreGraph } from 'layerchart/utils/graph/dagre'
import { curveBasis, line } from 'd3-shape'
import {
  serviceIdentity,
  type ServiceConnection,
  type ServiceSummary,
} from '@/services/discovery-types'

export type MapSelection =
  | { kind: 'service'; service: ServiceSummary }
  | { kind: 'connection'; connection: ServiceConnection }

export const SERVICE_CARD_WIDTH = 320
export const SERVICE_CARD_HEIGHT = 96
export type ServiceSize = { width: number; height: number }
export type ServiceSizes = Map<string, ServiceSize>
export type ServicePoint = { x: number; y: number }
export type ServicePositions = Map<string, ServicePoint>
export type ServiceLayout = {
  positions: ServicePositions
  paths: Map<string, string>
}
const GAP = 32
const EDGE_CLEARANCE = 6

export function connectionIdentity(connection: ServiceConnection): string {
  return JSON.stringify([
    serviceIdentity(connection.source),
    serviceIdentity(connection.target),
  ])
}

export function graphServices(
  services: readonly ServiceSummary[],
  connections: readonly ServiceConnection[]
) {
  const byID = new Map(
    services.map(service => [serviceIdentity(service), service])
  )
  for (const edge of connections) {
    for (const service of [edge.source, edge.target]) {
      const identity = serviceIdentity(service)
      if (!byID.has(identity)) byID.set(identity, service)
    }
  }
  return [...byID.values()].sort((a, b) =>
    serviceIdentity(a).localeCompare(serviceIdentity(b))
  )
}

function cardSize(sizes: ServiceSizes, identity: string): ServiceSize {
  return (
    sizes.get(identity) ?? {
      width: SERVICE_CARD_WIDTH,
      height: SERVICE_CARD_HEIGHT,
    }
  )
}

function components(
  services: readonly ServiceSummary[],
  connections: readonly ServiceConnection[]
) {
  const neighbours = new Map(
    services.map(service => [serviceIdentity(service), new Set<string>()])
  )
  for (const edge of connections) {
    const source = serviceIdentity(edge.source),
      target = serviceIdentity(edge.target)
    neighbours.get(source)?.add(target)
    neighbours.get(target)?.add(source)
  }
  const seen = new Set<string>()
  const groups: string[][] = []
  for (const id of [...neighbours.keys()].sort()) {
    if (seen.has(id)) continue
    const group = [id]
    seen.add(id)
    for (let i = 0; i < group.length; i++) {
      for (const neighbour of [...(neighbours.get(group[i]!) ?? [])].sort()) {
        if (seen.has(neighbour)) continue
        seen.add(neighbour)
        group.push(neighbour)
      }
    }
    groups.push(group)
  }
  // Connected flows lead; standalone cards fill the remaining columns.
  return groups.sort(
    (a, b) => b.length - a.length || a[0]!.localeCompare(b[0]!)
  )
}

/** Keep inspection and incremental discovery from shuffling existing cards. */
export function retainServicePositions(
  previous: ServicePositions,
  candidates: ServicePositions,
  sizes: ServiceSizes = new Map()
): ServicePositions {
  const positions = new Map([...previous].filter(([id]) => candidates.has(id)))
  for (const [id, candidate] of candidates) {
    if (positions.has(id)) continue
    const point = { ...candidate }
    const size = cardSize(sizes, id)
    let collision: [string, ServicePoint] | undefined
    do {
      collision = [...positions].find(([otherID, other]) => {
        const otherSize = cardSize(sizes, otherID)
        return (
          Math.abs(point.x - other.x) <
            (size.width + otherSize.width) / 2 + GAP &&
          Math.abs(point.y - other.y) <
            (size.height + otherSize.height) / 2 + GAP
        )
      })
      if (collision)
        point.y =
          collision[1].y +
          (cardSize(sizes, collision[0]).height + size.height) / 2 +
          GAP
    } while (collision)
    positions.set(id, point)
  }
  return positions
}

/** Dagre routes each real connected component; packing adds no synthetic edges. */
export function serviceGraphLayout(
  services: readonly ServiceSummary[],
  connections: readonly ServiceConnection[],
  sizes: ServiceSizes,
  availableWidth: number,
  previous: ServicePositions = new Map()
): ServiceLayout {
  const candidates: ServicePositions = new Map()
  const routes = new Map<
    string,
    { source: string; target: string; points: ServicePoint[] }
  >()
  const columnWidth = SERVICE_CARD_WIDTH + EDGE_CLEARANCE * 2 + GAP
  const columns = Math.max(
    1,
    Math.floor((availableWidth - 48 + GAP) / columnWidth)
  )
  const bottoms = Array<number>(columns).fill(0)
  for (const ids of components(services, connections)) {
    const members = new Set(ids)
    const edges = connections.filter(
      edge =>
        members.has(serviceIdentity(edge.source)) &&
        members.has(serviceIdentity(edge.target))
    )
    const graph = dagreGraph(
      {
        nodes: ids.map(id => {
          const size = cardSize(sizes, id)
          return {
            id,
            label: {
              width: size.width + EDGE_CLEARANCE * 2,
              height: size.height + EDGE_CLEARANCE * 2,
            },
          }
        }),
        edges: edges.map(edge => ({
          source: serviceIdentity(edge.source),
          target: serviceIdentity(edge.target),
        })),
      },
      {
        direction: 'top-bottom',
        rankSeparation: 56,
        nodeSeparation: GAP,
        edgeSeparation: 24,
      }
    )
    const width = graph.graph().width!,
      height = graph.graph().height!
    const span = Math.min(
      columns,
      Math.max(1, Math.ceil((width + GAP) / columnWidth))
    )
    let column = 0
    for (let i = 1; i <= columns - span; i++) {
      if (
        Math.max(...bottoms.slice(i, i + span)) <
        Math.max(...bottoms.slice(column, column + span))
      )
        column = i
    }
    const x = column * columnWidth
    const y = Math.max(...bottoms.slice(column, column + span))
    for (let i = column; i < column + span; i++) bottoms[i] = y + height + GAP
    for (const id of ids) {
      const node = graph.node(id)
      candidates.set(id, { x: node.x + x, y: node.y + y })
    }
    for (const edge of edges) {
      const source = serviceIdentity(edge.source),
        target = serviceIdentity(edge.target)
      const points: ServicePoint[] = graph.edge(source, target).points
      routes.set(connectionIdentity(edge), {
        source,
        target,
        points: points.map(point => ({ x: point.x + x, y: point.y + y })),
      })
    }
  }
  const positions = retainServicePositions(previous, candidates, sizes)
  const path = line<ServicePoint>()
    .x(point => point.x)
    .y(point => point.y)
    .curve(curveBasis)
  const paths = new Map<string, string>()
  for (const [id, route] of routes) {
    const source = positions.get(route.source)!,
      sourceCandidate = candidates.get(route.source)!
    const target = positions.get(route.target)!,
      targetCandidate = candidates.get(route.target)!
    // Carry Dagre's routing with retained endpoints when discovery extends a flow.
    const points = route.points.map((point, index) => {
      const t = index / (route.points.length - 1)
      return {
        x:
          point.x +
          (source.x - sourceCandidate.x) * (1 - t) +
          (target.x - targetCandidate.x) * t,
        y:
          point.y +
          (source.y - sourceCandidate.y) * (1 - t) +
          (target.y - targetCandidate.y) * t,
      }
    })
    paths.set(id, path(points) ?? '')
  }
  return { positions, paths }
}
