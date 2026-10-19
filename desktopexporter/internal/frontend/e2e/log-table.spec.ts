import { expect, test } from '@playwright/test'
import type { JsonLogData } from '../src/types/wire-types'
import type { LogErrorNavigation } from '../src/types/api-types'

const firstID = '00000000-0000-0000-0000-000000000001'
const secondID = '00000000-0000-0000-0000-000000000002'
const logs: JsonLogData[] = [firstID, secondID].map((logRef, index) => ({
  logRef,
  timestamp: index === 0 ? '0' : '1700000000123456789',
  observedTimestamp: '1700000000987654321',
  traceID: null,
  spanID: null,
  severityText: index === 0 ? 'ERROR' : 'INFO',
  severityNumber: index === 0 ? 17 : 9,
  body: { kind: 'string', value: `Full body ${index}: exact retained text` },
  resource: {
    attributes: [
      {
        key: 'service.name',
        value: { kind: 'string', value: index === 0 ? 'api' : 'worker' },
      },
    ],
    droppedAttributesCount: 0,
  },
  scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
  attributes: [
    {
      key: 'service.name',
      value: { kind: 'string', value: 'log-owned value' },
    },
  ],
  droppedAttributesCount: 0,
  flags: 0,
  eventName: '',
}))

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'true')
  )
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    const result =
      method === 'getLog'
        ? logs.find(log => log.logRef === params.logRef)
        : method === 'searchLogPage'
          ? {
              total: logs.length,
              logs: logs.map((log, index) => ({
                logRef: log.logRef,
                timestamp:
                  log.timestamp === '0' ? log.observedTimestamp : log.timestamp,
                severityText: log.severityText,
                severityNumber: log.severityNumber,
                serviceName: index === 0 ? 'api' : 'worker',
                bodyPreview: `Full body ${index}…`,
                bodyType: 'string',
                eventName: index === 0 ? 'payment.declined' : '',
              })),
            }
          : method === 'getLogErrorNavigation'
            ? {
                count: 1,
                index: params.selectedLogRef === firstID ? 1 : null,
                previous:
                  params.selectedLogRef === secondID
                    ? { logRef: firstID, offset: 0 }
                    : null,
                next: params.selectedLogRef
                  ? null
                  : { logRef: firstID, offset: 0 },
              }
            : method === 'getStats'
              ? {
                  storage: { sizeBytes: 0, maxSizeBytes: 0 },
                  traces: {
                    traceCount: 0,
                    spanCount: 0,
                    serviceCount: 0,
                    errorCount: 0,
                    lastReceived: null,
                  },
                  logs: { logCount: 2, errorCount: 1, lastReceived: null },
                  metrics: {
                    metricCount: 0,
                    dataPointCount: 0,
                    lastReceived: null,
                  },
                  rejections: [],
                }
              : []
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto(`/logs/${firstID}?time=all`)
  await expect(
    page.getByRole('grid', { name: 'Logs', exact: true })
  ).toBeVisible()
})

