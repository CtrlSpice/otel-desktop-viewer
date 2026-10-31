import { routeTelemetry } from './telemetry-fixtures'
import { expect, test, type Page } from '@playwright/test'
import type {
  ServiceSummary,
  ServiceConnection,
  ServiceConnectionSpan,
  DiscoveryOptions,
  DiscoveryTotals,
} from '../src/services/discovery-types'

const service = (serviceName: string): ServiceSummary => ({
  serviceName,
  serviceNamespace: '',
  serviceIdentity: serviceName,
  traceCount: 1,
  spanCount: 2,
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

type DiscoveryReply =
  | {
      result: {
        rows: unknown[][]
        truncated: boolean
        totals?: DiscoveryTotals
      }
    }
  | { error: { code: number; message: string } }

async function holdDiscovery(
  page: Page,
  kinds: DiscoveryOptions['kind'][] = ['services', 'connections']
) {
  const requests: Array<{
    kind: string
    owner?: string
    options: DiscoveryOptions
    reply: { resolve: (value: DiscoveryReply) => void }
  }> = []
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await routeTelemetry(page, async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'discoverTelemetry' && kinds.includes(params.options.kind)) {
      let resolve!: (value: DiscoveryReply) => void
      const pending = new Promise<DiscoveryReply>(done => (resolve = done))
      requests.push({
        kind: params.options.kind,
        owner: params.options.serviceIdentity,
        options: params.options,
        reply: { resolve },
      })
      await route.fulfill({
        json: { jsonrpc: '2.0', id, ...(await pending) },
      })
      return
    }
    const result =
      method === 'getStats'
        ? {
            traces: {
              traceCount: 0,
              spanCount: 0,
              serviceCount: 0,
              errorCount: 0,
              lastReceived: null,
            },
            logs: { logCount: 0, errorCount: 0, lastReceived: null },
            metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
            rejections: [],
          }
        : method === 'discoverTelemetry'
          ? { rows: [], truncated: false }
          : []
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  return requests
}

async function holdConnections(page: Page) {
  await page.setViewportSize({ width: 2400, height: 1100 })
  const requests = await holdDiscovery(page, [
    'services',
    'connections',
    'connection-spans',
  ])
  const get = async (kind: DiscoveryOptions['kind'], index: number) => {
    await expect
      .poll(() => requests.filter(request => request.kind === kind).length)
      .toBeGreaterThan(index)
    return requests.filter(request => request.kind === kind)[index]
  }
  return {
    requests,
    get,
    ok: async (
      kind: DiscoveryOptions['kind'],
      index: number,
      items: unknown[],
      truncated = false,
      totals?: DiscoveryTotals
    ) => {
      const request = await get(kind, index)
      request.reply.resolve({
        result: { rows: items.map(item => [item]), truncated, totals },
      })
    },
    fail: async (kind: DiscoveryOptions['kind'], index: number) => {
      const request = await get(kind, index)
      request.reply.resolve({
        error: { code: -32603, message: 'Discovery unavailable' },
      })
    },
    release: () => {
      for (const request of requests)
        request.reply.resolve({ result: { rows: [], truncated: false } })
    },
  }
}

const connection = (spanCount = 1, target = 'worker'): ServiceConnection => ({
  source: service('api'),
  target: service(target),
  spanCount,
  traceCount: 1,
  errorSpanCount: 0,
})
const connectedSpan = (name: string, digit = '2'): ServiceConnectionSpan => ({
  traceID: '1'.repeat(32),
  spanID: digit.repeat(16),
  name,
  startTime: '1000000000',
  endTime: '1000000100',
  statusCode: 0,
})

test('retained edge inspection owns its time range and survives unrelated service-list success', async ({
  page,
}) => {
  const h = await holdConnections(page)
  try {
    await page.goto('/?start=1000&end=2000')
    await h.ok('services', 0, [service('api'), service('worker')])
    await h.ok('connections', 0, [connection()])
    await h.ok('connections', 1, [connection()])
    const edge = page.getByRole('button', { name: /^api → worker: 1 / })
    await expect(edge).toBeAttached()
    await expect(
      page.getByRole('figure', { name: 'Service connections' })
    ).toBeVisible()
    await page.evaluate(() => {
      window.history.pushState(null, '', '/?start=3000&end=4000')
      window.dispatchEvent(new PopStateEvent('popstate'))
    })
    await h.fail('services', 1)
    await h.fail('connections', 2)
    await h.fail('connections', 3)
    await expect(page.locator('.service-map').getByRole('alert')).toHaveCount(3)
    await edge.focus()
    await edge.press('Enter')
    expect((await h.get('connection-spans', 0)).options).toMatchObject({
      startTime: '1000000000',
      endTime: '2000000000',
    })
    await h.ok('connection-spans', 0, [connectedSpan('retained evidence')])
    const link = page.getByRole('link', { name: /retained evidence/ })
    await expect(link).toBeVisible()
    const original = await link.elementHandle()
    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    await h.ok('services', 2, [service('api'), service('worker')])
    await h.fail('connections', 4)
    await h.fail('connections', 5)
    await expect(page.locator('.service-map').getByRole('alert')).toHaveCount(2)
    await expect(edge).toHaveAttribute('aria-pressed', 'true')
    await expect(
      page.getByRole('table', { name: 'Connection details' })
    ).toBeVisible()
    expect(
      await link.evaluate((element, previous) => element === previous, original)
    ).toBe(true)
    await expect(link).toHaveAttribute('href', /span=2222222222222222/)
  } finally {
    h.release()
  }
})

test('ready connection pagination does not wait for an unrelated pending owner', async ({
  page,
}) => {
  const h = await holdConnections(page)
  try {
    await page.goto('/?time=all')
    await h.ok('services', 0, [service('api'), service('worker')])
    await h.ok('connections', 0, [connection()], true)
    await h.ok('connections', 1, [])
    const more = page.getByRole('button', {
      name: 'More connections',
      exact: true,
    })
    await expect(more).toBeEnabled()
    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    await h.get('connections', 3)
    const cards = page.locator('.service-map__node')
    await cards.getByRole('button', { name: /^worker / }).click()
    await cards.getByRole('button', { name: /^api / }).click()
    await expect(more).toBeEnabled()
    await h.ok('connections', 2, [connection(9)], true)
    await expect(more).toBeEnabled()
    await more.click()
    expect((await h.get('connections', 4)).options).toMatchObject({
      serviceIdentity: 'api',
      offset: 1,
    })
    await h.ok('connections', 4, [])
    await expect(more).toHaveCount(0)
    await expect(page.locator('.service-map')).toHaveAttribute(
      'aria-busy',
      'true'
    )
  } finally {
    h.release()
  }
})

test('retained connection pagination owns its range and offset after failed replacement', async ({
  page,
}) => {
  const h = await holdConnections(page)
  try {
    await page.goto('/?start=1000&end=2000')
    await h.ok('services', 0, [service('api')])
    await h.ok('connections', 0, [connection()], true)
    const more = page.getByRole('button', {
      name: 'More connections',
      exact: true,
    })
    await expect(more).toBeEnabled()
    await page.evaluate(() => {
      window.history.pushState(null, '', '/?start=3000&end=4000')
      window.dispatchEvent(new PopStateEvent('popstate'))
    })
    await h.get('connections', 1)
    await expect(more).toBeEnabled()
    await h.fail('services', 1)
    await h.fail('connections', 1)
    await expect(page.locator('.service-map').getByRole('alert')).toHaveCount(2)
    await more.click()
    const retained = (await h.get('connections', 2)).options
    expect(retained).toMatchObject({
      startTime: '1000000000',
      endTime: '2000000000',
      serviceIdentity: 'api',
      offset: 1,
    })
    await h.fail('connections', 2)
    await expect(page.locator('.service-map').getByRole('alert')).toHaveCount(3)
    await more.click()
    expect((await h.get('connections', 3)).options).toEqual(retained)
    await h.ok('connections', 3, [connection(2, 'store')], true)
    await expect(
      page.getByRole('button', { name: /^api → store: 2 / })
    ).toBeAttached()
    await expect(
      page.getByRole('button', { name: /^api → worker: 1 / })
    ).toBeAttached()
    await more.click()
    expect((await h.get('connections', 4)).options).toEqual({
      ...retained,
      offset: 2,
    })
    await page
      .locator('.service-map')
      .getByRole('button', { name: 'Retry', exact: true })
      .click()
    expect((await h.get('connections', 5)).options).toMatchObject({
      startTime: '3000000000',
      endTime: '4000000000',
      offset: 0,
    })
    await h.ok('connections', 5, [connection(9)], true)
    await expect(
      page.getByRole('button', { name: /^api → worker: 9 / })
    ).toBeAttached()
    await expect(
      page.getByRole('button', { name: /^api → store:/ })
    ).toHaveCount(0)
    await h.ok('connections', 4, [connection(3, 'store')])
    await more.click()
    expect((await h.get('connections', 6)).options).toMatchObject({
      startTime: '3000000000',
      endTime: '4000000000',
      offset: 1,
    })
  } finally {
    h.release()
  }
})

test('a service-list page preserves another connection owner failure until its own Retry', async ({
  page,
}) => {
  const h = await holdConnections(page)
  try {
    await page.goto('/?time=all')
    await h.ok('services', 0, [service('api')], true)
    await h.fail('connections', 0)
    const map = page.locator('.service-map')
    const error = map.getByRole('alert')
    await expect(error).toContainText('api:')
    await page
      .getByRole('button', { name: 'More services', exact: true })
      .click()
    await h.ok('services', 1, [service('worker')])
    await h.get('connections', 1)
    expect(
      h.requests
        .filter(request => request.kind === 'connections')
        .map(request => request.owner)
    ).toEqual(['api', 'worker'])
    await expect(error).toContainText('api:')
    await map.getByRole('button', { name: 'Retry', exact: true }).click()
    expect((await h.get('connections', 2)).options).toEqual(
      (await h.get('connections', 0)).options
    )
    await h.ok('connections', 2, [connection(9)])
    await expect(
      page.getByRole('button', { name: /^api → worker: 9 / })
    ).toBeAttached()
    await expect(error).toHaveCount(0)
  } finally {
    h.release()
  }
})

for (const searchActive of [false, true]) {
  test(`retained count hrefs and clicks own their successful time window with search=${searchActive}`, async ({
    page,
  }) => {
    const h = await holdConnections(page)
    try {
      await page.goto(
        '/?start=1000&end=2000' + (searchActive ? '&q=search(%22A%22)' : '')
      )
      await h.ok('services', 0, [service('api')], false, {
        home: 1,
        traces: 31,
        metrics: 41,
        logs: 51,
      })
      await h.ok('connections', 0, [])
      const nav = page.getByRole('navigation', { name: 'Primary' })
      const traces = nav.getByRole('link', { name: 'Traces [31]', exact: true })
      await expect(traces).toBeVisible()
      const hrefs = await Promise.all(
        ['Traces [31]', 'Metrics [41]', 'Logs [51]'].map(name =>
          nav.getByRole('link', { name, exact: true }).getAttribute('href')
        )
      )
      await page.evaluate(() => {
        const url = new URL(window.location.href)
        url.searchParams.set('start', '3000')
        url.searchParams.set('end', '4000')
        window.history.pushState(null, '', url)
        window.dispatchEvent(new PopStateEvent('popstate'))
      })
      await h.get('services', 1)
      await expect(traces).toHaveAttribute('href', hrefs[0]!)
      await h.fail('services', 1)
      await h.fail('connections', 1)
      await expect(page.locator('.service-map').getByRole('alert')).toHaveCount(
        2
      )
      for (const [index, name] of [
        'Traces [31]',
        'Metrics [41]',
        'Logs [51]',
      ].entries())
        await expect(
          nav.getByRole('link', { name, exact: true })
        ).toHaveAttribute('href', hrefs[index]!)
      await traces.click()
      await expect(page).toHaveURL(
        url =>
          url.pathname === '/traces' &&
          url.searchParams.get('start') === '1000' &&
          url.searchParams.get('end') === '2000'
      )
      const query = new URL(page.url()).searchParams.get('q')
      expect(query ?? '').toBe(
        new URL(hrefs[0]!, page.url()).searchParams.get('q') ?? ''
      )
    } finally {
      h.release()
    }
  })
}

test('evidence pagination retries failed pages and keeps the displayed target during another selection', async ({
  page,
}) => {
  const h = await holdConnections(page)
  try {
    await page.goto('/?time=all')
    await h.ok('services', 0, [service('api')])
    await h.ok('connections', 0, [connection(), connection(9, 'store')])
    const worker = page.getByRole('button', { name: /^api → worker:/ })
    await worker.focus()
    await worker.press('Enter')
    await h.ok('connection-spans', 0, [connectedSpan('worker first')], true)
    const details = page.getByRole('region', { name: 'Home details' })
    const next = details.getByRole('button', { name: 'Next', exact: true })
    await next.click()
    await h.fail('connection-spans', 1)
    await expect(details.getByRole('alert')).toBeVisible()
    await next.click()
    expect((await h.get('connection-spans', 2)).options.offset).toBe(25)
    await h.ok(
      'connection-spans',
      2,
      [connectedSpan('worker second', '3')],
      true
    )
    await expect(details.getByText('Page 2', { exact: true })).toBeVisible()
    const store = page.getByRole('button', { name: /^api → store:/ })
    await store.focus()
    await store.press('Enter')
    await h.get('connection-spans', 3)
    await next.click()
    expect((await h.get('connection-spans', 4)).options).toMatchObject({
      targetIdentity: 'worker',
      offset: 50,
    })
    await h.ok(
      'connection-spans',
      4,
      [connectedSpan('worker third', '4')],
      false
    )
    await expect(
      details.getByRole('link', { name: /worker third/ })
    ).toBeVisible()
    await h.fail('connection-spans', 3)
    await expect(details.getByRole('alert')).toBeVisible()
    await details.getByRole('button', { name: 'Retry', exact: true }).click()
    expect((await h.get('connection-spans', 5)).options).toMatchObject({
      targetIdentity: 'store',
      offset: 0,
    })
    await h.ok('connection-spans', 5, [connectedSpan('store first', '5')])
    await expect(
      details.getByRole('link', { name: /store first/ })
    ).toBeVisible()
    await expect(details.getByText('Page 1', { exact: true })).toBeVisible()
    await expect(details.locator('[data-field="Child service"]')).toContainText(
      'store'
    )
    await expect(
      details.locator(
        '[data-field="Parent-child connections"] .detail-property__value'
      )
    ).toHaveText('9')
  } finally {
    h.release()
  }
})

test('service-map pagination keeps the displayed query and time range after failed replacements', async ({
  page,
}) => {
  const requests = await holdDiscovery(page, ['services'])
  const nodes = page.locator('.service-map__node')
  const identities = () =>
    nodes.evaluateAll(elements =>
      elements
        .map(element => element.getAttribute('data-service-identity'))
        .sort()
    )
  const reply = (index: number, name: string, truncated = true) =>
    requests[index].reply.resolve({
      result: { rows: [[service(name)]], truncated },
    })
  const more = page.getByRole('button', { name: 'More services', exact: true })
  try {
    await page.goto('/?start=1000&end=2000')
    await expect.poll(() => requests.length).toBe(1)
    reply(0, 'A-first')
    await expect.poll(identities).toEqual(['A-first'])
    await more.click()
    await expect.poll(() => requests.length).toBe(2)
    reply(1, 'A-second')
    await expect.poll(identities).toEqual(['A-first', 'A-second'])
    const canvas = await page.locator('.service-map__canvas').elementHandle()
    const initial = requests[0].options
    const search = page.getByRole('searchbox', { name: 'Search telemetry' })
    await search.fill('search("B")')
    await search.press('Enter')
    await expect.poll(() => requests.length).toBe(3)
    expect(JSON.stringify(requests[2].options.queries)).toContain('B')
    await expect.poll(identities).toEqual(['A-first', 'A-second'])
    requests[2].reply.resolve({
      error: { code: -32603, message: 'Search unavailable' },
    })
    await expect(page.getByRole('alert')).toContainText('Search unavailable')

    await page.evaluate(() => {
      const next = new URL(window.location.href)
      next.searchParams.set('start', '3000')
      next.searchParams.set('end', '4000')
      window.history.pushState(null, '', next)
      window.dispatchEvent(new PopStateEvent('popstate'))
    })
    await expect.poll(() => requests.length).toBe(4)
    expect(requests[3].options).toMatchObject({
      startTime: '3000000000',
      endTime: '4000000000',
      offset: 0,
    })
    requests[3].reply.resolve({
      error: { code: -32603, message: 'Time range unavailable' },
    })
    await expect(page.getByRole('alert')).toContainText(
      'Time range unavailable'
    )
    await more.click()
    await expect.poll(() => requests.length).toBe(5)
    expect(requests[4].options).toEqual({ ...initial, offset: 200 })
    reply(4, 'A-third')
    await expect.poll(identities).toEqual(['A-first', 'A-second', 'A-third'])
    expect(
      await page
        .locator('.service-map__canvas')
        .evaluate((element, previous) => element === previous, canvas)
    ).toBe(true)

    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    await expect.poll(() => requests.length).toBe(6)
    expect(requests[5].options).toMatchObject({
      startTime: '3000000000',
      endTime: '4000000000',
      offset: 0,
    })
    expect(JSON.stringify(requests[5].options.queries)).toContain('B')
    reply(5, 'B-first')
    await expect.poll(identities).toEqual(['B-first'])
    await more.click()
    await expect.poll(() => requests.length).toBe(7)
    expect(requests[6].options).toEqual({ ...requests[5].options, offset: 100 })
    reply(6, 'B-second', false)
    await expect.poll(identities).toEqual(['B-first', 'B-second'])
    await expect(more).toHaveCount(0)
  } finally {
    for (const request of requests)
      request.reply.resolve({ result: { rows: [], truncated: false } })
  }
})

test('initial service-map failure leaves counts unknown; successful empty discovery publishes and retains zero', async ({
  page,
}) => {
  const requests = await holdDiscovery(page)
  try {
    await page.goto('/?time=all')
    const header = page.getByRole('region', {
      name: 'Service map',
      exact: true,
    })
    await expect(header).toBeVisible()
    await expect.poll(() => requests.length).toBe(1)
    await expect(header).not.toContainText(/\d+ (services|connections)/)
    await expect(
      page.getByText('Loading service map…', { exact: true })
    ).toBeVisible()
    requests[0].reply.resolve({
      error: { code: -32603, message: 'Discovery unavailable' },
    })
    await expect(page.getByRole('alert')).toContainText('Discovery unavailable')
    await expect(header).not.toContainText(/\d+ (services|connections)/)
    await expect(
      page.getByText('No services match this search and time range.')
    ).toHaveCount(0)
    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    await expect.poll(() => requests.length).toBe(2)
    requests[1].reply.resolve({ result: { rows: [], truncated: false } })
    await expect(header).toContainText('0 services · 0 connections')
    await expect(
      page.getByText('No services match this search and time range.')
    ).toBeVisible()
    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    await expect.poll(() => requests.length).toBe(3)
    await expect(header).toContainText('0 services · 0 connections')
    requests[2].reply.resolve({
      error: { code: -32603, message: 'Refresh unavailable' },
    })
    await expect(page.getByRole('alert')).toContainText('Refresh unavailable')
    await expect(header).toContainText('0 services · 0 connections')
  } finally {
    for (const request of requests)
      request.reply.resolve({ result: { rows: [], truncated: false } })
  }
})

test('service-map connection counts publish independently and survive failed refresh with the same canvas', async ({
  page,
}) => {
  const requests = await holdDiscovery(page)
  const services = ['api', 'worker', 'store'].map(service)
  try {
    await page.goto('/?time=all')
    await expect.poll(() => requests.length).toBe(1)
    requests[0].reply.resolve({
      result: { rows: services.map(service => [service]), truncated: false },
    })
    await expect
      .poll(
        () => requests.filter(request => request.kind === 'connections').length
      )
      .toBe(3)
    const header = page.getByRole('region', {
      name: 'Service map',
      exact: true,
    })
    const map = page.locator('.service-map')
    await expect(header).toContainText('3 services')
    await expect(header).not.toContainText(/\d+ connections?/)
    requests
      .find(request => request.owner === 'api')!
      .reply.resolve({ result: { rows: [], truncated: false } })
    requests
      .find(request => request.owner === 'worker')!
      .reply.resolve({ error: { code: -32603, message: 'Worker unavailable' } })
    await expect(map.getByRole('alert')).toContainText('Worker unavailable')
    await expect(header).not.toContainText(/\d+ connections?/)
    await map.getByRole('button', { name: 'Retry', exact: true }).click()
    await expect
      .poll(() => requests.filter(request => request.owner === 'worker').length)
      .toBe(2)
    requests.at(-1)!.reply.resolve({
      result: {
        rows: [
          [
            {
              source: services[0],
              target: services[1],
              spanCount: 2,
              traceCount: 1,
              errorSpanCount: 0,
            },
          ],
        ],
        truncated: false,
      },
    })
    await expect(header).toContainText('3 services · 1 connection')
    await expect(map).toHaveAttribute('aria-busy', 'true')
    requests
      .find(request => request.owner === 'store')!
      .reply.resolve({ error: { code: -32603, message: 'Store unavailable' } })
    await expect(map.getByRole('alert')).toContainText('Store unavailable')
    const canvas = await map.locator('.service-map__canvas').elementHandle()
    const before = requests.length
    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    await expect.poll(() => requests.length).toBe(before + 4)
    await expect(header).toContainText('3 services · 1 connection')
    await expect(
      page.getByRole('button', { name: 'Zoom in service map' })
    ).toBeEnabled()
    for (const request of requests.slice(before))
      request.reply.resolve({
        error: { code: -32603, message: 'Refresh unavailable' },
      })
    await expect(map.getByRole('alert')).toHaveCount(4)
    await expect(header).toContainText('3 services · 1 connection')
    expect(
      await map
        .locator('.service-map__canvas')
        .evaluate((element, previous) => element === previous, canvas)
    ).toBe(true)
  } finally {
    for (const request of requests)
      request.reply.resolve({ result: { rows: [], truncated: false } })
  }
})

test('service cards are unique graph nodes with standalone services, stable inspection and exact connection evidence', async ({
  page,
}) => {
  await page.setViewportSize({ width: 2400, height: 1100 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const api = service('api'),
    worker = service('worker'),
    store = service('store'),
    standalone = {
      ...service('metrics-only'),
      traceCount: 0,
      spanCount: 0,
      metricCount: 1,
    }
  const edge = (source: typeof api, target: typeof api) => ({
    source,
    target,
    spanCount: 2,
    traceCount: 1,
    errorSpanCount: 0,
  })
  const expansions: string[] = []
  const evidencePages: number[] = []
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await routeTelemetry(page, async route => {
    const { id, method, params } = route.request().postDataJSON()
    const result = (() => {
      if (method === 'getStats')
        return {
          traces: {
            traceCount: 1,
            spanCount: 6,
            serviceCount: 3,
            errorCount: 0,
            lastReceived: null,
          },
          logs: { logCount: 0, errorCount: 0, lastReceived: null },
          metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
          rejections: [],
        }
      if (method === 'discoverTelemetry') {
        const { kind, serviceIdentity, offset = 0 } = params.options
        let truncated = false
        let items: (
          ServiceSummary | ServiceConnection | ServiceConnectionSpan
        )[] = []
        if (kind === 'services') {
          items = offset === 0 ? [api, worker] : [store, standalone]
          truncated = offset === 0
        }
        if (kind === 'connections') {
          expansions.push(`${serviceIdentity}:${offset}`)
          truncated = serviceIdentity === 'api' && offset === 0
          items =
            serviceIdentity === 'api'
              ? offset === 0
                ? [edge(api, worker), edge(worker, api)]
                : [edge(store, api)]
              : serviceIdentity === 'metrics-only'
                ? []
                : [
                    edge(api, worker),
                    edge(worker, api),
                    edge(worker, store),
                    edge(store, api),
                  ]
        }
        if (kind === 'connection-spans') {
          evidencePages.push(offset)
          truncated = offset === 0
          items = [
            {
              traceID: '1'.repeat(32),
              spanID: (offset === 0 ? '2' : '3').repeat(16),
              name: offset === 0 ? 'received child' : 'next child',
              startTime: '100',
              endTime: '200',
              statusCode: 0,
            },
          ]
        }
        return { rows: items.map(item => [item]), truncated }
      }
      return []
    })()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto('/?time=all')
  const map = page.locator('.service-map')
  const cards = map.locator('.service-map__node')
  const apiNode = cards.getByRole('button', { name: /^api\s/ })
  const workerNode = cards.getByRole('button', {
    name: /^worker\s/,
  })
  await expect(apiNode).toBeVisible()
  await expect(workerNode).toBeVisible()
  await expect(map.locator('.service-map__edge-target')).toHaveCount(4)
  await page
    .getByRole('button', { name: 'More connections', exact: true })
    .click()
  await expect(
    page.getByRole('button', { name: 'More connections', exact: true })
  ).toHaveCount(0)
  await expect(map.locator('.service-map__edge-target')).toHaveCount(4)
  expect(expansions).toEqual(
    expect.arrayContaining(['api:0', 'worker:0', 'api:2'])
  )
  const rootPosition = await apiNode.boundingBox()
  const workerPosition = (await workerNode.boundingBox())!
  const before = [...expansions]
  await workerNode.click()
  const details = page.getByRole('region', { name: 'Home details' })
  await expect(workerNode).toHaveAttribute('aria-pressed', 'true')
  await expect(details.locator('[data-detail-section]')).toHaveCount(1)
  await expect(details).toContainText('Telemetry')
  expect(expansions).toEqual(before)
  expect(await apiNode.boundingBox()).toEqual(rootPosition)
  const sidebar = page.locator('#signal-drawer-panel')
  await expect(sidebar.locator('.signal-row')).toHaveCount(0)
  await map.getByRole('button', { name: 'More services', exact: true }).click()
  await expect(
    cards.getByRole('button', { name: /^metrics-only\s/ })
  ).toHaveCount(1)
  await expect(workerNode).toHaveAttribute('aria-pressed', 'true')
  await expect(map).toHaveAttribute('aria-busy', 'false')
  const pagedApi = (await apiNode.boundingBox())!
  const pagedWorker = (await workerNode.boundingBox())!
  expect(pagedApi.x - pagedWorker.x).toBeCloseTo(
    rootPosition!.x - workerPosition.x,
    0
  )
  expect(pagedApi.y - pagedWorker.y).toBeCloseTo(
    rootPosition!.y - workerPosition.y,
    0
  )
  await expect(cards.getByRole('button', { name: /^store\s/ })).toHaveCount(1)
  await expect(map.locator('.service-map__edge-target')).toHaveCount(4)
  await expect(map.locator('.service-map__node')).toHaveCount(4)
  expect(
    await map
      .locator('.service-map__edge-target')
      .evaluateAll(elements =>
        elements.map(el => el.getAttribute('aria-label')).join(' ')
      )
  ).not.toContain('metrics-only')
  const connection = map.getByRole('button', { name: /^api → worker:/ })
  await connection.focus()
  await connection.press('Enter')
  await expect(
    details.getByRole('table', { name: 'Connection details' })
  ).toContainText('Parent-child connections')
  await expect(
    details.getByRole('link', { name: /received child/ })
  ).toHaveAttribute('href', /span=2222222222222222/)
  await details.getByRole('button', { name: 'Next', exact: true }).click()
  await expect(
    details.getByRole('link', { name: /next child/ })
  ).toHaveAttribute('href', /span=3333333333333333/)
  await expect(
    details.getByRole('button', { name: 'Next', exact: true })
  ).toBeDisabled()
  await details.getByRole('button', { name: 'Previous', exact: true }).click()
  await expect(
    details.getByRole('link', { name: /received child/ })
  ).toBeVisible()
  expect(evidencePages).toEqual([0, 25, 0])
  await workerNode.click()
  const beforeCentre = {
    api: (await apiNode.boundingBox())!,
    worker: (await workerNode.boundingBox())!,
  }
  const graphReads = [...expansions]
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  const afterCentre = {
    api: (await apiNode.boundingBox())!,
    worker: (await workerNode.boundingBox())!,
  }
  expect(afterCentre.api.x - afterCentre.worker.x).toBeCloseTo(
    beforeCentre.api.x - beforeCentre.worker.x,
    0
  )
  expect(afterCentre.api.y - afterCentre.worker.y).toBeCloseTo(
    beforeCentre.api.y - beforeCentre.worker.y,
    0
  )
  const viewport = (await map.locator('.service-map__canvas').boundingBox())!
  const overview = await map
    .locator('.service-map__node')
    .evaluateAll(nodes => {
      const boxes = nodes.map(node => node.getBoundingClientRect())
      return {
        left: Math.min(...boxes.map(box => box.left)),
        right: Math.max(...boxes.map(box => box.right)),
        top: Math.min(...boxes.map(box => box.top)),
        bottom: Math.max(...boxes.map(box => box.bottom)),
      }
    })
  expect((overview.left + overview.right) / 2).toBeCloseTo(
    viewport.x + viewport.width / 2,
    0
  )
  expect((overview.top + overview.bottom) / 2).toBeCloseTo(
    viewport.y + viewport.height / 2,
    0
  )
  await apiNode.focus()
  await apiNode.press('Enter')
  await expect(apiNode).toHaveAttribute('aria-pressed', 'true')
  const apiAgain = (await apiNode.boundingBox())!
  const workerAgain = (await workerNode.boundingBox())!
  expect(apiAgain.y - workerAgain.y).toBeCloseTo(
    beforeCentre.api.y - beforeCentre.worker.y,
    0
  )
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  expect(await apiNode.boundingBox()).toEqual(afterCentre.api)
  expect(await workerNode.boundingBox()).toEqual(afterCentre.worker)
  await workerNode.focus()
  await workerNode.press('Enter')
  expect(expansions).toEqual(graphReads)
  await expect(workerNode).toHaveAttribute('aria-pressed', 'true')
  await expect(apiNode).toHaveCount(1)
  const normalWidth = (await workerNode.boundingBox())!.width
  await page.getByRole('button', { name: 'Zoom in service map' }).click()
  await expect
    .poll(async () => (await workerNode.boundingBox())!.width)
    .toBeGreaterThan(normalWidth)
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  await expect(workerNode).toBeVisible()
  const centred = (await workerNode.boundingBox())!
  expect(centred).toEqual(afterCentre.worker)
  await workerNode.focus()
  await workerNode.press('ArrowRight')
  await expect
    .poll(async () => (await workerNode.boundingBox())!.x)
    .toBeCloseTo(centred.x + 48, 0)
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  const canvas = (await map.locator('.service-map__canvas').boundingBox())!
  await page.mouse.move(canvas.x + 20, canvas.y + 20)
  await page.mouse.down()
  await page.mouse.move(canvas.x + 80, canvas.y + 50, { steps: 4 })
  await page.mouse.up()
  await expect
    .poll(async () => (await workerNode.boundingBox())!.x)
    .toBeCloseTo(centred.x + 60, 0)
  const beforeScroll = (await workerNode.boundingBox())!
  await page.mouse.move(canvas.x + 20, canvas.y + 20)
  await page.mouse.wheel(40, 60)
  await expect
    .poll(async () => (await workerNode.boundingBox())!.x)
    .toBeCloseTo(beforeScroll.x - 40, 0)
  await expect
    .poll(async () => (await workerNode.boundingBox())!.y)
    .toBeCloseTo(beforeScroll.y - 60, 0)
  expect((await workerNode.boundingBox())!.width).toBeCloseTo(
    beforeScroll.width,
    0
  )
  await page.keyboard.down('Control')
  await page.mouse.wheel(0, -20)
  await page.keyboard.up('Control')
  await expect
    .poll(async () => (await workerNode.boundingBox())!.width)
    .toBeGreaterThan(beforeScroll.width)
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  await connection.focus()
  await connection.press('Enter')
  const connectionDetails = details.getByRole('table', {
    name: 'Connection details',
  })
  await expect(connectionDetails).toBeVisible()
  let refreshedEdges: ServiceConnection[] = [
    { ...edge(api, worker), spanCount: 7, traceCount: 3, errorSpanCount: 2 },
  ]
  await routeTelemetry(page, route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method !== 'discoverTelemetry' || params.options.kind !== 'connections')
      return route.fallback()
    return route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result: { rows: refreshedEdges.map(edge => [edge]), truncated: false },
      },
    })
  })
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(connectionDetails).toBeVisible()
  await expect(connection).toHaveAttribute('aria-pressed', 'true')
  await expect(connection).toHaveAccessibleName(
    'api → worker: 7 parent-child connections in 3 traces'
  )
  await expect(
    connectionDetails.locator(
      '[data-field="Parent-child connections"] .detail-property__value'
    )
  ).toHaveText('7')
  await expect(
    connectionDetails.locator('[data-field="Traces"] .detail-property__value')
  ).toHaveText('3')
  await expect(
    connectionDetails.locator(
      '[data-field="Error child spans"] .detail-property__value'
    )
  ).toHaveText('2')
  refreshedEdges = []
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(connectionDetails).toHaveCount(0)
  await expect(connection).toHaveCount(0)
  await expect(apiNode).toHaveAttribute('aria-pressed', 'true')
  expect(errors).toEqual([])
})

test('force layout fits a fan-out, reflows on resize and bounds pan without drifting on selection', async ({
  page,
}) => {
  const services = Array.from({ length: 19 }, (_, index) =>
    service(`service-${index}`)
  )
  const connections = services.slice(1, 8).map(target => ({
    source: services[0]!,
    target,
    spanCount: 1,
    traceCount: 1,
    errorSpanCount: 0,
  }))
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await routeTelemetry(page, route => {
    const { id, method, params } = route.request().postDataJSON()
    const items =
      method === 'discoverTelemetry'
        ? params.options.kind === 'services'
          ? services
          : params.options.kind === 'connections'
            ? connections
            : []
        : []
    return route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result:
          method === 'discoverTelemetry'
            ? { rows: items.map(item => [item]), truncated: false }
            : [],
      },
    })
  })
  await page.addInitScript(() => {
    localStorage.setItem('signal-drawer:open', 'false')
    localStorage.setItem('signal-detail:open', 'false')
  })
  const measure = () =>
    page.locator('.service-map__canvas').evaluate(canvas => ({
      viewport: canvas.getBoundingClientRect().toJSON(),
      cards: [...canvas.querySelectorAll('.service-map__node')].map(card =>
        card.getBoundingClientRect().toJSON()
      ),
    }))
  await page.setViewportSize({ width: 1100, height: 1200 })
  await page.goto('/?time=all')
  await expect(page.locator('.service-map__node')).toHaveCount(19)
  for (const viewport of [
    { width: 1100, height: 1200 },
    { width: 1600, height: 800 },
  ]) {
    await page.setViewportSize(viewport)
    await page.getByRole('button', { name: 'Reset service map view' }).click()
    await expect
      .poll(async () => {
        const { viewport: bounds, cards } = await measure()
        return cards.every(
          card =>
            card.left >= bounds.left + 31 &&
            card.right <= bounds.right - 31 &&
            card.top >= bounds.top + 31 &&
            card.bottom <= bounds.bottom - 31
        )
      })
      .toBe(true)
    const { cards } = await measure()
    expect(cards[0]!.width / 320).toBeGreaterThan(0.8)
    for (let i = 0; i < cards.length; i++)
      for (let j = i + 1; j < cards.length; j++) {
        const a = cards[i]!,
          b = cards[j]!
        expect(
          a.right <= b.left ||
            b.right <= a.left ||
            a.bottom <= b.top ||
            b.bottom <= a.top
        ).toBe(true)
      }
  }
  const card = page
    .locator('.service-map__node')
    .getByRole('button', { name: /^service-0\s/ })
  // Opening Details changes the viewport and intentionally reflows the graph.
  // Check selection stability with that pane already open.
  const fullWidth = (await measure()).viewport.width
  await page.getByRole('button', { name: 'Open details', exact: true }).focus()
  await page.keyboard.press('Enter')
  await expect
    .poll(async () => (await measure()).viewport.width)
    .toBeLessThan(fullWidth)
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  await page.evaluate(
    () =>
      new Promise<void>(resolve =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
      )
  )
  const positions = (await measure()).cards
  await card.click()
  await expect.poll(async () => (await measure()).cards).toEqual(positions)
  const { viewport } = await measure()
  await page.mouse.move(viewport.left + 10, viewport.top + 10)
  await page.mouse.wheel(100000, 100000)
  await expect
    .poll(async () => {
      const { viewport: bounds, cards } = await measure()
      return (
        Math.max(...cards.map(card => card.right)) >= bounds.left + 63 &&
        Math.max(...cards.map(card => card.bottom)) >= bounds.top + 63
      )
    })
    .toBe(true)
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  expect(errors).toEqual([])
})
