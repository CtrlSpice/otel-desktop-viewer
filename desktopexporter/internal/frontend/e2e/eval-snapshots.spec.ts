import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import type { EvalSnapshot } from '../src/services/telemetry-service'

type MockRPCResult = Record<string, unknown> | never[] | undefined

const id = 'a'.repeat(64)
const original =
  ' {"unknown":true,"duplicate":1.00,"duplicate":-0,"large":9007199254740993}\n'
const snapshot: EvalSnapshot = {
  id,
  digest: id,
  name: 'saved.json',
  source: 'local-file',
  format: 'unknown-json',
  importedAt: '2026-10-20T00:00:00Z',
  byteSize: Buffer.byteLength(original),
  originalJSON: original,
}

async function archive(page: Page, initial: EvalSnapshot = snapshot) {
  let saved: EvalSnapshot | undefined = initial
  const calls: { method: string; params: Record<string, unknown> }[] = []
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: true } } })
  )
  await page.context().route('**/api/evals/snapshots/*/download', route =>
    route.fulfill({
      contentType: 'application/json',
      headers: {
        'Content-Disposition': 'attachment; filename="original.json"',
      },
      body: saved!.originalJSON,
    })
  )
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON()
    calls.push(request)
    let result: MockRPCResult = []
    switch (request.method) {
      case 'searchEvalSnapshots': {
        const metadata = saved
          ? { ...saved, originalJSON: undefined }
          : undefined
        result = { snapshots: metadata ? [metadata] : [], hasMore: false }
        break
      }
      case 'getEvalSnapshot':
        result = saved
        break
      case 'importEvalSnapshot':
        saved = {
          ...snapshot,
          ...request.params,
          byteSize: Buffer.byteLength(request.params.originalJSON),
        }
        result = {
          snapshot: { ...saved, originalJSON: undefined },
          created: true,
        }
        break
      case 'deleteEvalSnapshot':
        saved = undefined
        result = { id: request.params.id, deleted: true }
        break
      case 'getTraceView':
        result = {
          traceID: request.params.traceID,
          traceStart: '0',
          unplacedSpanCount: 0,
          resources: {},
          scopes: {},
          spans: [],
        }
        break
      case 'getStats':
        result = {
          traces: { count: 0, lastReceived: null },
          logs: { count: 0, lastReceived: null },
          metrics: { count: 0, lastReceived: null },
          rejections: [],
        }
        break
    }
    await route.fulfill({ json: { jsonrpc: '2.0', id: request.id, result } })
  })
  return calls
}

test('explicit archive import/open, exact download and deletion preserve synthetic default', async ({
  page,
}) => {
  const calls = await archive(page)
  await page.goto('/evals?run=direct')
  await expect(page.getByRole('table', { name: /^Results for/ })).toBeVisible()
  expect(calls.some(call => call.method.includes('EvalSnapshot'))).toBe(false)
  await page
    .getByRole('button', { name: 'Snapshot library', exact: true })
    .click()
  await expect(
    page.getByRole('button', { name: 'Open snapshot saved.json' })
  ).toBeVisible()
  await expect(page).toHaveURL(/run=direct$/)
  await page.getByLabel('JSON file · up to 16 MiB').setInputFiles({
    name: 'saved.json',
    mimeType: 'application/json',
    buffer: Buffer.from(original),
  })
  await expect(
    page.getByRole('button', { name: 'Import snapshot', exact: true })
  ).toBeDisabled()
  await page.getByLabel('Source format').selectOption('unknown-json')
  await page
    .getByRole('button', { name: 'Import snapshot', exact: true })
    .click()
  await expect(
    page.getByRole('status').filter({ hasText: 'Archived saved.json' })
  ).toBeVisible()
  expect(
    calls.find(call => call.method === 'importEvalSnapshot')?.params
      .originalJSON
  ).toBe(original)
  await expect(page).toHaveURL(/run=direct$/)
  await page.getByRole('button', { name: 'Open snapshot saved.json' }).click()
  await expect(page).toHaveURL(new RegExp(`snapshot=${id}`))
  await expect(page.getByRole('table')).toHaveCount(0)
  await page
    .getByText('Original JSON and projection context', { exact: true })
    .click()
  await expect(page.getByLabel('Original snapshot JSON section')).toContainText(
    '9007199254740993'
  )
  const downloadEvent = page.waitForEvent('download')
  await page
    .getByRole('link', { name: 'Download original snapshot JSON', exact: true })
    .click()
  const download = await downloadEvent
  expect(await readFile((await download.path())!, 'utf8')).toBe(original)
  await page
    .getByRole('button', { name: 'Delete snapshot', exact: true })
    .click()
  await expect(page).toHaveURL(/\/evals\?run=direct$/)
  await expect(page.getByRole('table', { name: /^Results for/ })).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Open snapshot saved.json' })
  ).toHaveCount(0)
})