test('table cells reveal exact fields in separate Details, with observed-time fallback and keyboard selection', async ({
  page,
}) => {
  const first = page.locator(`[data-log-ref="${firstID}"]`)
  await expect(page.getByRole('columnheader').first()).toContainText('severity')
  await expect(page.getByRole('columnheader').nth(1)).toContainText('timestamp')
  await expect(page.getByRole('columnheader').last()).toContainText('body')
  const severityHeading = page.getByRole('columnheader').first()
  await expect(severityHeading.locator('.badge')).toHaveCount(0)
  await expect(
    page.getByRole('columnheader').nth(1).locator('.badge')
  ).toHaveCount(0)
  await expect(
    page.getByRole('columnheader').last().locator('.badge')
  ).toHaveCount(0)
  const severityCell = first.getByRole('gridcell').first()
  await expect(severityCell.getByRole('button')).toHaveCSS('text-align', 'left')
  const bodyCell = first.getByRole('button', {
    name: 'Show log: body in details',
  })
  await expect(bodyCell.locator('.badge')).toHaveCount(0)
  await expect(severityCell.locator('.badge')).toHaveCount(0)
  await expect(severityCell.locator('.number-circle')).toHaveText('17')
  await expect(severityCell.locator('.log-severity__label')).toHaveText('ERROR')
  await expect(page.getByRole('columnheader').nth(1)).toHaveCSS(
    'padding-left',
    '16px'
  )
  await expect(first.locator('.table-value-cell').nth(1)).toHaveCSS(
    'padding-left',
    '16px'
  )
  await expect(
    page.getByRole('region', { name: 'Log table summary', exact: true })
  ).toContainText('2 records')
  await expect(
    page.getByRole('region', { name: 'Log table summary', exact: true })
  ).toContainText('Logs')
  await expect(
    page.getByRole('region', { name: 'Log table summary', exact: true })
  ).not.toContainText('on this page')
  const timestampCell = first.getByRole('button', {
    name: 'Show log: timestamp in details',
  })
  const fullTimestamp = (await timestampCell.getAttribute('title'))!
  const zone = fullTimestamp.slice(fullTimestamp.lastIndexOf(' ') + 1)
  await expect(
    page.getByRole('columnheader').filter({ hasText: 'timestamp' })
  ).toContainText(`timestamp (${zone})`)
  await expect(timestampCell).toHaveText(
    fullTimestamp.slice(0, fullTimestamp.lastIndexOf(' '))
  )
  await expect(first.getByRole('gridcell').last()).toHaveCSS(
    'border-right-width',
    '0px'
  )
  const detail = page.getByRole('table', { name: 'Log fields' })
  await severityCell.getByRole('button').click()
  await expect(detail.locator('[data-field="severityNumber"]')).toHaveClass(
    /detail-property--revealed/
  )
  await first
    .getByRole('button', { name: 'Show log: timestamp in details' })
    .click()
  await expect(
    detail.locator('[data-field="observedTimeUnixNano"]')
  ).toHaveClass(/detail-property--revealed/)
  await expect(
    detail.locator('[data-field="observedTimeUnixNano"]')
  ).toContainText('1700000000987654321')
  await first
    .getByRole('button', { name: 'Show resource: service.name in details' })
    .click()
  await expect(
    detail.locator('[data-field="service.name"][data-field-source="resource"]')
  ).toHaveClass(/detail-property--revealed/)
  await expect(
    detail.locator('[data-field="service.name"]:not([data-field-source])')
  ).not.toHaveClass(/detail-property--revealed/)
  await first.getByRole('button', { name: 'Show log: body in details' }).click()
  await expect(detail.locator('[data-field="body"]')).toContainText(
    'exact retained text'
  )
  await expect(first.getByRole('button', { name: /Copy/ })).toHaveCount(0)
  await first.focus()
  await page.keyboard.press('ArrowDown')
  const second = page.locator(`[data-log-ref="${secondID}"]`)
  await expect(second).toHaveAttribute('aria-selected', 'true')
  await expect(second).toBeFocused()
  await expect(detail.locator('[data-field="body"]')).toContainText(
    'Full body 1: exact retained text'
  )
})

test('external provenance is inline, shortens before crowding the title and expands again', async ({
  page,
}) => {
  const heading = page
    .getByRole('columnheader')
    .filter({ hasText: 'service.name' })
  const badge = heading.getByRole('img', { name: 'Source: Resource' })
  const title = heading.locator('.table-column-heading__title')
  await expect(badge).toHaveText('R')
  await expect(badge).toHaveAttribute('title', 'Resource')
  const titleBounds = (await title.boundingBox())!
  const badgeBounds = (await badge.boundingBox())!
  expect(badgeBounds.x).toBeGreaterThanOrEqual(
    titleBounds.x + titleBounds.width
  )
  expect(badgeBounds.y + badgeBounds.height / 2).toBeCloseTo(
    titleBounds.y + titleBounds.height / 2,
    0
  )
  const guide = page.getByRole('separator', {
    name: 'Resize service.name column',
    exact: true,
  })
  await guide.focus()
  await page.keyboard.press('Shift+ArrowRight')
  await expect(badge).toHaveText('Resource')
  await page.keyboard.press('Shift+ArrowLeft')
  await expect(badge).toHaveText('R')
  await expect(title).toHaveAttribute('title', 'service.name')
  await expect(page.getByRole('columnheader').first()).toHaveCSS(
    'height',
    '40px'
  )
})

