import { expect, test } from '@playwright/test'

const stats = {
  storage: { sizeBytes: 0, maxSizeBytes: 0 },
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

test.beforeEach(async ({ page }) => {
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    const result =
      request.method === 'getStats'
        ? stats
        : request.method === 'getImportConfig'
          ? { otlpHttpPort: 54318 }
          : []
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ jsonrpc: '2.0', id: request.id, result }),
    })
  })
})

test('picker sends supported signal groups and lists profiles/invalid files without blocking later files', async ({
  page,
}) => {
  const requests: { path: string; body: string }[] = []
  await page.route('http://127.0.0.1:54318/v1/**', async route => {
    requests.push({
      path: new URL(route.request().url()).pathname,
      body: route.request().postData() ?? '',
    })
    await route.fulfill({
      contentType: 'application/json',
      body: '{}',
      headers: { 'Access-Control-Allow-Origin': '*' },
    })
  })
  await page.goto('/')
  const resource =
    '{"scopeLogs":[{"logRecords":[{"body":{"doubleValue":-0},"timeUnixNano":"18446744073709551615"}]}]}'
  await page.getByLabel('Choose OTLP JSON files').setInputFiles([
    {
      name: 'mixed.json',
      mimeType: 'application/json',
      buffer: Buffer.from(
        `{"resourceSpans":[{}],"resourceLogs":[${resource}],"resourceMetrics":[{}],"resourceProfiles":[]}`
      ),
    },
    {
      name: 'broken.json',
      mimeType: 'application/json',
      buffer: Buffer.from('{"resourceLogs":['),
    },
    {
      name: 'later.json',
      mimeType: 'application/json',
      buffer: Buffer.from('{"resourceLogs":[{}]}'),
    },
  ])
  await expect.poll(() => requests.length).toBe(4)
  expect(requests.map(request => request.path)).toEqual([
    '/v1/traces',
    '/v1/logs',
    '/v1/metrics',
    '/v1/logs',
  ])
  expect(requests[1].body).toBe(`{"resourceLogs":[${resource}]}`)
  const issues = page.getByRole('region', { name: 'Ingestion issues' })
  await expect(issues).toContainText('Profiles support coming soon')
  await expect(issues).toContainText('broken.json')
  await expect(issues).toContainText('2 files')
  await expect(issues).not.toContainText('later.json')
})

test('a dropped file uses the current signal page error display and links to Home issues', async ({
  page,
}) => {
  await page.route('http://127.0.0.1:54318/v1/logs', route =>
    route.fulfill({
      status: 400,
      contentType: 'application/json',
      body: '{"message":"Invalid OTLP log body"}',
      headers: { 'Access-Control-Allow-Origin': '*' },
    })
  )
  await page.goto('/logs')
  await page.evaluate(() => {
    const files = new DataTransfer()
    files.items.add(
      new File(['{"resourceLogs":[{}]}'], 'dropped.json', {
        type: 'application/json',
      })
    )
    window.dispatchEvent(new DragEvent('drop', { dataTransfer: files }))
  })
  const main = page.getByRole('main')
  await expect(main).toContainText('Import issue in dropped.json.')
  await main.getByRole('link', { name: 'View issue' }).click()
  const issues = page.getByRole('region', { name: 'Ingestion issues' })
  await expect(issues).toBeFocused()
  await expect(issues).toContainText('Invalid OTLP log body')
  await expect(page).toHaveURL(/\/#ingestion-issues$/)
})
