import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { exampleExperiments } from '../src/components/evals/examples'
import { getTheme, themeRgb } from './theme-colours'

async function openExamples(page: Page, selection = '') {
  await page.goto(`/evals?preview=examples${selection ? `&${selection}` : ''}`)
}

async function showOnly(page: Page, label: 'Results' | 'Tool calls' | 'Cost') {
  await page
    .getByRole('radiogroup', { name: 'Eval view' })
    .getByRole('radio', {
      name: label === 'Cost' ? 'Cost · CAD est.' : label,
      exact: true,
    })
    .click()
}

async function enableEvals(page: Page, enabled = true) {
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON()
    if (request.method !== 'getTraceView') return route.fallback()
    const rows = exampleExperiments.flatMap(experiment =>
      experiment.results.flatMap(result => {
        if (!result.telemetry?.spanID) return []
        const executionSpanID = result.telemetry.spanID
        const started = BigInt(Date.parse(result.startedAt!)) * 1_000_000n
        const references = [
          {
            reference: result.telemetry,
            name: result.name,
            duration: BigInt(Math.round((result.seconds ?? 1) * 1e9)),
            error: result.state === 'Error',
          },
          ...result.tools.map(tool => ({
            reference: tool.telemetry,
            name: tool.name,
            duration: 500000000n,
            error: tool.state === 'Error',
          })),
          ...result.assessments.map(assessment => ({
            reference: assessment.telemetry,
            name: `${assessment.evaluator} · ${assessment.id}`,
            duration: 500000000n,
            error: false,
          })),
        ]
        return references.flatMap(({ reference, name, duration, error }) =>
          reference?.spanID &&
          request.params.traceID === reference.traceID &&
          request.params.spanIDs.includes(reference.spanID)
            ? [
                [
                  reference.traceID,
                  reference.spanID,
                  name,
                  'eval-agent',
                  String(started),
                  String(started + duration),
                  error ? 2 : 1,
                  reference.spanID === executionSpanID ? null : executionSpanID,
                ],
              ]
            : []
        )
      })
    )
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result: {
          traceID: request.params.traceID,
          traceStart: String(rows[0]?.[4] ?? '1700000000000000001'),
          unplacedSpanCount: 0,
          resources: {
            1: {
              attributes: [
                {
                  key: 'service.name',
                  value: { kind: 'string', value: 'eval-agent' },
                },
              ],
              droppedAttributesCount: 0,
            },
          },
          scopes: {
            1: {
              name: 'eval',
              version: '',
              attributes: [],
              droppedAttributesCount: 0,
            },
          },
          spans: rows.map(row => ({
            depth: row[7] ? 1 : 0,
            matched: true,
            spanData: {
              spanID: row[1],
              parentSpanID: row[7],
              name: row[2],
              traceState: '',
              flags: 0,
              start: '0',
              dur: String(BigInt(row[5]!) - BigInt(row[4]!)),
              r: 1,
              s: 1,
              kind: 'Internal',
              kindCode: 1,
              statusCodeValue: row[6],
              statusCode: row[6] === 2 ? 'Error' : 'Ok',
              statusMessage: '',
              attributes: [],
              events: [],
              links: [],
              droppedAttributesCount: 0,
              droppedEventsCount: 0,
              droppedLinksCount: 0,
            },
          })),
        },
      },
    })
  })
  await page.route('**/api/features', route =>
    route.fulfill({
      json: { evals: { enabled, label: 'Experimental and unsupported' } },
    })
  )
}

test('case telemetry summarizes every repetition and links to its exact span', async ({
  page,
}) => {
  await enableEvals(page)
  await openExamples(page, 'experiment=direct&row=case%2Frequest-latency')
  const detail = page.getByRole('region', {
    name: 'Find the slow request details',
    exact: true,
  })
  await expect(
    detail
      .locator('details')
      .filter({ has: page.locator('summary').filter({ hasText: 'Telemetry' }) })
  ).toHaveAttribute('open', '')
  const links = detail
    .locator('[aria-label="Referenced telemetry"]')
    .getByRole('link')
  const results = exampleExperiments
    .find(experiment => experiment.id === 'direct')!
    .results.filter(item => item.caseID === 'request-latency')
  await expect(links).toHaveCount(results.length)
  for (const [index, result] of results.entries()) {
    await expect(links.nth(index).getByTitle('eval-agent')).toBeVisible()
    await expect(links.nth(index)).toContainText('37.000 s')
    await expect(links.nth(index)).toHaveAttribute(
      'href',
      `/traces/${result.telemetry!.traceID}?span=${result.telemetry!.spanID}`
    )
  }
  await links.first().focus()
  await expect(links.first()).toBeFocused()
  await openExamples(page, 'experiment=direct&result=request-latency')
  await expect(
    detail.locator('summary').filter({ hasText: /^Question/ })
  ).toHaveCount(0)
})

test('Evals stays closed when its capability is disabled', async ({ page }) => {
  await enableEvals(page, false)
  await page.goto('/evals')
  await expect(
    page.getByRole('heading', { name: 'Evals is not enabled' })
  ).toBeVisible()
  await expect(
    page.getByRole('link', { name: 'Evals', exact: true })
  ).toHaveCount(0)
  await expect(page.getByRole('table')).toHaveCount(0)
})

test('sidebar outcomes filter experiment cards and preserve full results, selection and history', async ({
  page,
}) => {
  await enableEvals(page)
  await openExamples(page, 'experiment=guide-assisted&result=request-latency')
  const controls = page.getByRole('group', { name: 'Evals list controls' })
  const trigger = controls.getByRole('button', {
    name: 'Outcomes',
    exact: true,
  })
  const options = page.getByRole('group', { name: 'Outcomes options' })
  const cards = page.locator('[data-drawer-item-key]')
  const table = page.getByRole('table', {
    name: 'Results for Guide-assisted agent',
  })
  const legend = cards.filter({
    has: page.getByRole('button', { name: /^Guide-assisted agent/ }),
  })
  await expect(cards).toHaveCount(3)
  await expect(legend.locator('.breakdown-segment')).toHaveCount(3)
  await expect(legend.getByRole('button')).toHaveAccessibleName(
    /Pass: 11 results.*Fail: 2 results/
  )
  const originalRows = await table.getByRole('row').count()

  await trigger.click()
  await expect(
    options.getByRole('button', { name: 'Pass [2]', exact: true })
  ).toBeVisible()
  await expect(
    options.getByRole('button', { name: 'Partial [2]', exact: true })
  ).toBeVisible()
  await expect(
    options.getByRole('button', { name: 'Fail [2]', exact: true })
  ).toBeVisible()
  const internalFault = options.getByRole('button', {
    name: 'Internal fault [1]',
    exact: true,
  })
  await internalFault.focus()
  await page.keyboard.press('Space')
  await expect(internalFault).toHaveAttribute('aria-pressed', 'true')
  await expect(cards).toHaveCount(3)
  await options.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(cards).toHaveCount(1)
  await expect(cards.first()).toContainText('Direct agent')
  await expect(table.getByRole('row')).toHaveCount(originalRows)
  await expect(legend).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('result')).toBe('request-latency')

  await trigger.click()
  await options
    .getByRole('button', { name: 'Planned [2]', exact: true })
    .click()
  await options.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(cards).toHaveCount(2)
  await expect(cards.nth(1)).toContainText('Revised instructions')
  await expect(trigger).toBeFocused()
  const filteredURL = page.url()
  await page.reload()
  await expect(cards).toHaveCount(2)
  await expect(page).toHaveURL(filteredURL)
  await trigger.click()
  await options.getByRole('button', { name: 'Clear outcomes filter' }).click()
  await options.getByRole('button', { name: 'Apply', exact: true }).click()
  await expect(cards).toHaveCount(3)
  expect(new URL(page.url()).searchParams.has('outcomes')).toBe(false)
  await page.goBack()
  await expect(cards).toHaveCount(2)
  await expect(page).toHaveURL(filteredURL)
  await page.goForward()
  await expect(cards).toHaveCount(3)
})

