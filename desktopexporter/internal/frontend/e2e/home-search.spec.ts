import { expect, test } from '@playwright/test'

test('Home counts, attributes and enabled links share the submitted cross-signal query', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const service = {
    serviceName: 'checkout',
    serviceNamespace: '7',
    serviceNameKind: 'string',
    serviceNamespaceKind: 'string',
    serviceIdentity: JSON.stringify([
      { kind: 'string', value: 'checkout' },
      { kind: 'string', value: '7' },
    ]),
    traceCount: 2,
    spanCount: 3,
    errorSpanCount: 0,
    logCount: 4,
    errorLogCount: 0,
    logSeverityCounts: {},
    metricCount: 1,
    dataPointCount: 8,
    exemplarCount: 0,
    lastSeen: null,
  }
  const discovery: Record<string, unknown>[] = []
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    const result = (() => {
      if (method === 'getStats')
        return {
          traces: {
            traceCount: 9,
            spanCount: 12,
            serviceCount: 2,
            errorCount: 0,
            lastReceived: null,
          },
          logs: { logCount: 10, errorCount: 0, lastReceived: null },
          metrics: { metricCount: 5, dataPointCount: 20, lastReceived: null },
          rejections: [],
        }
      if (method === 'getImportConfig') return { otlpHttpPort: 4318 }
      if (method === 'searchLogPage') return { total: 0, logs: [] }
      if (method === 'discoverTelemetry') {
        const options = params.options
        discovery.push(options)
        if (options.kind === 'services') {
          const value = options.queries?.logs?.query?.value
          const items =
            value === 'missing'
              ? []
              : value
                ? [
                    {
                      ...service,
                      traceCount: 0,
                      spanCount: 0,
                      metricCount: 0,
                      dataPointCount: 0,
                      logCount: 1,
                    },
                  ]
                : [
                    service,
                    {
                      ...service,
                      serviceName: 'worker',
                      serviceIdentity: JSON.stringify([
                        { kind: 'string', value: 'worker' },
                        { kind: 'string', value: '7' },
                      ]),
                    },
                    {
                      ...service,
                      serviceNamespaceKind: 'int64',
                      serviceIdentity: JSON.stringify([
                        { kind: 'string', value: 'checkout' },
                        { kind: 'int64', value: '7' },
                      ]),
                    },
                  ]
          return {
            rows: items.map(item => [item]),
            truncated: false,
            totals: {
              home: items.length,
              traces: value ? 0 : 3,
              metrics: value ? 0 : 2,
              logs: value === 'missing' ? 0 : value ? 1 : 8,
            },
          }
        } else return { rows: [], truncated: false }
      }
      return []
    })()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto('/?time=all')
  await expect(page.getByRole('article')).toHaveCount(3)
  await expect(
    page.getByRole('button', {
      name: 'Inspect checkout · 7 · namespace int64',
      exact: true,
    })
  ).toBeVisible()
  const input = page.getByRole('searchbox', { name: 'Search all telemetry' })
  await input.fill('declined')
  await input.press('Enter')
  await expect(page.getByRole('article')).toHaveCount(1)
  await expect(
    page.getByRole('link', { name: 'Logs', exact: true })
  ).toHaveAccessibleDescription('Matching logs: [1]')
  const traces = page
    .getByRole('navigation', { name: 'Primary' })
    .getByLabel('Traces', { exact: true })
  await expect(traces).toHaveAttribute('aria-disabled', 'true')
  await expect(traces).not.toHaveAttribute('href')
  await expect(page.getByLabel('Open checkout traces')).not.toHaveAttribute(
    'href'
  )
  await input.fill('an unsubmitted draft')
  await page.getByRole('button', { name: /^Change time range,/ }).click()
  await page.getByRole('button', { name: 'Last 5 min', exact: true }).click()
  await expect(page).toHaveURL(/start=/)
  await page.keyboard.press('Escape')
  await expect(input).toHaveValue('an unsubmitted draft')
  expect(new URL(page.url()).searchParams.get('q')).toBe('declined')
  await expect
    .poll(() =>
      discovery.some(
        options =>
          options.kind === 'keys' &&
          JSON.stringify(options.queries).includes('declined')
      )
    )
    .toBe(true)
  const link = page.getByRole('link', { name: 'Open checkout logs' })
  const target = new URL((await link.getAttribute('href'))!, page.url())
  expect(target.searchParams.get('q')).toContain('search("declined")')
  expect(target.searchParams.get('q')).toContain(
    `serviceIdentity = ${JSON.stringify(service.serviceIdentity)}`
  )
  await link.click()
  await expect(page).toHaveURL(/\/logs\?q=/)
  await expect(
    page.getByRole('link', { name: 'Logs', exact: true })
  ).toHaveAccessibleDescription('Stored logs: [10]')
  await page.goBack()
  expect(new URL(page.url()).searchParams.get('q')).toBe('declined')
  await expect(page.getByRole('article')).toHaveCount(1)
  await input.fill('body !~ "healthcheck"')
  await input.press('Enter')
  await page.getByRole('link', { name: 'Open checkout logs' }).click()
  expect(new URL(page.url()).searchParams.get('q')).toContain(
    'body !~ "healthcheck"'
  )
  await expect(page.getByRole('alert')).toHaveCount(0)
  // Workspace navigation carries time, but each signal keeps its own query.
  const logsQuery = new URL(page.url()).searchParams.get('q')
  await page.getByRole('link', { name: 'Metrics', exact: true }).click()
  expect(new URL(page.url()).searchParams.get('q')).toBeNull()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await page.getByRole('link', { name: 'Logs', exact: true }).click()
  expect(new URL(page.url()).searchParams.get('q')).toBe(logsQuery)
  await page.goBack()
  await page.goBack()
  await page.goBack()
  await input.fill('missing')
  await input.press('Enter')
  await expect(page.getByRole('article')).toHaveCount(0)
  await expect(
    page.getByRole('link', { name: 'Home', exact: true })
  ).toHaveAccessibleDescription('Matching services: [0]')
  await expect(
    page.getByText('Select a service to discover its attributes.')
  ).toBeVisible()
  expect(errors).toEqual([])
})
