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
  resource: { attributes: [], droppedAttributesCount: 0 },
  scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
  attributes: [],
  droppedAttributesCount: 0,
  flags: 0,
  eventName: '',
}

function rpcFixture(method: string) {
  if (method === 'getLog') return log
  if (method === 'searchLogSummaries') {
    return [
      {
        logRef,
        timestamp: log.timestamp,
        serviceName: null,
        severityText: 'INFO',
        severityNumber: 9,
        bodyPreview: '-0',
      },
    ]
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
  await page.route('**/rpc', async route => {
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
  await expect(page.getByRole('button', { name: 'Export log' })).toBeVisible()
})

for (const format of ['json', 'protobuf'] as const) {
  test(`downloads exact ${format} bytes from the signal header`, async ({
    page,
  }) => {
    const body =
      format === 'json'
        ? Buffer.from(
            '{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"doubleValue":-0.0}}]}]}]}\n'
          )
        : Buffer.from([0x0a, 0x04, 0x12, 0x02, 0x12, 0x00])
    let releaseResponse!: () => void
    const responseReady = new Promise<void>(resolve => {
      releaseResponse = resolve
    })
    await page.route(
      `**/export/logs/${logRef}?format=${format}`,
      async route => {
        await responseReady
        await route.fulfill({
          contentType:
            format === 'json' ? 'application/json' : 'application/x-protobuf',
          body,
        })
      }
    )
    const trigger = page.getByRole('button', { name: 'Export log' })
    const style = await trigger.evaluate(element => ({
      radius: parseFloat(getComputedStyle(element).borderTopLeftRadius),
      width: element.getBoundingClientRect().width,
    }))
    expect(style.radius).toBeGreaterThanOrEqual(style.width / 2)
    await trigger.focus()
    await page.keyboard.press('Enter')
    await expect(
      page.getByRole('menuitem', { name: 'OTLP JSON' })
    ).toBeFocused()
    if (format === 'protobuf') await page.keyboard.press('ArrowDown')
    const downloaded = page.waitForEvent('download')
    await page.keyboard.press('Enter')
    try {
      await expect(trigger).toHaveAttribute('aria-busy', 'true')
      await expect(trigger).toBeFocused()
      await page.keyboard.press('Enter')
      await expect(page.getByRole('menu')).not.toBeVisible()
    } finally {
      releaseResponse()
    }
    const download = await downloaded
    expect(download.suggestedFilename()).toBe(
      `log-${logRef}.${format === 'json' ? 'json' : 'pb'}`
    )
    const path = await download.path()
    expect(path).not.toBeNull()
    expect(await readFile(path!)).toEqual(body)
    await expect(trigger).toHaveAttribute('aria-busy', 'false')
    await expect(trigger).toBeFocused()
  })
}

test('uses the shared tooltip style on hover and keyboard focus without clipping', async ({
  page,
}) => {
  const trigger = page.getByRole('button', { name: 'Export log', exact: true })
  await expect(trigger).toHaveAttribute('data-tip', 'Export log')
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
  const trigger = page.getByRole('button', { name: 'Export log' })
  await trigger.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('menuitem', { name: 'OTLP JSON' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('alert')).toContainText('export record not found')
  await expect(trigger).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('menuitem', { name: 'OTLP JSON' })).toBeFocused()
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
  const trigger = page.getByRole('button', { name: 'Export log' })
  await trigger.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('menuitem', { name: 'OTLP JSON' })).toBeFocused()
  const downloaded = page.waitForEvent('download')
  await page.keyboard.press('Enter')
  const elsewhere = page.getByRole('link', { name: 'Traces', exact: true })
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