test('structured previews have static syntax colours and Details folds Body and agent attributes', async ({
  page,
}) => {
  const structured: JsonLogData = {
    ...logs[0]!,
    body: {
      kind: 'map',
      value: [
        { key: 'message', value: { kind: 'string', value: 'ready' } },
        {
          key: 'request',
          value: {
            kind: 'map',
            value: [
              {
                key: 'sequence',
                value: { kind: 'int64', value: '9223372036854775807' },
              },
            ],
          },
        },
      ],
    },
    attributes: [
      {
        key: 'gen_ai.input.messages',
        value: {
          kind: 'array',
          value: [
            {
              kind: 'map',
              value: [
                { key: 'role', value: { kind: 'string', value: 'user' } },
                {
                  key: 'content',
                  value: { kind: 'string', value: 'Explain the trace' },
                },
              ],
            },
          ],
        },
      },
      {
        key: 'gen_ai.output.messages',
        value: {
          kind: 'string',
          value: '[{"role":"assistant","tokens":9007199254740993}]',
        },
      },
    ],
  }
  const preview = '{message: "ready", request: object (1 fields)}'
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'getLog' && params.logRef === firstID) {
      return route.fulfill({ json: { jsonrpc: '2.0', id, result: structured } })
    }
    if (method === 'searchLogPage') {
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: {
            total: 1,
            logs: [
              {
                logRef: firstID,
                timestamp: structured.observedTimestamp,
                severityText: 'ERROR',
                severityNumber: 17,
                serviceName: 'api',
                bodyPreview: preview,
                bodyType: 'map',
              },
            ],
          },
        },
      })
    }
    return route.fallback()
  })
  await page.reload()
  const row = page.locator(`[data-log-ref="${firstID}"]`)
  const body = row.locator('.log-table__pin-body')
  await expect(body).toHaveText(preview)
  await expect(body.getByRole('button')).toHaveCount(1)
  await expect(body.locator('.badge')).toHaveCount(0)
  await expect(body.locator('.detail-code--key').first()).toHaveText('message')
  const keyColour = await body
    .locator('.detail-code--key')
    .first()
    .evaluate(element => getComputedStyle(element).color)
  const stringColour = await body
    .locator('.detail-code--string')
    .evaluate(element => getComputedStyle(element).color)
  expect(keyColour).not.toBe(stringColour)
  const service = row.getByRole('gridcell').nth(2)
  await expect(service).toHaveClass(/log-table__cell--attribute/)
  const serviceColour = await service
    .getByRole('button')
    .evaluate(element => getComputedStyle(element).color)
  const timestampColour = await row
    .getByRole('gridcell')
    .nth(1)
    .getByRole('button')
    .evaluate(element => getComputedStyle(element).color)
  expect(serviceColour).not.toBe(timestampColour)

  const detailBody = page.locator('[data-field="body"]')
  await expect(detailBody).toContainText('9223372036854775807')
  await detailBody
    .getByRole('button', { name: 'Collapse body.request', exact: true })
    .click()
  await expect(detailBody).not.toContainText('9223372036854775807')
  await expect(detailBody).toContainText('… 1 field')
  await detailBody
    .getByRole('button', { name: 'Expand body.request', exact: true })
    .focus()
  await page.keyboard.press('Enter')
  await expect(detailBody).toContainText('9223372036854775807')

  const input = page.locator('[data-field="gen_ai.input.messages"]')
  await input.scrollIntoViewIfNeeded()
  await input
    .getByRole('button', {
      name: 'Collapse gen_ai.input.messages[0]',
      exact: true,
    })
    .click()
  await expect(input).not.toContainText('Explain the trace')
  await input
    .getByRole('button', {
      name: 'Expand gen_ai.input.messages[0]',
      exact: true,
    })
    .click()
  await expect(input).toContainText('Explain the trace')
  const output = page.locator('[data-field="gen_ai.output.messages"]')
  await output.scrollIntoViewIfNeeded()
  await expect(output).toContainText('9007199254740993')
  await output
    .getByRole('button', {
      name: 'Collapse gen_ai.output.messages',
      exact: true,
    })
    .click()
  await expect(output).not.toContainText('9007199254740993')
  await expect(output).toContainText('[… 1 item]')
  await expect(row).toHaveAttribute('aria-selected', 'true')
})

