import { routeTelemetry } from './telemetry-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { projectPromptfooDataset } from '../src/components/evals/promptfoo-adapter'

const datasetID = 'c'.repeat(64)

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
  const projection = projectPromptfooDataset({
    id: datasetID,
    source: 'local',
    name: 'copy.json',
    format: 'promptfoo-v3',
    payload,
  })
  if (projection.status !== 'supported') throw new Error(projection.reason)
  const context = JSON.stringify({
    evalId: 'copy-eval',
    config: { description: 'Copy eval' },
    results: { version: 3, results: [] },
  })
  const summaries = projection.experiments.map((experiment, index) => ({
    id: String(index + 1).padStart(64, '0'),
    adapterExperimentID: experiment.id,
    datasetID,
    name: experiment.name,
    evaluation: experiment.evaluation,
    source: experiment.source,
    format: 'promptfoo-v3' as const,
    state: 'Unknown' as const,
    stateReason: 'Received rows do not establish lifecycle.',
    pauseReason: null,
    startedAt: null,
    plannedCount: null,
    resultCount: experiment.results.length,
    assessmentName: 'Recorded outcome' as const,
    outcomes: [
      {
        key: 'neutral:Unscored',
        label: 'Unscored' as const,
        tone: 'neutral' as const,
        count: experiment.results.length,
      },
    ],
  }))
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: true } } })
  )
  await page.route('**/api/evals/experiments**', route => {
    const url = new URL(route.request().url())
    const detail = url.pathname.match(
      /^\/api\/evals\/experiments\/([^/]+)\/rows$/
    )
    if (!detail)
      return route.fulfill({
        json: {
          experiments: summaries,
          total: summaries.length,
          hasMore: false,
          facets: summaries[0]?.outcomes ?? [],
          indexing: {
            remaining: 0,
            unsupported: 0,
            empty: 0,
            issues: [],
            hasMoreIssues: false,
          },
        },
      })
    const summary = summaries.find(item => item.id === detail[1])
    const experiment = projection.experiments.find(
      item => item.id === summary?.adapterExperimentID
    )
    if (!summary || !experiment) return route.fulfill({ status: 404 })
    const resultIDs = new Set(experiment.results.map(result => result.id))
    const sourceRows = projection.rows
      .map((entry, index) => ({ entry, index }))
      .filter(({ entry }) => resultIDs.has(entry.resultID))
      .map(({ index }) => ({
        ordinal: index,
        json: JSON.stringify(rows[index]),
      }))
    return route.fulfill({
      json: {
        summary,
        contextJSON: context,
        rows: sourceRows,
        nextAfter: sourceRows.at(-1)?.ordinal ?? -1,
        hasMore: false,
      },
    })
  })
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON()
    const result =
      request.method === 'getStats'
        ? {
            traces: { count: 0 },
            logs: { count: 0 },
            metrics: { count: 0 },
            rejections: [],
          }
        : request.method === 'discoverTelemetry'
          ? { rows: [], truncated: false }
          : []
    await route.fulfill({ json: { jsonrpc: '2.0', id: request.id, result } })
  })
  const experiment = projection.experiments[0]
  const summary = summaries[0]
  const result = experiment.results[selected]
  const query = new URLSearchParams({
    experiment: summary.id,
    result: result.id,
  })
  await page.goto(`/evals?${query}`)
  return { experiment, result, query }
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

