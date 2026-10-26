import { expect, test } from '@playwright/test'
import type {
  ServiceSummary,
  ServiceConnection,
  ServiceConnectionSpan,
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
  await page.route('**/rpc', async route => {
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
  await page.route('**/rpc', route => {
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
  await expect(connectionDetails).toHaveCount(0)
  await expect(apiNode).toHaveAttribute('aria-pressed', 'true')
  await expect(connection).toHaveAccessibleName(
    'api → worker: 7 parent-child connections in 3 traces'
  )
  await connection.focus()
  await connection.press('Enter')
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
  await page.route('**/rpc', route => {
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