test('experiment selection, assessments, filtering and browser history stay connected', async ({
  page,
}) => {
  await enableEvals(page)
  await openExamples(page)
  const toolbar = page.getByRole('toolbar', {
    name: 'Workspace controls',
  })
  await expect(
    toolbar.getByRole('searchbox', { name: 'Search evals' })
  ).toBeVisible()
  await expect(
    toolbar.getByRole('searchbox', { name: 'Search evals' })
  ).toHaveCSS('font-size', '14px')
  expect(
    await toolbar.getByRole('search').evaluate(element =>
      [...element.querySelectorAll('button')].flatMap(button => {
        if (button.closest('[popover], dialog')) return []
        const name = button.getAttribute('aria-label')
        return [name?.startsWith('Change time range') ? 'Time' : name]
      })
    )
  ).toEqual(['Refresh', 'Clear search', 'Search query help', 'Time'])
  await expect
    .poll(() =>
      toolbar.getByRole('search').evaluate(element => {
        const surface = element.querySelector('.search-bar__query')!
        const help = element.querySelector(
          'button[aria-label="Search query help"]'
        )!
        return Math.abs(
          surface.getBoundingClientRect().right -
            help.getBoundingClientRect().right
        )
      })
    )
    .toBeLessThan(1)
  await expect(
    toolbar.getByRole('button', { name: 'Refresh', exact: true })
  ).toBeVisible()
  expect(
    await toolbar
      .getByRole('button', { name: 'Refresh', exact: true })
      .evaluate(element => getComputedStyle(element).backgroundColor)
  ).toBe('rgba(0, 0, 0, 0)')
  await expect(page.getByRole('searchbox')).toHaveCount(1)
  await expect(
    page.getByRole('combobox', { name: 'Execution status' })
  ).toHaveCount(0)
  await expect(
    page.getByRole('combobox', { name: 'Sort results' })
  ).toHaveCount(0)
  await expect(
    toolbar.getByRole('button', { name: /^Change time range/ })
  ).toHaveText('')
  await expect(
    page.getByRole('region', { name: 'Evaluation results', exact: true })
  ).toContainText('Guide-assisted agent')
  await openExamples(page, 'experiment=guide-assisted&result=request-latency')
  const detail = page.getByRole('region', {
    name: 'Result details',
    exact: true,
  })
  const bubbles = detail.locator('details.collapse')
  await expect(bubbles).toHaveCount(3)
  await expect(detail.getByRole('tablist')).toHaveCount(0)
  for (const bubble of await bubbles.all()) {
    await expect(bubble).toHaveAttribute('open')
    await expect(bubble.locator(':scope > summary')).toBeInViewport({
      ratio: 1,
    })
  }
  await expect(
    page.getByText('The causal claim about total latency', { exact: false })
  ).toBeVisible()
  await page.reload()
  await expect(
    detail.locator('summary').filter({ hasText: 'Tool calls' })
  ).toHaveCount(0)
  await expect(bubbles.nth(1)).toHaveAttribute('open')
  await page.getByRole('searchbox', { name: 'Search evals' }).fill('Mixed')
  await expect(page).toHaveURL(/q=Mixed/)
  await expect(
    page
      .getByRole('table', { name: 'Results for Guide-assisted agent' })
      .locator('[data-result-id]')
  ).toHaveCount(16)
  await page
    .getByRole('searchbox', { name: 'Search evals' })
    .fill('nonexistent-score')
  await expect(page).toHaveURL(/q=nonexistent-score/)
  await expect(
    page
      .getByRole('table', { name: 'Results for Guide-assisted agent' })
      .locator('[data-result-id]')
  ).toHaveCount(16)
  await expect(page.locator('[data-result-id="request-latency"]')).toHaveClass(
    /table-row--selected/
  )
  await toolbar.getByRole('button', { name: 'Clear search' }).click()
  await page
    .getByRole('button', { name: /Direct agent/ })
    .locator('.signal-row__title')
    .click()
  await expect(
    page.getByRole('region', { name: 'Evaluation results', exact: true })
  ).toContainText('Direct agent')
  await openExamples(page, 'experiment=direct&result=cpu-evidence')
  await expect(
    page.getByText('The investigator process exited', { exact: false })
  ).toBeVisible()
  await showOnly(page, 'Cost')
  const accounting = detail.getByRole('table', { name: 'Result accounting' })
  for (const key of [
    'investigator API estimate (CAD)',
    'grader API estimate (CAD)',
  ]) {
    await expect(
      accounting.locator(`[data-field="${key}"] .detail-property__body`)
    ).toHaveText('Unknown')
  }
  await page
    .getByRole('table', { name: 'Cost for Direct agent' })
    .locator('[data-row-kind="experiment"] .tree-row-identity__title button')
    .click()
  await page.goBack()
  await expect(page).toHaveURL(/result=cpu-evidence/)
  await expect(accounting).toBeVisible()
  await page.goForward()
  await expect(
    page.getByRole('region', { name: 'Experiment details', exact: true })
  ).toBeVisible()
  await page
    .getByRole('region', { name: 'Experiment details', exact: true })
    .locator('summary')
    .filter({ hasText: /Grading/ })
    .click()
  await expect(
    page.getByRole('table', { name: 'Grading for this experiment' })
  ).toBeVisible()
  await page.getByRole('button', { name: /Revised instructions/ }).click()
  await showOnly(page, 'Results')
  await expect(
    page
      .getByRole('region', { name: 'Experiment details', exact: true })
      .locator('[data-detail-section="grading"] details')
  ).toHaveAttribute('open')
  const plannedResults = page.getByRole('table', {
    name: 'Results for Revised instructions',
  })
  await expect(
    plannedResults
      .locator('[data-result-id]')
      .getByText('Unscored', { exact: true })
  ).toHaveCount(16)
  await expect(
    page
      .getByRole('table', { name: 'Grading for this experiment' })
      .getByRole('group', { name: 'assessed results', exact: true })
  ).toContainText('0 / 16')
  await expect(
    page.getByRole('radio', { name: 'Cost · CAD est.', exact: true })
  ).toBeDisabled()
  await expect(
    page
      .getByRole('region', { name: 'Experiment details', exact: true })
      .getByRole('button', { name: 'Cost', exact: true })
  ).toHaveAttribute('aria-disabled', 'true')
})