test('assessment packets stay readable and retain exact full-input Copy', async ({
  page,
}) => {
  const input =
    'Grade the answer.\n\n' +
    JSON.stringify({
      question: 'Why did checkout fail?',
      criteria: [
        {
          id: 'cause',
          text: 'Identify the **reported cause**.',
          scoring: { Fail: 'Wrong cause.', Pass: 'Supported cause.' },
        },
      ],
      reference: 'Payment declined: insufficient funds',
      viewerSkill:
        '# Viewer instructions\n' + 'Recorded commands only. '.repeat(2000),
      evidence: { events: [{ raw: '{"type":"tool_execution_end"}' }] },
    })
  await openSource(page, [
    {
      ...row(
        0,
        '<img src=x onerror="alert(1)">\n\n[unsafe](javascript:alert(1))'
      ),
      gradingResult: {
        pass: true,
        reason: 'Combined judgment',
        componentResults: [
          {
            pass: true,
            score: 1,
            reason: 'Cites the **payment decline**.',
            assertion: { type: 'llm-rubric', value: input },
          },
        ],
      },
    },
  ])
  const detail = page.locator('#signal-drawer-detail')
  await expect(
    detail.getByRole('group', { name: 'criterion', exact: true })
  ).toContainText('Identify the reported cause.')
  await expect(
    detail.locator('strong').filter({ hasText: 'reported cause' })
  ).toBeVisible()
  await expect(
    detail.getByRole('group', { name: 'Pass', exact: true })
  ).toContainText('Supported cause.')
  await expect(
    detail.locator('img:not(.cm-widgetBuffer), a[href^="javascript:"]')
  ).toHaveCount(0)
  await expect(
    detail.locator('summary', { hasText: /^\s*Evidence\s*$/ })
  ).toHaveCount(0)
  const evidence = detail
    .getByRole('table', { name: 'Assessment evidence' })
    .last()
  const jsonField = evidence.getByRole('group', {
    name: 'evidence',
    exact: true,
  })
  await expect(jsonField.locator('.cm-foldPlaceholder')).toHaveCount(1)
  await expect(
    jsonField.getByRole('button', { name: 'Show more' })
  ).toHaveCount(0)
  await jsonField
    .getByRole('button', { name: 'Expand all evidence JSON' })
    .click()
  await expect(jsonField.locator('.cm-foldPlaceholder')).toHaveCount(0)
  await expect(jsonField.locator('.cm-content')).toContainText(
    'tool_execution_end'
  )
  await jsonField
    .getByRole('button', { name: 'Collapse all evidence JSON' })
    .click()
  await expect(jsonField.locator('.cm-content')).not.toContainText(
    'tool_execution_end'
  )
  await jsonField
    .getByRole('button', { name: 'Copy evidence', exact: true })
    .click()
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(
      JSON.stringify({ events: [{ raw: '{"type":"tool_execution_end"}' }] })
    )
  const instructions = detail
    .locator('details')
    .filter({
      has: page.locator('summary', { hasText: /^\s*Grader instructions\s*$/ }),
    })
    .last()
  await expect(instructions).not.toHaveAttribute('open')
  await expect(
    instructions.locator(':scope > summary .field-group__caret')
  ).toBeVisible()
  await expect(
    detail.locator('summary button[aria-label="Copy grader input"]')
  ).toHaveCount(0)
  const assessment = detail.locator('.eval-assessment-detail').last()
  const disclosure = assessment.locator(':scope > details')
  const copyInput = assessment.getByRole('button', {
    name: 'Copy grader input',
    exact: true,
  })
  await expect(copyInput.locator('xpath=ancestor::summary')).toHaveCount(0)
  await disclosure.locator(':scope > summary').click()
  await expect(disclosure).not.toHaveAttribute('open')
  await copyInput.click()
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(input)
  await expect(disclosure).not.toHaveAttribute('open')
  await expect(
    detail.getByRole('button', { name: 'Telemetry', exact: true })
  ).toHaveAttribute('aria-disabled', 'true')
})

test('native results need no ordinal or execution wrapper and show their own grades', async ({
  page,
}) => {
  await openSource(
    page,
    [0, 1, 2].map(index => ({
      ...row(index, `Output ${index}`),
      repeatIndex: undefined,
      testCase: { description: 'Shared test case' },
      score: 7,
      gradingResult: {
        pass: true,
        score: 7,
        reason: 'Native outcome',
        componentResults: [
          {
            pass: true,
            score: 7,
            reason: `Reason ${index}`,
            assertion: { type: 'llm-rubric', value: 'Approved criterion' },
          },
        ],
      },
    }))
  )
  const table = page.getByRole('table', {
    name: 'Results for provider',
    exact: true,
  })
  await expect(
    table.getByRole('columnheader', { name: 'Experiment · Test case · Result' })
  ).toBeVisible()
  await expect(table.locator('[data-row-kind="case"]')).toHaveCount(1)
  await expect(table.locator('[data-row-kind="result"]')).toHaveCount(3)
  await expect(
    table.getByRole('columnheader', { name: 'Sort by Assessment', exact: true })
  ).toBeVisible()
  await table
    .getByRole('button', { name: 'Result result-1', exact: true })
    .click()
  const details = page.locator('#signal-drawer-detail')
  await expect(
    details.getByRole('group', { name: 'result ID', exact: true })
  ).toContainText('result-1')
  await expect(details).toContainText('Output 1')
  await expect(
    details.locator('summary').filter({ hasText: /^\s*Assessments/ })
  ).toContainText('Pass')
  await expect(
    details.locator('summary').filter({ hasText: /^Overall/ })
  ).toHaveCount(0)
  await expect(details).toContainText('Reason 1')
  await expect(details).toContainText('Approved criterion')
  await expect(
    details.getByRole('group', { name: 'score', exact: true }).last()
  ).toContainText('7')
  await expect(
    page.getByRole('radio', { name: 'Tool calls', exact: true })
  ).toBeDisabled()
  await expect(
    table.locator('[data-row-kind="execution"], [data-row-kind="grading"]')
  ).toHaveCount(0)
})

