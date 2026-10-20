import { expect, test, type Page } from '@playwright/test'
import { projectEvalSnapshot } from '../src/components/evals/snapshot-adapters'

const snapshotID = 'c'.repeat(64)

async function openSource(
  page: Page,
  rows: Record<string, unknown>[],
  selected = 0
) {
  const payload = JSON.stringify({
    evalId: 'copy-eval',
    config: { description: 'Copy eval' },
    results: { version: 3, results: rows },
  })
  const projection = projectEvalSnapshot({
    id: snapshotID,
    source: 'local',
    name: 'copy.json',
    format: 'promptfoo-v3',
    payload,
  })
  if (projection.status !== 'supported') throw new Error(projection.reason)
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: true } } })
  )
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON()
    const result =
      request.method === 'getEvalSnapshot'
        ? {
            id: snapshotID,
            digest: snapshotID,
            source: 'local',
            name: 'copy.json',
            format: 'promptfoo-v3',
            originalJSON: payload,
            byteSize: Buffer.byteLength(payload),
            importedAt: '2026-10-20T00:00:00Z',
          }
        : request.method === 'getStats'
          ? {
              traces: { count: 0 },
              logs: { count: 0 },
              metrics: { count: 0 },
              rejections: [],
            }
          : []
    await route.fulfill({ json: { jsonrpc: '2.0', id: request.id, result } })
  })
  const run = projection.runs[0]
  const attempt = run.attempts[selected]
  const query = new URLSearchParams({
    snapshot: snapshotID,
    run: run.id,
    attempt: attempt.id,
  })
  await page.goto(`/evals?${query}`)
  return { run, attempt, query }
}

function row(index: number, output: unknown) {
  return {
    id: `result-${index}`,
    provider: { id: 'provider' },
    promptId: 'prompt',
    testIdx: index,
    repeatIndex: 0,
    testCase: { description: `Case ${index}` },
    prompt: { raw: 'Question' },
    response: { output },
    gradingResult: { reason: 'Explanation', pass: true },
  }
}

for (const mode of ['field limit', 'aggregate budget']) {
  test(`field Copy uses exact source after ${mode}`, async ({ page }) => {
    const exact = '🙂\\\n"é'.repeat(7000)
    const rows =
      mode === 'field limit'
        ? [row(0, exact)]
        : Array.from({ length: 40 }, (_, index) =>
            row(index, 'x'.repeat(32768))
          )
    const selected = rows.length - 1
    rows[selected] = {
      ...row(selected, exact),
      prompt: { raw: exact },
      gradingResult: { reason: exact, pass: true },
    }
    const { attempt, query } = await openSource(page, rows, selected)
    const detail = page.locator('#signal-drawer-detail')
    await expect(detail).toContainText('… [see original JSON]')
    await detail
      .getByRole('button', { name: 'Copy answer', exact: true })
      .click()
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(exact)
    await detail.locator('summary').filter({ hasText: 'Assessments' }).click()
    await page.evaluate(() => navigator.clipboard.writeText('sentinel'))
    await detail
      .getByRole('button', { name: 'Copy explanation', exact: true })
      .focus()
    await page.keyboard.press('Enter')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(exact)
    query.delete('attempt')
    query.set('row', `case/${attempt.caseID}`)
    await page.goto(`/evals?${query}`)
    await expect(detail).toContainText('… [see original JSON]')
    await page.evaluate(() => navigator.clipboard.writeText('sentinel'))
    await detail
      .getByRole('button', { name: `Copy ${attempt.name}`, exact: true })
      .click()
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(exact)
  })
}

test('answer Copy preserves empty, zero, null and missing semantics', async ({
  page,
}) => {
  const { run, query } = await openSource(
    page,
    ['', 0, null, undefined].map((output, index) => row(index, output))
  )
  for (const [index, expected] of ['', '0', 'null', undefined].entries()) {
    query.set('attempt', run.attempts[index].id)
    await page.goto(`/evals?${query}`)
    const copy = page
      .locator('#signal-drawer-detail')
      .getByRole('button', { name: 'Copy answer', exact: true })
    if (expected === undefined) {
      await expect(page.locator('#signal-drawer-detail')).toContainText(
        'No answer recorded'
      )
      await expect(copy).toHaveCount(0)
    } else {
      await page.evaluate(() => navigator.clipboard.writeText('sentinel'))
      await copy.click()
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()))
        .toBe(expected)
    }
  }
})

test('switching imported runs preserves each run disclosure and sort', async ({
  page,
}) => {
  await openSource(page, [
    row(0, 'A'),
    row(1, 'B'),
    { ...row(2, 'C'), provider: { id: 'other' } },
  ])
  const table = page.getByRole('table', {
    name: 'Results for Copy eval · provider',
    exact: true,
  })
  await table.getByRole('button', { name: /^Collapse Case 0/ }).click()
  const header = table.getByRole('columnheader', { name: /^Duration/ })
  await header.getByRole('button', { name: 'Sort by Duration' }).click()
  await expect(header).toHaveAttribute('aria-sort', 'ascending')
  await page.getByRole('button', { name: /Copy eval · other/ }).click()
  await expect(
    page.getByRole('table', {
      name: 'Results for Copy eval · other',
      exact: true,
    })
  ).toBeVisible()
  await page.getByRole('button', { name: /Copy eval · provider/ }).click()
  await expect(
    table.getByRole('button', { name: /^Expand Case 0/ })
  ).toHaveAttribute('aria-expanded', 'false')
  await expect(header).toHaveAttribute('aria-sort', 'ascending')
})
