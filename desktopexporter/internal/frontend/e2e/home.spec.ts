import { expect, test } from '@playwright/test'
import { themeRgb } from './theme-colours'

test('Home reflows services, discovers exact values, and keeps incoming imports out of the current reading', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  let serviceReads = 0
  const attributeScopes: Record<string, unknown>[] = []
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
        const foundOn = [
          { signal: options.signal, ownerType: options.ownerType },
        ]
        if (options.kind === 'services') {
          serviceReads++
          return {
            rows: services.map(service => [service]),
            truncated: false,
            totals: { home: services.length, traces: 42, metrics: 8, logs: 36 },
          }
        } else if (options.kind === 'keys') {
          attributeScopes.push(options)
          return {
            rows: [[{ key: 'exact.number', kind: 'int64', foundOn }]],
            truncated: false,
          }
        } else
          return {
            rows: [
              [
                {
                  value: { kind: 'int64', value: '9007199254740993' },
                  foundOn,
                  count: 2,
                  kindCount: 3,
                  denominator: 3,
                  relativeFrequency: 2 / 3,
                },
              ],
              [
                {
                  value: { kind: 'int64', value: '2' },
                  foundOn,
                  count: 1,
                  kindCount: 3,
                  denominator: 3,
                  relativeFrequency: 1 / 3,
                },
              ],
            ],
            truncated: false,
          }
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
  const serviceCards = page.locator('.service-grid .signal-row')
  await expect(serviceCards).toHaveCount(8)
  const homeTab = page.getByRole('link', { name: 'Home', exact: true })
  await expect(homeTab).toHaveAccessibleDescription('Stored services: [8]')
  const card = page
    .locator('.service-grid')
    .getByRole('button', { name: /^checkout/ })
  await expect(card).toHaveAttribute('aria-pressed', 'true')
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
  const colour = page.getByRole('button', {
    name: 'Colour for checkout',
    exact: true,
  })
  const colourDot = colour.locator('span[aria-hidden="true"]')
  const theme = (await page.locator('html').getAttribute('data-theme'))!
  await expect(colour).toHaveCSS('width', '24px')
  await expect(colour).toHaveCSS('height', '24px')
  await expect(colourDot).toHaveCSS('width', '8px')
  await expect(colourDot).toHaveCSS('height', '8px')
  const dotBox = (await colourDot.boundingBox())!
  const titleBox = (await card.locator('.signal-row__title').boundingBox())!
  const colourBox = (await colour.boundingBox())!
  expect(titleBox.x - dotBox.x - dotBox.width).toBeCloseTo(8, 0)
  expect(colourBox.x + colourBox.width).toBeLessThanOrEqual(titleBox.x)
  const labelBox = (await card
    .locator('.service-summary__label > span:last-child')
    .first()
    .boundingBox())!
  const iconBox = (await card
    .locator('.service-summary__icon svg')
    .first()
    .boundingBox())!
  expect(labelBox.x).toBeCloseTo(titleBox.x, 0)
  expect(iconBox.x + iconBox.width / 2).toBeCloseTo(
    dotBox.x + dotBox.width / 2,
    0
  )
  await expect(colour.locator('svg')).toHaveCount(0)
  await expect(colour).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  const automaticColour = await colourDot.evaluate(
    element => getComputedStyle(element).backgroundColor
  )
  await colour.hover()
  await expect(colour).toHaveCSS(
    'background-color',
    themeRgb(theme, 'highlightLow')
  )
  await colour.click()
  const picker = page.getByRole('group', {
    name: 'Colour for checkout options',
    exact: true,
  })
  await expect(picker).toBeVisible()
  await page.mouse.move(0, 0)
  await expect(colour).toHaveCSS(
    'background-color',
    themeRgb(theme, 'highlightMedium')
  )
  await picker
    .locator('[data-ink-choice]:not(:disabled):not([aria-pressed="true"])')
    .first()
    .click()
  await expect(colourDot).not.toHaveCSS('background-color', automaticColour)
  await expect(card).toHaveAttribute('aria-pressed', 'true')
  await colour.click()
  await picker
    .getByRole('button', { name: 'Reset to automatic colour' })
    .click()
  await expect(colourDot).toHaveCSS('background-color', automaticColour)
  await expect(colour).toBeFocused()
  await expect(card.locator('.service-summary__row')).toHaveText([
    /^\s*Traces:\s*42\s*·\s*248\s*spans\s*3\s*err\s*$/,
    /^\s*Metrics:\s*8\s*·\s*640\s*pts\s*·\s*12\s*exemplars\s*$/,
    /^\s*Logs:\s*36\s*2\s*err\s*$/,
  ])
  for (const text of ['248 spans', '640 pts', '12 exemplars']) {
    await expect(card).toContainText(text)
  }
  for (const width of [1600, 2400, 3200]) {
    await page.setViewportSize({ width, height: 1000 })
    await expect(page.locator('.service-grid')).toHaveCSS('display', 'grid')
    const list = (await page.locator('.service-grid').boundingBox())!
    for (const service of await serviceCards.all()) {
      expect((await service.boundingBox())!.width).toBeCloseTo(list.width, 0)
    }
  }
  for (const row of await page.locator('.service-summary__row').all())
    await expect(row).toHaveCSS('border-width', '0px')
  await page.setViewportSize({ width: 1600, height: 1000 })
  const discovery = page.locator('.page-layout__main-chrome')
  const table = discovery.getByRole('grid', {
    name: 'Attributes',
    exact: true,
  })
  const tableNode = await table.elementHandle()
  const countSort = table.getByRole('button', {
    name: 'Sort by records',
    exact: true,
  })
  for (const direction of ['descending', 'ascending']) {
    await countSort.click()
    await expect(countSort.locator('..')).toHaveAttribute(
      'aria-sort',
      direction
    )
    const heading = countSort.locator('.table-column-heading__title')
    await expect(heading).toHaveText('Records')
    const size = await heading.evaluate(element => ({
      client: element.clientWidth,
      scroll: element.scrollWidth,
    }))
    expect(size.scroll).toBeLessThanOrEqual(size.client)
  }
  await table.getByRole('button', { name: 'Sort by name', exact: true }).click()
  const serviceHeader = discovery.getByRole('region', {
    name: 'Attributes',
    exact: true,
  })
  await expect(serviceHeader).toContainText('checkout')
  await expect(serviceHeader).toContainText('storefront')
  await expect(
    serviceHeader.locator(
      '.pane-header__leading > span[style*="background-color"]'
    )
  ).toHaveCSS('background-color', automaticColour)
  const details = page.getByRole('region', { name: 'Home details' })
  await expect(
    details.getByRole('grid', { name: 'Attributes', exact: true })
  ).toHaveCount(0)
  const left = page.locator('#signal-drawer-panel')
  const serviceSection = left.locator('[data-detail-section="services"]')
  const serviceScroll = serviceSection.locator('.service-scroll')
  const servicesBox = (await serviceSection.boundingBox())!
  const attributesBox = (await discovery.boundingBox())!
  const detailsBox = await details.boundingBox()
  const drop = (await left
    .getByRole('button', { name: 'Import files', exact: true })
    .boundingBox())!
  const storage = (await left
    .locator('[data-detail-section="storage"]')
    .boundingBox())!
  expect(drop.y + drop.height).toBeLessThanOrEqual(servicesBox.y)
  expect(servicesBox.y + servicesBox.height).toBeLessThanOrEqual(storage.y)
  expect(servicesBox.x + servicesBox.width).toBeLessThanOrEqual(attributesBox.x)
  await expect(discovery.locator('.service-grid')).toHaveCount(0)
  await expect(serviceScroll).toHaveCSS('overflow-y', 'auto')
  const headerY = (await serviceHeader.boundingBox())!.y
  await serviceScroll.evaluate(element => {
    element.scrollTop = element.scrollHeight
  })
  await expect
    .poll(() => serviceScroll.evaluate(element => element.scrollTop))
    .toBeGreaterThan(0)
  expect((await serviceHeader.boundingBox())!.y).toBeCloseTo(headerY, 0)
  await serviceScroll.evaluate(element => {
    element.scrollTop = 0
  })
  expect(await discovery.boundingBox()).toEqual(attributesBox)
  expect(await details.boundingBox()).toEqual(detailsBox)
  expect(await tableNode!.evaluate(node => node.isConnected)).toBe(true)
  const filter = discovery.getByRole('searchbox', {
    name: 'Filter attribute keys',
  })
  await discovery
    .getByRole('button', { name: 'Filter attribute keys', exact: true })
    .click()
  await expect(filter).toHaveCSS('border-width', '0px')
  await expect(serviceHeader).toHaveCSS('border-width', '0px')
  await filter.focus()
  await expect(filter).toHaveCSS('outline-style', 'solid')
  await expect(filter).toHaveCSS('outline-color', themeRgb(theme, 'primary'))
  const attribute = table.locator(
    'tr[data-attribute-heading="exact.number"][data-attribute-source="resource"]'
  )
  const values = table.locator(
    'tr[data-attribute-key="exact.number"][data-attribute-source="resource"]'
  )
  await expect(attribute).toHaveCount(1)
  expect(attributeScopes.length).toBeGreaterThan(0)
  for (const scope of attributeScopes) {
    expect(scope.serviceName).toBe('checkout')
    expect(scope.namespace).toBe('storefront')
  }
  await expect(values.first()).toBeHidden()
  await expect(attribute.locator('.attribute-frequency-bar')).toHaveCount(0)
  await attribute
    .getByRole('button', { name: 'Expand exact.number · resource values' })
    .click()
  await expect(values.first()).toContainText('33.3%')
  const exactValue = values.filter({ hasText: '9007199254740993' })
  await expect(exactValue).toContainText('66.7%')
  await expect(exactValue.locator('.attribute-frequency-bar')).toBeVisible()
  await expect(
    exactValue.locator('.table-value-cell__inspect')
  ).toHaveAttribute('title', '9007199254740993')
  await expect(
    exactValue.getByTitle(
      '2 / 3 spans · 2 / 3 datapoints · 2 / 3 logs · Of records carrying this attribute'
    )
  ).toBeVisible()
  await table.getByRole('button', { name: 'Sort by name', exact: true }).click()
  await expect(
    table.getByRole('columnheader', { name: 'Sort by name' })
  ).toHaveAttribute('aria-sort', 'descending')
  await expect(values.first()).toContainText('9007199254740993')
  await table.getByRole('button', { name: 'Sort by name', exact: true }).click()
  await expect(
    table.getByRole('columnheader', { name: 'Sort by name' })
  ).toHaveAttribute('aria-sort', 'ascending')
  await expect(values.first()).toContainText('33.3%')
  await table.getByRole('button', { name: 'Sort by percentage' }).click()
  await expect(
    table.getByRole('columnheader', { name: 'Sort by percentage' })
  ).toHaveAttribute('aria-sort', 'descending')
  await table.getByRole('button', { name: 'Sort by percentage' }).click()
  await expect(
    table.getByRole('columnheader', { name: 'Sort by percentage' })
  ).toHaveAttribute('aria-sort', 'ascending')
  await page
    .locator('.service-grid')
    .getByRole('button', { name: /^worker-1(?:\s|·)/ })
    .click()
  await expect(serviceHeader).toContainText('worker-1')
  await expect
    .poll(() => attributeScopes.some(scope => scope.serviceName === 'worker-1'))
    .toBe(true)
  await card.click()
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