test('a singleton without repetition metadata shows output and identity on its test case', async ({
  page,
}) => {
  await openSource(page, [
    { ...row(0, 'Singleton output'), repeatIndex: undefined },
  ])
  const details = page.locator('#signal-drawer-detail')
  await expect(page.locator('[data-row-kind="result"]')).toHaveCount(0)
  await expect(details).toContainText('Singleton output')
  await expect(
    details.getByRole('group', { name: 'result ID', exact: true })
  ).toContainText('result-0')
})

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
    const { result, query } = await openSource(page, rows, selected)
    const detail = page.locator('#signal-drawer-detail')
    await expect(detail).toContainText('… [see original JSON]')
    await detail.getByRole('group', { name: 'output', exact: true }).hover()
    await detail
      .getByRole('button', { name: 'Copy output', exact: true })
      .click()
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(exact)
    await page.evaluate(() => navigator.clipboard.writeText('sentinel'))
    await detail
      .getByRole('button', { name: 'Copy explanation', exact: true })
      .focus()
    await page.keyboard.press('Enter')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(exact)
    query.delete('result')
    query.set('row', `case/${result.caseID}`)
    await page.goto(`/evals?${query}`)
    await expect(detail).toContainText('… [see original JSON]')
    await page.evaluate(() => navigator.clipboard.writeText('sentinel'))
    await detail.getByRole('group', { name: 'input', exact: true }).hover()
    await detail
      .getByRole('button', { name: 'Copy input', exact: true })
      .click()
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(exact)
  })
}

test('answer Copy preserves empty, zero, null and missing semantics', async ({
  page,
}) => {
  const { experiment, query } = await openSource(
    page,
    ['', 0, null, undefined].map((output, index) => row(index, output))
  )
  for (const [index, expected] of ['', '0', 'null', undefined].entries()) {
    query.set('result', experiment.results[index].id)
    await page.goto(`/evals?${query}`)
    const copy = page
      .locator('#signal-drawer-detail')
      .getByRole('button', { name: 'Copy output', exact: true })
    if (expected === undefined) {
      await expect(page.locator('#signal-drawer-detail')).toContainText(
        'No answer recorded'
      )
      await expect(copy).toHaveCount(0)
    } else {
      await page.evaluate(() => navigator.clipboard.writeText('sentinel'))
      await page
        .locator('#signal-drawer-detail')
        .getByRole('group', { name: 'output', exact: true })
        .hover()
      await copy.click()
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()))
        .toBe(expected)
    }
  }
})

test('switching imported experiments preserves each experiment disclosure and sort', async ({
  page,
}) => {
  await openSource(page, [
    row(0, 'A'),
    row(1, 'B'),
    { ...row(2, 'C'), provider: { id: 'other' } },
  ])
  const table = page.getByRole('table', {
    name: 'Results for provider',
    exact: true,
  })
  await table.getByRole('button', { name: /^Collapse Case 0/ }).click()
  await expect(
    table.getByRole('button', { name: 'Sort by Duration' })
  ).toHaveCount(0)
  const header = table.getByRole('columnheader', {
    name: 'Experiment · Test case · Result',
    exact: true,
  })
  await header.getByRole('button', { name: 'Sort by Test case' }).click()
  await expect(header).toHaveAttribute('aria-sort', 'ascending')
  await page
    .getByRole('button', { name: /^other/ })
    .locator('.signal-row__title')
    .click()
  const otherTable = page.getByRole('table', {
    name: 'Results for other',
    exact: true,
  })
  await expect(otherTable).toBeVisible()
  const otherHeader = otherTable.getByRole('columnheader', {
    name: 'Experiment · Test case · Result',
    exact: true,
  })
  await expect(otherHeader).not.toHaveAttribute('aria-sort', 'ascending')
  await otherHeader.getByRole('button', { name: 'Sort by Test case' }).click()
  await otherHeader.getByRole('button', { name: 'Sort by Test case' }).click()
  await expect(otherHeader).toHaveAttribute('aria-sort', 'descending')
  await page
    .getByRole('button', { name: /^provider/ })
    .locator('.signal-row__title')
    .click()
  await expect(
    table.getByRole('button', { name: /^Expand Case 0/ })
  ).toHaveAttribute('aria-expanded', 'false')
  await expect(header).toHaveAttribute('aria-sort', 'ascending')
  await page
    .getByRole('button', { name: /^other/ })
    .locator('.signal-row__title')
    .click()
  await expect(otherHeader).toHaveAttribute('aria-sort', 'descending')
})
