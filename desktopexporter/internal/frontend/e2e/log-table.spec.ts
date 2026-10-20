import { expect, test } from '@playwright/test'
import type {
  JsonLogData,
  JsonTraceData,
  JsonAttributeValue,
} from '../src/types/wire-types'
import type { LogErrorNavigation, LogColumn } from '../src/types/api-types'

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
  await expect(severityCell.locator('.log-severity__dot')).toHaveCSS(
    'width',
    '8px'
  )
  await expect(severityCell.locator('.log-severity__label')).toHaveText(
    'ERROR (17)'
  )
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

test('Details pins enriched trace context above scrolling fields', async ({
  page,
}) => {
  // Finish the initial record load before replacing its RPC fixture and
  // reloading; otherwise this handler can also catch the old page's getLog.
  await expect(page.getByRole('table', { name: 'Log fields' })).toBeVisible()
  const traceID = '00000000000000000000000000000011'
  const spanID = '0000000000000001'
  const traceData: JsonTraceData = {
    traceID,
    traceStart: '1700000000000000000',
    unplacedSpanCount: 0,
    resources: { '0': { attributes: [], droppedAttributesCount: 0 } },
    scopes: {
      '0': { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
    },
    spans: [
      {
        depth: 0,
        matched: true,
        spanData: {
          traceState: '',
          spanID,
          parentSpanID: null,
          flags: 0,
          name: 'POST /checkout',
          kindCode: 2,
          kind: 'Server',
          start: '0',
          dur: '1000000000',
          attributes: [],
          events: [],
          links: [],
          r: 0,
          s: 0,
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCodeValue: 0,
          statusCode: 'Unset',
          statusMessage: '',
        },
      },
    ],
  }
  let waterfallFetches = 0
  let associatedFetches = 0
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'getLog' && params.logRef === firstID) {
      return route.fulfill({
        json: { jsonrpc: '2.0', id, result: { ...logs[0]!, traceID, spanID } },
      })
    }
    if (method === 'searchTraceSummaries') {
      if (params.query) {
        associatedFetches++
        expect(params).toMatchObject({
          startTime: null,
          endTime: null,
          limit: 1,
          query: {
            query: {
              field: { name: 'traceID' },
              fieldOperator: '=',
              value: traceID,
            },
          },
        })
      }
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: [
            {
              traceID,
              hasRootSpan: true,
              rootSpan: { name: 'POST /checkout', serviceName: 'api' },
              startTime: '1700000000000000000',
              durationNs: '1000000000',
              spanCount: 5,
              errorCount: 2,
            },
          ],
        },
      })
    }
    if (method === 'getTraceView') {
      waterfallFetches++
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: traceData,
        },
      })
    }
    return route.fallback()
  })
  await page.reload()
  const panel = page.locator('.log-detail-panel')
  const headings = panel.locator('summary.detail-bubble__heading')
  await expect(headings).toHaveCount(0)
  const trace = panel.getByRole('region', { name: 'Trace context' })
  const card = trace.getByText('POST /checkout', { exact: true })
  await expect(card).toBeVisible()
  await expect(trace.locator('.signal-row')).toHaveCount(0)
  await expect(
    trace.getByRole('link', { name: `Open trace ${traceID}`, exact: true })
  ).toHaveAttribute('href', `/traces/${traceID}?time=all`)
  expect(waterfallFetches).toBe(0)
  expect(associatedFetches).toBe(1)
  expect(await trace.evaluate(element => element.closest('details'))).toBeNull()
  expect(
    await trace.evaluate(element => {
      const fields = element
        .closest('.log-detail-panel')!
        .querySelector('table[aria-label="Log fields"]')!
      return !!(
        element.compareDocumentPosition(fields) &
        Node.DOCUMENT_POSITION_FOLLOWING
      )
    })
  ).toBe(true)
  const pinnedY = (await card.boundingBox())!.y
  const scroll = panel.locator('.log-detail-panel__scroll')
  await scroll.evaluate(element => (element.scrollTop = element.scrollHeight))
  expect(await scroll.evaluate(element => element.scrollTop)).toBeGreaterThan(0)
  expect((await card.boundingBox())!.y).toBeCloseTo(pinnedY, 0)
  await expect(panel.locator('table [data-field="traceId"]')).toHaveCount(0)
  await panel.getByRole('tab', { name: 'JSON', exact: true }).click()
  await expect(card).toBeVisible()
  await panel.getByRole('tab', { name: 'Properties', exact: true }).click()
  await expect(panel.getByRole('table', { name: 'Log fields' })).toBeVisible()
  await expect(card).toBeVisible()
  const exportButton = panel.getByRole('button', { name: 'Download log' })
  const header = panel.getByRole('region', { name: 'Log record' })
  const tabs = header.getByRole('tablist', { name: 'Detail view' })
  await expect(tabs).toBeVisible()
  await expect(header.getByLabel('Log timestamp')).toHaveCount(0)
  await expect(header.locator('.badge')).toHaveCount(0)
  await expect(header.locator('.pane-header__meta-row')).toHaveCount(0)
  const tabsBounds = (await tabs.boundingBox())!
  const headerBounds = (await header.boundingBox())!
  const exportBounds = (await exportButton.boundingBox())!
  expect(exportBounds.x + exportBounds.width).toBeGreaterThan(
    headerBounds.x + headerBounds.width - 24
  )
  await expect(
    panel.locator('.log-detail-panel__scroll').getByRole('tablist')
  ).toHaveCount(0)
  await expect(exportButton).toHaveClass(/header-icon-button/)
  await expect(exportButton).toHaveText('')
  const titleBounds = (await panel
    .locator('.pane-header__title')
    .boundingBox())!
  expect(exportBounds.x).toBeGreaterThanOrEqual(tabsBounds.x + tabsBounds.width)
  expect(tabsBounds.y).toBeLessThan(titleBounds.y + titleBounds.height)
  expect(tabsBounds.y + tabsBounds.height).toBeGreaterThan(titleBounds.y)
  const errorColour = await panel.evaluate(element => {
    const probe = document.createElement('span')
    probe.style.color = 'var(--color-error)'
    element.append(probe)
    const colour = getComputedStyle(probe).color
    probe.remove()
    return colour
  })
  await expect(
    panel.locator('[data-field="severityText"] .attribute-value')
  ).toHaveCSS('color', errorColour)
  await expect(
    panel.locator('[data-field="severityNumber"] .detail-property__value')
  ).toHaveCSS('color', errorColour)
  await trace
    .getByRole('link', {
      name: `Open span ${spanID} in trace ${traceID}`,
      exact: true,
    })
    .click()
  await expect(page).toHaveURL(new RegExp(`/traces/${traceID}\\?`))
  expect(new URL(page.url()).searchParams.get('span')).toBe(spanID)
  expect(new URL(page.url()).searchParams.get('time')).toBe('all')
})

test('Raw JSON stays selected across log navigation until a field is inspected', async ({
  page,
}) => {
  await page.route('**/export/logs/*', async route => {
    const id = route.request().url().split('/').at(-1)
    const payload = JSON.stringify({ logRef: id })
    await route.fulfill({
      contentType: 'application/json',
      headers: { 'Content-Length': String(Buffer.byteLength(payload)) },
      body: route.request().method() === 'HEAD' ? '' : payload,
    })
  })
  const rawTab = page.getByRole('tab', { name: 'JSON', exact: true })
  const inspector = page.getByRole('region', { name: 'Raw JSON inspector' })
  await rawTab.click()
  await expect(inspector.locator('.cm-content')).toContainText(firstID)
  const first = page.locator(`[data-log-ref="${firstID}"]`)
  await first.focus()
  await page.keyboard.press('ArrowDown')
  const second = page.locator(`[data-log-ref="${secondID}"]`)
  await expect(second).toHaveAttribute('aria-selected', 'true')
  await expect(rawTab).toHaveAttribute('aria-selected', 'true')
  await expect(inspector.locator('.cm-content')).toContainText(secondID)
  await expect(inspector.locator('.cm-content')).not.toContainText(firstID)
  await second
    .getByRole('button', { name: 'Show log: body in details' })
    .click()
  await expect(
    page.getByRole('tab', { name: 'Properties', exact: true })
  ).toHaveAttribute('aria-selected', 'true')
  await expect(
    page
      .getByRole('table', { name: 'Log fields' })
      .locator('[data-field="body"]')
  ).toHaveClass(/detail-property--revealed/)
})

