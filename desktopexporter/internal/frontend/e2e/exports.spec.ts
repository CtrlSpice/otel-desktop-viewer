import { routeTelemetry } from './telemetry-fixtures'
import { readFile } from 'node:fs/promises'
import { expect, test } from '@playwright/test'
import type { JsonLogData } from '../src/types/wire-types'

const logRef = '00000000-0000-0000-0000-000000000001'
const log: JsonLogData = {
  logRef,
  timestamp: '1700000000000000000',
  observedTimestamp: '1700000000000000000',
  traceID: null,
  spanID: null,
  severityText: 'INFO',
  severityNumber: 9,
  body: { kind: 'double', value: '0x8000000000000000' },
  resource: {
    attributes: [
      { key: 'service.name', value: { kind: 'string', value: 'checkout-api' } },
    ],
    droppedAttributesCount: 0,
  },
  scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
  attributes: [],
  droppedAttributesCount: 0,
  flags: 0,
  eventName: '',
}

function rpcFixture(method: string) {
  if (method === 'getLog') return log
  if (method === 'getLogErrorNavigation')
    return { count: 0, index: null, previous: null, next: null }
  if (method === 'searchLogSummaries' || method === 'searchLogPage') {
    const logs = [
      {
        logRef,
        timestamp: log.timestamp,
        serviceName: 'checkout-api',
        severityText: 'INFO',
        severityNumber: 9,
        bodyPreview: '-0',
      },
    ]
    return method === 'searchLogPage' ? { total: logs.length, logs } : logs
  }
  if (method === 'getStats') {
    return {
      storage: { sizeBytes: 0, maxSizeBytes: 0 },
      traces: {
        traceCount: 0,
        spanCount: 0,
        serviceCount: 0,
        errorCount: 0,
        lastReceived: null,
      },
      logs: { logCount: 1, errorCount: 0, lastReceived: null },
      metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
      rejections: [],
    }
  }
  return []
}

test.beforeEach(async ({ page }) => {
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        jsonrpc: '2.0',
        id: request.id,
        result: rpcFixture(request.method),
      }),
    })
  })
  await page.goto(`/logs/${logRef}`)
  await expect(page.getByRole('button', { name: 'Download log' })).toBeVisible()
  await expect(
    page.getByRole('region', { name: 'Log record', exact: true })
  ).toHaveCSS('border-radius', '12px')
  const downloadLayout = await page
    .getByRole('region', { name: 'Log record', exact: true })
    .evaluate(header => {
      const bounds = header.getBoundingClientRect()
      const download = header
        .querySelector('[aria-label="Download log"]')!
        .getBoundingClientRect()
      const copy = header
        .querySelector('[aria-label="Copy log"]')!
        .getBoundingClientRect()
      return {
        inset: bounds.right - download.right,
        padding: parseFloat(getComputedStyle(header).paddingRight),
        afterCopy: download.left >= copy.right,
        centred:
          Math.abs(
            bounds.top + bounds.height / 2 - download.top - download.height / 2
          ) < 1,
      }
    })
  // The shared single-row header places actions beside the title sub-row.
  expect(downloadLayout.padding).toBe(16)
  expect(downloadLayout.inset).toBeCloseTo(downloadLayout.padding, 0)
  expect(downloadLayout.afterCopy).toBe(true)
  expect(downloadLayout.centred).toBe(true)
  await expect(
    page
      .getByRole('region', { name: 'Log record', exact: true })
      .getByRole('tab', { name: 'Properties', exact: true })
  ).toHaveAttribute('aria-selected', 'true')
  const darkest = await page.evaluate(() => {
    const probe = document.createElement('span')
    probe.style.backgroundColor = 'var(--color-base-100)'
    document.body.append(probe)
    const color = getComputedStyle(probe).backgroundColor
    probe.remove()
    return color
  })
  await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
    'background-color',
    darkest
  )
  await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
    'border-width',
    '0px'
  )
})

