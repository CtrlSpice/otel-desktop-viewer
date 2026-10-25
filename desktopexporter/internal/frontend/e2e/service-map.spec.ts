import { expect, test } from '@playwright/test'
import type {
  ServiceSummary,
  ServiceConnection,
  ServiceConnectionSpan,
} from '../src/services/discovery-types'

test('radial service map inspects without recentering and expands real directed connections', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1800, height: 1100 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
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
  const api = service('api'),
    worker = service('worker'),
    store = service('store')
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
          items = offset === 0 ? [api, worker] : [store]
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
  const apiNode = map.getByRole('button', { name: 'Inspect api', exact: true })
  const workerNode = map.getByRole('button', {
    name: 'Inspect worker',
    exact: true,
  })
  await expect(apiNode).toBeVisible()
  await expect(workerNode).toBeVisible()
  await expect(map.locator('.service-map__edge-target')).toHaveCount(2)
  await page
    .getByRole('button', { name: 'More connections', exact: true })
    .click()
  await expect(map.locator('.service-map__edge-target')).toHaveCount(3)
  expect(expansions).toEqual(['api:0', 'api:2'])
  const rootPosition = await apiNode.boundingBox()
  const before = [...expansions]
  await workerNode.click()
  const details = page.getByRole('region', { name: 'Home details' })
  await expect(
    details.getByRole('table', { name: 'Service details' })
  ).toContainText('worker')
  expect(expansions).toEqual(before)
  expect(await apiNode.boundingBox()).toEqual(rootPosition)
  const sidebar = page.locator('#signal-drawer-panel')
  await sidebar.getByRole('button', { name: 'Next', exact: true }).click()
  await expect(sidebar.locator('.service-grid')).toContainText('store')
  await expect(
    details.getByRole('table', { name: 'Service details' })
  ).toContainText('worker')
  await sidebar.getByRole('button', { name: 'Previous', exact: true }).click()
  await expect(sidebar.locator('.service-grid')).toContainText('api')
  expect(expansions).toEqual(before)
  await expect(apiNode).toHaveClass(/service-map__node--root/)
  await page
    .getByRole('button', { name: 'Expand connections', exact: true })
    .click()
  await expect(
    map.getByRole('button', { name: 'Inspect store', exact: true })
  ).toBeVisible()
  await expect(map.locator('.service-map__edge-target')).toHaveCount(4)
  await expect(map.locator('.service-map__node')).toHaveCount(3)
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
  await sidebar
    .locator('.service-grid')
    .getByRole('button', { name: /^worker/ })
    .click()
  await expect(workerNode).toHaveClass(/service-map__node--root/)
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
  expect(afterCentre.worker.x + afterCentre.worker.width / 2).toBeCloseTo(
    viewport.x + viewport.width / 2,
    0
  )
  expect(afterCentre.worker.y + afterCentre.worker.height / 2).toBeCloseTo(
    viewport.y + viewport.height / 2,
    0
  )
  await sidebar
    .locator('.service-grid')
    .getByRole('button', { name: /^api/ })
    .click()
  await expect(apiNode).toHaveClass(/service-map__node--root/)
  const apiAgain = (await apiNode.boundingBox())!
  const workerAgain = (await workerNode.boundingBox())!
  expect(apiAgain.y - workerAgain.y).toBeCloseTo(
    beforeCentre.api.y - beforeCentre.worker.y,
    0
  )
  await sidebar
    .locator('.service-grid')
    .getByRole('button', { name: /^worker/ })
    .click()
  await expect(workerNode).toHaveClass(/service-map__node--root/)
  expect(expansions).toEqual(graphReads)
  await expect(
    page.locator('.service-grid').getByRole('button', { name: /^worker/ })
  ).toHaveAttribute('aria-pressed', 'true')
  await expect(
    map.getByRole('button', { name: 'Inspect api', exact: true })
  ).toHaveCount(1)
  const normalWidth = (await workerNode.boundingBox())!.width
  await page.getByRole('button', { name: 'Zoom in service map' }).click()
  await expect
    .poll(async () => (await workerNode.boundingBox())!.width)
    .toBeGreaterThan(normalWidth)
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  await expect(workerNode).toBeVisible()
  const centred = (await workerNode.boundingBox())!
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
  expect(errors).toEqual([])
})
