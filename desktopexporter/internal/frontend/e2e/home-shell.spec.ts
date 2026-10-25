import { expect, test } from '@playwright/test'

test('Home keeps services in List, the radial map in Main and service inspection in Details', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1800, height: 1100 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => {
    localStorage.setItem('signal-drawer:open', 'true')
    localStorage.setItem('signal-detail:open', 'true')
  })
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    const result = (() => {
      if (method === 'getImportConfig') return { otlpHttpPort: 4318 }
      if (method === 'getStats')
        return {
          storage: { sizeBytes: 4096, maxSizeBytes: 0 },
          traces: {
            traceCount: 2,
            spanCount: 2,
            serviceCount: 1,
            errorCount: 0,
            lastReceived: null,
          },
          logs: { logCount: 0, errorCount: 0, lastReceived: null },
          metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
          rejections: [],
        }
      if (method === 'discoverTelemetry') {
        const items =
          params.options.kind === 'services'
            ? [
                {
                  serviceName: 'shell-service',
                  serviceNamespace: '',
                  traceCount: 2,
                  spanCount: 2,
                  errorSpanCount: 0,
                  logCount: 0,
                  errorLogCount: 0,
                  logSeverityCounts: {},
                  metricCount: 0,
                  dataPointCount: 0,
                  exemplarCount: 0,
                  lastSeen: null,
                },
              ]
            : []
        return { rows: items.map(item => [item]), truncated: false }
      }
      return []
    })()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto('/?time=all')
  await expect(
    page
      .locator('.service-grid')
      .getByRole('button', { name: /^shell-service/ })
  ).toBeVisible()
  const layout = await page.evaluate(() => {
    const rect = (selector: string) =>
      document.querySelector(selector)!.getBoundingClientRect().toJSON()
    return {
      mapHeader: rect('.page-layout__main-chrome .pane-header'),
      main: rect('.page-layout__main-chrome'),
      details: rect('#signal-drawer-detail'),
    }
  })
  await expect(page.locator('.home-welcome')).toBeVisible()
  expect(layout.mapHeader.y).toBeCloseTo(layout.main.y, 0)
  expect(layout.details.y).toBeCloseTo(layout.main.y, 0)
  expect(layout.details.x).toBeGreaterThan(layout.main.x)
  await expect(
    page.locator('.page-layout__main-chrome .service-grid .signal-row')
  ).toHaveCount(0)
  await expect(
    page.locator('#signal-drawer-panel .service-grid .signal-row')
  ).toHaveCount(1)
  await expect(page.locator('#signal-drawer-panel')).toBeVisible()
  await expect(
    page.getByRole('grid', { name: 'Attributes', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByRole('table', { name: 'Service details' })
  ).toContainText('shell-service')
  await expect(page.locator('.page-layout__footer')).toHaveCount(0)
  await expect(
    page
      .locator('.page-layout__main-chrome')
      .getByRole('tab', { name: 'Traces', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByRole('region', { name: 'Home sections' })
  ).toContainText('Storage')
  const imports = page.getByRole('region', {
    name: 'Import telemetry',
    exact: true,
  })
  await expect(imports).toBeVisible()
  await expect(imports.locator('summary')).toHaveCount(0)
  await expect(
    page.getByRole('heading', { name: 'Getting started', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByText('Connect an exporter', { exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByText('Send a test trace', { exact: true })
  ).toHaveCount(0)
  await expect(page.locator('#signal-drawer-panel')).toContainText(
    'Ingestion issues'
  )
  await expect(
    page
      .locator('.home-welcome')
      .getByRole('button', { name: 'Import files', includeHidden: true })
  ).toBeAttached()
  await expect(
    page.getByRole('region', { name: 'Service map', exact: true })
  ).toContainText('shell-service')
  expect(errors).toEqual([])
})