test('Severity header sorts by the circle number rather than the received label', async ({
  page,
}) => {
  const records = [
    { logRef: firstID, severityNumber: 2, severityText: 'ZZZ' },
    { logRef: secondID, severityNumber: 10, severityText: 'AAA' },
    {
      logRef: '00000000-0000-0000-0000-000000000003',
      severityNumber: 17,
      severityText: 'MMM',
    },
  ].map(record => ({
    ...record,
    timestamp: '1700000000123456789',
    serviceName: 'api',
    bodyPreview: 'Numeric severity sort',
    bodyType: 'string',
  }))
  const sorts: { field: string; direction: string }[] = []
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method !== 'searchLogPage') return route.fallback()
    sorts.push(params.sort)
    const ordered = [...records].sort(
      (a, b) =>
        (params.sort.direction === 'asc' ? 1 : -1) *
        (a.severityNumber - b.severityNumber)
    )
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result: { total: ordered.length, logs: ordered },
      },
    })
  })
  await page.reload()
  const circles = page.locator('[data-log-ref] .number-circle')
  await expect(circles).toHaveCount(3)
  const heading = page.getByRole('columnheader').first()
  await expect(heading).toHaveAttribute('aria-sort', 'descending')
  await expect(circles).toHaveText(['17', '10', '2'])
  expect(sorts.at(-1)).toEqual({ field: 'severity', direction: 'desc' })
  await heading
    .getByRole('button', { name: 'Sort by severity', exact: true })
    .click()
  await expect(heading).toHaveAttribute('aria-sort', 'ascending')
  await expect(circles).toHaveText(['2', '10', '17'])
  expect(sorts.at(-1)).toEqual({ field: 'severity', direction: 'asc' })
  await heading
    .getByRole('button', { name: 'Sort by severity', exact: true })
    .click()
  await expect(heading).toHaveAttribute('aria-sort', 'descending')
  await expect(circles).toHaveText(['17', '10', '2'])
})

test('Severity fits the longest received label without adding a resize handle', async ({
  page,
}) => {
  const longest = 'REMOTE_PROVIDER_TIMEOUT'
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    if (method !== 'searchLogPage') return route.fallback()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result: {
          total: 2,
          logs: [
            {
              logRef: firstID,
              timestamp: '1700000000123456789',
              severityNumber: 17,
              severityText: longest,
              serviceName: 'api',
              bodyPreview: 'Custom severity',
              bodyType: 'string',
            },
            {
              logRef: secondID,
              timestamp: '1700000000123456789',
              severityNumber: 0,
              severityText: 'UNSPECIFIED',
              serviceName: 'api',
              bodyPreview: 'Unspecified severity',
              bodyType: 'string',
            },
          ],
        },
      },
    })
  })
  await page.reload()
  const labels = page.locator('[data-log-ref] .log-severity__label')
  await expect(labels).toHaveText([longest, 'UNSPECIFIED'])
  await expect
    .poll(async () =>
      labels.evaluateAll(elements =>
        elements.every(element => element.scrollWidth <= element.clientWidth)
      )
    )
    .toBe(true)
  const severity = page.locator(
    `[data-log-ref="${firstID}"] .log-table__pin-severity`
  )
  expect((await severity.boundingBox())!.width).toBeGreaterThan(160)
  await expect(
    page.getByRole('separator', { name: 'Resize severity column', exact: true })
  ).toHaveCount(0)
  const before = (await severity.boundingBox())!.width
  await page.setViewportSize({ width: 1700, height: 1000 })
  await expect
    .poll(async () => (await severity.boundingBox())!.width)
    .toBe(before)
  await expect(labels.first()).toHaveText(longest)
})

test('toolbar filters edit the visible query, respect drafts and clear from query edits', async ({
  page,
}) => {
  const editor = page.locator('.cm-content')
  const service = page.getByRole('checkbox', {
    name: 'Service api',
    exact: true,
  })
  const error = page.getByRole('checkbox', {
    name: 'Severity Error',
    exact: true,
  })
  const filtersToggle = page.getByRole('button', {
    name: 'Filters',
    exact: true,
  })
  await expect(page.locator('.drawer-side')).toHaveCount(0)
  await expect(page.locator('.log-table__row').first()).toHaveCSS(
    'font-size',
    '14px'
  )
  await expect(page.locator('.log-table__row').first()).toHaveCSS(
    'height',
    '40px'
  )
  await filtersToggle.click()
  await service.check()
  await expect(filtersToggle).toHaveAttribute('aria-expanded', 'true')
  await expect(filtersToggle).toContainText('(1)')
  await expect(editor).toContainText(
    'attr(resource, "service.name", string) IN ["api"]'
  )
  await error.check()
  await expect(editor).toContainText(
    'severityNumber >= 17 AND severityNumber <= 20'
  )
  await page.getByRole('button', { name: 'Clear service filter' }).click()
  await expect(service).not.toBeChecked()
  await expect(error).toBeChecked()
  await expect(editor).not.toContainText('service.name')
  await editor.click()
  await editor.fill('severityNumber =')
  await filtersToggle.click()
  await expect(error).toBeDisabled()
  await expect(editor).toHaveText('severityNumber =')
  await editor.click()
  await editor.fill('')
  await filtersToggle.click()
  await expect(error).not.toBeChecked()
  await expect(error).toBeEnabled()
  await editor.click()
  await editor.fill('body CONTAINS "exact" | LIMIT 10')
  const search = page.waitForRequest(
    request =>
      request.url().endsWith('/rpc') &&
      request.postDataJSON().method === 'searchLogPage' &&
      request.postDataJSON().params.limit === 10
  )
  await filtersToggle.click()
  await service.check()
  expect((await search).postDataJSON().params.query).toBeTruthy()
  await expect(editor).toContainText('body CONTAINS "exact"')
  await expect(editor).toContainText('| LIMIT 10')
  await expect(page).toHaveURL(new RegExp(`/logs/${firstID}`))
  await page.keyboard.press('Escape')
  await expect(filtersToggle).toHaveAttribute('aria-expanded', 'false')
  await expect(filtersToggle).toBeFocused()
})

