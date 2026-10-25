import { expect, test } from '@playwright/test'

test('Home service nodes retain card controls and keep incoming imports out of the current reading', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  let serviceReads = 0
  const services = Array.from({ length: 8 }, (_, index) => ({
    serviceName: index === 0 ? 'checkout' : `worker-${index}`,
    serviceNamespace: 'storefront',
    traceCount: 42,
    spanCount: 248,
    errorSpanCount: 3,
    logCount: 36,
    errorLogCount: 2,
    logSeverityCounts: {
      trace: 0,
      debug: 0,
      info: 30,
      warn: 4,
      error: 1,
      fatal: 1,
      other: 0,
    },
    metricCount: 8,
    dataPointCount: 640,
    exemplarCount: 12,
    lastSeen: '1792692000000000000',
  }))
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    const result = (() => {
      if (method === 'searchLogPage') return { total: 0, logs: [] }
      if (method === 'getImportConfig') return { otlpHttpPort: 54318 }
      if (method === 'getStats')
        return {
          storage: { sizeBytes: 4096, maxSizeBytes: 0 },
          traces: {
            traceCount: 8,
            spanCount: 248,
            errorCount: 3,
            serviceCount: 8,
            lastReceived: null,
          },
          logs: { logCount: 36, errorCount: 2, lastReceived: null },
          metrics: { metricCount: 8, dataPointCount: 640, lastReceived: null },
          rejections: [],
        }
      if (method === 'discoverTelemetry') {
        const options = params.options
        if (options.kind === 'connections')
          return { rows: [], truncated: false }
        if (options.kind === 'services') {
          serviceReads++
          return {
            rows: services.map(service => [service]),
            truncated: false,
            totals: { home: services.length, traces: 42, metrics: 8, logs: 36 },
          }
        }
        return { rows: [], truncated: false }
      }
      return []
    })()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.route('http://127.0.0.1:54318/v1/logs', route =>
    route.fulfill({ json: {}, headers: { 'Access-Control-Allow-Origin': '*' } })
  )
  await page.addInitScript(() => {
    localStorage.setItem('signal-drawer:open', 'true')
    localStorage.setItem('signal-detail:open', 'true')
    localStorage.setItem('signal-drawer-width', '40')
  })
  await page.setViewportSize({ width: 1600, height: 1000 })
  await page.goto('/?time=all')
  const serviceCards = page.locator('.service-map__node .signal-row')
  await expect(serviceCards).toHaveCount(8)
  const homeTab = page.getByRole('link', { name: 'Home', exact: true })
  await expect(homeTab).toHaveAccessibleDescription('Stored services: [8]')
  const card = page
    .locator('.service-map__node')
    .getByRole('button', { name: /^checkout/ })
  await expect(card).toHaveAttribute('aria-pressed', 'true')
  await page.getByRole('button', { name: 'Reset service map view' }).click()
  await expect(card.locator('.signal-row__title')).toHaveCSS(
    'font-size',
    '17px'
  )
  await expect(card.locator('.signal-row__title')).toHaveCSS(
    'font-weight',
    '400'
  )
  await expect(card.locator('.signal-row__title')).toHaveCSS(
    'font-family',
    /Atkinson Hyperlegible Next/
  )
  await expect(card.getByRole('link')).toHaveCount(0)
  await expect(card.getByRole('button')).toHaveCount(0)
  await expect(page.getByRole('button', { name: /^Colour for/ })).toHaveCount(0)
  const colourDot = card.locator('.signal-row__title span[aria-hidden="true"]')
  await expect(colourDot).toHaveCSS('width', '8px')
  await expect(colourDot).toHaveCSS('height', '8px')
  const dotBox = (await colourDot.boundingBox())!
  const titleBox = (await card
    .locator('.signal-row__title > span > span:last-child')
    .boundingBox())!
  expect(titleBox.x - dotBox.x - dotBox.width).toBeCloseTo(8, 0)
  const dateBox = (await card.locator('.signal-date').boundingBox())!
  expect(dateBox.x).toBeCloseTo(titleBox.x, 0)
  await expect(card.locator('svg')).toHaveCount(0)
  const cardBox = (await card.boundingBox())!
  const countsBox = (await card.locator('.signal-row__badge').boundingBox())!
  expect(countsBox.x).toBeGreaterThanOrEqual(titleBox.x + titleBox.width)
  expect(countsBox.x + countsBox.width).toBeCloseTo(
    cardBox.x + cardBox.width - 16,
    0
  )
  expect(countsBox.y + countsBox.height / 2).toBeCloseTo(
    titleBox.y + titleBox.height / 2,
    0
  )
  const automaticColour = await colourDot.evaluate(
    element => getComputedStyle(element).backgroundColor
  )
  await page.getByRole('button', { name: /^Season:/ }).click()
  await page.getByRole('radio', { name: 'Winter' }).click()
  await page.keyboard.press('Escape')
  await expect(card).toHaveAttribute('aria-pressed', 'true')
  await expect(colourDot).toHaveCSS('background-color', automaticColour)
  await expect(card.locator('.signal-row__lead')).toHaveText(
    '42 traces · 8 metrics · 36 logs'
  )
  await expect(card.locator('.signal-row__badge')).toHaveText('[5 err]')
  await expect(
    card.getByLabel('3 error spans · 2 error or fatal logs')
  ).toBeVisible()
  await expect(card.locator('.signal-row__meta-row')).toHaveCount(0)
  for (const width of [1600, 2400, 3200]) {
    await page.setViewportSize({ width, height: 1000 })
    for (const service of await serviceCards.all()) {
      expect((await service.boundingBox())!.width).toBeCloseTo(320, 0)
    }
  }
  await page.setViewportSize({ width: 1600, height: 1000 })
  const serviceHeader = page.getByRole('region', {
    name: 'Service map',
    exact: true,
  })
  await expect(serviceHeader).toContainText('checkout')
  await expect(card).toContainText('storefront')
  const details = page.getByRole('region', { name: 'Home details' })
  // Wait for the default 28rem Details width after restoring the viewport.
  await expect(details).toHaveCSS('width', '448px')
  await expect(
    details.getByRole('grid', { name: 'Attributes', exact: true })
  ).toHaveCount(0)
  const left = page.locator('#signal-drawer-panel')
  const guide = left.locator('[data-detail-section="guide"]')
  const guideScroll = guide.locator('.guide-scroll')
  const guideBox = (await guide.boundingBox())!
  const detailsBox = (await details.boundingBox())!
  const drop = (await left
    .getByRole('button', { name: 'Import files', exact: true })
    .boundingBox())!
  const storage = (await left
    .locator('[data-detail-section="storage"]')
    .boundingBox())!
  const connectBox = (await guideScroll
    .locator('section')
    .first()
    .boundingBox())!
  expect(connectBox.y).toBeCloseTo(drop.y + drop.height + 24, 0)
  const terminal = guide.getByRole('tabpanel', { name: 'HTTP' }).locator('..')
  const terminalBox = (await terminal.boundingBox())!
  expect(drop.x).toBeCloseTo(terminalBox.x, 0)
  expect(drop.width).toBeCloseTo(terminalBox.width, 0)
  await expect(
    left.getByRole('button', { name: 'Import files', exact: true })
  ).toHaveCSS(
    'border-color',
    await terminal.evaluate(element => getComputedStyle(element).borderColor)
  )
  expect(drop.height).toBe(96)
  expect(drop.y + drop.height).toBeLessThanOrEqual(guideBox.y + guideBox.height)
  expect(guideBox.y + guideBox.height).toBeLessThanOrEqual(storage.y)
  expect(guideBox.x + guideBox.width).toBeLessThanOrEqual(detailsBox.x)
  await expect(left.locator('.signal-row')).toHaveCount(0)
  await expect(guideScroll).toHaveCSS('overflow-y', 'auto')
  await expect(guide).toContainText('http://localhost:54318')
  const headerY = (await serviceHeader.boundingBox())!.y
  await guideScroll.evaluate(element => {
    element.scrollTop = element.scrollHeight
  })
  await expect
    .poll(() =>
      guideScroll.evaluate(
        element =>
          element.scrollTop === element.scrollHeight - element.clientHeight
      )
    )
    .toBe(true)
  expect((await serviceHeader.boundingBox())!.y).toBeCloseTo(headerY, 0)
  await expect(
    left.getByRole('button', { name: 'Import files', exact: true })
  ).toBeVisible()
  await guideScroll.evaluate(element => {
    element.scrollTop = 0
  })
  expect(await details.boundingBox()).toEqual(detailsBox)
  await expect(serviceHeader).toHaveCSS('border-width', '0px')
  const worker = page
    .locator('.service-map__node')
    .getByRole('button', { name: /^worker-1(?:\s|·)/ })
  await worker.focus()
  await worker.press('Enter')
  await expect(serviceHeader).toContainText('worker-1')
  await expect(worker).toHaveAttribute('aria-pressed', 'true')
  await card.focus()
  await card.press('Enter')
  await expect(serviceHeader).toContainText('checkout')
  const imported = page.waitForResponse('http://127.0.0.1:54318/v1/logs')
  services.push({ ...services[0]!, serviceName: 'new-worker' })
  await page.getByLabel('Choose telemetry JSON files').setInputFiles({
    name: 'logs.json',
    mimeType: 'application/json',
    buffer: Buffer.from('{"resourceLogs":[{}]}'),
  })
  await imported
  await expect(page).toHaveURL(/\/\?time=all$/)
  expect(serviceReads).toBe(2) // Home's page and the database-wide tab total.
  await expect(homeTab).toHaveAccessibleDescription('Stored services: [8]')
  await expect(serviceCards).toHaveCount(8)
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect.poll(() => serviceReads).toBe(4)
  await expect(homeTab).toHaveAccessibleDescription('Stored services: [9]')
  await expect(serviceCards).toHaveCount(9)
  // Workspace navigation carries the submitted Home query. Service cards
  // select Details; individual telemetry cards navigate to exact records.
  const search = page.getByRole('searchbox', { name: 'Search telemetry' })
  await search.fill(
    'attr(resource, "service.namespace", string) = "storefront"'
  )
  await search.press('Enter')
  const logs = page
    .getByRole('navigation', { name: 'Primary' })
    .getByRole('link', { name: 'Logs', exact: true })
  await expect(logs).toHaveAttribute('href', /service.namespace/)
  const destination = new URL((await logs.getAttribute('href'))!, page.url())
  expect(destination.pathname).toBe('/logs')
  expect(destination.searchParams.get('q')).toContain(
    '"service.namespace", string) = "storefront"'
  )
  await logs.click()
  await expect(page.locator('.cm-content[contenteditable="true"]')).toHaveText(
    destination.searchParams.get('q')!
  )
  await page.getByRole('button', { name: 'Clear search', exact: true }).click()
  await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBeNull()
  await page.reload()
  await expect(
    page.locator('.cm-content[contenteditable="true"] .cm-placeholder')
  ).toBeVisible()
  expect(errors).toEqual([])
})
