import {
  forceSimulation,
  forceLink,
  forceManyBody,
  forceX,
  forceY,
  type Force,
  type SimulationNodeDatum,
  type SimulationLinkDatum,
} from 'd3-force'
import {
  serviceIdentity,
  type ServiceConnection,
  type ServiceSummary,
  type DiscoveryOptions,
} from '@/services/discovery-types'

export type ConnectionReading = {
  connection: ServiceConnection
  options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>
  revision: number
}
export type MapSelection =
  | { kind: 'service'; service: ServiceSummary }
  | ({ kind: 'connection' } & ConnectionReading)

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
export type ServiceBounds = {
  left: number
  right: number
  top: number
  bottom: number
}
export const SERVICE_MAP_PADDING = 32
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

export function serviceGraphBounds(
  positions: ServicePositions,
  sizes: ServiceSizes
): ServiceBounds | undefined {
  if (!positions.size) return undefined
  const bounds = {
    left: Infinity,
    right: -Infinity,
    top: Infinity,
    bottom: -Infinity,
  }
  for (const [identity, point] of positions) {
    const size = cardSize(sizes, identity)
    bounds.left = Math.min(bounds.left, point.x - size.width / 2)
    bounds.right = Math.max(bounds.right, point.x + size.width / 2)
    bounds.top = Math.min(bounds.top, point.y - size.height / 2)
    bounds.bottom = Math.max(bounds.bottom, point.y + size.height / 2)
  }
  return bounds
}

export function fitServiceGraph(bounds: ServiceBounds, viewport: ServiceSize) {
  const scale = Math.min(
    1,
    Math.max(1, viewport.width - SERVICE_MAP_PADDING * 2) /
      (bounds.right - bounds.left),
    Math.max(1, viewport.height - SERVICE_MAP_PADDING * 2) /
      (bounds.bottom - bounds.top)
  )
  return {
    scale,
    translate: {
      x: (viewport.width - (bounds.left + bounds.right) * scale) / 2,
      y: (viewport.height - (bounds.top + bounds.bottom) * scale) / 2,
    },
  }
}

/** Allow free inspection, but keep part of the graph within reach. */
export function constrainServiceGraph(
  bounds: ServiceBounds,
  viewport: ServiceSize,
  scale: number,
  translate: ServicePoint
): ServicePoint {
  const visibleWidth = Math.min(
    SERVICE_MAP_PADDING * 2,
    viewport.width / 2,
    (bounds.right - bounds.left) * scale
  )
  const visibleHeight = Math.min(
    SERVICE_MAP_PADDING * 2,
    viewport.height / 2,
    (bounds.bottom - bounds.top) * scale
  )
  return {
    x: Math.max(
      visibleWidth - bounds.right * scale,
      Math.min(viewport.width - visibleWidth - bounds.left * scale, translate.x)
    ),
    y: Math.max(
      visibleHeight - bounds.bottom * scale,
      Math.min(
        viewport.height - visibleHeight - bounds.top * scale,
        translate.y
      )
    ),
  }
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
            (size.width + otherSize.width) / 2 + GAP - 0.001 &&
          Math.abs(point.y - other.y) <
            (size.height + otherSize.height) / 2 + GAP - 0.001
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

type ServiceForceNode = SimulationNodeDatum &
  ServicePoint &
  ServiceSize & {
    id: string
  }
type ServiceForceLink = SimulationLinkDatum<ServiceForceNode>

/** Rectangular collision uses measured cards rather than oversized circles. */
function cardCollision(
  viewport: ServiceSize
): Force<ServiceForceNode, ServiceForceLink> {
  let nodes: ServiceForceNode[] = []
  const force = () => {
    for (let pass = 0; pass < 3; pass++) {
      for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
          const a = nodes[i]!,
            b = nodes[j]!
          const dx = a.x + (a.vx ?? 0) - b.x - (b.vx ?? 0)
          const dy = a.y + (a.vy ?? 0) - b.y - (b.vy ?? 0)
          const overlapX = (a.width + b.width) / 2 + GAP - Math.abs(dx)
          const overlapY = (a.height + b.height) / 2 + GAP - Math.abs(dy)
          if (overlapX <= 0 || overlapY <= 0) continue
          const aFree = a.fx == null,
            bFree = b.fx == null
          const free = Number(aFree) + Number(bFree)
          if (!free) continue
          if (overlapX / viewport.width < overlapY / viewport.height) {
            const push = (Math.sign(dx || 1) * overlapX) / free
            if (aFree) a.vx = (a.vx ?? 0) + push
            if (bFree) b.vx = (b.vx ?? 0) - push
          } else {
            const push = (Math.sign(dy || 1) * overlapY) / free
            if (aFree) a.vy = (a.vy ?? 0) + push
            if (bFree) b.vy = (b.vy ?? 0) - push
          }
        }
      }
    }
  }
  force.initialize = (items: ServiceForceNode[]) => {
    nodes = items
  }
  return force
}