for (const theme of [
  'madoromi-aki-yoru',
  'madoromi-fuyu-yoru',
  'madoromi-fuyu-hiru',
]) {
  test(`summary views are exclusive radios with selection highlights in ${theme}`, async ({
    page,
  }) => {
    await enableEvals(page)
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await openExamples(page, 'experiment=direct&result=request-latency')
    const url = page.url()
    const toggles = page.getByRole('radiogroup', { name: 'Eval view' })
    const results = toggles.getByRole('radio', {
      name: 'Results',
      exact: true,
    })
    const time = toggles.getByRole('radio', {
      name: 'Tool calls',
      exact: true,
    })
    const cost = toggles.getByRole('radio', {
      name: 'Cost · CAD est.',
      exact: true,
    })
    await expect(results).toHaveAttribute('aria-checked', 'true')
    await expect(time).toHaveAttribute('aria-checked', 'false')
    await expect(cost).toHaveAttribute('aria-checked', 'false')
    await results.focus()
    await page.keyboard.press('ArrowRight')
    await expect(time).toBeFocused()
    await expect(time).toHaveAttribute('aria-checked', 'true')
    await page.mouse.move(0, 0)
    const table = page.getByRole('table', {
      name: 'Tool calls for Direct agent',
    })
    await expect(table).toBeVisible()
    const headings = await table
      .locator('thead button')
      .evaluateAll(buttons =>
        buttons.map(button => button.getAttribute('aria-label'))
      )
    expect(headings).toEqual(
      ['Test case', 'Started', 'Duration', 'Tool calls'].map(
        label => `Sort by ${label}`
      )
    )
    for (const toggle of [results, time, cost]) {
      await toggle.click()
      await page.mouse.move(0, 0)
      await expect(toggle).toHaveAttribute('aria-checked', 'true')
      await expect(toggles.locator('[aria-checked="true"]')).toHaveCount(1)
      const colors = await toggle.evaluate(button => {
        const probe = document.createElement('span')
        probe.style.color = 'var(--color-primary)'
        button.append(probe)
        const primary = getComputedStyle(probe).color
        probe.style.color = 'var(--item-selected-bg)'
        const selected = getComputedStyle(probe).color
        probe.style.color = 'var(--item-hover-bg)'
        const hover = getComputedStyle(probe).color
        probe.remove()
        return {
          icon: getComputedStyle(button.querySelector('.header-tab__icon')!)
            .color,
          primary,
          selected,
          hover,
          label: getComputedStyle(button.querySelector('.header-tab__label')!)
            .color,
          text: getComputedStyle(button).color,
        }
      })
      expect(colors.icon).toBe(colors.primary)
      expect(colors.label).toBe(colors.text)
      await expect(toggle).toHaveCSS('background-color', colors.selected)
      await toggle.hover()
      await expect(toggle).toHaveCSS('background-color', colors.hover)
      await page.mouse.move(0, 0)
      await expect(toggle).toHaveCSS('background-color', colors.selected)
    }
    await cost.click()
    await page.mouse.move(0, 0)
    await expect(results).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    expect(
      await results.evaluate(
        button =>
          getComputedStyle(button.querySelector('.header-tab__icon')!).color ===
          getComputedStyle(button).color
      )
    ).toBe(true)
    await time.click()
    expect(
      await table
        .locator('thead button')
        .evaluateAll(buttons =>
          buttons.map(button => button.getAttribute('aria-label'))
        )
    ).toEqual(headings)
    await expect(
      table.locator('[data-result-id="request-latency"].table-row--selected')
    ).toHaveCount(1)
    expect(page.url()).toBe(url)
    await test.info().attach('combined-groups', {
      body: await page.screenshot({
        path: test.info().outputPath('radio-summary.png'),
      }),
      contentType: 'image/png',
    })
    await cost.click()
    await cost.click()
    await expect(cost).toHaveAttribute('aria-checked', 'true')
    const cases = page.getByRole('table', { name: 'Cost for Direct agent' })
    await expect(cases.locator('thead th')).toHaveCount(5)
    await expect(cases.locator('[data-result-id]')).toHaveCount(0)
    await expect(cases.locator('[data-row-kind="case"]')).toHaveCount(8)
  })
}

