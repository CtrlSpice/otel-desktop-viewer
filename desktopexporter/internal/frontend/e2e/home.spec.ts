import { expect, test } from '@playwright/test'

test('Home reflows services, discovers exact values, and keeps incoming imports out of the current reading', async ({
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
        const foundOn = [
          { signal: options.signal, ownerType: options.ownerType },
        ]
        if (options.kind === 'services') {
          serviceReads++
          return { rows: services.map(service => [service]), truncated: false }
        } else if (options.kind === 'keys')
          return {
            rows: [[{ key: 'exact.number', kind: 'int64', foundOn }]],
            truncated: false,
          }
        else
          return {
            rows: [
              [
                {
                  value: { kind: 'int64', value: '9007199254740993' },
                  foundOn,
                  count: 2,
                  denominator: 3,
                  relativeFrequency: 2 / 3,
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
  })
  await page.setViewportSize({ width: 1600, height: 1000 })
  await page.goto('/?time=all')
  await expect(page.getByRole('article')).toHaveCount(8)
  const card = page.getByRole('article').first()
  await expect(
    card.getByRole('link', { name: 'Open checkout traces' })
  ).toContainText('42')
  await expect(
    card.getByRole('link', { name: 'Open checkout logs' })
  ).toContainText('36')
  await expect(
    card.getByRole('link', { name: 'Open checkout metrics' })
  ).toContainText('8')
  for (const text of ['248 spans', '640 pts', '12 exemplars']) {
    await expect(card).toContainText(text)
  }
  const columns: number[] = []
  for (const width of [1600, 2400, 3200]) {
    await page.setViewportSize({ width, height: 1000 })
    await expect
      .poll(() =>
        page
          .locator('.service-grid')
          .evaluate(
            element =>
              getComputedStyle(element).gridTemplateColumns.split(' ').length
          )
      )
      .toBeGreaterThan(columns.at(-1) ?? 0)
    columns.push(
      await page
        .locator('.service-grid')
        .evaluate(
          element =>
            getComputedStyle(element).gridTemplateColumns.split(' ').length
        )
    )
  }
  for (const row of await page.locator('.service-summary__row').all())
    await expect(row).toHaveCSS('border-width', '0px')
  const attribute = page.getByRole('region', {
    name: 'exact.number · resource',
    exact: true,
  })
  const values = attribute.getByRole('group', {
    name: 'Values of exact.number',
    exact: true,
  })
  await expect(values).toContainText('9007199254740993')
  await expect(values).toContainText('66.7%')
  await expect(values.getByTitle('2 / 3 records')).toBeVisible()
  const imported = page.waitForResponse('http://127.0.0.1:54318/v1/logs')
  await page
    .getByLabel('Choose telemetry or evaluation JSON / JSONL files')
    .setInputFiles({
      name: 'logs.json',
      mimeType: 'application/json',
      buffer: Buffer.from('{"resourceLogs":[{}]}'),
    })
  await imported
  await expect(page).toHaveURL(/\/\?time=all$/)
  expect(serviceReads).toBe(1)
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect.poll(() => serviceReads).toBe(2)
  const destination = new URL(
    (await page
      .getByRole('link', { name: 'Open checkout logs' })
      .getAttribute('href')) ?? '',
    page.url()
  )
  expect(destination.pathname).toBe('/logs')
  expect(destination.searchParams.get('q')).toContain(
    '"service.namespace", string) = "storefront"'
  )
  await page.getByRole('link', { name: 'Open checkout logs' }).click()
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
