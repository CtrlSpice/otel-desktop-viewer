import { expect, test } from '@playwright/test'
import { themeRgb } from './theme-colours'

const command = `# recorded command\nprintf "%s\\n" "$HOME" | head -1\n./bin/otel-desktop-viewer query --endpoint http://127.0.0.1:8000 --limit 200 "SELECT '${'long value '.repeat(12)}'"`

test('Home keeps Services below file drop in the left pane, Attributes in Main and Telemetry in Details', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1800, height: 1100 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => {
    localStorage.setItem('signal-drawer:open', 'true')
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
                    kindCount: options.key === 'payload' ? 1 : 100,
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
    page
      .locator('.service-grid')
      .getByRole('button', { name: /^shell-service/ })
  ).toBeVisible()
  const filterToggle = page.getByRole('button', {
    name: 'Filter attribute keys',
    exact: true,
  })
  await expect(filterToggle).toHaveAttribute('aria-expanded', 'false')
  await filterToggle.click()
  const filter = page.getByRole('searchbox', { name: 'Filter attribute keys' })
  await expect(filter).toHaveAttribute('placeholder', 'Filter')
  await expect(filter).toBeFocused()
  await filter.press('Escape')
  await expect(filter).toBeHidden()
  await expect(filterToggle).toBeFocused()
  const layout = await page.evaluate(() => {
    const rect = (selector: string) =>
      document.querySelector(selector)!.getBoundingClientRect().toJSON()
    return {
      attributesHeader: rect('.page-layout__main-chrome .pane-header'),
      main: rect('.page-layout__main-chrome'),
      details: rect('#signal-drawer-detail'),
    }
  })
  await expect(page.locator('.home-welcome')).toBeVisible()
  expect(layout.attributesHeader.y).toBeCloseTo(layout.main.y, 0)
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
    page.locator('.page-layout__main-chrome').getByRole('grid', {
      name: 'Attributes',
      exact: true,
    })
  ).toHaveCount(1)
  await expect(
    page.locator('#signal-drawer-detail').getByRole('table')
  ).toHaveCount(0)
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
  const discovery = page.locator('.page-layout__main-chrome')
  const table = discovery.getByRole('grid', {
    name: 'Attributes',
    exact: true,
  })
  await expect(table).toHaveCount(1)
  await expect(table.getByRole('columnheader')).toHaveCount(5)
  await expect(table.getByRole('columnheader').nth(1)).toHaveText(
    'Distinct values'
  )
  await expect(table.getByRole('columnheader').nth(2)).toHaveText('Records')
  const tableHandle = await table.elementHandle()
  const rows = table.locator(
    'tr[data-attribute-key="bash_command"][data-attribute-source="event"]'
  )
  await expect(rows).toHaveCount(4)
  await expect(rows.first()).toBeHidden()
  await expect(rows.last()).toBeHidden()
  await expect(
    table.locator('tr[data-attribute-heading] .attribute-frequency-bar')
  ).toHaveCount(0)
  await table.getByRole('button', { name: 'Sort by percentage' }).click()
  await table
    .getByRole('button', {
      name: 'Expand bash_command · event values',
      exact: true,
    })
    .click()
  await expect(rows.last()).toBeVisible()
  await expect(rows.locator('[data-attribute-column="count"]')).toHaveText([
    '75',
    '75',
    '5',
    '1',
  ])
  await expect(rows.locator('[data-attribute-column="frequency"]')).toHaveText([
    '[75%]',
    '[75%]',
    '[5%]',
    '[1%]',
  ])
  const bars = rows.locator('.attribute-frequency-bar')
  await expect(bars).toHaveCount(4)
  const colors = await bars.evaluateAll(elements =>
    elements.map(element => getComputedStyle(element).backgroundColor)
  )
  for (const [index, bar] of (await bars.all()).entries()) {
    await expect(bar).toHaveCSS('height', '8px')
    const sizes = await bar.evaluate(element => ({
      width: element.getBoundingClientRect().width,
      track: element.parentElement!.getBoundingClientRect().width,
    }))
    expect(sizes.width).toBeCloseTo(
      Math.max(8, sizes.track * [0.75, 0.75, 0.05, 0.01][index]!),
      0
    )
  }
  const value = rows.first().locator('.table-value-cell__inspect')
  expect(await value.textContent()).toContain(command)
  await expect(value).toHaveAttribute('title', command)
  const clipping = await value.evaluate(element => ({
    width: element.clientWidth,
    contentWidth: element.scrollWidth,
    whiteSpace: getComputedStyle(element).whiteSpace,
    overflow: getComputedStyle(element).textOverflow,
  }))
  expect(clipping.contentWidth).toBeGreaterThan(clipping.width)
  expect(clipping.whiteSpace).toBe('nowrap')
  expect(clipping.overflow).toBe('ellipsis')
  expect(clipping.width).toBeLessThanOrEqual(192)
  await expect(
    rows
      .first()
      .locator('td:last-child')
      .getByRole('button', { name: 'Copy bash_command value' })
  ).toHaveClass(/tooltip-left/)
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
  await expect(bars.first()).toBeVisible()
  expect(
    await bars.evaluateAll(elements =>
      elements.map(element => getComputedStyle(element).backgroundColor)
    )
  ).toEqual(colors)
  await page.setViewportSize({ width: 1920, height: 1100 })
  await discovery.getByRole('button', { name: 'Columns', exact: true }).click()
  const picker = page.getByRole('group', {
    name: 'Attribute columns',
    exact: true,
  })
  await expect(
    picker.getByRole('checkbox', { name: 'Name', exact: true })
  ).toBeDisabled()
  await picker
    .getByRole('checkbox', { name: 'All Attribute columns', exact: true })
    .check()
  await picker
    .getByRole('checkbox', { name: 'All Records columns', exact: true })
    .check()
  await picker
    .getByRole('button', { name: 'Pin Records column', exact: true })
    .click()
  await picker
    .getByRole('button', { name: 'Pin Source column', exact: true })
    .click()
  await page.keyboard.press('Escape')
  const order = [
    'name',
    'source',
    'count',
    'type',
    'signals',
    'values',
    'traces',
    'metrics',
    'logs',
    'frequency',
    'copy',
  ]
  expect(
    await table
      .locator('thead th')
      .evaluateAll(headings =>
        headings.map(heading => heading.getAttribute('data-attribute-column'))
      )
  ).toEqual(order)
  await expect(heading.locator('[data-attribute-column="source"]')).toHaveText(
    'event'
  )
  await expect(heading.locator('[data-attribute-column="type"]')).toHaveText(
    'string'
  )
  const theme = (await page.locator('html').getAttribute('data-theme'))!
  await expect(
    heading.locator('[data-attribute-column="type"] > span')
  ).toHaveCSS('color', themeRgb(theme, 'secondary'))
  await expect(
    heading.locator('[data-attribute-column="source"] > span')
  ).toHaveCSS('color', themeRgb(theme, 'secondary'))
  await expect(heading.locator('[data-attribute-column="signals"]')).toHaveText(
    'Traces'
  )
  await expect(heading.locator('[data-attribute-column="values"]')).toHaveText(
    '4'
  )
  await expect(heading.locator('[data-attribute-column="count"]')).toHaveText(
    '100'
  )
  await expect(heading.locator('[data-attribute-column="traces"]')).toHaveText(
    '100'
  )
  await expect(heading.locator('[data-attribute-column="metrics"]')).toHaveText(
    '0'
  )
  const pinnedHeader = table.locator('thead [data-attribute-column="count"]')
  await expect(pinnedHeader).toHaveCSS('position', 'sticky')
  const before = (await pinnedHeader.boundingBox())!
  const scroll = discovery.getByRole('region', {
    name: 'Attributes table viewport',
  })
  await scroll.evaluate(element => {
    element.scrollLeft = 200
  })
  await expect
    .poll(() => scroll.evaluate(element => element.scrollLeft))
    .toBeGreaterThan(0)
  expect((await pinnedHeader.boundingBox())!.x).toBeCloseTo(before.x, 0)
  expect(
    (await heading.locator('[data-attribute-column="count"]').boundingBox())!.x
  ).toBeCloseTo(before.x, 0)
  expect(await tableHandle!.evaluate(element => element.isConnected)).toBe(true)
  await page.reload()
  await expect(table).toBeVisible()
  expect(
    await table
      .locator('thead th')
      .evaluateAll(headings =>
        headings.map(heading => heading.getAttribute('data-attribute-column'))
      )
  ).toEqual(order)
  expect(
    JSON.parse(
      (await page.evaluate(() =>
        localStorage.getItem('home-attribute-pinned-columns')
      )) ?? '[]'
    )
  ).toEqual(['name', 'count', 'source'])
  expect(errors).toEqual([])
})