function logCopyFixture(logRef: string) {
  return `{"resourceLogs":[{
  "schemaUrl":"https://opentelemetry.io/schemas/1.21.0",
  "resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},
  "scopeLogs":[{
    "schemaUrl":"https://opentelemetry.io/schemas/1.22.0",
    "scope":{"name":"clipboard-fixture","version":"1.2"},
    "logRecords":[{
      "timeUnixNano":"1700000000123456789",
      "body":{"stringValue":"Résumé 🧭 ${logRef}"},
      "attributes":[
        {"key":"zero","value":{"doubleValue":-0.0}},
        {"key":"large","value":{"doubleValue":18446744073709551615}},
        {"key":"exponent","value":{"doubleValue":1.2300e+40}},
        {"key":"sequence","value":{"intValue":"9223372036854775807"}}
      ]
    }]
  }]
}]}\n`
}

test('header Copy log preserves exact export text with pointer and Enter', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const source = logCopyFixture(firstID)
  const requests: string[] = []
  await page.route('**/export/logs/**', route => {
    requests.push(new URL(route.request().url()).pathname)
    expect(route.request().method()).toBe('GET')
    return route.fulfill({ contentType: 'application/json', body: source })
  })
  const header = page.getByRole('region', { name: 'Log record', exact: true })
  const copy = header.getByRole('button', { name: 'Copy log', exact: true })
  await expect(copy).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Copy log', exact: true })
  ).toHaveCount(1)
  await expect(copy).toHaveClass(/header-icon-button/)
  expect(await copy.evaluate(element => element.tagName)).toBe('BUTTON')
  const copyBounds = (await copy.boundingBox())!
  const downloadBounds = (await header
    .getByRole('button', { name: 'Download log', exact: true })
    .boundingBox())!
  expect(copyBounds.x + copyBounds.width).toBeLessThan(downloadBounds.x)
  expect(requests).toEqual([])
  for (const activation of ['pointer', 'Enter']) {
    await page.evaluate(() => navigator.clipboard.writeText('before log copy'))
    if (activation === 'pointer') await copy.click()
    else await copy.press('Enter')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(source)
    await expect(header.getByRole('status')).toHaveText('Copied log.')
    await expect(copy).toHaveAttribute('aria-busy', 'false')
    await expect(copy).toBeFocused()
    await expect(page).toHaveURL(`/logs/${firstID}?time=all`)
  }
  expect(requests).toEqual([
    `/export/logs/${firstID}`,
    `/export/logs/${firstID}`,
  ])
})

test('a delayed old-log Copy response cannot overwrite the newly selected log clipboard', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const requests: string[] = []
  await page.route('**/export/logs/**', route => {
    const path = new URL(route.request().url()).pathname
    requests.push(path)
    return route.fulfill({
      contentType: 'application/json',
      body: logCopyFixture(path.split('/').at(-1)!),
    })
  })
  // Let the old HTTP response arrive, then hold text completion even after abort.
  // This exercises stale-result protection, not just browser network cancellation.
  const pending = await page.evaluateHandle(oldPath => {
    const fetch = window.fetch.bind(window)
    const writeText = navigator.clipboard.writeText.bind(navigator.clipboard)
    const writes: string[] = []
    navigator.clipboard.writeText = text => {
      writes.push(text)
      return writeText(text)
    }
    let release!: () => void
    let notifyStarted!: () => void
    let notifyFinished!: () => void
    let signal: AbortSignal | null | undefined
    const gate = new Promise<void>(resolve => {
      release = resolve
    })
    const started = new Promise<void>(resolve => {
      notifyStarted = resolve
    })
    const finished = new Promise<void>(resolve => {
      notifyFinished = resolve
    })
    window.fetch = async (input, init) => {
      const response = await fetch(input, init)
      const url = input instanceof Request ? input.url : String(input)
      if (new URL(url, location.href).pathname === oldPath) {
        signal = init?.signal
        const text = response.text.bind(response)
        response.text = async () => {
          const result = await text()
          notifyStarted()
          await gate
          notifyFinished()
          return result
        }
      }
      return response
    }
    return {
      release,
      started,
      finished,
      writes,
      aborted: () => signal?.aborted,
    }
  }, `/export/logs/${firstID}`)
  const header = page.getByRole('region', { name: 'Log record', exact: true })
  const copy = header.getByRole('button', { name: 'Copy log', exact: true })
  try {
    await copy.press('Enter')
    await pending.evaluate(state => state.started)
    await expect(copy).toHaveAttribute('aria-busy', 'true')
    await page.locator(`[data-log-ref="${firstID}"]`).press('ArrowDown')
    await expect(page.locator(`[data-log-ref="${secondID}"]`)).toHaveAttribute(
      'aria-selected',
      'true'
    )
    await expect(
      page
        .getByRole('table', { name: 'Log fields' })
        .locator('[data-field="body"]')
    ).toContainText('Full body 1: exact retained text')
    await expect(copy).toHaveAttribute('aria-busy', 'false')
    expect(await pending.evaluate(state => state.aborted())).toBe(true)
    await copy.click()
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(logCopyFixture(secondID))
    await pending.evaluate(async state => {
      state.release()
      await state.finished
    })
    expect(await pending.evaluate(state => state.writes)).toEqual([
      logCopyFixture(secondID),
    ])
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      logCopyFixture(secondID)
    )
    await expect(header.getByRole('status')).toHaveText('Copied log.')
    await expect(page).toHaveURL(`/logs/${secondID}?time=all`)
    expect(requests).toEqual([
      `/export/logs/${firstID}`,
      `/export/logs/${secondID}`,
    ])
  } finally {
    await pending.evaluate(state => state.release())
    await pending.dispose()
  }
})

