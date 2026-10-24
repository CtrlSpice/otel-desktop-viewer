import { expect, test } from '@playwright/test'

test('public frontend cannot activate Evals from a server capability', async ({
  page,
}) => {
  test.skip(process.env.EVALS === '1', 'Public build boundary')
  const evalRequests: string[] = []
  page.on('request', request => {
    if (new URL(request.url()).pathname.startsWith('/api/evals'))
      evalRequests.push(request.url())
  })
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: true } } })
  )
  await page.route('**/rpc', route => {
    const { id, method } = route.request().postDataJSON()
    return route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result:
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
                metrics: {
                  metricCount: 0,
                  dataPointCount: 0,
                  lastReceived: null,
                },
                rejections: [],
              }
            : method === 'discoverTelemetry'
              ? { rows: [], truncated: false }
              : [],
      },
    })
  })
  await page.goto('/evals')
  await expect(
    page.getByRole('link', { name: 'Experimental Evals' })
  ).toHaveCount(0)
  await expect(page.getByLabel('Choose telemetry JSON files')).toBeAttached()
  await page.getByLabel('Choose telemetry JSON files').setInputFiles({
    name: 'eval.json',
    mimeType: 'application/json',
    buffer: Buffer.from(
      '{"evalId":"example","config":{},"results":{"version":3,"results":[]}}'
    ),
  })
  await page
    .locator('#signal-drawer-panel summary')
    .filter({ hasText: 'Ingestion issues' })
    .click()
  await expect(
    page.getByRole('region', { name: 'Ingestion issues' })
  ).toContainText('eval.json')
  expect(evalRequests).toEqual([])
})
