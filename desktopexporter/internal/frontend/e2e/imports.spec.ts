import { routeTelemetry } from './telemetry-fixtures'
import { expect, test } from './test-fixtures'
import AxeBuilder from '@axe-core/playwright'

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
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    const result =
      request.method === 'getStats'
        ? stats
        : request.method === 'getImportConfig'
          ? { otlpHttpPort: 54318 }
          : request.method === 'discoverTelemetry'
            ? { rows: [], truncated: false }
            : request.method === 'searchLogPage'
              ? { total: 0, logs: [] }
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
  await page.getByLabel('Choose telemetry JSON files').setInputFiles([
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
  await page
    .locator('#signal-drawer-panel summary')
    .filter({ hasText: 'Ingestion issues' })
    .click()
  const issues = page.getByRole('region', { name: 'Ingestion issues' })
  const files = issues.getByRole('button', {
    name: /^File import issues 2 files Last seen /,
  })
  await expect(files).toHaveAttribute('aria-expanded', 'false')
  await files.click()
  await expect(
    issues.getByRole('list', { name: 'File import issues' })
  ).toBeVisible()
  await expect(issues).toContainText('Profiles support coming soon')
  await expect(issues).toContainText('broken.json')
  await expect(issues).toContainText('2 files')
  await expect(issues).not.toContainText('later.json')
})