test('fetches numbered pages, retains Details, and resets paging for sort, query and page size', async ({
  page,
}) => {
  const requests: {
    offset: number
    pageSize: number
    query?: unknown
    sort: { field: string; direction: string }
  }[] = []
  const records = Array.from({ length: 235 }, (_, index) => ({
    logRef:
      index === 0
        ? firstID
        : `00000000-0000-0000-0001-${String(index).padStart(12, '0')}`,
    timestamp: String(1700000000000000000n + BigInt(index)),
    severityText: 'INFO',
    severityNumber: 9,
    serviceName: 'api',
    bodyPreview: `Record ${index + 1}`,
  }))
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method !== 'searchLogPage') return route.fallback()
    requests.push(params)
    const filtered = params.query ? records.slice(0, 7) : records
    const ordered =
      params.sort.direction === 'asc' ? [...filtered].reverse() : filtered
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result: {
          total: ordered.length,
          logs: ordered.slice(params.offset, params.offset + params.pageSize),
        },
      },
    })
  })
  await page.reload()
  const range = page.getByRole('status', { name: 'Log page range' })
  await expect(range).toContainText('1–100 of 235 logs')
  const detail = page.getByRole('table', { name: 'Log fields' })
  await expect(detail).toContainText('Full body 0: exact retained text')
  await page.getByRole('button', { name: 'Next page', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Page 2', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  expect(requests.at(-1)?.offset).toBe(100)
  await expect(page.locator('[data-log-ref]').first()).toContainText(
    'Record 101'
  )
  await expect(detail).toContainText('Full body 0: exact retained text')
  await expect(page).toHaveURL(new RegExp(`/logs/${firstID}`))
  await page.getByRole('button', { name: 'Page 3', exact: true }).click()
  await expect(range).toContainText('201–235 of 235 logs')
  await expect(
    page.getByRole('button', { name: 'Next page', exact: true })
  ).toBeDisabled()
  await page
    .getByRole('button', { name: 'Sort by timestamp', exact: true })
    .click()
  await expect(range).toContainText('1–100 of 235 logs')
  expect(requests.at(-1)).toMatchObject({
    offset: 0,
    sort: { field: 'timestamp', direction: 'desc' },
  })
  await page.getByRole('button', { name: 'Page 2', exact: true }).click()
  await expect(range).toContainText('101–200')
  await page.getByRole('combobox', { name: 'Logs per page' }).selectOption('50')
  await expect(range).toContainText('1–50 of 235 logs')
  expect(requests.at(-1)).toMatchObject({ offset: 0, pageSize: 50 })
  await page.getByRole('button', { name: 'Page 2', exact: true }).click()
  await expect(range).toContainText('51–100')
  await page.getByRole('button', { name: 'Filters', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Service api', exact: true }).check()
  await expect(range).toContainText('1–7 of 7 logs')
  expect(requests.at(-1)).toMatchObject({ offset: 0, pageSize: 50 })
  expect(requests.at(-1)?.query).toBeTruthy()
  await expect(detail).toContainText('Full body 0: exact retained text')
})

test('column visibility and keyboard resizing persist without changing selection', async ({
  page,
}) => {
  const separator = page.getByRole('separator', {
    name: 'Resize timestamp column',
    exact: true,
  })
  const before = Number(await separator.getAttribute('aria-valuenow'))
  await separator.focus()
  await page.keyboard.press('ArrowRight')
  await expect(separator).toHaveAttribute('aria-valuenow', String(before + 16))
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page
    .getByRole('checkbox', { name: 'Resource: service.name', exact: true })
    .uncheck()
  await expect(
    page.getByRole('columnheader').filter({ hasText: 'service.name' })
  ).toHaveCount(0)
  const widths = await page.evaluate(() =>
    localStorage.getItem('logs-column-widths')
  )
  await page.reload()
  await expect(
    page.getByRole('grid', { name: 'Logs', exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('columnheader').filter({ hasText: 'service.name' })
  ).toHaveCount(0)
  expect(
    await page.evaluate(() => localStorage.getItem('logs-column-widths'))
  ).toBe(widths)
  await expect(page.locator(`[data-log-ref="${firstID}"]`)).toHaveAttribute(
    'aria-selected',
    'true'
  )
})

test('resize guides align with header boundaries and follow pointer resizing', async ({
  page,
}) => {
  const guide = page.getByRole('separator', {
    name: 'Resize timestamp column',
    exact: true,
  })
  const heading = page
    .getByRole('columnheader')
    .filter({ hasText: 'timestamp' })
  async function aligned() {
    const bar = (await guide.boundingBox())!
    const cell = (await heading.boundingBox())!
    const paintedOffset = await guide.evaluate(element =>
      parseFloat(getComputedStyle(element, '::before').left)
    )
    expect(
      Math.abs(bar.x + paintedOffset - (cell.x + cell.width))
    ).toBeLessThan(1)
    expect(Math.abs(bar.y - cell.y)).toBeLessThan(1)
    const guideHeight = await guide.evaluate(element =>
      parseFloat(getComputedStyle(element, '::before').height)
    )
    expect(guideHeight).toBe(bar.height)
    return bar
  }
  const before = await aligned()
  await page.mouse.move(before.x + before.width / 2, before.y + 20)
  await expect(guide.locator('.col-resize-bar__line')).toHaveCSS(
    'opacity',
    '0.25'
  )
  await page.mouse.down()
  await page.mouse.move(before.x + before.width / 2 + 32, before.y + 20, {
    steps: 6,
  })
  await expect(guide).toHaveClass(/col-resize-bar--active/)
  await page.mouse.up()
  const after = await aligned()
  expect(after.x).toBeGreaterThan(before.x)
  await expect(page.locator(`[data-log-ref="${firstID}"]`)).toHaveAttribute(
    'aria-selected',
    'true'
  )
})

test('pins Severity first, Timestamp next and Body right while the middle scrolls', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1050, height: 900 })
  const viewport = page.locator('.log-table__scroll')
  const row = page.locator(`[data-log-ref="${firstID}"]`)
  const timestamp = row.locator('.log-table__pin-left')
  const severity = row.locator('.log-table__pin-severity')
  const body = row.locator('.log-table__pin-body')
  await expect(severity).toHaveCSS('box-shadow', 'none')
  await expect(
    page.getByRole('separator', { name: 'Resize severity column', exact: true })
  ).toHaveCount(0)
  const before = {
    timestamp: (await timestamp.boundingBox())!,
    severity: (await severity.boundingBox())!,
    body: (await body.boundingBox())!,
  }
  await viewport.evaluate(
    element => (element.scrollLeft = element.scrollWidth - element.clientWidth)
  )
  await expect
    .poll(() => viewport.evaluate(element => element.scrollLeft))
    .toBeGreaterThan(0)
  const after = {
    timestamp: (await timestamp.boundingBox())!,
    severity: (await severity.boundingBox())!,
    body: (await body.boundingBox())!,
  }
  expect(after.timestamp.x).toBeCloseTo(before.timestamp.x, 0)
  expect(after.severity.x).toBeCloseTo(before.severity.x, 0)
  expect(after.body.x).toBeCloseTo(before.body.x, 0)
  const viewportBounds = (await viewport.boundingBox())!
  expect(after.severity.x).toBeCloseTo(viewportBounds.x, 0)
  expect(after.timestamp.x).toBeCloseTo(
    after.severity.x + after.severity.width,
    0
  )
  expect(after.body.x + after.body.width).toBeCloseTo(
    viewportBounds.x + viewportBounds.width,
    0
  )
  expect(after.severity.width).toBe(160)
  await expect(severity.getByRole('button')).toHaveCSS('text-align', 'left')
  await expect(row).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('table', { name: 'Log fields' })).toContainText(
    'exact retained text'
  )
})