test('Results, Time and Cost preserve selection and distinguish missing measurements and totals', async ({
  page,
}) => {
  await enableEvals(page)
  await openExamples(page, 'experiment=direct&result=cpu-evidence')
  const selected = page.locator('[data-result-id="cpu-evidence"]')
  const url = page.url()
  await expect(selected).toHaveClass(/table-row--selected/)
  await expect(
    page.getByRole('radiogroup', { name: 'Eval view' }).getByRole('radio')
  ).toHaveCount(3)
  await expect(
    page.getByRole('button', { name: 'Columns', exact: true })
  ).toHaveCount(0)
  await showOnly(page, 'Tool calls')
  const time = page.getByRole('table', { name: 'Tool calls for Direct agent' })
  await expect(time.locator('[data-result-id="cpu-evidence"]')).toContainText(
    '1.5 minutes'
  )
  await expect(
    time.locator('[data-result-id="checkout-latency"] td').first()
  ).toHaveText('Not started')
  await expect(
    time.getByRole('columnheader', { name: 'Tool calls' })
  ).toBeVisible()
  const failedResult = time.locator('[data-result-id="request-latency"]')
  await expect(failedResult.locator('td').nth(1)).toHaveText('')
  await expect(
    time
      .locator('.eval-results__case[data-case-id="request-latency"] td')
      .nth(1)
  ).toHaveText('')
  const trace = failedResult.getByRole('button', {
    name: 'Span Find the slow request, 37.000 s',
  })
  await expect(trace).toBeVisible()
  await expect(trace).not.toHaveAttribute('href')
  await expect(
    time.locator('[data-outline-id="execution/request-latency"]')
  ).toHaveCount(0)
  const execution = failedResult
  const toolRows = time.locator('[data-tool-result-id="request-latency"]')
  await expect(toolRows).toHaveCount(3)
  const toggle = execution.getByRole('button', {
    name: /^Collapse Result/,
  })
  expect(
    await toggle.evaluate(button =>
      Math.abs(
        button.getBoundingClientRect().height -
          button.closest('tr')!.getBoundingClientRect().height
      )
    )
  ).toBeLessThan(1)
  await test.info().attach('time-span-bars', {
    body: await page.screenshot({
      path: test.info().outputPath('time-span-bars.png'),
    }),
    contentType: 'image/png',
  })
  await expect(
    toolRows.nth(1).getByRole('button', { name: 'Span bash, 500.000 ms' })
  ).toBeVisible()
  expect(page.url()).toBe(url)
  await execution.getByRole('button', { name: /^Collapse Result/ }).focus()
  await page.keyboard.press('Space')
  await expect(toolRows).toHaveCount(0)
  await expect(
    time.locator('[data-result-id="checkout-latency"] td').first()
  ).toHaveText('Not started')
  await expect(
    time.locator('[data-result-id="checkout-latency"]').getByRole('link')
  ).toHaveCount(0)
  await time
    .getByRole('button', { name: 'Sort by Tool calls', exact: true })
    .click()
  await time
    .getByRole('button', { name: 'Sort by Tool calls', exact: true })
    .click()
  await expect(time.locator('[data-result-id]').first()).toHaveAttribute(
    'data-case-id',
    'request-latency'
  )
  await expect(
    time.locator('[data-result-id="checkout-latency"] td').nth(1)
  ).toHaveText('')
  await showOnly(page, 'Cost')
  const costs = page.getByRole('table', { name: 'Cost for Direct agent' })
  for (const row of await costs.locator('[data-row-kind="case"]').all())
    await row.locator('button[aria-expanded="false"]').click()
  expect(
    await costs
      .locator('[data-row-kind="case"]')
      .evaluateAll(rows =>
        rows.every(
          row =>
            Math.abs(
              row.getBoundingClientRect().height -
                row.querySelector('.tree-toggle')!.getBoundingClientRect()
                  .height
            ) < 1
        )
      )
  ).toBe(true)
  await expect(
    costs.getByRole('columnheader', { name: /Total cost/ })
  ).toBeVisible()
  await expect(
    costs.locator('[data-result-id="cpu-evidence"]').locator('td').last()
  ).toHaveText('Unknown')
  const totals = costs.getByRole('row', {
    name: 'Estimated cost for shown results',
  })
  await expect(totals.locator('[title="Partial estimate · CAD"]')).toHaveCount(
    3
  )
  const footer = costs.locator('tfoot')
  await expect(footer).toHaveCSS('opacity', '1')
  const divider = await footer.evaluate(element => {
    const style = getComputedStyle(element, '::before')
    return { opacity: style.opacity, height: style.height, top: style.top }
  })
  expect(divider).toEqual({ opacity: '0.18', height: '1px', top: '0px' })
  expect(
    await costs.evaluate(table => {
      const headers = [
        ...table.querySelectorAll(
          '.eval-results__case-heading, .eval-results__result-heading, thead th[scope="col"]'
        ),
      ]
      return [...table.querySelectorAll('tfoot tr > *')].every(
        (cell, index) => {
          if (index < 2) return true
          const header = headers[index].getBoundingClientRect()
          const value = cell.getBoundingClientRect()
          return (
            Math.abs(header.left - value.left) < 1 &&
            Math.abs(header.right - value.right) < 1
          )
        }
      )
    })
  ).toBe(true)
  await expect(selected).toHaveClass(/table-row--selected/)
  expect(page.url()).toBe(url)
  await showOnly(page, 'Results')
  const results = page.getByRole('table', { name: 'Results for Direct agent' })
  await expect(
    results
      .locator('[data-result-id="cpu-evidence"]')
      .getByText('Unscored', { exact: true })
  ).toHaveCount(1)
  await expect(results.locator('[data-result-id="trace-logs"]')).toContainText(
    '(0/1)'
  )
  await expect(selected).toHaveClass(/table-row--selected/)
  expect(page.url()).toBe(url)
  const parent = page.locator(
    '.eval-results__case[data-case-id="cpu-evidence"]'
  )
  await parent.locator('button[aria-expanded]').click()
  await showOnly(page, 'Tool calls')
  await expect(parent.locator('button[aria-expanded]')).toHaveAttribute(
    'aria-expanded',
    'false'
  )
  await expect(
    time.locator('[data-case-id="cpu-evidence"][data-result-id]')
  ).toHaveCount(0)
  await showOnly(page, 'Cost')
  await expect(parent.locator('button[aria-expanded]')).toHaveAttribute(
    'aria-expanded',
    'false'
  )
  await parent.locator('button[aria-expanded]').click()
  await expect(selected).toHaveClass(/table-row--selected/)
  expect(page.url()).toBe(url)
  await page
    .getByRole('searchbox', { name: 'Search evals' })
    .fill('CPU evidence')
  await expect(page).toHaveURL(/q=CPU\+evidence/)
  await expect(costs.locator('[data-result-id]')).toHaveCount(16)
  await expect(totals.locator('[title="Partial estimate · CAD"]')).toHaveCount(
    3
  )
  await expect(selected).toHaveClass(/table-row--selected/)
  await page.getByRole('button', { name: 'Clear search', exact: true }).click()
  await expect(costs.locator('[data-result-id]')).toHaveCount(16)
  expect(page.url()).toBe(url)
})