test('Raw JSON folding uses waterfall-style icon controls on the top line', async ({
  page,
}) => {
  const payload = JSON.stringify({
    resourceLogs: [
      {
        scopeLogs: [
          {
            logRecords: [
              { body: { stringValue: 'Folding fixture' }, severityNumber: 17 },
            ],
          },
        ],
      },
    ],
  })
  await page.route(`**/export/logs/${firstID}`, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      headers: { 'Content-Length': String(Buffer.byteLength(payload)) },
      body: route.request().method() === 'HEAD' ? '' : payload,
    })
  })
  await page.getByRole('tab', { name: 'JSON', exact: true }).click()
  const inspector = page.getByRole('region', { name: 'Raw JSON inspector' })
  const editor = inspector.locator('.raw-json-editor')
  await expect(editor).toBeVisible()
  await expect(editor.locator('.cm-content')).toHaveCSS(
    'font-family',
    /Atkinson Hyperlegible Mono/
  )
  await expect(editor).toHaveCSS('border-width', '0px')
  await expect(editor).toHaveCSS('font-size', '14px')
  await expect(editor.locator('.cm-lineNumbers')).toHaveCount(0)
  await expect(editor.locator('.json-indent-guides')).toHaveCount(0)
  await expect(editor.locator('.json-indent-bands')).toHaveCount(0)
  await expect(editor.locator('.cm-gutters')).toHaveCSS(
    'background-color',
    'rgba(0, 0, 0, 0)'
  )
  await expect(editor.locator('.cm-foldGutter')).not.toContainText('⌄')
  await expect(editor.locator('.cm-foldGutter')).not.toContainText('›')
  const rawFold = editor.locator('.cm-foldGutter [title="Fold line"]').first()
  const rawCaretBounds = (await rawFold.locator('svg').boundingBox())!
  const rawTextBounds = await editor
    .locator('.cm-line')
    .first()
    .evaluate(element => {
      const range = document.createRange()
      range.selectNodeContents(element)
      const bounds = range.getBoundingClientRect()
      return { y: bounds.y, height: bounds.height }
    })
  expect(
    Math.abs(
      rawCaretBounds.y +
        rawCaretBounds.height / 2 -
        (rawTextBounds.y + rawTextBounds.height / 2)
    )
  ).toBeLessThan(1.5)
  await expect
    .poll(() =>
      rawFold.evaluate(
        element =>
          getComputedStyle(element.querySelector('.json-fold-icon')!).opacity
      )
    )
    .toBe('1')
  await rawFold.hover()
  await expect
    .poll(() =>
      rawFold.evaluate(
        element =>
          getComputedStyle(element.querySelector('.json-fold-icon')!).opacity
      )
    )
    .toBe('1')
  await page.mouse.move(0, 0)
  await editor.locator('.cm-content').focus()
  await expect
    .poll(() =>
      rawFold.evaluate(
        element =>
          getComputedStyle(element.querySelector('.json-fold-icon')!).opacity
      )
    )
    .toBe('1')
  const stringColour = await editor.evaluate(element => {
    const probe = document.createElement('span')
    probe.style.color = 'var(--color-base-content)'
    element.append(probe)
    const colour = getComputedStyle(probe).color
    probe.remove()
    return colour
  })
  await expect(
    editor.getByText('"Folding fixture"', { exact: true })
  ).toHaveCSS('color', stringColour)
  const folding = inspector.getByRole('group', { name: 'JSON folding' })
  const collapse = folding.getByRole('button', {
    name: 'Collapse JSON',
    exact: true,
  })
  const expand = folding.getByRole('button', {
    name: 'Expand JSON',
    exact: true,
  })
  for (const button of [collapse, expand]) {
    await expect(button).toBeEnabled()
    await expect(button).toHaveText('')
    await expect(button).toHaveClass(/chart-header-control/)
    await expect(button.locator('svg')).toBeVisible()
  }
  const metadataBounds = (await page
    .getByRole('region', { name: 'Log record', exact: true })
    .getByRole('status')
    .filter({ hasText: /^OTLP JSON/ })
    .boundingBox())!
  const groupBounds = (await folding.boundingBox())!
  const toolbarBounds = (await inspector
    .locator('.raw-inspector__controls')
    .boundingBox())!
  const editorBounds = (await editor.boundingBox())!
  // Status now shares the record header. Folding stays on the first raw-toolbar
  // line, above the editor and right-aligned within its horizontal padding.
  expect(metadataBounds.y + metadataBounds.height).toBeLessThanOrEqual(
    groupBounds.y
  )
  expect(groupBounds.y).toBeCloseTo(toolbarBounds.y, 0)
  expect(groupBounds.x + groupBounds.width).toBeCloseTo(
    toolbarBounds.x + toolbarBounds.width - 8,
    0
  )
  expect(groupBounds.y + groupBounds.height).toBeLessThanOrEqual(editorBounds.y)
  await expect(inspector.getByRole('button', { name: /^Copy/ })).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: 'Copy log', exact: true })
  ).toHaveCount(1)
  await expect(
    inspector.getByRole('button', { name: 'Search JSON' })
  ).toHaveCount(0)
  await expect(
    inspector.getByRole('link', { name: 'Download log' })
  ).toHaveCount(0)
  const section = editor.locator('.cm-foldGutter [title="Fold line"]').nth(1)
  await section.locator('..').click({ position: { x: 2, y: 8 } })
  await expect(editor.locator('.cm-foldPlaceholder')).toHaveCount(1)
  await expect(editor.locator('.cm-line').first()).toHaveText('{')
  await expect(
    editor.getByText('"Folding fixture"', { exact: true })
  ).toHaveCount(0)
  await editor
    .locator('.cm-foldGutter [title="Unfold line"]:visible')
    .first()
    .locator('..')
    .click()
  await expect(
    editor.getByText('"Folding fixture"', { exact: true })
  ).toBeVisible()
  await collapse.click()
  await expect(editor.locator('.cm-foldPlaceholder').first()).toBeVisible()
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.getByRole('button', { name: 'Copy log', exact: true }).click()
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(payload)
  await expand.click()
  await expect(editor.locator('.cm-foldPlaceholder')).toHaveCount(0)
})

test('external provenance shrinks inline without replacing its source name', async ({
  page,
}) => {
  const heading = page
    .getByRole('columnheader')
    .filter({ hasText: 'service.name' })
  const provenance = heading.getByRole('img', { name: 'Source: resource' })
  const title = heading.locator('.table-column-heading__title')
  await expect(heading.getByText('·', { exact: true })).toHaveAttribute(
    'aria-hidden',
    'true'
  )
  await expect(provenance).toHaveText('resource')
  await expect(provenance).toHaveAttribute('title', 'resource')
  await expect(provenance).toHaveCSS('flex-shrink', '1')
  await expect(provenance).toHaveCSS('text-overflow', 'clip')
  await expect(provenance.locator('span').first()).toHaveText('r')
  await expect(
    provenance.locator('.table-column-heading__source-rest')
  ).toHaveCSS('overflow', 'hidden')
  const titleBounds = (await title.boundingBox())!
  const sourceBounds = (await provenance.boundingBox())!
  expect(sourceBounds.x).toBeGreaterThanOrEqual(
    titleBounds.x + titleBounds.width
  )
  expect(sourceBounds.y + sourceBounds.height / 2).toBeCloseTo(
    titleBounds.y + titleBounds.height / 2,
    0
  )
  const guide = page.getByRole('separator', {
    name: 'Resize service.name column',
    exact: true,
  })
  await guide.focus()
  await page.keyboard.press('Shift+ArrowRight')
  await expect(provenance).toHaveText('resource')
  expect((await provenance.boundingBox())!.width).toBeGreaterThan(
    sourceBounds.width
  )
  await page.keyboard.press('Shift+ArrowLeft')
  await expect(provenance).toHaveText('resource')
  await expect(title).toHaveAttribute('title', 'service.name')
  await expect(page.getByRole('columnheader').first()).toHaveCSS(
    'height',
    '40px'
  )
})

test('column search uses the compact shared filter input and retains filtering', async ({
  page,
}) => {
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  const picker = page.getByRole('group', { name: 'Log columns' })
  const search = picker.getByRole('searchbox', { name: 'Find columns' })
  await expect(search).toHaveCSS('height', '32px')
  await expect(search).toHaveCSS('line-height', '21px')
  await search.fill('service.name')
  await expect(picker.getByRole('checkbox')).toHaveCount(2)
  await expect(
    picker.getByRole('checkbox', {
      name: 'Resource: service.name',
      exact: true,
    })
  ).toBeVisible()
  await search.fill('not-a-column')
  await expect(picker.getByRole('checkbox')).toHaveCount(0)
})

test('column groups toggle together and retain the final required column', async ({
  page,
}) => {
  const url = page.url()
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  const picker = page.getByRole('group', { name: 'Log columns' })
  const logFields = picker.getByRole('checkbox', {
    name: 'All log fields columns',
    exact: true,
  })
  await logFields.check()
  await logFields.uncheck()
  const fields = picker.getByRole('group', { name: 'log fields', exact: true })
  for (const checkbox of await fields.getByRole('checkbox').all())
    await expect(checkbox).not.toBeChecked()
  await expect(
    picker.getByRole('checkbox', {
      name: 'All resource attributes columns',
      exact: true,
    })
  ).toBeDisabled()
  await expect(
    page
      .getByRole('grid', { name: 'Logs', exact: true })
      .getByRole('columnheader')
  ).toHaveCount(1)
  expect(page.url()).toBe(url)
  await logFields.check()
  const groupX = (await logFields.boundingBox())!.x
  const childX = (await fields
    .getByRole('checkbox', { name: 'Log: timestamp', exact: true })
    .boundingBox())!.x
  expect(childX - groupX).toBe(24)
})

