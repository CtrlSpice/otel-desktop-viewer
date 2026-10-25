import { describe, expect, it } from 'vitest'
import {
  connectionIdentity,
  graphServices,
  serviceGraphLayout,
  retainServicePositions,
  type ServicePositions,
} from './service-map'
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
function expectNoOverlap(positions: ServicePositions) {
  const points = [...positions.values()]
  for (let i = 0; i < points.length; i++) {
    for (let j = i + 1; j < points.length; j++) {
      expect(
        Math.abs(points[i]!.x - points[j]!.x) >= 352 ||
          Math.abs(points[i]!.y - points[j]!.y) >= 128
      ).toBe(true)
    }
  }
}

describe('service graph layout', () => {
  it('keeps complete service summaries and exact identities through cycles and shared dependencies', () => {
    const a = service('api'),
      b = { ...service('worker'), metricCount: 12 },
      c = service('store'),
      other = service('api', 'other')
    const edges = [
      edge(a, { ...b, metricCount: 0 }),
      edge(b, a),
      edge(b, c),
      edge(a, c),
      edge(c, other),
    ]
    const services = graphServices([a, b], edges)
    expect(services.map(serviceIdentity).sort()).toEqual(
      [a, b, c, other].map(serviceIdentity).sort()
    )
    expect(services.find(s => s.serviceName === 'worker')!.metricCount).toBe(12)
    const layout = serviceGraphLayout(services, edges, new Map(), 1000)
    expect(layout.positions.size).toBe(4)
    expect(layout.paths.size).toBe(5)
    for (const path of layout.paths.values()) {
      expect(path).toMatch(/^M/)
      expect(path).not.toMatch(/NaN|Infinity/)
    }
    expect(layout.paths.get(connectionIdentity(edges[0]!))).not.toEqual(
      layout.paths.get(connectionIdentity(edges[1]!))
    )
    expectNoOverlap(layout.positions)
  })

  it('lays out directed flows vertically and packs standalone cards beside them without invented edges', () => {
    const a = service('api'),
      b = service('worker'),
      c = service('metrics-only'),
      d = service('logs-only')
    const edges = [edge(a, b)]
    const services = graphServices([a, b, c, d], edges)
    const layout = serviceGraphLayout(services, edges, new Map(), 900)
    const point = (s: ServiceSummary) =>
      layout.positions.get(serviceIdentity(s))!
    expect(point(a).x).toBe(point(b).x)
    expect(point(a).y).toBeLessThan(point(b).y)
    expect(point(c).x).toBe(point(d).x)
    expect(point(c).x).toBeGreaterThan(point(a).x)
    expect(layout.paths.size).toBe(1)
    expectNoOverlap(layout.positions)
    const path = layout.paths.get(connectionIdentity(edges[0]!))!
    const start = path.match(/^M([^,]+),([^LC]+)/)!
    expect(Number(start[2])).toBeGreaterThan(point(a).y + 48)
    expect(
      Math.max(...[...layout.positions.values()].map(p => p.x + 160))
    ).toBeLessThan(900)
  })

  it('keeps discovered positions when new nodes arrive or the viewport changes', () => {
    const a = service('api'),
      b = service('worker'),
      c = service('store')
    const initial = serviceGraphLayout([a, b], [edge(a, b)], new Map(), 900)
    const next = serviceGraphLayout(
      [a, b, c],
      [edge(a, b), edge(b, c)],
      new Map(),
      448,
      initial.positions
    )
    expect(next.positions.get(serviceIdentity(a))).toEqual(
      initial.positions.get(serviceIdentity(a))
    )
    expect(next.positions.get(serviceIdentity(b))).toEqual(
      initial.positions.get(serviceIdentity(b))
    )
    expectNoOverlap(next.positions)
    const collision = retainServicePositions(
      new Map([['a', { x: 0, y: 0 }]]),
      new Map([
        ['a', { x: 100, y: 100 }],
        ['b', { x: 0, y: 0 }],
      ])
    )
    expect(collision.get('a')).toEqual({ x: 0, y: 0 })
    expectNoOverlap(collision)
  })

  it('packs a page of standalone cards at readable size without overlap', () => {
    const services = Array.from({ length: 100 }, (_, i) =>
      service(`service-${i}`)
    )
    const layout = serviceGraphLayout(services, [], new Map(), 900)
    expect(layout.positions.size).toBe(100)
    expect(layout.paths.size).toBe(0)
    expectNoOverlap(layout.positions)
    const narrow = serviceGraphLayout(services.slice(0, 4), [], new Map(), 448)
    expect(new Set([...narrow.positions.values()].map(p => p.x)).size).toBe(1)
  })
})