function cardBoundary(
  viewport: ServiceSize
): Force<ServiceForceNode, ServiceForceLink> {
  let nodes: ServiceForceNode[] = []
  const force = () => {
    for (const node of nodes) {
      if (node.fx != null) continue
      const x = node.x + (node.vx ?? 0),
        y = node.y + (node.vy ?? 0)
      node.vx =
        (node.vx ?? 0) +
        Math.max(node.width / 2, Math.min(viewport.width - node.width / 2, x)) -
        x
      node.vy =
        (node.vy ?? 0) +
        Math.max(
          node.height / 2,
          Math.min(viewport.height - node.height / 2, y)
        ) -
        y
    }
  }
  force.initialize = (items: ServiceForceNode[]) => {
    nodes = items
  }
  return force
}

/** Seed neighbours together so rectangular cards need not pass through each other. */
function connectedSeeds(
  ids: string[],
  connections: readonly ServiceConnection[],
  columns: number,
  cardHeight: number
): ServicePositions {
  const neighbours = new Map(ids.map(id => [id, new Set<string>()]))
  for (const edge of connections) {
    const source = serviceIdentity(edge.source),
      target = serviceIdentity(edge.target)
    if (source === target) continue
    neighbours.get(source)?.add(target)
    neighbours.get(target)?.add(source)
  }
  const order = [...ids].sort(
    (a, b) =>
      neighbours.get(b)!.size - neighbours.get(a)!.size || a.localeCompare(b)
  )
  const remaining = new Set(order)
  const cells = Array.from(
    { length: Math.ceil(ids.length / columns) * columns },
    (_, index) => ({ x: index % columns, y: Math.floor(index / columns) })
  )
  const placed = new Map<string, ServicePoint>()
  for (const root of order) {
    if (!remaining.delete(root)) continue
    const group = [root]
    for (let index = 0; index < group.length; index++) {
      for (const id of neighbours.get(group[index]!)!) {
        if (remaining.delete(id)) group.push(id)
      }
    }
    const centre = {
      x: (columns - 1) / 2,
      y: cells[0]!.y + Math.floor((Math.ceil(group.length / columns) - 1) / 2),
    }
    for (const id of group) {
      const adjacent = [...neighbours.get(id)!].flatMap(neighbour =>
        placed.has(neighbour) ? [placed.get(neighbour)!] : []
      )
      const targets = adjacent.length ? adjacent : [centre]
      const distance = (cell: ServicePoint) =>
        targets.reduce(
          (sum, target) =>
            sum + (cell.x - target.x) ** 2 + (cell.y - target.y) ** 2,
          0
        )
      let best = 0
      for (let index = 1; index < cells.length; index++) {
        if (distance(cells[index]!) < distance(cells[best]!)) best = index
      }
      placed.set(id, cells.splice(best, 1)[0]!)
    }
  }
  return new Map(
    [...placed].map(([id, cell]) => [
      id,
      {
        x: SERVICE_CARD_WIDTH / 2 + cell.x * (SERVICE_CARD_WIDTH + GAP),
        y: cardHeight / 2 + cell.y * (cardHeight + GAP),
      },
    ])
  )
}