test('embedded JSON previews are coloured and Details wraps at keys with working fold carets', async ({
  page,
}) => {
  const command = `curl -d '{"method":"getTraceView"}'`
  const inner = JSON.stringify({
    path: '/recorded/configuration/'.repeat(12) + 'config.jsonc',
    message:
      'ordinary words should wrap between words without splitting them '.repeat(
        5
      ),
    pattern: command,
  })
  const body: JsonAttributeValue = {
    kind: 'array',
    value: [
      { kind: 'string', value: 'loading' },
      { kind: 'string', value: inner },
    ],
  }
  const preview = `["loading", ${JSON.stringify(inner.slice(0, 48) + '…')}]`
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    if (method === 'getLog')
      return route.fulfill({
        json: { jsonrpc: '2.0', id, result: { ...logs[0], body } },
      })
    if (method === 'searchLogPage')
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: {
            total: 1,
            logs: [
              {
                logRef: firstID,
                timestamp: logs[0].observedTimestamp,
                severityText: 'ERROR',
                severityNumber: 17,
                serviceName: 'api',
                bodyPreview: preview,
                bodyType: 'array',
                eventName: '',
              },
            ],
          },
        },
      })
    return route.fallback()
  })
  await page.reload()
  const cell = page.locator(
    `[data-log-ref="${firstID}"] .log-table__cell--body`
  )
  await expect(cell).not.toContainText('\\"')
  await expect(cell.locator('.detail-code--key')).toHaveText('"path"')
  await expect(cell).toContainText('…')
  await expect(cell.locator('.cm-editor')).toHaveCount(0)
  const bodyField = page.locator('[data-field="body"]')
  const editor = bodyField.locator('.cm-editor')
  await expect
    .poll(() =>
      editor
        .locator('.cm-line')
        .filter({ hasText: '"pattern"' })
        .evaluate(line => {
          const content = line.cloneNode(true) as HTMLElement
          content
            .querySelectorAll('.cm-widgetBuffer')
            .forEach(node => node.remove())
          return content.textContent
        })
    )
    .toBe(`    "pattern": ${command}`)
  for (const width of [320, 480]) {
    await editor.evaluate(
      (element, width) => (element.style.maxWidth = `${width}px`),
      width
    )
    await expect
      .poll(() =>
        editor.evaluate(element => {
          const line = [...element.querySelectorAll('.cm-line')].find(line =>
            line.textContent?.includes('"path"')
          )!
          const key = [...line.children].find(
            span => span.textContent === '"path"'
          )!
          const value = [...line.children].find(span =>
            span.textContent?.startsWith('/recorded')
          )!
          const range = document.createRange()
          range.setStart(key.firstChild!, 1)
          range.setEnd(key.firstChild!, 2)
          const anchor = range.getBoundingClientRect()
          const rects = [...value.getClientRects()]
          return (
            rects.length > 1 &&
            rects
              .filter(rect => rect.y > anchor.y + 1)
              .every(rect => Math.abs(rect.x - anchor.x) < 1) &&
            rects.every(
              rect => rect.right <= line.getBoundingClientRect().right + 1
            )
          )
        })
      )
      .toBe(true)
    const words = editor.locator('.cm-line').filter({ hasText: '"message"' })
    await expect
      .poll(() =>
        words.evaluate(line => {
          const value = [...line.children].find(span =>
            span.textContent?.startsWith('ordinary')
          )!
          const node = value.firstChild!
          return [...node.textContent!.matchAll(/[a-z]+/g)].every(word => {
            const range = document.createRange()
            range.setStart(node, word.index!)
            range.setEnd(node, word.index! + word[0].length)
            return range.getClientRects().length === 1
          })
        })
      )
      .toBe(true)
  }
  await editor.locator('.cm-content').focus()
  await page.keyboard.press('ControlOrMeta+f')
  const search = editor.locator('.cm-search input[name="search"]')
  await search.fill('getTraceView')
  await search.press('Enter')
  await expect(editor.locator('.cm-searchMatch')).toHaveText('getTraceView')
  await page.keyboard.press('Escape')
  const braces = bodyField.locator('.json-fold-bracket')
  await braces.nth(1).hover()
  await expect(bodyField.locator('.json-fold-bracket--active')).toHaveText([
    '{',
    '}',
  ])
  await expect(
    bodyField.locator(
      '.cm-foldGutter .json-fold-group-active .json-fold-marker'
    )
  ).toHaveCount(1)
  await bodyField.locator('.cm-foldGutter [title="Fold line"]').nth(1).hover()
  await expect(bodyField.locator('.json-fold-bracket--active')).toHaveText([
    '{',
    '}',
  ])
  await braces.nth(2).click()
  await expect(bodyField.locator('.cm-foldPlaceholder')).toHaveText('…')
  await expect(bodyField.locator('.cm-content')).not.toContainText('"path"')
  await braces.nth(1).click()
  await expect(bodyField.locator('.cm-content')).toContainText('"path"')
  await braces.first().hover()
  await expect(bodyField.locator('.json-fold-bracket--active')).toHaveText([
    '[',
    ']',
  ])
  await braces.first().click()
  await expect(bodyField.locator('.cm-foldPlaceholder')).toHaveText('…')
  await expect(bodyField.locator('.cm-content')).not.toContainText('"path"')
  await bodyField
    .locator('.cm-foldGutter [title="Unfold line"]:visible')
    .click()
  await expect(bodyField.locator('.cm-content')).toContainText('"path"')
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await bodyField
    .getByRole('button', { name: 'Copy body', exact: true })
    .click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    JSON.stringify(['loading', inner], null, 2)
  )
  const raw = JSON.stringify({ resourceLogs: [{ body }] })
  await page.route('**/export/logs/**', route =>
    route.fulfill({
      status: 200,
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': String(Buffer.byteLength(raw)),
      },
      body: route.request().method() === 'HEAD' ? '' : raw,
    })
  )
  const header = page.getByRole('region', { name: 'Log record', exact: true })
  const jsonStatus = header.getByRole('status', {
    name: /^OpenTelemetry Protocol JSON\./,
  })
  await header.getByRole('tab', { name: 'JSON', exact: true }).click()
  await expect(jsonStatus).toHaveText(/^OTLP JSON · /)
  await expect(header.locator('.pane-header__title-row')).toContainText('api')
  await expect(header.locator('.pane-header__title-row')).toContainText(
    'OTLP JSON'
  )
  await expect(page.locator('.raw-inspector__controls')).not.toContainText(
    'OTLP JSON'
  )
  await expect(
    page.getByRole('button', { name: 'Collapse JSON', exact: true })
  ).toBeEnabled()
  await header.getByRole('tab', { name: 'Properties', exact: true }).click()
  await expect(jsonStatus).toHaveCount(0)
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
  const valueColour = await page.evaluate(() => {
    const probe = document.createElement('span')
    probe.style.color = 'var(--color-base-content)'
    document.body.append(probe)
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  })
  await expect(service.getByRole('button')).toHaveCSS('color', valueColour)
  await expect(row.getByRole('gridcell').nth(1).getByRole('button')).toHaveCSS(
    'color',
    valueColour
  )

  const detailBody = page.locator('[data-field="body"]')
  await expect(detailBody).toContainText('9223372036854775807')
  const bodyHeader = detailBody.locator('.detail-property__header')
  const copyBody = bodyHeader.getByRole('button', {
    name: 'Copy body',
    exact: true,
  })
  const keyBounds = (await bodyHeader
    .locator('.detail-property__key')
    .boundingBox())!
  const copyBounds = (await copyBody.boundingBox())!
  const headerBounds = (await bodyHeader.boundingBox())!
  await expect(bodyHeader.locator('.detail-property__key')).toHaveText(
    'body · map'
  )
  await expect(bodyHeader.locator('.detail-property__badges')).toHaveCount(0)
  expect(copyBounds.x).toBeGreaterThan(keyBounds.x + keyBounds.width)
  expect(copyBounds.x + copyBounds.width).toBeCloseTo(
    headerBounds.x + headerBounds.width,
    0
  )
  // Copy is top-aligned with the first heading line (20px control, 21px text).
  expect(copyBounds.y).toBeCloseTo(keyBounds.y, 0)
  await expect(
    detailBody.locator('.detail-property__body button.value-copy')
  ).toHaveCount(0)
  await expect(detailBody.locator('.json-indent-guides')).toHaveCount(0)
  await expect(detailBody.locator('.json-indent-bands')).toHaveCount(0)
  await expect(detailBody.locator('.cm-editor')).toHaveCount(1)
  await expect(body.locator('.cm-editor')).toHaveCount(0)
  const bodyFold = detailBody
    .locator('.cm-foldGutter [title="Fold line"]')
    .nth(1)
  const bodyCenterOffset = await bodyFold.evaluate(element => {
    const icon = element.querySelector('svg')!.getBoundingClientRect()
    const editor = element.closest('.cm-editor')!
    const line = [...editor.querySelectorAll('.cm-line')].find(line =>
      line.textContent?.includes('"request"')
    )!
    const range = document.createRange()
    range.selectNodeContents(line)
    const text = range.getBoundingClientRect()
    return Math.abs(icon.y + icon.height / 2 - (text.y + text.height / 2))
  })
  expect(bodyCenterOffset).toBeLessThan(1.5)
  await expect
    .poll(() =>
      bodyFold.evaluate(
        element =>
          getComputedStyle(element.querySelector('.json-fold-icon')!).opacity
      )
    )
    .toBe('1')
  await bodyFold.hover()
  await expect
    .poll(() =>
      bodyFold.evaluate(
        element =>
          getComputedStyle(element.querySelector('.json-fold-icon')!).opacity
      )
    )
    .toBe('1')
  const bodyContent = detailBody.getByLabel('JSON value of body', {
    exact: true,
  })
  await expect(bodyContent).toHaveAttribute('contenteditable', 'false')
  await bodyContent.focus()
  await page.mouse.move(0, 0)
  await expect
    .poll(() =>
      bodyFold.evaluate(
        element =>
          getComputedStyle(element.querySelector('.json-fold-icon')!).opacity
      )
    )
    .toBe('1')
  await bodyFold.click()
  await expect(detailBody).not.toContainText('9223372036854775807')
  await expect(detailBody.locator('.cm-foldPlaceholder')).toHaveText('…')
  await detailBody.locator('.cm-foldPlaceholder').click()
  await expect(detailBody).toContainText('9223372036854775807')
  await bodyContent.focus()
  await page.keyboard.press('Control+Alt+[')
  await expect(detailBody.locator('.cm-foldPlaceholder')).toHaveText('…')
  await expect(detailBody).not.toContainText('9223372036854775807')
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await detailBody
    .getByRole('button', { name: 'Copy body', exact: true })
    .click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    '{\n  "message": "ready",\n  "request": {\n    "sequence": 9223372036854775807\n  }\n}'
  )
  await detailBody.locator('.cm-foldPlaceholder').click()
  await bodyContent.focus()
  await page.keyboard.insertText('MUTATED')
  await expect(bodyContent).not.toContainText('MUTATED')
  await expect(bodyContent).toContainText('9223372036854775807')
  await page.keyboard.press('ControlOrMeta+f')
  const searchInput = detailBody.locator('.cm-search input[name="search"]')
  await expect(searchInput).toBeVisible()
  await searchInput.fill('9223372036854775807')
  await searchInput.press('Enter')
  await expect(detailBody.locator('.cm-searchMatch')).toHaveText(
    '9223372036854775807'
  )
  await page.keyboard.press('Escape')
  await expect(searchInput).toHaveCount(0)

  const input = page.locator('[data-field="gen_ai.input.messages"]')
  await expect(input.locator('.json-indent-guides')).toHaveCount(0)
  await expect(input.locator('.json-indent-bands')).toHaveCount(0)
  await expect(input.locator('.json-fold-icon').first()).toHaveCSS(
    'opacity',
    '1'
  )
  await input.scrollIntoViewIfNeeded()
  await input.locator('.cm-foldGutter [title="Fold line"]').nth(1).click()
  await expect(input).not.toContainText('Explain the trace')
  await expect(input.locator('.cm-foldPlaceholder')).toHaveText('…')
  await input.locator('.cm-foldPlaceholder').click()
  await expect(input).toContainText('Explain the trace')
  const output = page.locator('[data-field="gen_ai.output.messages"]')
  await expect(output.locator('.json-indent-guides')).toHaveCount(0)
  await expect(output.locator('.json-indent-bands')).toHaveCount(0)
  await expect(output.locator('.json-fold-icon').first()).toHaveCSS(
    'opacity',
    '1'
  )
  await output.scrollIntoViewIfNeeded()
  await expect(output).toContainText('9007199254740993')
  await output.locator('.cm-foldGutter [title="Fold line"]').first().click()
  await expect(output).not.toContainText('9007199254740993')
  await expect(output.locator('.cm-foldPlaceholder')).toHaveText('…')
  await expect(row).toHaveAttribute('aria-selected', 'true')
})

