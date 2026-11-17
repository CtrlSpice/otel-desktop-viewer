import { describe, expect, it } from 'vitest'
import {
  connectionIdentity,
  graphServices,
  serviceGraphLayout,
  retainServicePositions,
  serviceGraphBounds,
  fitServiceGraph,
  constrainServiceGraph,
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
function settle(
  services: ServiceSummary[],
  connections: ServiceConnection[],
  width: number,
  height = 900,
  previous: ServicePositions = new Map()
) {
  return serviceGraphLayout(
    services,
    connections,
    new Map(),
    { width, height },
    previous
  )
}

function expectNoOverlap(positions: ServicePositions) {
  const points = [...positions.values()]
  for (let i = 0; i < points.length; i++) {
    for (let j = i + 1; j < points.length; j++) {
      expect(
        Math.abs(points[i]!.x - points[j]!.x) >= 320 ||
          Math.abs(points[i]!.y - points[j]!.y) >= 96
      ).toBe(true)
    }
  }
}

describe('service graph layout', () => {
  it('fits every measured card inside padded wide and tall viewports', () => {
    const bounds = serviceGraphBounds(
      new Map([
        ['a', { x: -500, y: 100 }],
        ['b', { x: 1800, y: 1200 }],
      ]),
      new Map([['b', { width: 320, height: 200 }]])
    )!
    expect(bounds).toEqual({ left: -660, right: 1960, top: 52, bottom: 1300 })
    for (const viewport of [
      { width: 995, height: 1100 },
      { width: 1400, height: 400 },
      { width: 300, height: 500 },
    ]) {
      const { scale, translate } = fitServiceGraph(bounds, viewport)
      expect(bounds.left * scale + translate.x).toBeGreaterThanOrEqual(
        32 - 1e-9
      )
      expect(bounds.top * scale + translate.y).toBeGreaterThanOrEqual(32 - 1e-9)
      expect(bounds.right * scale + translate.x).toBeLessThanOrEqual(
        viewport.width - 32 + 1e-9
      )
      expect(bounds.bottom * scale + translate.y).toBeLessThanOrEqual(
        viewport.height - 32 + 1e-9
      )
    }
    expect(serviceGraphBounds(new Map(), new Map())).toBeUndefined()
  })

  it('bounds extreme panning at different zoom levels without blocking ordinary movement', () => {
    const bounds = { left: -100, right: 1000, top: 50, bottom: 900 }
    const viewport = { width: 800, height: 600 }
    for (const scale of [0.1, 1, 6]) {
      for (const offset of [-100000, 100000]) {
        const translate = constrainServiceGraph(bounds, viewport, scale, {
          x: offset,
          y: offset,
        })
        expect(bounds.left * scale + translate.x).toBeLessThanOrEqual(
          viewport.width - 64
        )
        expect(bounds.right * scale + translate.x).toBeGreaterThanOrEqual(64)
        expect(bounds.top * scale + translate.y).toBeLessThanOrEqual(
          viewport.height - 64
        )
        expect(bounds.bottom * scale + translate.y).toBeGreaterThanOrEqual(64)
      }
    }
    expect(
      constrainServiceGraph(bounds, viewport, 1, { x: 48, y: -48 })
    ).toEqual({ x: 48, y: -48 })
  })

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
    const layout = settle(services, edges, 1000)
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

  it('keeps standalone cards without invented edges and clips arrows outside cards', () => {
    const a = service('api'),
      b = service('worker'),
      c = service('metrics-only'),
      d = service('logs-only')
    const edges = [edge(a, b)]
    const services = graphServices([a, b, c, d], edges)
    const layout = settle(services, edges, 900)
    const point = (s: ServiceSummary) =>
      layout.positions.get(serviceIdentity(s))!
    expect(layout.paths.size).toBe(1)
    expectNoOverlap(layout.positions)
    const path = layout.paths.get(connectionIdentity(edges[0]!))!
    const start = path.match(/^M([^,]+),([^Q]+)/)!
    expect(
      Math.abs(Number(start[1]) - point(a).x) >= 165 ||
        Math.abs(Number(start[2]) - point(a).y) >= 53
    ).toBe(true)
    expect(
      Math.max(...[...layout.positions.values()].map(p => p.x + 160))
    ).toBeLessThan(900)
  })

  it('pins discovered positions when more nodes arrive', () => {
    const a = service('api'),
      b = service('worker'),
      c = service('store')
    const initial = settle([a, b], [edge(a, b)], 900)
    const next = settle(
      [a, b, c],
      [edge(a, b), edge(b, c)],
      900,
      900,
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
    const layout = settle(services, [], 900)
    expect(layout.positions.size).toBe(100)
    expect(layout.paths.size).toBe(0)
    expectNoOverlap(layout.positions)
    const narrow = settle(services.slice(0, 4), [], 448)
    const bounds = serviceGraphBounds(narrow.positions, new Map())!
    expect(bounds.right - bounds.left).toBeLessThan(384)
  })

  it('reflows a wide fan-out and disconnected cards for portrait and landscape panes', () => {
    const services = Array.from({ length: 19 }, (_, i) =>
      service(`service-${i}`)
    )
    const edges = services.slice(1, 8).map(target => edge(services[0]!, target))
    for (const viewport of [
      { width: 995, height: 1100 },
      { width: 1500, height: 650 },
    ]) {
      const layout = settle(services, edges, viewport.width, viewport.height)
      expectNoOverlap(layout.positions)
      expect(
        fitServiceGraph(
          serviceGraphBounds(layout.positions, new Map())!,
          viewport
        ).scale,
        JSON.stringify(viewport)
      ).toBeGreaterThan(0.8)
      expect(
        settle(services, edges, viewport.width, viewport.height).positions
      ).toEqual(layout.positions)
    }
  })
})