test('Body fills available space first and can be hidden and restored at the right edge', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 900 })
  const row = page.locator(`[data-log-ref="${firstID}"]`)
  const body = row.locator('.log-table__pin-body')
  const severity = row.locator('.log-table__pin-severity')
  const timestamp = row.locator('.log-table__pin-left')
  const service = row.getByRole('gridcell').nth(2)
  const viewport = page.locator('.log-table__scroll')
  const separator = page.getByRole('separator', {
    name: 'Resize timestamp column',
    exact: true,
  })
  await separator.focus()
  await page.keyboard.press('ArrowRight')
  const before = {
    body: (await body.boundingBox())!.width,
    severity: (await severity.boundingBox())!.width,
    timestamp: (await timestamp.boundingBox())!.width,
    service: (await service.boundingBox())!.width,
    viewport: (await viewport.boundingBox())!.width,
  }
  await page.setViewportSize({ width: 1800, height: 900 })
  await expect
    .poll(async () => (await body.boundingBox())!.width)
    .toBeGreaterThan(before.body)
  const growth = (await viewport.boundingBox())!.width - before.viewport
  expect((await body.boundingBox())!.width - before.body).toBeCloseTo(growth, 0)
  expect((await timestamp.boundingBox())!.width).toBeCloseTo(
    before.timestamp,
    0
  )
  expect((await service.boundingBox())!.width).toBeCloseTo(before.service, 0)
  expect((await severity.boundingBox())!.width).toBe(before.severity)
  await page.setViewportSize({ width: 1600, height: 900 })
  await expect
    .poll(async () => (await body.boundingBox())!.width)
    .toBeCloseTo(before.body, 0)
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Log: body', exact: true }).uncheck()
  await expect(body).toHaveCount(0)
  await page.reload()
  await expect(row).toBeVisible()
  await expect(body).toHaveCount(0)
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Log: body', exact: true }).check()
  await expect(body).toBeVisible()
  const bodyBounds = (await body.boundingBox())!
  expect(bodyBounds.x + bodyBounds.width).toBeCloseTo(
    (await viewport.boundingBox())!.x + (await viewport.boundingBox())!.width,
    0
  )
  await expect(row).toHaveAttribute('aria-selected', 'true')
})