test('downloads exact JSON bytes with one activation from the signal header', async ({
  page,
}) => {
  const body = Buffer.from(
    '{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"doubleValue":-0.0}}]}]}]}\n'
  )
  let requestCount = 0
  let releaseResponse!: () => void
  const responseReady = new Promise<void>(resolve => {
    releaseResponse = resolve
  })
  await page.route(`**/export/logs/${logRef}`, async route => {
    requestCount++
    await responseReady
    await route.fulfill({
      contentType: 'application/json',
      body,
    })
  })
  const trigger = page.getByRole('button', { name: 'Download log' })
  const style = await trigger.evaluate(element => ({
    radius: parseFloat(getComputedStyle(element).borderTopLeftRadius),
    width: element.getBoundingClientRect().width,
  }))
  expect(style.radius).toBeGreaterThanOrEqual(style.width / 2)
  await trigger.focus()
  const downloaded = page.waitForEvent('download')
  await page.keyboard.press('Enter')
  try {
    await expect(trigger).toHaveAttribute('aria-busy', 'true')
    await expect(trigger).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(page.getByRole('menu')).toHaveCount(0)
  } finally {
    releaseResponse()
  }
  const download = await downloaded
  expect(download.suggestedFilename()).toBe(`log-${logRef}.json`)
  const path = await download.path()
  expect(path).not.toBeNull()
  expect(await readFile(path!)).toEqual(body)
  await expect(trigger).toHaveAttribute('aria-busy', 'false')
  await expect(trigger).toBeFocused()
  expect(requestCount).toBe(1)
})

test('uses the shared tooltip style on hover and keyboard focus without clipping', async ({
  page,
}) => {
  const trigger = page.getByRole('button', {
    name: 'Download log',
    exact: true,
  })
  const buttonBox = (await trigger.boundingBox())!
  const titleBox = (await page
    .getByRole('region', { name: 'Log record', exact: true })
    .getByText('Log', { exact: true })
    .boundingBox())!
  expect(buttonBox.x).toBeGreaterThan(titleBox.x + titleBox.width)
  await expect(trigger).toHaveAttribute('data-tip', 'Download log')
  await expect(trigger).not.toHaveAttribute('title')
  await expect(trigger.locator('svg')).toHaveAttribute('aria-hidden', 'true')
  await trigger.hover()
  await expect
    .poll(() =>
      trigger.evaluate(button => getComputedStyle(button, '::before').opacity)
    )
    .toBe('1')
  expect(
    await trigger.evaluate(
      button => getComputedStyle(button.closest('.pane-header')!).overflowY
    )
  ).toBe('visible')
  await page.mouse.move(0, 0)
  await trigger.focus()
  await page.keyboard.press('Tab')
  await page.keyboard.press('Shift+Tab')
  await expect(trigger).toBeFocused()
  await expect
    .poll(() =>
      trigger.evaluate(button => getComputedStyle(button, '::before').opacity)
    )
    .toBe('1')
  await page.keyboard.press('Escape')
  await expect(trigger).toBeFocused()
  await expect
    .poll(() =>
      trigger.evaluate(button => getComputedStyle(button, '::before').opacity)
    )
    .toBe('0')
})

test('shows an export failure instead of downloading an error document', async ({
  page,
}) => {
  await page.route('**/export/logs/**', route =>
    route.fulfill({
      status: 404,
      contentType: 'text/plain',
      body: 'export record not found',
    })
  )
  const downloads: string[] = []
  page.on('download', download => downloads.push(download.suggestedFilename()))
  const trigger = page.getByRole('button', { name: 'Download log' })
  await trigger.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('alert')).toContainText('export record not found')
  await expect(trigger).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('alert')).toContainText('export record not found')
  expect(downloads).toEqual([])
})

test('export completion does not take focus back after the user moves it', async ({
  page,
}) => {
  let releaseResponse!: () => void
  const responseReady = new Promise<void>(resolve => {
    releaseResponse = resolve
  })
  await page.route('**/export/logs/**', async route => {
    await responseReady
    await route.fulfill({ contentType: 'application/json', body: '{}' })
  })
  const trigger = page.getByRole('button', { name: 'Download log' })
  await trigger.focus()
  const downloaded = page.waitForEvent('download')
  await page.keyboard.press('Enter')
  const elsewhere = page.getByRole('link', { name: /^Traces(?: \[|$)/ })
  try {
    await expect(trigger).toHaveAttribute('aria-busy', 'true')
    await elsewhere.focus()
  } finally {
    releaseResponse()
  }
  await downloaded
  await expect(trigger).toHaveAttribute('aria-busy', 'false')
  await expect(elsewhere).toBeFocused()
})