test('waterfall labels and bars open eval details while tree controls only expand', async ({
  page,
}) => {
  await enableEvals(page)
  await openExamples(page, 'experiment=direct&result=request-latency')
  const url = page.url()
  const row = page.locator('[data-result-id="request-latency"]')
  await expect(
    row.getByRole('img', { name: 'Span error', exact: true })
  ).toBeVisible()
  await expect(
    page
      .locator('.eval-results__case[data-case-id="request-latency"]')
      .getByRole('img', { name: 'Span error', exact: true })
  ).toBeVisible()
  const errorColours = await row.evaluate(element => {
    const probe = document.createElement('span')
    element.append(probe)
    probe.style.backgroundColor = 'var(--color-error)'
    const spanError = getComputedStyle(probe).backgroundColor
    probe.style.backgroundColor = 'var(--color-papaya)'
    const internalFault = getComputedStyle(probe).backgroundColor
    probe.remove()
    return { spanError, internalFault }
  })
  await expect(
    row
      .getByRole('img', { name: 'Span error', exact: true })
      .locator('span')
      .first()
  ).toHaveCSS('background-color', errorColours.spanError)
  await expect(
    page
      .locator('[data-result-id="cpu-evidence"]')
      .getByRole('img', { name: 'Internal fault', exact: true })
      .locator('span')
      .first()
  ).toHaveCSS('background-color', errorColours.internalFault)
  const bar = row.getByRole('button', {
    name: 'Span Find the slow request, 37.000 s',
  })
  await expect(bar).toBeVisible()
  await expect(bar.locator('.span-bar')).toHaveCSS(
    'background-color',
    errorColours.spanError
  )
  expect(
    await bar
      .locator('.span-bar')
      .evaluate(element => getComputedStyle(element).backgroundColor)
  ).not.toBe('rgb(224, 222, 244)')
  await bar.click()
  expect(page.url()).toBe(url)
  const details = page.locator('#signal-drawer-detail')
  await expect(
    details.getByRole('table', { name: 'Span fields', exact: true })
  ).toHaveCount(0)
  await expect(row).toHaveClass(/table-row--selected/)
  await expect(
    details.getByRole('region', { name: 'Result details', exact: true })
  ).toBeVisible()
  await row.getByRole('button', { name: /^Expand Result/ }).click()
  await expect(
    page.locator('[data-outline-id="execution/request-latency"]')
  ).toHaveCount(0)
  await expect(bar.locator('.span-bar')).toHaveCSS(
    'background-color',
    errorColours.spanError
  )
  const tool = page.locator('[data-tool-result-id="request-latency"]').first()
  await tool.locator('.tree-row-identity__title button').click()
  await expect(tool).toHaveClass(/table-row--selected/)
  await expect(
    details.getByRole('table', { name: 'bash fields', exact: true })
  ).toBeVisible()
  expect(new URL(page.url()).searchParams.get('row')).toBe(
    'request-latency/tool/0'
  )
  await page.reload()
  await expect(
    details.getByRole('table', { name: 'bash fields', exact: true })
  ).toBeVisible()
  await showOnly(page, 'Tool calls')
  await expect(details.locator('details.collapse:visible')).toHaveCount(2)
  await expect(
    details.getByRole('table', { name: 'bash output', exact: true })
  ).toBeVisible()
  await tool.locator('.span-bar').click()
  await expect(tool).toHaveClass(/table-row--selected/)
  await page
    .locator(
      '[data-outline-id="request-latency/assessment/completion"] .tree-row-identity__title button'
    )
    .click()
  await expect(
    details.getByRole('table', {
      name: 'Completion grader · completion fields',
      exact: true,
    })
  ).toBeVisible()
  await expect(
    details.getByRole('group', { name: 'evaluator', exact: true }).first()
  ).toBeVisible()
  await page
    .locator('[data-result-id="cpu-evidence"] .tree-row-identity__title button')
    .click()
  expect(new URL(page.url()).searchParams.get('result')).toBe('cpu-evidence')
  await expect(page.locator('[data-result-id="cpu-evidence"]')).toHaveClass(
    /table-row--selected/
  )
  await expect(
    details.locator('summary').filter({ hasText: /^(Span|Events|Links)/ })
  ).toHaveCount(0)
  await expect(details.locator('details.collapse:visible')).toHaveCount(2)
  await expect(
    details.getByRole('table', { name: 'Result fields', exact: true })
  ).toBeVisible()
  await expect(
    details.getByRole('group', { name: 'internal fault', exact: true })
  ).toBeVisible()
  await expect(details.getByText('Question', { exact: true })).toHaveCount(0)
  await expect(details.getByText('Assessments', { exact: true })).toHaveCount(0)
  await page
    .locator(
      '[data-outline-id="case/request-latency"] .tree-row-identity__title button'
    )
    .click()
  await expect(
    details.getByRole('group', { name: 'results', exact: true })
  ).toContainText('3')
  await expect(
    details.getByRole('group', { name: 'tool calls', exact: true })
  ).toHaveCount(0)
  await showOnly(page, 'Results')
  await expect(
    details.getByText(
      'Identify the slowest request and explain where it spent its time.',
      { exact: true }
    )
  ).toBeVisible()
  await showOnly(page, 'Tool calls')
  await row.locator('.tree-row-identity__title button').click()
  await expect(
    details.getByRole('table', { name: 'Result fields', exact: true })
  ).toBeVisible()
  await expect(
    details.getByText(
      'Identify the slowest request and explain where it spent its time.',
      { exact: true }
    )
  ).toHaveCount(0)
  await showOnly(page, 'Results')
  await expect(
    details.getByRole('region', { name: 'Result details', exact: true })
  ).toBeVisible()
  await expect(details.locator('details.collapse[open]:visible')).toHaveCount(3)
  await expect(
    details
      .locator('details.collapse[open]:visible > summary')
      .filter({ hasText: 'Telemetry' })
  ).toBeVisible()
})

test('row cells open mode-specific Details with stable Telemetry and exact IDs', async ({
  page,
}) => {
  await enableEvals(page)
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await openExamples(page, 'experiment=direct')
  const row = page.locator('[data-result-id="request-latency"]')
  await row.locator('.eval-results__assessment').click()
  const detail = page.locator('#signal-drawer-detail')
  await expect(row).toHaveClass(/table-row--selected/)
  await expect(
    detail.getByRole('group', { name: 'score', exact: true }).first()
  ).toBeVisible()
  const telemetry = detail
    .locator('details.collapse:visible')
    .filter({ has: page.locator('summary').filter({ hasText: 'Telemetry' }) })
  const identity = telemetry.getByRole('group', {
    name: 'Result · Execution identifiers',
  })
  const reference = exampleExperiments
    .find(experiment => experiment.id === 'direct')!
    .results.find(result => result.id === 'request-latency')!.telemetry!
  await expect(
    identity.getByTitle(reference.traceID, { exact: true })
  ).toBeVisible()
  await expect(
    identity.getByTitle(reference.spanID!, { exact: true })
  ).toBeVisible()
  const url = page.url()
  await expect(identity).toContainText(reference.traceID)
  await expect(identity).toContainText(reference.spanID!)
  await expect(identity.getByRole('button', { name: /^Copy/ })).toHaveCount(0)
  expect(page.url()).toBe(url)
  await expect(telemetry.locator('a').first()).toHaveAttribute(
    'href',
    `/traces/${reference.traceID}?span=${reference.spanID}`
  )
  await telemetry.locator(':scope > summary').click()

  await showOnly(page, 'Tool calls')
  await expect(telemetry).not.toHaveAttribute('open')
  await expect(
    detail.getByRole('group', { name: 'tool calls', exact: true })
  ).toBeVisible()
  await expect(
    detail.getByRole('group', { name: 'score', exact: true })
  ).toHaveCount(0)
  await showOnly(page, 'Cost')
  await expect(telemetry).not.toHaveAttribute('open')
  const accounting = detail.getByRole('table', {
    name: 'Result accounting',
    exact: true,
  })
  await expect(
    accounting.getByRole('group', {
      name: 'total API estimate (CAD)',
      exact: true,
    })
  ).toContainText('C$0.123')
  const caseRow = page.locator('[data-outline-id="case/request-latency"]')
  await caseRow.locator('td').last().click()
  await expect(caseRow).toHaveClass(/table-row--selected/)
  await expect(
    detail.getByRole('group', { name: 'total API estimate (CAD)', exact: true })
  ).toContainText('C$0.369')

  await showOnly(page, 'Tool calls')
  const tool = page.locator('[data-outline-id="request-latency/tool/1"]')
  await tool.locator('td').last().click()
  await expect(tool).toHaveClass(/table-row--selected/)
  await expect(
    detail.getByRole('group', { name: 'error', exact: true })
  ).toContainText('SQL binder error')
  const toolIdentity = detail.getByRole('group', { name: 'bash identifiers' })
  const toolTelemetry = detail
    .getByRole('region', { name: 'bash details', exact: true })
    .locator('details.collapse')
    .filter({
      has: page.locator(':scope > summary').filter({ hasText: 'Telemetry' }),
    })
  await expect(toolTelemetry).not.toHaveAttribute('open')
  await toolTelemetry.locator(':scope > summary').click()
  await expect(
    toolIdentity.getByTitle('eeee000000000003', { exact: true })
  ).toBeVisible()
  const toolSections = detail
    .getByRole('region', { name: 'bash details', exact: true })
    .locator('details.collapse')
  await expect(toolSections).toHaveCount(2)
  await expect(toolSections.nth(0).locator(':scope > summary')).toContainText(
    'bash'
  )
  await expect(toolSections.nth(1).locator(':scope > summary')).toContainText(
    'Telemetry'
  )
})

