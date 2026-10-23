import { expect, test } from '@playwright/test'

const command = `# recorded command\nprintf "%s\\n" "$HOME" | head -1\n./bin/otel-desktop-viewer query --endpoint http://127.0.0.1:8000 --limit 200 "SELECT '${'long value '.repeat(12)}'"`

test('Home keeps services, attributes, and storage in their workspaces with exact attribute values', async ({
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
              ? [
                  {
                    key: 'bash_command',
                    kind: 'string',
                    foundOn: [{ signal: 'traces', ownerType: 'event' }],
                  },
                ]
              : options.kind === 'values'
                ? [command, 'echo done', 'echo three', 'echo four'].map(
                    (text, index) => ({
                      identity: `${index}`,
                      value: { kind: 'string', value: text },
                      foundOn: [{ signal: 'traces', ownerType: 'event' }],
                      count: [75, 75, 5, 1][index],
                      denominator: 100,
                      relativeFrequency: [0.75, 0.75, 0.05, 0.01][index],
                    })
                  )
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
      list: rect('#signal-drawer-panel'),
      servicesHeader: rect('#signal-drawer-panel .pane-header'),
      main: rect('.page-layout__main-chrome'),
      details: rect('#signal-drawer-detail'),
    }
  })
  await expect(page.locator('.getting-started')).toBeHidden()
  expect(layout.main.x).toBeGreaterThanOrEqual(layout.list.right)
  expect(layout.servicesHeader.y).toBeCloseTo(layout.main.y, 0)
  expect(layout.details.y).toBeCloseTo(layout.main.y, 0)
  expect(layout.details.x).toBeGreaterThan(layout.main.x)
  await expect(
    page.locator('#signal-drawer-panel').getByRole('article')
  ).toHaveCount(1)
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
  const table = page.getByRole('table', { name: 'Attributes', exact: true })
  await expect(table).toHaveCount(1)
  await expect(table.getByRole('columnheader')).toHaveCount(4)
  const tableHandle = await table.elementHandle()
  const rows = table.locator(
    'tr[data-attribute-key="bash_command"][data-attribute-source="event"]'
  )
  await expect(rows).toHaveCount(4)
  await expect(rows.first()).toBeHidden()
  await expect(rows.last()).toBeHidden()
  await expect(table.getByText('Overlapping values')).toBeVisible()
  await table
    .getByRole('button', {
      name: 'Expand bash_command · event values',
      exact: true,
    })
    .click()
  await expect(rows.last()).toBeVisible()
  await expect(rows.locator('td:nth-child(3)')).toHaveText([
    '[75%]',
    '[75%]',
    '[5%]',
    '[1%]',
  ])
  const markerSizes = await rows
    .locator('.breakdown-segment__fill')
    .evaluateAll(elements =>
      elements.slice(0, 3).map(element => {
        const cell = element.closest('td')!
        const style = getComputedStyle(cell)
        return {
          width: element.getBoundingClientRect().width,
          height: element.getBoundingClientRect().height,
          available:
            cell.getBoundingClientRect().width -
            Number.parseFloat(style.paddingLeft) -
            Number.parseFloat(style.paddingRight),
        }
      })
    )
  for (const [index, marker] of markerSizes.entries()) {
    expect(marker.height).toBe(8)
    expect(marker.width).toBeCloseTo(
      Math.max(8, marker.available * [0.75, 0.75, 0.05][index]),
      1
    )
  }
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
  const marker = await rows
    .first()
    .locator('.breakdown-segment__fill')
    .boundingBox()
  expect(valueText!.x).toBeCloseTo(headingText!.x, 0)
  expect(valueText!.y).toBeGreaterThan(headingText!.y + headingText!.height)
  expect(marker!.x).toBeGreaterThan(valueText!.x + valueText!.width)
  const dot = await rows
    .last()
    .locator('.breakdown-segment__fill')
    .boundingBox()
  expect(dot!.width).toBe(8)
  expect(dot!.height).toBe(8)
  const collapse = table.getByRole('button', {
    name: 'Collapse bash_command · event values',
    exact: true,
  })
  await collapse.focus()
  await collapse.press('Enter')
  await expect(rows.nth(2)).toBeHidden()
  await expect(rows.last()).toBeHidden()
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
  expect(errors).toEqual([])
})