test('Severity header sorts by the severity number rather than the received label', async ({
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
  const labels = page.locator('[data-log-ref] .log-severity__label')
  await expect(labels).toHaveCount(3)
  const heading = page.getByRole('columnheader').first()
  await expect(heading).toHaveAttribute('aria-sort', 'descending')
  await expect(labels).toHaveText(['MMM (17)', 'AAA (10)', 'ZZZ (2)'])
  expect(sorts.at(-1)).toEqual({ field: 'severity', direction: 'desc' })
  await heading
    .getByRole('button', { name: 'Sort by severity', exact: true })
    .click()
  await expect(heading).toHaveAttribute('aria-sort', 'ascending')
  await expect(labels).toHaveText(['ZZZ (2)', 'AAA (10)', 'MMM (17)'])
  expect(sorts.at(-1)).toEqual({ field: 'severity', direction: 'asc' })
  await heading
    .getByRole('button', { name: 'Sort by severity', exact: true })
    .click()
  await expect(heading).toHaveAttribute('aria-sort', 'descending')
  await expect(labels).toHaveText(['MMM (17)', 'AAA (10)', 'ZZZ (2)'])
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
  await expect(labels).toHaveText([`${longest} (17)`, 'UNSPECIFIED (0)'])
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
  await expect(labels.first()).toHaveText(`${longest} (17)`)
})

test('extreme severity labels have bounded width and retain their exact inspectable value', async ({
  page,
}) => {
  const label = 'VERY_LONG_ERROR_LABEL_'.repeat(25)
  await page.setViewportSize({ width: 1700, height: 900 })
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    if (method === 'getLog')
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: { ...logs[0]!, severityText: label },
        },
      })
    if (method !== 'searchLogPage') return route.fallback()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result: {
          total: 1,
          logs: [
            {
              logRef: firstID,
              timestamp: '1700000000123456789',
              severityNumber: 17,
              severityText: label,
              serviceName: 'api',
              bodyPreview: 'Retained body',
              bodyType: 'string',
            },
          ],
        },
      },
    })
  })
  await page.reload()
  const severity = page.locator(
    `[data-log-ref="${firstID}"] .log-table__pin-severity`
  )
  await expect(severity.locator('.log-severity__label')).toHaveText(
    `${label} (17)`
  )
  await expect.poll(async () => (await severity.boundingBox())!.width).toBe(320)
  const viewport = (await page.locator('.log-table__scroll').boundingBox())!
  expect((await severity.boundingBox())!.width).toBeLessThan(viewport.width / 2)
  await expect(severity.getByRole('button')).toHaveAttribute(
    'title',
    `${label} (17)`
  )
  await severity.getByRole('button').click()
  const detail = page.getByRole('table', { name: 'Log fields' })
  await expect(detail.locator('[data-field="severityNumber"]')).toHaveClass(
    /detail-property--revealed/
  )
  await expect(detail.locator('[data-field="severityText"]')).toContainText(
    label
  )
})