test('column sorting preserves selection and keeps unknown measurements last within cases', async ({
  page,
}) => {
  await enableEvals(page)
  await openExamples(page, 'experiment=direct&result=request-latency')
  const selected = page.locator('[data-result-id="request-latency"]')
  const url = page.url()
  const results = page.getByRole('table', { name: 'Results for Direct agent' })
  const header = results.getByRole('columnheader', {
    name: 'Sort by Assessment',
  })
  await header.getByRole('button').click()
  await expect(header).toHaveAttribute('aria-sort', 'ascending')
  await expect(results.locator('[data-row-kind="case"]').first()).toContainText(
    'Correlate the trace and logs'
  )
  await header.getByRole('button').click()
  await expect(header).toHaveAttribute('aria-sort', 'descending')
  await expect(results.locator('[data-row-kind="case"]').first()).toContainText(
    'Explain the checkout failure'
  )
  await expect(results.locator('[data-result-id]').last()).toHaveAttribute(
    'data-case-id',
    'service-errors'
  )
  await expect(selected).toHaveClass(/table-row--selected/)
  expect(page.url()).toBe(url)
  await showOnly(page, 'Tool calls')
  const time = page.getByRole('table', { name: 'Tool calls for Direct agent' })
  await time.getByRole('button', { name: 'Sort by Duration' }).click()
  await time.getByRole('button', { name: 'Sort by Duration' }).click()
  await expect(time.locator('[data-row-kind="case"]').first()).toContainText(
    'Compare CPU evidence'
  )
  await expect(time.locator('[data-result-id]').last()).toHaveAttribute(
    'data-case-id',
    'service-errors'
  )
  await showOnly(page, 'Cost')
  const costs = page.getByRole('table', { name: 'Cost for Direct agent' })
  await costs.getByRole('button', { name: 'Sort by Total cost' }).click()
  await costs.getByRole('button', { name: 'Sort by Total cost' }).click()
  await expect(costs.locator('[data-row-kind="case"]').first()).toContainText(
    'Inspect sequence types'
  )
  await expect(costs.locator('[data-row-kind="case"]').last()).toContainText(
    'Locate service errors'
  )
  await showOnly(page, 'Results')
  await expect(header).toHaveAttribute('aria-sort', 'descending')
  await expect(selected).toHaveClass(/table-row--selected/)
  expect(page.url()).toBe(url)
})

test('column resizing uses shared handles and retains widths, sorting and selection', async ({
  page,
}) => {
  await enableEvals(page)
  await page.setViewportSize({ width: 2560, height: 1000 })
  await openExamples(page, 'experiment=direct&result=request-latency')
  const url = page.url()
  const table = page.getByRole('table', { name: 'Results for Direct agent' })
  const heading = table.getByRole('columnheader', {
    name: /Duration/,
  })
  const handle = page.getByRole('separator', {
    name: 'Resize Duration column',
    exact: true,
  })
  const width = () =>
    heading.evaluate(element => element.getBoundingClientRect().width)
  await expect(handle).toBeVisible()
  const before = await width()
  const hitAreas = await page.evaluate(() => {
    const handle = document.querySelector<HTMLElement>(
      '[aria-label="Resize Duration column"]'
    )!
    const line = handle
      .querySelector('.col-resize-bar__line')!
      .getBoundingClientRect()
    const x = line.left + line.width / 2
    const parent = document
      .querySelector('[data-row-kind="case"]')!
      .getBoundingClientRect()
    const child = document
      .querySelector('[data-result-id]')!
      .getBoundingClientRect()
    return {
      parent: handle.contains(
        document.elementFromPoint(x, parent.top + parent.height / 2)
      ),
      child: handle.contains(
        document.elementFromPoint(x, child.top + child.height / 2)
      ),
    }
  })
  expect(hitAreas).toEqual({ parent: true, child: true })
  await handle.focus()
  await page.keyboard.press('ArrowLeft')
  await expect.poll(width).toBeCloseTo(before - 16, 0)
  const box = (await handle.boundingBox())!
  await page.mouse.move(box.x + box.width / 2, box.y + 20)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 - 24, box.y + 20, { steps: 4 })
  await page.mouse.up()
  await expect.poll(width).toBeCloseTo(before - 40, 0)
  await heading.getByRole('button').click()
  await expect(heading).toHaveAttribute('aria-sort', 'ascending')
  await expect(page.locator('[data-result-id="request-latency"]')).toHaveClass(
    /table-row--selected/
  )
  expect(page.url()).toBe(url)
  await showOnly(page, 'Tool calls')
  const duration = page
    .getByRole('table', { name: 'Tool calls for Direct agent' })
    .getByRole('columnheader', { name: /Duration/ })
  const durationWidth = await duration.evaluate(
    element => element.getBoundingClientRect().width
  )
  await page
    .getByRole('separator', { name: 'Resize Duration column', exact: true })
    .focus()
  await page.keyboard.press('ArrowRight')
  await expect
    .poll(() =>
      duration.evaluate(element => element.getBoundingClientRect().width)
    )
    .toBeCloseTo(durationWidth + 16, 0)
  await showOnly(page, 'Results')
  await expect.poll(width).toBeCloseTo(before - 40, 0)
  await expect(heading).toHaveAttribute('aria-sort', 'ascending')
  await page.reload()
  await expect.poll(width).toBeCloseTo(before - 40, 0)
  const viewport = page.getByRole('region', {
    name: 'Results for Direct agent table viewport',
  })
  await viewport.evaluate(element => (element.scrollLeft = 80))
  await expect
    .poll(async () => {
      const edge = await heading.evaluate(
        element => element.getBoundingClientRect().right
      )
      const bounds = (await handle
        .locator('.col-resize-bar__line')
        .boundingBox())!
      return Math.abs(edge - (bounds.x + bounds.width / 2))
    })
    .toBeLessThan(1)
  await page
    .getByRole('searchbox', { name: 'Search evals' })
    .fill('slow request')
  await expect(page).toHaveURL(/q=slow\+request/)
  await expect(table.locator('[data-result-id]')).toHaveCount(16)
  for (const toggle of await table
    .locator(
      '[data-row-kind="case"]:not([data-case-id="request-latency"]) button[aria-expanded]'
    )
    .all())
    await toggle.click()
  await expect(table.locator('[data-result-id]')).toHaveCount(3)
  await expect
    .poll(async () => {
      const bottom = await table.evaluate(
        element => element.getBoundingClientRect().bottom
      )
      const bounds = (await handle.boundingBox())!
      return Math.abs(bottom - (bounds.y + bounds.height))
    })
    .toBeLessThan(1)
  await table
    .locator(
      '[data-row-kind="case"][data-case-id="request-latency"] button[aria-expanded]'
    )
    .click()
  await expect(table.locator('[data-result-id]')).toHaveCount(0)
  await expect(handle).toHaveCSS('clip-path', 'none')
  const parent = (await table
    .locator('[data-row-kind="case"][data-case-id="request-latency"]')
    .boundingBox())!
  const grip = (await handle.locator('.col-resize-bar__line').boundingBox())!
  const x = grip.x + grip.width / 2
  const y = parent.y + parent.height / 2
  await page.mouse.move(x, y)
  await expect(handle.locator('.col-resize-bar__line')).toHaveCSS(
    'opacity',
    '1'
  )
  const collapsedWidth = await width()
  await page.mouse.down()
  await page.mouse.move(x + 24, y, { steps: 4 })
  await page.mouse.up()
  await expect.poll(width).toBeCloseTo(collapsedWidth + 24, 0)
  for (const key of ['End', 'Home']) {
    await handle.focus()
    await page.keyboard.press(key)
    const bounds = await table.evaluate(element => ({
      width: element.getBoundingClientRect().width,
      viewport: element.closest('[role="region"]')!.clientWidth,
      answers: [...element.querySelectorAll('thead th[scope="col"]')].map(
        cell => cell.getBoundingClientRect().width
      ),
    }))
    expect(bounds.width).toBeLessThanOrEqual(bounds.viewport + 1)
    expect(bounds.answers.every(width => width >= 144)).toBe(true)
    const limit = await handle.getAttribute(
      key === 'End' ? 'aria-valuemax' : 'aria-valuemin'
    )
    await expect(handle).toHaveAttribute('aria-valuenow', limit!)
  }
})

