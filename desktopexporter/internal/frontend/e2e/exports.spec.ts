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
    await page.route(`**/export/logs/${logRef}?format=${format}`, route =>
      route.fulfill({
        contentType:
          format === 'json' ? 'application/json' : 'application/x-protobuf',
        body,
      })
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
    const download = await downloaded
    expect(download.suggestedFilename()).toBe(
      `log-${logRef}.${format === 'json' ? 'json' : 'pb'}`
    )
    const path = await download.path()
    expect(path).not.toBeNull()
    expect(await readFile(path!)).toEqual(body)
  })
}

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
  await page.getByRole('button', { name: 'Export log' }).click()
  await page.getByRole('menuitem', { name: 'OTLP JSON' }).click()
  await expect(page.getByRole('alert')).toContainText('export record not found')
  expect(downloads).toEqual([])
})