test('pagination and row count stay within the main pane with Details open or closed', async ({
  page,
}) => {
  const main = page.locator('.page-layout__main-chrome')
  const footer = main.locator('.signal-footer')
  const size = main.getByRole('combobox', { name: 'Logs per page' })
  async function contained() {
    await expect(size).toBeVisible()
    await expect(
      main.getByRole('navigation', { name: 'Log pages' })
    ).toBeVisible()
    const mainBounds = (await main.boundingBox())!
    const footerBounds = (await footer.boundingBox())!
    const sizeBounds = (await size.boundingBox())!
    expect(footerBounds.x).toBeGreaterThanOrEqual(mainBounds.x)
    expect(footerBounds.x + footerBounds.width).toBeLessThanOrEqual(
      mainBounds.x + mainBounds.width + 1
    )
    expect(sizeBounds.x + sizeBounds.width).toBeLessThanOrEqual(
      mainBounds.x + mainBounds.width
    )
    expect(footerBounds.y + footerBounds.height).toBeCloseTo(
      mainBounds.y + mainBounds.height,
      0
    )
    const tableBounds = (await page
      .locator('.log-table__scroll')
      .boundingBox())!
    expect(tableBounds.y + tableBounds.height).toBeLessThanOrEqual(
      footerBounds.y + 1
    )
  }
  await contained()
  await page
    .getByRole('button', { name: 'Collapse details', exact: true })
    .click()
  await contained()
  await page.getByRole('button', { name: 'Open details', exact: true }).click()
  await contained()
})