test('case branches stay connected to aligned repetitions through sorting and resizing', async ({
  page,
}) => {
  await enableEvals(page)
  await page.addInitScript(() =>
    localStorage.setItem('theme', 'madoromi-aki-yoru')
  )
  await page.setViewportSize({ width: 1920, height: 1000 })
  await openExamples(page, 'experiment=direct&result=request-latency-result-2')
  const table = page.getByRole('table', { name: 'Results for Direct agent' })
  await expect(table.locator('[data-row-kind="case"]')).toHaveCount(8)
  await expect(table.locator('[data-result-id]')).toHaveCount(16)
  const parent = table.locator(
    '.eval-results__case[data-case-id="request-latency"]'
  )
  const children = table.locator(
    '[data-case-id="request-latency"][data-result-id]'
  )
  await expect(parent).toContainText('Find the slow request')
  await expect(
    parent.getByRole('img', { name: 'Span error', exact: true })
  ).toBeVisible()
  await expect(children.nth(1)).toHaveClass(/table-row--selected/)
  await expect(
    table.locator('.eval-results__case[data-case-id="cpu-evidence"] .bg-papaya')
  ).toHaveCount(1)
  await expect(
    table.locator(
      '.eval-results__case[data-case-id="checkout-latency"] .tree-toggle .bg-primary'
    )
  ).toHaveCount(1)
  const palette = await table.evaluate(element => {
    const dot = element.querySelector(
      '.eval-results__case[data-case-id="checkout-latency"] .bg-primary'
    )!
    const pass = element.querySelector('[data-result-id] .bg-accent')!
    return {
      uncorrelated: getComputedStyle(dot).backgroundColor,
      success: getComputedStyle(pass).backgroundColor,
    }
  })
  expect(palette).toEqual({
    uncorrelated: themeRgb('madoromi-aki-yoru', 'primary'),
    success: themeRgb('madoromi-aki-yoru', 'accent'),
  })

  async function checkGeometry() {
    const geometry = await table.evaluate(element => {
      const rect = (node: Element) => node.getBoundingClientRect()
      const parent = element.querySelector(
        '.eval-results__case[data-case-id="request-latency"]'
      )!
      const dot = rect(parent.querySelector('.size-2')!)
      const stem = rect(parent.querySelector('.gutter__stem')!)
      const children = [
        ...element.querySelectorAll(
          '[data-case-id="request-latency"][data-result-id]'
        ),
      ]
      const headers = [...element.querySelectorAll('thead th')]
      return {
        stemOffset: Math.abs(dot.x + dot.width / 2 - stem.x - stem.width / 2),
        stemStart: Math.abs(dot.bottom - stem.top),
        join: Math.abs(
          stem.bottom - rect(children[0].querySelector('.seg')!).top
        ),
        rows: children.map(row => {
          const arm = rect([...row.querySelectorAll('.seg__arm')].at(-1)!)
          const childDot = rect(row.querySelector('.gutter__node .size-2')!)
          const count = rect(row.querySelector('.tree-child-count')!)
          const parentCount = rect(parent.querySelector('.tree-child-count')!)
          return {
            gap: Math.abs(arm.right - childDot.left),
            level: Math.abs(
              arm.bottom - (childDot.top + childDot.height / 2 + 0.5)
            ),
            indent: childDot.left - dot.left,
            countIndent: count.left - parentCount.left,
            countAfterDot: count.left > childDot.right,
            alignment: [...row.children].every(
              (cell, index) =>
                Math.abs(rect(cell).right - rect(headers[index]).right) < 1
            ),
          }
        }),
        elbows: children.map(
          row => row.querySelectorAll('.seg__arm--elbow').length
        ),
      }
    })
    expect(geometry.stemOffset).toBeLessThan(1)
    expect(geometry.stemStart).toBeLessThan(1)
    expect(geometry.join).toBeLessThan(1)
    for (const row of geometry.rows) {
      expect(row.gap).toBeLessThan(1)
      expect(row.level).toBeLessThan(1)
      expect(row.indent).toBeCloseTo(18, 0)
      expect(row.countIndent).toBeCloseTo(row.indent, 0)
      expect(row.countAfterDot).toBe(true)
      expect(row.alignment).toBe(true)
    }
    expect(geometry.elbows).toEqual([0, 0, 1])
  }
  await checkGeometry()
  const toggle = parent.locator('button[aria-expanded]')
  const gutterBounds = await toggle.evaluate(button => ({
    width: button.getBoundingClientRect().width,
    height: button.getBoundingClientRect().height,
    rowHeight: button.closest('tr')!.getBoundingClientRect().height,
  }))
  expect(gutterBounds.width).toBeGreaterThanOrEqual(44)
  expect(gutterBounds.height).toBeCloseTo(gutterBounds.rowHeight, 0)
  const totals = await parent.locator('td').allTextContents()
  const selectedURL = page.url()
  await toggle.click({ position: { x: 1, y: 1 } })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(children).toHaveCount(0)
  expect(await parent.locator('td').allTextContents()).toEqual(totals)
  expect(page.url()).toBe(selectedURL)
  await page.keyboard.press('Space')
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(children).toHaveCount(3)
  await expect(children.nth(1)).toHaveClass(/table-row--selected/)
  await expect(parent.locator('th')).toHaveAttribute('colspan', '2')
  const title = parent.locator('.tree-row-identity__title > button')
  const initialHeight = await parent.evaluate(
    element => element.getBoundingClientRect().height
  )
  await title.evaluate(
    element =>
      (element.textContent =
        'A long evaluation case name that explains the question and its context. '.repeat(
          12
        ))
  )
  expect(
    await toggle.evaluate(button =>
      Math.abs(
        button.getBoundingClientRect().height -
          button.closest('tr')!.getBoundingClientRect().height
      )
    )
  ).toBeLessThan(1)
  expect(
    await parent.evaluate(element => element.getBoundingClientRect().height)
  ).toBeCloseTo(initialHeight, 0)
  await checkGeometry()
  await table.getByRole('button', { name: 'Sort by Started' }).click()
  await table.getByRole('button', { name: 'Sort by Started' }).click()
  await expect
    .poll(() =>
      children.evaluateAll(rows =>
        rows.map(row => row.getAttribute('data-result-id'))
      )
    )
    .toEqual([
      'request-latency-result-3',
      'request-latency-result-2',
      'request-latency',
    ])
  await page
    .getByRole('separator', {
      name: 'Resize Duration column',
      exact: true,
    })
    .focus()
  await page.keyboard.press('ArrowRight')
  await table.locator('..').evaluate(element => (element.scrollLeft = 40))
  await checkGeometry()
  await children.first().locator('.tree-row-identity__title button').click()
  expect(new URL(page.url()).searchParams.get('result')).toBe(
    await children.first().getAttribute('data-result-id')
  )
})