test('a dropped file uses the current signal page error display and links to Home issues', async ({
  page,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem('signal-drawer:open', 'false')
  )
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
  await expect(issues).toBeVisible()
  await expect(page.locator('#signal-drawer-panel')).not.toHaveAttribute(
    'inert',
    ''
  )
  await expect(issues).toContainText('Invalid OTLP log body')
  await issues
    .getByRole('button', { name: /^File import issues 1 file Last seen / })
    .click()
  await expect(
    issues.getByRole('list', { name: 'File import issues' })
  ).toBeVisible()
  await expect(page).toHaveURL(/\/#ingestion-issues$/)
})

for (const theme of ['madoromi-aki-hiru', 'madoromi-aki-yoru']) {
  test(`issue cards disclose independently with separate units in ${theme}`, async ({
    page,
  }) => {
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await routeTelemetry(page, async route => {
      const request = route.request().postDataJSON() as {
        id: number
        method: string
      }
      if (request.method !== 'getStats') return route.fallback()
      const timestamp = '1792692000000000001'
      await route.fulfill({
        json: {
          jsonrpc: '2.0',
          id: request.id,
          result: {
            ...stats,
            rejections: [
              {
                signal: 'traces',
                kind: 'span_already_stored',
                occurrences: 2,
                samples: [{ traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }],
                firstSeen: timestamp,
                lastSeen: timestamp,
              },
              {
                signal: 'traces',
                kind: 'span_refused',
                occurrences: 1,
                samples: [],
                firstSeen: timestamp,
                lastSeen: timestamp,
              },
              {
                signal: 'logs',
                kind: 'receiver_recursion_depth',
                occurrences: 3,
                samples: [],
                firstSeen: timestamp,
                lastSeen: timestamp,
              },
              {
                signal: 'metrics',
                kind: 'receiver_recursion_depth',
                occurrences: 1,
                samples: [],
                firstSeen: timestamp,
                lastSeen: timestamp,
              },
            ],
          },
        },
      })
    })
    await page.goto('/')
    await page.getByLabel('Choose telemetry JSON files').setInputFiles({
      name: 'broken.json',
      mimeType: 'application/json',
      buffer: Buffer.from('{'),
    })
    await page
      .locator('#signal-drawer-panel summary')
      .filter({ hasText: 'Ingestion issues' })
      .click()
    const issues = page.getByRole('region', { name: 'Ingestion issues' })
    const files = issues.getByRole('button', {
      name: /^File import issues 1 file Last seen /,
    })
    const records = issues.getByRole('button', {
      name: /^duplicate span id 2 records Last seen /,
    })
    const storeRejections = issues.getByRole('button', {
      name: /^rejected by the store 1 record Last seen /,
    })
    const requests = issues.getByRole('button', {
      name: /^receiver protobuf nesting limit 4 requests Last seen /,
    })
    await expect(issues.getByRole('button')).toHaveCount(4)
    for (const card of [files, records, storeRejections, requests]) {
      await expect(card).toBeVisible()
      await expect(card).toHaveAttribute('aria-expanded', 'false')
      await expect(card.getByText(/^Last seen /)).toHaveCount(1)
    }
    await expect(
      issues.getByRole('list', { name: 'File import issues' })
    ).toBeHidden()
    await expect(
      issues.getByRole('table', { name: 'duplicate span id' })
    ).toBeHidden()
    await expect(issues.getByRole('link', { name: 'Learn more' })).toBeHidden()

    await requests.focus()
    await page.keyboard.press('Enter')
    await expect(requests).toHaveAttribute('aria-expanded', 'true')
    await expect(files).toHaveAttribute('aria-expanded', 'false')
    await expect(records).toHaveAttribute('aria-expanded', 'false')
    const requestDetails = issues.getByRole('table', {
      name: 'receiver protobuf nesting limit',
    })
    await expect(requestDetails).toBeVisible()
    await expect(requestDetails).toContainText('logs, metrics')
    await expect(requestDetails.getByRole('link')).toHaveCount(0)
    await expect(
      issues.getByText(
        'The receiver rejected these requests because their data is nested too deeply.'
      )
    ).toBeVisible()
    await expect(
      issues.getByRole('link', { name: 'Learn more' })
    ).toHaveAttribute(
      'href',
      'https://github.com/CtrlSpice/otel-desktop-viewer/blob/main/docs/sdk-configuration.md#deeply-nested-telemetry'
    )
    await expect(
      issues.getByRole('link', { name: 'Learn more' })
    ).toHaveAttribute('target', '_blank')
    await expect(
      issues.getByRole('link', { name: 'Learn more' })
    ).toHaveAttribute('rel', 'noopener noreferrer')
    await page.keyboard.press('Space')
    await expect(requests).toHaveAttribute('aria-expanded', 'false')
    await expect(requestDetails).toBeHidden()
    await expect(requests).toBeFocused()

    for (const card of [files, records, storeRejections, requests])
      await card.click()
    await expect(issues.getByText(/^Last seen /)).toHaveCount(4)
    await expect(
      issues.getByRole('list', { name: 'File import issues' })
    ).toContainText('broken.json')
    await expect(
      issues.getByRole('link', { name: 'bbbbbbbbbbbbbbbb' })
    ).toHaveAttribute(
      'href',
      `/traces/${'a'.repeat(32)}?span=${'b'.repeat(16)}`
    )
    // Scan the expanded content without a sticky section header covering a link.
    await page.setViewportSize({ width: 1280, height: 1800 })
    await page
      .locator('#signal-drawer-panel summary')
      .filter({ hasText: 'Ingestion issues' })
      .press('Home')
    await files.scrollIntoViewIfNeeded()
    await expect(
      issues.getByRole('link', { name: 'bbbbbbbbbbbbbbbb' })
    ).toBeInViewport({ ratio: 1 })
    await issues.evaluate(async element => {
      await Promise.all(
        element.getAnimations().map(animation => animation.finished)
      )
    })
    const scan = await new AxeBuilder({ page })
      .include('#ingestion-issues')
      .withTags([
        'wcag2a',
        'wcag2aa',
        'wcag21a',
        'wcag21aa',
        'wcag22a',
        'wcag22aa',
      ])
      .analyze()
    expect(scan.violations).toEqual([])
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(files).toBeVisible()
    await expect(requests).toBeVisible()
    expect(
      await issues.evaluate(
        element => element.scrollWidth <= element.clientWidth
      )
    ).toBe(true)
  })
}
