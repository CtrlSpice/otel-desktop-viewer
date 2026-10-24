import { expect, test } from '@playwright/test'

const command = `# recorded command\nprintf "%s\\n" "$HOME" | head -1\n./bin/otel-desktop-viewer query --endpoint http://127.0.0.1:8000 --limit 200 "SELECT '${'long value '.repeat(12)}'"`

test('Home keeps service cards in Main and collapsible attributes in Details without bars', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1800, height: 1100 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => {
    localStorage.setItem('signal-detail:open', 'true')
    Object.assign(window, { copiedValue: '' })
    Object.defineProperty(navigator, 'clipboard', {
      value: {
        writeText: async (text: string) => {
          Object.assign(window, { copiedValue: text })
        },
      },
    })
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
        const options = params.options
        const values =
          options.key === 'payload'
            ? ['{"resourceSpans":[{"name":"exact"}]}']
            : options.key === 'many_values'
              ? Array.from({ length: 100 }, (_, index) => `value-${index}`)
              : [command, 'echo done', 'echo three', 'echo four']
        const items =
          options.kind === 'services'
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
            : options.kind === 'keys' && options.ownerType === 'event'
              ? ['bash_command', 'payload', 'many_values'].map(key => ({
                  key,
                  kind: 'string',
                  foundOn: [{ signal: 'traces', ownerType: 'event' }],
                }))
              : options.kind === 'values'
                ? values.map((text, index) => ({
                    identity: `${index}`,
                    value: { kind: 'string', value: text },
                    foundOn: [{ signal: 'traces', ownerType: 'event' }],
                    count:
                      options.key === 'bash_command'
                        ? [75, 75, 5, 1][index]
                        : 1,
                    denominator: options.key === 'payload' ? 1 : 100,
                    relativeFrequency:
                      options.key === 'bash_command'
                        ? [0.75, 0.75, 0.05, 0.01][index]
                        : options.key === 'payload'
                          ? 1
                          : 0.01,
                  }))
                : []
        return { rows: items.map(item => [item]), truncated: false }
      }
      return []
    })()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto('/?time=all')
  await expect(
    page.getByRole('button', { name: 'Inspect shell-service' })
  ).toBeVisible()
  await expect(
    page.getByRole('searchbox', { name: 'Filter attribute keys' })
  ).toHaveAttribute('placeholder', 'Filter')
  const layout = await page.evaluate(() => {
    const rect = (selector: string) =>
      document.querySelector(selector)!.getBoundingClientRect().toJSON()
    return {
      servicesHeader: rect('.page-layout__main-chrome .pane-header'),
      main: rect('.page-layout__main-chrome'),
      details: rect('#signal-drawer-detail'),
    }
  })
  await expect(page.locator('.getting-started')).toBeHidden()
  expect(layout.servicesHeader.y).toBeCloseTo(layout.main.y, 0)
  expect(layout.details.y).toBeCloseTo(layout.main.y, 0)
  expect(layout.details.x).toBeGreaterThan(layout.main.x)
  await expect(
    page.locator('.page-layout__main-chrome').getByRole('article')
  ).toHaveCount(1)
  await expect(page.locator('#signal-drawer-panel')).toBeHidden()
  await expect(
    page.locator('.page-layout__main-chrome').getByRole('table')
  ).toHaveCount(0)
  await expect(page.locator('.page-layout__footer')).toHaveCount(0)
  await expect(
    page
      .locator('.page-layout__main-chrome')
      .getByRole('tab', { name: 'Traces', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByRole('region', { name: 'Home details' })
  ).toContainText('Storage')
  await expect(
    page.getByRole('region', { name: 'Home details' })
  ).toContainText('Ingestion issues')
  await expect(
    page
      .locator('.getting-started')
      .getByRole('button', { name: 'Import files', includeHidden: true })
  ).toBeAttached()
  const details = page.getByRole('region', { name: 'Home details' })
  const table = details.getByRole('table', { name: 'Attributes', exact: true })
  await expect(table).toHaveCount(1)
  await expect(table.getByRole('columnheader')).toHaveCount(4)
  await expect(table.getByRole('columnheader').nth(1)).toHaveText('Cnt')
  const tableHandle = await table.elementHandle()
  const rows = table.locator(
    'tr[data-attribute-key="bash_command"][data-attribute-source="event"]'
  )
  await expect(rows).toHaveCount(4)
  await expect(rows.first()).toBeHidden()
  await expect(rows.last()).toBeHidden()
  await expect(details.locator('.breakdown-segment')).toHaveCount(0)
  await table
    .getByRole('button', {
      name: 'Expand bash_command · event values',
      exact: true,
    })
    .click()
  await expect(rows.last()).toBeVisible()
  await expect(rows.locator('td:nth-child(2)')).toHaveText([
    '75',
    '75',
    '5',
    '1',
  ])
  await expect(rows.locator('td:nth-child(3)')).toHaveText([
    '[75%]',
    '[75%]',
    '[5%]',
    '[1%]',
  ])
  const value = rows.first().locator('.table-value-cell__inspect')
  expect(await value.textContent()).toContain(command)
  const clipping = await value.evaluate(element => ({
    width: element.clientWidth,
    contentWidth: element.scrollWidth,
    whiteSpace: getComputedStyle(element).whiteSpace,
    overflow: getComputedStyle(element).textOverflow,
  }))
  expect(clipping.contentWidth).toBeGreaterThan(clipping.width)
  expect(clipping.whiteSpace).toBe('nowrap')
  expect(clipping.overflow).toBe('ellipsis')
  const heading = table.locator(
    'tr[data-attribute-heading="bash_command"][data-attribute-source="event"]'
  )
  const headingText = await heading
    .locator('span[title="bash_command · string · event"]')
    .boundingBox()
  const valueText = await value.boundingBox()
  expect(valueText!.x).toBeCloseTo(headingText!.x, 0)
  expect(valueText!.y).toBeGreaterThan(headingText!.y + headingText!.height)
  await table
    .getByRole('button', { name: 'Expand payload · event values', exact: true })
    .click()
  const payload = table.locator('tr[data-attribute-key="payload"]')
  await expect(payload).toBeVisible()
  const collapse = table.getByRole('button', {
    name: 'Collapse bash_command · event values',
    exact: true,
  })
  await collapse.focus()
  await collapse.press('Enter')
  await expect(rows.nth(2)).toBeHidden()
  await expect(rows.last()).toBeHidden()
  await expect(payload).toBeVisible()
  expect(await tableHandle!.evaluate(element => element.isConnected)).toBe(true)
  const sortArrow = table.getByRole('button', { name: 'Sort by percentage' })
  const gap = await sortArrow.evaluate(element => {
    const label = element
      .querySelector('.table-column-heading')!
      .getBoundingClientRect()
    const arrow = element
      .querySelector('.table-sort__direction')!
      .getBoundingClientRect()
    return arrow.left - label.right
  })
  expect(gap).toBeGreaterThanOrEqual(0)
  expect(gap).toBeLessThanOrEqual(8)
  await table
    .getByRole('button', {
      name: 'Expand bash_command · event values',
      exact: true,
    })
    .click()
  await table
    .getByRole('button', { name: 'Copy bash_command value' })
    .first()
    .click()
  expect(
    await page.evaluate(
      () => (window as Window & { copiedValue?: string }).copiedValue
    )
  ).toBe(command)
  await page.setViewportSize({ width: 3200, height: 1100 })
  await expect(details.locator('.breakdown-segment')).toHaveCount(0)
  expect(errors).toEqual([])
})