for (const theme of ['madoromi-aki-yoru', 'madoromi-fuyu-hiru']) {
  test(`Case and Result stay pinned with shared row feedback in ${theme}`, async ({
    page,
  }) => {
    await enableEvals(page)
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.emulateMedia({
      reducedMotion: theme === 'madoromi-aki-yoru' ? 'reduce' : 'no-preference',
    })
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await openExamples(page, 'experiment=direct&result=request-latency')
    const table = page.getByRole('table', { name: 'Results for Direct agent' })
    const viewport = page.getByRole('region', {
      name: 'Results for Direct agent table viewport',
    })
    const selected = table.locator('[data-result-id="request-latency"]')
    await expect(table.locator('thead tr')).toHaveCount(1)
    await expect(
      table.getByRole('columnheader', {
        name: 'Experiment · Test case · Result',
        exact: true,
      })
    ).toBeVisible()
    for (const header of await table.locator('thead th').all())
      await expect(header).toHaveCSS('font-weight', '400')
    const measure = () =>
      table.evaluate(element => {
        const rect = (selector: string) =>
          element.querySelector(selector)!.getBoundingClientRect()
        const caseHeader = rect('.eval-results__case-heading')
        const resultHeader = rect('.eval-results__result-heading')
        const caseCell = rect(
          '[data-result-id="request-latency"] > :nth-child(1)'
        )
        return {
          caseX: caseHeader.x,
          caseRight: caseHeader.right,
          resultX: resultHeader.x,
          labelGap: Number.parseFloat(
            getComputedStyle(
              element.querySelector('.eval-results__case-heading')!
            ).columnGap
          ),
          right: resultHeader.right,
          caseCellX: caseCell.x,
          identityRight: caseCell.right,
          headingRight: rect('thead th').right,
          titleX: rect('.tree-row-identity__title').x,
          assessmentX: rect('thead th[scope="col"]').x,
        }
      })
    const before = await measure()
    for (const x of [160, 80, 0, 240]) {
      await viewport.evaluate((element, x) => (element.scrollLeft = x), x)
      const actualScroll = await viewport.evaluate(
        element => element.scrollLeft
      )
      await expect
        .poll(async () => (await measure()).caseX)
        .toBeCloseTo(before.caseX, 0)
      const after = await measure()
      expect(after.resultX).toBeCloseTo(before.resultX, 0)
      expect(after.caseCellX).toBeCloseTo(before.caseX, 0)
      expect(after.identityRight).toBeCloseTo(after.headingRight, 0)
      expect(after.titleX).toBeCloseTo(before.titleX, 0)
      expect(after.assessmentX).toBeCloseTo(
        before.assessmentX - actualScroll,
        0
      )
    }
    await expect(
      page.getByRole('separator', {
        name: /^Resize (Test case|Result) column$/,
      })
    ).toHaveCount(1)
    expect(before.resultX - before.caseRight).toBeCloseTo(before.labelGap, 0)
    expect(before.right - before.resultX).toBeLessThan(72)
    const result = selected.locator('.tree-row-identity__title')
    await result.hover()
    const backgrounds = () =>
      selected.evaluate(row =>
        [...row.children].map(cell => getComputedStyle(cell).backgroundColor)
      )
    await expect.poll(async () => new Set(await backgrounds()).size).toBe(1)
    expect((await backgrounds())[0]).not.toBe('rgba(0, 0, 0, 0)')
    if (theme === 'madoromi-aki-yoru') {
      await expect(selected.locator('th')).toHaveCSS(
        'transition-duration',
        '0s'
      )
    }
    await result.click()
    await expect(selected).toHaveClass(/table-row--selected/)
    await expect(page).toHaveURL(/result=request-latency/)
    await page.setViewportSize({ width: 390, height: 900 })
    await expect(table.locator('thead .table-cell--pinned')).toHaveCount(0)
  })
}

test('waterfall selection reveals closed Details and experiment selection restores experiment context', async ({
  page,
}) => {
  await enableEvals(page)
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'false')
  )
  await openExamples(page)
  const detail = page.locator('#signal-drawer-detail')
  await expect(detail).toBeHidden()
  const result = page.locator('[data-result-id]').first()
  await result.locator('.tree-row-identity__title button').click()
  await result.locator('.span-bar').click()
  await expect(
    detail.getByRole('region', { name: 'Result details', exact: true })
  ).toBeVisible()
  await page
    .getByRole('button', { name: /Guide-assisted agent/ })
    .locator('.signal-row__title')
    .click()
  await expect(
    detail.getByRole('region', { name: 'Experiment details', exact: true })
  ).toBeVisible()
  expect(new URL(page.url()).searchParams.has('result')).toBe(false)
})

for (const theme of ['madoromi-aki-yoru', 'madoromi-fuyu-hiru']) {
  for (const width of [1440, 390]) {
    test(`Evals accessibility retains approved native metadata inks in ${theme} at ${width}px`, async ({
      page,
    }) => {
      await enableEvals(page)
      await page.setViewportSize({ width, height: 900 })
      await page.addInitScript(
        ({ theme, width }) => {
          localStorage.setItem('theme', theme)
          if (width < 600) localStorage.setItem('signal-drawer:open', 'false')
        },
        { theme, width }
      )
      await openExamples(
        page,
        'experiment=guide-assisted&result=request-latency'
      )
      await expect(
        page.getByRole('region', { name: 'Result details', exact: true })
      ).toBeVisible()
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      ).toBe(true)
      const scan = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'])
        .analyze()
      // Native Subtle is intentionally not contrast-adjusted. These two
      // selected-state metadata labels retain it; all other axe findings fail.
      // Keep the full report so this is not represented as WCAG conformance.
      await test.info().attach('accessibility-scan', {
        body: JSON.stringify(scan.violations, null, 2),
        contentType: 'application/json',
      })
      if (theme === 'madoromi-fuyu-hiru') {
        const targets = [
          ...(width === 1440
            ? [/\.signal-row--selected.*\.signal-date__label/]
            : []),
          /\.table-row--selected.*\.tree-child-count/,
        ]
        expect(scan.violations).toMatchObject([
          {
            id: 'color-contrast',
            nodes: targets.map(target => ({
              target: [expect.stringMatching(target)],
              any: [
                {
                  id: 'color-contrast',
                  data: { fgColor: getTheme(theme).colors.subtle },
                },
              ],
            })),
          },
        ])
      } else expect(scan.violations).toEqual([])
    })
  }
}