test('discovered columns retain typed values, provenance, drafts and saved selections', async ({
  page,
}) => {
  const values: { column: LogColumn; values: JsonAttributeValue[] }[] = [
    {
      column: { source: 'log', kind: 'field', key: 'flags' },
      values: [{ kind: 'int64', value: '0' }],
    },
    {
      column: { source: 'log', kind: 'attribute', key: 'flags' },
      values: [{ kind: 'int64', value: '9223372036854775807' }],
    },
    {
      column: { source: 'log', kind: 'attribute', key: 'timestamp' },
      values: [{ kind: 'string', value: 'received attribute' }],
    },
    {
      column: { source: 'resource', kind: 'attribute', key: 'flags' },
      values: [{ kind: 'bool', value: false }],
    },
    {
      column: { source: 'scope', kind: 'attribute', key: 'flags' },
      values: [{ kind: 'double', value: '0x8000000000000000' }],
    },
    {
      column: { source: 'scope', kind: 'field', key: 'name' },
      values: [{ kind: 'string', value: '' }],
    },
    {
      column: { source: 'resource', kind: 'field', key: 'schemaUrl' },
      values: [{ kind: 'string', value: 'https://example.test/schema' }],
    },
    {
      column: { source: 'log', kind: 'attribute', key: 'missing' },
      values: [],
    },
  ]
  const detail: JsonLogData = {
    ...logs[0]!,
    resourceSchemaURL: 'https://example.test/schema',
    attributes: values
      .filter(
        entry =>
          entry.column.source === 'log' && entry.column.kind === 'attribute'
      )
      .flatMap(entry =>
        entry.values.map(value => ({ key: entry.column.key, value }))
      ),
    resource: {
      ...logs[0]!.resource,
      attributes: [{ key: 'flags', value: { kind: 'bool', value: false } }],
    },
    scope: {
      ...logs[0]!.scope,
      attributes: [
        {
          key: 'flags',
          value: { kind: 'double', value: '0x8000000000000000' },
        },
      ],
    },
  }
  const requests: { columns?: LogColumn[]; query?: unknown }[] = []
  let discovery = true
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    let result
    if (method === 'getLog') result = detail
    else if (method === 'getLogAttributeDefinitions')
      result = discovery
        ? values.flatMap(entry =>
            entry.column.kind === 'attribute'
              ? [
                  {
                    name: entry.column.key,
                    attributeScope: entry.column.source,
                    type: 'string',
                  },
                ]
              : []
          )
        : []
    else if (method === 'searchLogPage') {
      requests.push(params)
      result = {
        total: 1,
        logs: [
          {
            ...detail,
            timestamp: detail.observedTimestamp,
            serviceName: 'api',
            bodyPreview: 'body preview',
            columnValues: (params.columns ?? []).map((column: LogColumn) => ({
              ...column,
              values: values.find(
                entry =>
                  entry.column.source === column.source &&
                  entry.column.kind === column.kind &&
                  entry.column.key === column.key
              )!.values,
            })),
          },
        ],
      }
    } else return route.fallback()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.reload()
  const editor = page.locator('.cm-content')
  await editor.fill('body CONTAINS "unsent draft"')
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  const picker = page.getByRole('group', { name: 'Log columns' })
  for (const entry of values) {
    const { source, kind, key } = entry.column
    const label = `${source[0]!.toUpperCase()}${source.slice(1)} ${kind}: ${key}`
    await picker.getByRole('checkbox', { name: label, exact: true }).check()
  }
  await page.keyboard.press('Escape')
  const row = page.locator(`[data-log-ref="${firstID}"]`)
  for (const [name, text] of [
    ['log field: flags', '0'],
    ['log attribute: flags', '9223372036854775807'],
    ['resource attribute: flags', 'false'],
    ['scope attribute: flags', '-0'],
    ['scope field: name', '""'],
    ['log attribute: missing', '—'],
  ])
    await expect(
      row.getByRole('button', { name: `Show ${name} in details`, exact: true })
    ).toHaveText(text!)
  await expect.poll(() => requests.at(-1)?.columns?.length).toBe(values.length)
  expect(requests.at(-1)?.query).toBeUndefined()
  await expect(editor).toHaveText('body CONTAINS "unsent draft"')
  await expect(
    page.getByRole('button', { name: 'Sort by flags', exact: true })
  ).toHaveCount(0)
  for (const { column } of values.filter(entry => entry.values.length)) {
    const { source, kind, key } = column
    await row
      .getByRole('button', {
        name: `Show ${source} ${kind}: ${key} in details`,
        exact: true,
      })
      .click()
    const target = page
      .getByRole('table', { name: 'Log fields' })
      .locator(
        `[data-field="${key}"][data-field-kind="${kind}"]${source === 'log' ? ':not([data-field-source="resource"]):not([data-field-source="scope"])' : `[data-field-source="${source}"]`}`
      )
    await expect(target).toHaveClass(/detail-property--revealed/)
    await expect(page).toHaveURL(new RegExp(`/logs/${firstID}`))
  }
  // Stored descriptors survive even if discovery no longer returns their keys.
  discovery = false
  await page.reload()
  await expect(
    row.getByRole('button', {
      name: 'Show log attribute: flags in details',
      exact: true,
    })
  ).toHaveText('9223372036854775807')
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await expect(
    picker.getByRole('checkbox', {
      name: 'Scope attribute: flags',
      exact: true,
    })
  ).toBeChecked()
})