test('footer delete controls dispatch only their intended mocked operations', async ({
  page,
}) => {
  let deleted = false
  let cleared = false
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'deleteLogsByRefs') {
      expect(params).toEqual([firstID])
      deleted = true
      return route.fulfill({
        json: { jsonrpc: '2.0', id, result: { count: 1 } },
      })
    }
    if (method === 'clearLogs') {
      cleared = true
      return route.fulfill({
        json: { jsonrpc: '2.0', id, result: 'Logs cleared successfully' },
      })
    }
    if (method === 'searchLogPage' && (deleted || cleared)) {
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: {
            total: cleared ? 0 : 1,
            logs: cleared
              ? []
              : [
                  {
                    logRef: secondID,
                    timestamp: logs[1]!.timestamp,
                    severityText: 'INFO',
                    severityNumber: 9,
                    serviceName: 'worker',
                    bodyPreview: 'Retained record',
                    bodyType: 'string',
                  },
                ],
          },
        },
      })
    }
    return route.fallback()
  })
  await expect(
    page.getByRole('button', { name: 'Delete this log', exact: true })
  ).toBeEnabled()
  await page
    .getByRole('button', { name: 'Delete this log', exact: true })
    .click()
  await expect.poll(() => deleted).toBe(true)
  await expect(page.locator(`[data-log-ref="${firstID}"]`)).toHaveCount(0)
  await expect(page.locator(`[data-log-ref="${secondID}"]`)).toBeVisible()
  expect(cleared).toBe(false)
  await page
    .getByRole('button', { name: 'Delete all logs', exact: true })
    .click()
  await expect.poll(() => cleared).toBe(true)
  await expect(page.locator('[data-log-ref]')).toHaveCount(0)
  await expect(
    page.getByRole('status', { name: 'Log page range' })
  ).toContainText('0 logs')
})

test('header error navigation crosses pages within the current result', async ({
  page,
}) => {
  const records = Array.from({ length: 205 }, (_, index) => ({
    logRef:
      index === 0
        ? firstID
        : `00000000-0000-0000-0002-${String(index).padStart(12, '0')}`,
    timestamp: String(1700000000000000000n + BigInt(index)),
    severityText: [1, 101, 201].includes(index) ? 'ERROR' : 'INFO',
    severityNumber: [1, 101, 201].includes(index) ? 17 : 9,
    serviceName: 'api',
    bodyPreview: `Record ${index + 1}`,
    bodyType: 'string',
  }))
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    let result:
      { total: number; logs: typeof records } | JsonLogData | LogErrorNavigation
    if (method === 'searchLogPage')
      result = {
        total: records.length,
        logs: records.slice(params.offset, params.offset + params.pageSize),
      }
    else if (method === 'getLog')
      result = { ...logs[0]!, logRef: params.logRef }
    else if (method === 'getLogErrorNavigation') {
      const position = records.findIndex(
        record => record.logRef === params.selectedLogRef
      )
      const errors = records.flatMap((record, offset) =>
        record.severityNumber >= 17 ? [{ logRef: record.logRef, offset }] : []
      )
      const index = errors.findIndex(
        target => target.logRef === params.selectedLogRef
      )
      result = {
        count: errors.length,
        index: index >= 0 ? index + 1 : null,
        previous:
          [...errors].reverse().find(target => target.offset < position) ??
          null,
        next: errors.find(target => target.offset > position) ?? null,
      }
    } else return route.fallback()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.reload()
  const header = page.getByRole('region', {
    name: 'Log table summary',
    exact: true,
  })
  await expect(header).toContainText('Logs')
  await expect(header).toContainText('205 records')
  await expect(header).toContainText('3 err')
  await expect(
    header.getByRole('button', { name: 'Previous error', exact: true })
  ).toBeDisabled()
  await header.getByRole('button', { name: 'Next error', exact: true }).click()
  await expect(header).toContainText('1/3 errors')
  await header.getByRole('button', { name: 'Next error', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Page 2', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await expect(header).toContainText('2/3 errors')
  await expect(
    page.locator(`[data-log-ref="${records[101]!.logRef}"]`)
  ).toHaveAttribute('aria-selected', 'true')
  await header.getByRole('button', { name: 'Next error', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Page 3', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await expect(header).toContainText('3/3 errors')
  await expect(
    header.getByRole('button', { name: 'Next error', exact: true })
  ).toBeDisabled()
  await header
    .getByRole('button', { name: 'Previous error', exact: true })
    .click()
  await expect(header).toContainText('2/3 errors')
  await expect(
    page.getByRole('button', { name: 'Page 2', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await expect(
    header.getByRole('button', { name: 'Filters', exact: true })
  ).toBeVisible()
  await expect(
    header.getByRole('button', { name: 'Columns', exact: true })
  ).toBeVisible()
})
