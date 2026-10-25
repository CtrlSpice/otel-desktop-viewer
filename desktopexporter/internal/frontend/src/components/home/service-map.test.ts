import { describe, expect, it } from 'vitest'
import {
  connectionIdentity,
  serviceHierarchy,
  servicePositions,
  retainServicePositions,
  connectionPath,
} from './service-map'
import { tree } from 'd3-hierarchy'
import {
  serviceIdentity,
  type ServiceSummary,
  type ServiceConnection,
} from '@/services/discovery-types'

const service = (name: string, namespace = ''): ServiceSummary => ({
  serviceName: name,
  serviceNamespace: namespace,
  traceCount: 1,
  spanCount: 1,
  errorSpanCount: 0,
  logCount: 0,
  errorLogCount: 0,
  logSeverityCounts: {
    trace: 0,
    debug: 0,
    info: 0,
    warn: 0,
    error: 0,
    fatal: 0,
    other: 0,
  },
  metricCount: 0,
  dataPointCount: 0,
  exemplarCount: 0,
  lastSeen: null,
})
const edge = (
  source: ServiceSummary,
  target: ServiceSummary
): ServiceConnection => ({
  source,
  target,
  spanCount: 1,
  traceCount: 1,
  errorSpanCount: 0,
})

describe('service map layout hierarchy', () => {
  it('keeps discovered positions when tree angles change and places new nodes without overlap', () => {
    const previous = new Map([
      ['api', { x: 0, y: 0 }],
      ['worker', { x: 0, y: 220 }],
    ])
    const candidates = new Map([
      ['api', { x: 0, y: 0 }],
      ['worker', { x: 220, y: 0 }],
      ['store', { x: 0, y: 220 }],
    ])
    const positions = retainServicePositions(previous, candidates)
    expect(positions.get('api')).toEqual(previous.get('api'))
    expect(positions.get('worker')).toEqual(previous.get('worker'))
    expect(positions.get('store')!.x).toBeCloseTo(0)
    expect(positions.get('store')!.y).toBeCloseTo(440)
    expect(previous.size).toBe(2)
  })
  it('keeps one node per service through cycles, shared dependencies and reverse edges', () => {
    const a = service('api'),
      b = service('worker'),
      c = service('store'),
      other = service('api', 'other')
    const edges = [
      edge(a, b),
      edge(b, a),
      edge(b, c),
      edge(a, c),
      edge(c, other),
    ]
    const nodes = serviceHierarchy(a, edges).descendants()
    expect(
      nodes.map(node => serviceIdentity(node.data.service)).sort()
    ).toEqual([a, b, c, other].map(serviceIdentity).sort())
    expect(nodes[0]!.data.service).toEqual(a)
    expect(new Set(edges.map(connectionIdentity)).size).toBe(5)
    expect(nodes.find(node => node.data.service === c)!.depth).toBe(1)
  })
  it('ignores disconnected stale edges and remains deterministic under response ordering', () => {
    const a = service('api'),
      b = service('worker'),
      c = service('store'),
      d = service('elsewhere'),
      e = service('other')
    const edges = [edge(a, b), edge(a, c), edge(d, e)]
    const ids = (links: ServiceConnection[]) =>
      serviceHierarchy(a, links)
        .descendants()
        .map(node => serviceIdentity(node.data.service))
    expect(ids(edges)).toEqual(ids([...edges].reverse()))
    expect(ids(edges)).toHaveLength(3)
    expect(serviceHierarchy(a, []).descendants()).toHaveLength(1)
  })
  it('leaves room for readable nodes on crowded rings and keeps arrowheads outside labels', () => {
    const root = service('root')
    const edges = Array.from({ length: 100 }, (_, index) =>
      edge(root, service(`worker-${index}`))
    )
    const layout = tree<ReturnType<typeof serviceHierarchy>['data']>().size([
      2 * Math.PI,
      50,
    ])(serviceHierarchy(root, edges))
    const positions = [...servicePositions(layout.descendants()).values()]
    expect(Math.hypot(positions[0]!.x, positions[0]!.y)).toBe(0)
    for (let i = 0; i < positions.length; i++) {
      for (let j = i + 1; j < positions.length; j++) {
        expect(
          Math.hypot(
            positions[i]!.x - positions[j]!.x,
            positions[i]!.y - positions[j]!.y
          )
        ).toBeGreaterThanOrEqual(209.9)
      }
    }
    const path = connectionPath({ x: 0, y: 0 }, { x: 220, y: 0 })
    const numbers = path.match(/-?\d+(?:\.\d+)?/g)!.map(Number)
    expect(numbers[0]).toBeGreaterThan(90)
    expect(numbers[4]).toBeLessThan(130)
    expect(connectionPath({ x: 220, y: 0 }, { x: 0, y: 0 })).not.toEqual(path)
  })
})