test('toolbar filters draft service and severity selections and search once on Apply', async ({
  page,
}) => {
  const editor = page.locator('.cm-content')
  const service = page.getByRole('button', {
    name: 'Service api',
    exact: true,
  })
  const error = page.getByRole('button', {
    name: 'Severity Error',
    exact: true,
  })
  const filtersToggle = page.getByRole('button', {
    name: 'Filters',
    exact: true,
  })
  let searches = 0
  page.on('request', request => {
    if (
      request.url().endsWith('/rpc') &&
      request.postDataJSON().method === 'searchLogPage'
    )
      searches++
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
  await service.click()
  await expect(filtersToggle).toHaveAttribute('aria-expanded', 'true')
  await expect(service).toHaveAttribute('aria-pressed', 'true')
  await expect(service).toHaveClass(/item-state/)
  await expect(service.locator('.log-filters__dot')).toBeVisible()
  await error.click()
  await expect(error).toHaveClass(/item-state/)
  await expect(error.locator('.log-filters__dot')).toBeVisible()
  await expect(error).toHaveAttribute('aria-pressed', 'true')
  await expect(editor.locator('.cm-placeholder')).toBeVisible()
  expect(searches).toBe(0)
  await page.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect.poll(() => searches).toBe(1)
  await expect(filtersToggle).toHaveAttribute('aria-expanded', 'false')
  await expect(filtersToggle).toBeFocused()
  await expect(filtersToggle).toContainText('(2)')
  await expect(editor).toContainText(
    'attr(resource, "service.name", string) IN ["api"]'
  )
  await expect(editor).toContainText(
    'severityNumber >= 17 AND severityNumber <= 20'
  )
  await filtersToggle.click()
  await service.click()
  await expect(service).toHaveAttribute('aria-pressed', 'false')
  await expect(error).toHaveAttribute('aria-pressed', 'true')
  await page.keyboard.press('Escape')
  await expect(editor).toContainText('service.name')
  expect(searches).toBe(1)
  await filtersToggle.click()
  await expect(service).toHaveAttribute('aria-pressed', 'true')
  await page.getByRole('button', { name: 'Clear filters', exact: true }).click()
  await expect(service).toHaveAttribute('aria-pressed', 'false')
  await expect(error).toHaveAttribute('aria-pressed', 'false')
  await page.keyboard.press('Escape')
  await filtersToggle.click()
  await expect(service).toHaveAttribute('aria-pressed', 'true')
  expect(searches).toBe(1)
  await editor.click()
  await editor.fill('severityNumber =')
  await filtersToggle.click()
  await expect(error).toBeDisabled()
  await expect(editor).toHaveText('severityNumber =')
  await editor.click()
  await editor.fill('')
  await filtersToggle.click()
  await expect(error).toHaveAttribute('aria-pressed', 'false')
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
  await service.click()
  await page.getByRole('button', { name: 'Apply', exact: true }).click()
  expect((await search).postDataJSON().params.query).toBeTruthy()
  await expect(editor).toContainText('body CONTAINS "exact"')
  await expect(editor).toContainText('| LIMIT 10')
  await expect(page).toHaveURL(new RegExp(`/logs/${firstID}`))
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
  const range = page
    .getByRole('region', { name: 'Log table summary' })
    .locator('.badge-count')
  await expect(range).toContainText('1–100 of 235 records')
  await expect(page.getByRole('status', { name: 'Log count' })).toHaveText(
    '235 logs'
  )
  await expect(
    page.getByRole('button', { name: 'First page', exact: true })
  ).toBeDisabled()
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
  await page.getByRole('button', { name: 'Last page', exact: true }).click()
  await expect(range).toContainText('201–235 of 235 records')
  await expect(
    page.getByRole('button', { name: 'Last page', exact: true })
  ).toBeDisabled()
  await page.getByRole('button', { name: 'First page', exact: true }).click()
  await expect(range).toContainText('1–100 of 235 records')
  await page.getByRole('button', { name: 'Last page', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Next page', exact: true })
  ).toBeDisabled()
  await page
    .getByRole('button', { name: 'Sort by timestamp', exact: true })
    .click()
  await expect(range).toContainText('1–100 of 235 records')
  expect(requests.at(-1)).toMatchObject({
    offset: 0,
    sort: { field: 'timestamp', direction: 'desc' },
  })
  await page.getByRole('button', { name: 'Page 2', exact: true }).click()
  await expect(range).toContainText('101–200')
  await page.getByRole('combobox', { name: 'Logs per page' }).selectOption('50')
  await expect(range).toContainText('1–50 of 235 records')
  expect(requests.at(-1)).toMatchObject({ offset: 0, pageSize: 50 })
  await page.getByRole('button', { name: 'Page 2', exact: true }).click()
  await expect(range).toContainText('51–100')
  await page.getByRole('combobox', { name: 'Logs per page' }).selectOption('25')
  await expect(range).toContainText('1–25 of 235 records')
  expect(requests.at(-1)).toMatchObject({ offset: 0, pageSize: 25 })
  await page.getByRole('button', { name: 'Page 2', exact: true }).click()
  await expect(range).toContainText('26–50 of 235 records')
  expect(requests.at(-1)).toMatchObject({ offset: 25, pageSize: 25 })
  await expect(detail).toContainText('Full body 0: exact retained text')
  await page.getByRole('button', { name: 'Filters', exact: true }).click()
  await page.getByRole('button', { name: 'Service api', exact: true }).click()
  await expect(range).toContainText('26–50 of 235 records')
  await page.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(range).toContainText('1–7 of 7 records')
  expect(requests.at(-1)).toMatchObject({ offset: 0, pageSize: 25 })
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

test('reload restores resized columns against the measured wide viewport', async ({
  page,
}) => {
  await page.setViewportSize({ width: 2400, height: 1000 })
  const viewport = page.locator('.log-table__scroll')
  await expect
    .poll(() => viewport.evaluate(element => element.clientWidth))
    .toBeGreaterThan(1200)
  for (const name of ['timestamp', 'service.name']) {
    const separator = page.getByRole('separator', {
      name: `Resize ${name} column`,
      exact: true,
    })
    await separator.focus()
    await page.keyboard.press('Shift+ArrowRight')
    await page.keyboard.press('ArrowRight')
  }
  const saved = await page.evaluate(() =>
    JSON.parse(localStorage.getItem('logs-column-widths')!)
  )
  expect(saved).toMatchObject({ severity: 160, timestamp: 320, service: 240 })
  const cells = page.locator(`[data-log-ref="${firstID}"] [role="gridcell"]`)
  const expected = ['severity', 'timestamp', 'service', 'body'].map(
    column => saved[column]
  )
  await expect
    .poll(() =>
      cells.evaluateAll(elements =>
        elements.map(element => element.getBoundingClientRect().width)
      )
    )
    .toEqual(expected)
  await page.reload()
  await expect
    .poll(() =>
      cells.evaluateAll(elements =>
        elements.map(element => element.getBoundingClientRect().width)
      )
    )
    .toEqual(expected)
})

test('short resize grips align with header boundaries and follow pointer resizing', async ({
  page,
}) => {
  const guide = page.getByRole('separator', {
    name: 'Resize timestamp column',
    exact: true,
  })
  const heading = page
    .getByRole('columnheader')
    .filter({ hasText: 'timestamp' })
  const grip = guide.locator('.col-resize-bar__line')
  async function aligned() {
    const bar = (await guide.boundingBox())!
    const cell = (await heading.boundingBox())!
    const painted = (await grip.boundingBox())!
    expect(
      Math.abs(painted.x + painted.width / 2 - (cell.x + cell.width))
    ).toBeLessThan(1)
    expect(Math.abs(bar.y - cell.y)).toBeLessThan(1)
    expect(painted.height).toBe(48)
    expect(painted.width).toBe(3)
    expect(bar.height).toBeGreaterThan(painted.height)
    expect(
      await guide.evaluate(
        element => getComputedStyle(element, '::before').content
      )
    ).toBe('none')
    return bar
  }
  await expect(grip).toHaveCSS('opacity', '0')
  const before = await aligned()
  await page.mouse.move(before.x + before.width / 2, before.y + 20)
  await expect(guide.locator('.col-resize-bar__line')).toHaveCSS('opacity', '1')
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
  await page.setViewportSize({ width: 1360, height: 900 })
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page
    .getByRole('checkbox', { name: 'Log: eventName', exact: true })
    .check()
  await page.keyboard.press('Escape')
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

test('narrow panes expose every column by scrolling instead of overlapping pinned cells', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1024, height: 900 })
  const preview = 'id: abc123-END'
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    if (method !== 'searchLogPage') return route.fallback()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result: {
          total: 1,
          logs: [
            {
              logRef: firstID,
              timestamp: '1700000000123456789',
              severityNumber: 17,
              severityText: 'ERROR',
              serviceName: 'api-service',
              bodyPreview: preview,
              bodyType: 'string',
            },
          ],
        },
      },
    })
  })
  await page.reload()
  const viewport = page.locator('.log-table__scroll')
  await expect
    .poll(() => viewport.evaluate(element => element.clientWidth))
    .toBeLessThan(640)
  const cells = page.locator(`[data-log-ref="${firstID}"] [role="gridcell"]`)
  for (const index of [2, 3, 1, 0]) {
    const cell = cells.nth(index)
    await cell.scrollIntoViewIfNeeded()
    await expect
      .poll(() =>
        cell.evaluate(element => {
          const bounds = element.getBoundingClientRect()
          const viewport = element
            .closest('.log-table__scroll')!
            .getBoundingClientRect()
          const y = bounds.y + bounds.height / 2
          return (
            bounds.left >= viewport.left &&
            bounds.right <= viewport.right &&
            [bounds.left + 8, bounds.right - 8].every(x =>
              element.contains(document.elementFromPoint(x, y))
            )
          )
        })
      )
      .toBe(true)
    expect(
      await cell
        .getByRole('button')
        .evaluate(element => element.scrollWidth <= element.clientWidth)
    ).toBe(true)
  }
  await expect(cells.nth(3).getByRole('button')).toHaveText(preview)
  await cells.nth(2).getByRole('button').click()
  await expect(
    page
      .getByRole('table', { name: 'Log fields' })
      .locator('[data-field="service.name"][data-field-source="resource"]')
  ).toHaveClass(/detail-property--revealed/)
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
  await page.getByRole('button', { name: 'Select logs', exact: true }).click()
  await contained()
  await page.setViewportSize({ width: 1000, height: 800 })
  await contained()
})