/** A deterministic, viewport-shaped seed gives the static simulation room to settle. */
function createServiceSimulation(
  services: readonly ServiceSummary[],
  connections: readonly ServiceConnection[],
  sizes: ServiceSizes,
  viewport: ServiceSize,
  previous: ServicePositions = new Map()
) {
  const ids = services.map(serviceIdentity).sort()
  const cardHeight = Math.max(
    SERVICE_CARD_HEIGHT,
    ...[...sizes.values()].map(size => size.height)
  )
  const usableWidth = Math.max(
    SERVICE_CARD_WIDTH,
    viewport.width - SERVICE_MAP_PADDING * 2
  )
  const usableHeight = Math.max(
    cardHeight,
    viewport.height - SERVICE_MAP_PADDING * 2
  )
  let columns = 1,
    bestScale = 0
  for (let count = 1; count <= ids.length; count++) {
    const width = count * (SERVICE_CARD_WIDTH + GAP) - GAP
    const height = Math.ceil(ids.length / count) * (cardHeight + GAP) - GAP
    const scale = Math.min(1, usableWidth / width, usableHeight / height)
    if (scale > bestScale) {
      columns = count
      bestScale = scale
    }
  }
  const area = {
    width: Math.max(usableWidth, columns * (SERVICE_CARD_WIDTH + GAP) - GAP),
    height: Math.max(
      usableHeight,
      Math.ceil(ids.length / columns) * (cardHeight + GAP) - GAP
    ),
  }
  const seeds = connectedSeeds(ids, connections, columns, cardHeight)
  const positions = retainServicePositions(previous, seeds, sizes)
  const nodes: ServiceForceNode[] = ids.map(id => {
    const node: ServiceForceNode = {
      id,
      ...positions.get(id)!,
      ...cardSize(sizes, id),
    }
    const retained = previous.get(id)
    if (retained) {
      node.fx = retained.x
      node.fy = retained.y
    }
    return node
  })
  const links: ServiceForceLink[] = connections.map(edge => ({
    source: serviceIdentity(edge.source),
    target: serviceIdentity(edge.target),
  }))
  return {
    data: { nodes, links },
    forces: {
      link: forceLink<ServiceForceNode, ServiceForceLink>(links)
        .id(node => node.id)
        .distance(220)
        .strength(0.15),
      charge: forceManyBody<ServiceForceNode>().strength(-300),
      x: forceX<ServiceForceNode>(area.width / 2).strength(0.04),
      y: forceY<ServiceForceNode>(area.height / 2).strength(0.04),
      boundary: cardBoundary(area),
      collision: cardCollision(area),
    },
  }
}

/** Settle the same d3-force engine used by LayerChart before publishing positions. */
export function serviceGraphLayout(
  services: readonly ServiceSummary[],
  connections: readonly ServiceConnection[],
  sizes: ServiceSizes,
  viewport: ServiceSize,
  previous: ServicePositions = new Map()
): ServiceLayout {
  const { data, forces } = createServiceSimulation(
    services,
    connections,
    sizes,
    viewport,
    previous
  )
  const simulation = forceSimulation<ServiceForceNode, ServiceForceLink>(
    data.nodes
  ).stop()
  for (const [name, force] of Object.entries(forces))
    simulation.force(name, force)
  simulation.tick(
    Math.ceil(
      Math.log(simulation.alphaMin()) / Math.log(1 - simulation.alphaDecay())
    )
  )
  return serviceForceLayout(simulation.nodes(), connections, sizes)
}

function cardPort(
  point: ServicePoint,
  target: ServicePoint,
  size: ServiceSize
) {
  const dx = target.x - point.x,
    dy = target.y - point.y
  const ratio =
    1 /
    Math.max(
      Math.abs(dx) / (size.width / 2 + EDGE_CLEARANCE),
      Math.abs(dy) / (size.height / 2 + EDGE_CLEARANCE)
    )
  return { x: point.x + dx * ratio, y: point.y + dy * ratio }
}

function serviceForceLayout(
  nodes: readonly ServiceForceNode[],
  connections: readonly ServiceConnection[],
  sizes: ServiceSizes
): ServiceLayout {
  const positions = new Map(
    nodes.map(node => [node.id, { x: node.x, y: node.y }])
  )
  const paths = new Map<string, string>()
  const pairs = new Set(connections.map(connectionIdentity))
  for (const edge of connections) {
    const sourceID = serviceIdentity(edge.source),
      targetID = serviceIdentity(edge.target)
    const source = positions.get(sourceID)!,
      target = positions.get(targetID)!
    const sourceSize = cardSize(sizes, sourceID),
      targetSize = cardSize(sizes, targetID)
    if (sourceID === targetID) {
      const x = source.x + sourceSize.width / 2 + EDGE_CLEARANCE,
        y = source.y
      paths.set(
        connectionIdentity(edge),
        `M${x},${y - 16} C${x + 48},${y - 64} ${x + 48},${y + 64} ${x},${y + 16}`
      )
      continue
    }
    const reciprocal = pairs.has(
      connectionIdentity({ ...edge, source: edge.target, target: edge.source })
    )
    const dx = target.x - source.x,
      dy = target.y - source.y
    const length = Math.hypot(dx, dy)
    const control = {
      x: (source.x + target.x) / 2 - (reciprocal ? (dy / length) * 40 : 0),
      y: (source.y + target.y) / 2 + (reciprocal ? (dx / length) * 40 : 0),
    }
    const start = cardPort(source, control, sourceSize),
      end = cardPort(target, control, targetSize)
    paths.set(
      connectionIdentity(edge),
      `M${start.x},${start.y} Q${control.x},${control.y} ${end.x},${end.y}`
    )
  }
  return { positions, paths }
}
