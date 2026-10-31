import { routeTelemetry } from './telemetry-fixtures'
import { expect, test } from '@playwright/test'

test('a selected nested suggestion retains keys, indices, owner and type through reload', async ({
  page,
}) => {
  const requests: unknown[] = []
  let releaseDefinitions!: () => void
  const definitionsReady = new Promise<void>(resolve => {
    releaseDefinitions = resolve
  })
  const path = ['attributes', 'nested', 0, '0', '']
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await routeTelemetry(page, async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'getLogAttributeDefinitions') await definitionsReady
    if (params?.query) requests.push(params.query)
    const result =
      method === 'getLogAttributeDefinitions'
        ? ['log', 'resource'].map(owner => ({
            name: JSON.stringify(path.slice(1)),
            owner,
            path,
            type: 'int64',
            attributeScope: owner,
          }))
        : method === 'getStats'
          ? {
              traces: {
                traceCount: 0,
                spanCount: 0,
                serviceCount: 0,
                errorCount: 0,
                lastReceived: null,
              },
              logs: { logCount: 0, errorCount: 0, lastReceived: null },
              metrics: {
                metricCount: 0,
                dataPointCount: 0,
                lastReceived: null,
              },
              rejections: [],
            }
          : method === 'searchLogPage'
            ? { logs: [], total: 0 }
            : method === 'discoverTelemetry'
              ? { rows: [], truncated: false }
              : []
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  const definitions = page.waitForResponse(response =>
    response.url().endsWith('/api/data/getLogAttributeDefinitions')
  )
  await page.goto('/logs')
  const editor = page.locator('.cm-content[contenteditable="true"]')
  await editor.fill('nested')
  await expect(editor).toHaveText('nested')
  releaseDefinitions()
  await definitions
  await page
    .getByRole('option')
    .filter({ hasText: 'Log · Integer · Nested' })
    .click()
  await page.keyboard.type('= 9007199254740993')
  await page.keyboard.press('Escape')
  await page.keyboard.press('Enter')
  const query =
    'log.attributes["nested"][0]["0"][""]::BIGINT = 9007199254740993'
  await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBe(query)
  const expected = expect.objectContaining({
    type: 'condition',
    query: {
      field: { owner: 'log', path, type: 'int64' },
      fieldOperator: '=',
      value: '9007199254740993',
    },
  })
  await expect.poll(() => requests).toContainEqual(expected)
  requests.length = 0
  const range =
    'WHERE log.attributes["nested"][0]["0"][""]::BIGINT BETWEEN 9007199254740993 AND 9007199254740995 LIMIT 7'
  await editor.fill(range)
  await page.keyboard.press('Escape')
  await editor.press('Enter')
  await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBe(range)
  const expectedRange = expect.objectContaining({
    type: 'group',
    group: {
      logicalOperator: 'AND',
      children: [
        expect.objectContaining({
          query: {
            field: { owner: 'log', path, type: 'int64' },
            fieldOperator: '>=',
            value: '9007199254740993',
          },
        }),
        expect.objectContaining({
          query: {
            field: { owner: 'log', path, type: 'int64' },
            fieldOperator: '<=',
            value: '9007199254740995',
          },
        }),
      ],
    },
  })
  await expect.poll(() => requests).toContainEqual(expectedRange)
  requests.length = 0
  await page.reload()
  await expect(editor.locator('.cm-attribute-token')).toContainText('Nested')
  await expect.poll(() => requests).toContainEqual(expectedRange)
})