test('projected locator survives reload and synthetic navigation clears snapshot selection', async ({
  page,
}) => {
  const payload = JSON.stringify({
    evalId: 'saved-eval',
    config: { description: 'Saved eval' },
    results: {
      version: 3,
      results: [
        {
          id: 'result-0',
          provider: { id: 'saved-provider' },
          promptId: 'prompt-0',
          prompt: { raw: 'Saved question' },
          testIdx: 0,
          repeatIndex: 0,
          testCase: { description: 'Saved case' },
          response: { output: '' },
          latencyMs: 0,
          success: true,
        },
      ],
    },
  })
  const calls = await archive(page, {
    ...snapshot,
    format: 'promptfoo-v3',
    originalJSON: payload,
    byteSize: Buffer.byteLength(payload),
  })
  await page.goto(`/evals?snapshot=${id}`)
  await expect(page.getByRole('table', { name: /^Results for/ })).toContainText(
    'Saved case'
  )
  await expect
    .poll(() => new URL(page.url()).searchParams.get('run'))
    .toBeTruthy()
  await page.getByRole('searchbox', { name: 'Search evals' }).fill('Saved case')
  await expect(page.getByRole('table', { name: /^Results for/ })).toContainText(
    'Saved case'
  )
  expect(calls.filter(call => call.method === 'getEvalSnapshot')).toHaveLength(
    1
  )
  await page.getByRole('button', { name: 'Outcomes filter' }).click()
  await page
    .getByRole('group', { name: 'Outcomes options' })
    .getByRole('button', { name: 'Pass [1]', exact: true })
    .click()
  await page.keyboard.press('Escape')
  const locator = page.url()
  await page.reload()
  await expect(page.getByRole('table', { name: /^Results for/ })).toContainText(
    'Saved case'
  )
  await expect(page).toHaveURL(locator)
  await page
    .getByText('Original JSON and projection context', { exact: true })
    .click()
  await expect(page.getByText(/Costs are omitted:/)).toBeVisible()
  await page
    .getByRole('button', { name: 'Synthetic preview', exact: true })
    .click()
  await expect(page).toHaveURL(/\/evals\?run=direct$/)
  await page.goBack()
  await expect(page.getByRole('table', { name: /^Results for/ })).toContainText(
    'Saved case'
  )
  await expect(
    page.getByRole('button', { name: 'Outcomes filter' })
  ).toHaveText('Outcomes (1)')
})

test('leaving a pending snapshot ignores its late response', async ({
  page,
}) => {
  await archive(page)
  let release!: () => void
  const gate = new Promise<void>(resolve => {
    release = resolve
  })
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON()
    if (request.method !== 'getEvalSnapshot') return route.fallback()
    await gate
    await route.fulfill({
      json: { jsonrpc: '2.0', id: request.id, result: snapshot },
    })
  })
  await page.goto(`/evals?snapshot=${id}`)
  await expect(
    page.getByText('Loading snapshot…', { exact: true })
  ).toBeVisible()
  await page
    .getByRole('button', { name: 'Synthetic preview', exact: true })
    .click()
  release()
  await expect(page).toHaveURL(/\/evals\?run=direct$/)
  await expect(page.getByRole('table', { name: /^Results for/ })).toBeVisible()
  await expect(
    page.getByRole('link', { name: 'Download original snapshot JSON' })
  ).toHaveCount(0)
})
