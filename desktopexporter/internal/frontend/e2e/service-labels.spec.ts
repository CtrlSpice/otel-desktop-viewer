import { expect, test } from '@playwright/test'
import { routeTelemetry } from './telemetry-fixtures'

test('bounded service labels keep exact references and load complete text only for inspection or Copy', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const name = 'shared-' + 'long value '.repeat(1000) + '-tail'
  const ref = 'a'.repeat(64)
  const services = [0, 1].map(i => ({
    serviceName: 'shared-long value…',
    serviceNamespace: '',
    serviceIdentity: i ? 'b'.repeat(64) : ref,
    serviceColorKey: `sha256:${String(i).repeat(64)}`,
    labelsComplete: false,
    serviceNameKind: 'string',
    serviceNamespaceKind: 'string',
    traceCount: 0,
    spanCount: 0,
    errorSpanCount: 0,
    logCount: 1,
    errorLogCount: 0,
    logSeverityCounts: {
      trace: 0,
      debug: 0,
      info: 1,
      warn: 0,
      error: 0,
      fatal: 0,
      other: 0,
    },
    metricCount: 0,
    dataPointCount: 0,
    exemplarCount: 0,
    lastSeen: null,
  }))
  const requests: string[] = []
  await page.route('**/api/features', r => r.fulfill({ json: {} }))
  await routeTelemetry(page, route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'getServiceLabels') {
      requests.push(params.serviceRef)
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: { serviceName: name, serviceNamespace: '' },
        },
      })
    }
    const result =
      method === 'getServiceCount'
        ? 2
        : method === 'discoverTelemetry'
          ? {
              rows:
                params.options.kind === 'services'
                  ? services.map(s => [s])
                  : [],
              truncated: false,
            }
          : method === 'searchLogPage'
            ? { total: 0, logs: [] }
            : []
    return route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto('/?time=all')
  await page
    .locator(`[data-service-identity="${ref}"] button.signal-row`)
    .click()
  await page
    .getByRole('button', { name: 'Service labels', exact: true })
    .click()
  const labels = page.getByRole('table', { name: 'Service labels' })
  await expect(labels).toBeVisible()
  expect(requests).toEqual([])
  await labels
    .getByRole('button', { name: 'Copy service.name', exact: true })
    .press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(name)
  expect(requests).toEqual([ref])
  await page.getByRole('button', { name: 'Show full labels' }).click()
  await expect(labels.locator('[data-field="service.name"]')).toContainText(
    '-tail'
  )
  expect(requests).toEqual([ref, ref])
})