test('explicit selection mode supports all, some and none and deletes only checked logs', async ({
  page,
}) => {
  const calls: string[][] = []
  let fail = true
  let deleted = false
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'deleteLogsByRefs') {
      calls.push(params)
      if (fail) {
        fail = false
        return route.fulfill({
          json: {
            jsonrpc: '2.0',
            id,
            error: { code: -32000, message: 'Delete failed for this test' },
          },
        })
      }
      deleted = true
      return route.fulfill({
        json: { jsonrpc: '2.0', id, result: { count: params.length } },
      })
    }
    if (method === 'searchLogPage' && deleted)
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: {
            total: 1,
            logs: [
              {
                logRef: firstID,
                timestamp: logs[0].observedTimestamp,
                severityText: 'ERROR',
                severityNumber: 17,
                serviceName: 'api',
                bodyPreview: 'Retained first log',
                bodyType: 'string',
              },
            ],
          },
        },
      })
    return route.fallback()
  })
  const grid = page.getByRole('grid', { name: 'Logs', exact: true })
  const mode = page.getByRole('button', { name: 'Select logs', exact: true })
  const all = grid.getByRole('checkbox', {
    name: 'Select all logs on this page',
  })
  const first = grid.getByRole('checkbox', {
    name: `Select log ${firstID}`,
    exact: true,
  })
  const second = grid.getByRole('checkbox', {
    name: `Select log ${secondID}`,
    exact: true,
  })
  await expect(grid.getByRole('checkbox')).toHaveCount(0)
  await mode.click()
  await expect(all).not.toBeChecked()
  await all.check()
  await expect(first).toBeChecked()
  await expect(second).toBeChecked()
  await all.uncheck()
  await expect(first).not.toBeChecked()
  await expect(second).not.toBeChecked()
  await all.check()
  await first.uncheck()
  await expect(all).toBeChecked({ indeterminate: true })
  expect(page.url()).toContain(`/logs/${firstID}`)
  await mode.click()
  await expect(grid.getByRole('checkbox')).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: /Delete \d+ selected/ })
  ).toHaveCount(0)
  await mode.click()
  await expect(first).not.toBeChecked()
  await expect(second).not.toBeChecked()
  await second.check()
  const remove = page.getByRole('button', {
    name: 'Delete 1 selected log',
    exact: true,
  })
  await remove.click()
  await expect(page.getByText(/Delete failed for this test/)).toBeVisible()
  await expect(second).toBeChecked()
  await remove.click()
  await expect(grid.locator(`[data-log-ref="${secondID}"]`)).toHaveCount(0)
  await expect(grid.locator(`[data-log-ref="${firstID}"]`)).toBeVisible()
  expect(calls).toEqual([[secondID], [secondID]])
  expect(page.url()).toContain(`/logs/${firstID}`)
  await expect(
    page.getByRole('button', { name: 'Delete 0 selected logs', exact: true })
  ).toBeDisabled()
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
  const detail = page.locator('.log-detail-panel')
  const recordFooter = detail.locator('.signal-footer')
  const remove = recordFooter.getByRole('button', {
    name: 'Delete this log',
    exact: true,
  })
  await expect(remove).toBeEnabled()
  await expect(recordFooter.getByRole('navigation')).toHaveCount(0)
  const listFooter = page
    .getByRole('navigation', { name: 'Log pages' })
    .locator('..')
  await expect(
    listFooter.getByRole('button', { name: 'Delete this log', exact: true })
  ).toHaveCount(0)
  const panelBounds = (await detail.boundingBox())!
  const footerBounds = (await recordFooter.boundingBox())!
  const deleteBounds = (await remove.boundingBox())!
  expect(footerBounds.y + footerBounds.height).toBeCloseTo(
    panelBounds.y + panelBounds.height,
    0
  )
  expect(footerBounds.width).toBeCloseTo(panelBounds.width, 0)
  expect(
    panelBounds.x + panelBounds.width - deleteBounds.x - deleteBounds.width
  ).toBeCloseTo(16, 0)
  await remove.click()
  await expect.poll(() => deleted).toBe(true)
  await expect(page.locator(`[data-log-ref="${firstID}"]`)).toHaveCount(0)
  await expect(page.locator(`[data-log-ref="${secondID}"]`)).toBeVisible()
  expect(cleared).toBe(false)
  page.once('dialog', async dialog => {
    expect(dialog.message()).toContain(
      'including those outside the current filters'
    )
    await dialog.dismiss()
  })
  await page
    .getByRole('button', { name: 'Delete all logs', exact: true })
    .click()
  expect(cleared).toBe(false)
  await expect(page.locator(`[data-log-ref="${secondID}"]`)).toBeVisible()
  page.once('dialog', dialog => dialog.accept())
  await page
    .getByRole('button', { name: 'Delete all logs', exact: true })
    .click()
  await expect.poll(() => cleared).toBe(true)
  await expect(page.locator('[data-log-ref]')).toHaveCount(0)
  await expect(page.getByRole('status', { name: 'Log count' })).toContainText(
    '0 logs'
  )
})

test('header error navigation crosses pages within the current result', async ({
  page,
}) => {
  let pageResponse: Promise<void> | undefined
  const records = Array.from({ length: 205 }, (_, index) => ({
    logRef:
      index === 0
        ? firstID
        : `00000000-0000-0000-0002-${String(index).padStart(12, '0')}`,
    timestamp: String(1700000000000000000n + BigInt(index)),
    severityText: [1, 180, 201].includes(index) ? 'ERROR' : 'INFO',
    severityNumber: [1, 180, 201].includes(index) ? 17 : 9,
    serviceName: 'api',
    bodyPreview: `Record ${index + 1}`,
    bodyType: 'string',
  }))
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    let result:
      { total: number; logs: typeof records } | JsonLogData | LogErrorNavigation
    if (method === 'searchLogPage') {
      await pageResponse
      result = {
        total: records.length,
        logs: records.slice(params.offset, params.offset + params.pageSize),
      }
    } else if (method === 'getLog')
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
  await expect(
    header
      .getByRole('group', { name: 'Error navigation' })
      .locator('[aria-live]')
  ).toHaveText('err')
  await expect(
    header.getByRole('button', { name: 'Previous error', exact: true })
  ).toBeDisabled()
  await header.getByRole('button', { name: 'Next error', exact: true }).click()
  await expect(header).toContainText('err (1/3)')
  const nextError = header.getByRole('button', {
    name: 'Next error',
    exact: true,
  })
  await nextError.focus()
  await page.keyboard.press('Enter')
  await expect(
    page.getByRole('button', { name: 'Page 2', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await expect(header).toContainText('err (2/3)')
  await expect(nextError).toBeFocused()
  const selected = page.locator(`[data-log-ref="${records[180]!.logRef}"]`)
  await expect(selected).toHaveAttribute('aria-selected', 'true')
  const viewport = page.locator('.log-table__scroll')
  async function expectSelectedInViewport() {
    await expect
      .poll(() =>
        selected.evaluate(element => {
          const viewport = element.closest('.log-table__scroll')!
          const bounds = viewport.getBoundingClientRect()
          const header = viewport.querySelector('.log-table__header')!
          const row = element.getBoundingClientRect()
          return (
            row.top >= header.getBoundingClientRect().bottom &&
            row.bottom <= bounds.bottom
          )
        })
      )
      .toBe(true)
  }
  await expectSelectedInViewport()
  await page.keyboard.press('Enter')
  await expect(
    page.getByRole('button', { name: 'Page 3', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await expect(header).toContainText('err (3/3)')
  await expect(
    header.getByRole('button', { name: 'Next error', exact: true })
  ).toBeDisabled()
  let releasePage!: () => void
  pageResponse = new Promise(resolve => (releasePage = resolve))
  await header
    .getByRole('button', { name: 'Previous error', exact: true })
    .click()
  await expect(
    page.getByRole('grid', { name: 'Logs', exact: true })
  ).toHaveAttribute('aria-busy', 'true')
  const columns = header.getByRole('button', { name: 'Columns', exact: true })
  await columns.focus()
  releasePage()
  await expect(header).toContainText('err (2/3)')
  await expect(columns).toBeFocused()
  await expect(
    page.getByRole('button', { name: 'Page 2', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await expectSelectedInViewport()
  // Browsing away and back retains Details, but ordinary pagination starts
  // at the top even when the selected error is near the bottom of that page.
  await page.getByRole('button', { name: 'Page 1', exact: true }).click()
  await expect(page.locator('[data-log-ref]').first()).toHaveAttribute(
    'data-log-ref',
    firstID
  )
  await expect
    .poll(() => viewport.evaluate(element => element.scrollTop))
    .toBe(0)
  await page.getByRole('button', { name: 'Page 2', exact: true }).click()
  await expect(selected).toHaveAttribute('aria-selected', 'true')
  await expect
    .poll(() => viewport.evaluate(element => element.scrollTop))
    .toBe(0)
  // A same-selection rerender must not pull the viewport back to the error.
  await viewport.evaluate(element => (element.scrollTop = 320))
  const refreshed = page.waitForResponse(
    response =>
      response.url().endsWith('/rpc') &&
      response.request().postDataJSON().method === 'searchLogPage'
  )
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await refreshed
  await expect(
    page.getByRole('grid', { name: 'Logs', exact: true })
  ).toHaveAttribute('aria-busy', 'false')
  await expect(header).toContainText('err (2/3)')
  await expect(selected).toHaveAttribute('aria-selected', 'true')
  await expect
    .poll(() => viewport.evaluate(element => element.scrollTop))
    .toBe(320)
  await expect(
    header.getByRole('button', { name: 'Filters', exact: true })
  ).toBeVisible()
  await expect(
    header.getByRole('button', { name: 'Columns', exact: true })
  ).toBeVisible()
})
