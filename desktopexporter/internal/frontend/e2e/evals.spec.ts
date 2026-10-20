import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { exampleRuns } from '../src/components/evals/examples'

async function enableEvals(page: Page, enabled = true) {
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON()
    if (request.method !== 'query') return route.fallback()
    const sql = request.params.sql.replaceAll('-', '')
    const rows = exampleRuns.flatMap(run =>
      run.attempts.flatMap(attempt => {
        if (!attempt.telemetry?.spanID) return []
        const references = [
          {
            reference: attempt.telemetry,
            name: attempt.name,
            duration: BigInt(Math.round((attempt.seconds ?? 1) * 1e9)),
            error: attempt.state === 'Error',
          },
          ...attempt.tools.map(tool => ({
            reference: tool.telemetry,
            name: tool.name,
            duration: 500000000n,
            error: tool.state === 'Error',
          })),
        ]
        return references.flatMap(({ reference, name, duration, error }) =>
          reference?.spanID &&
          sql.includes(reference.traceID) &&
          sql.includes(reference.spanID)
            ? [
                [
                  reference.traceID,
                  reference.spanID,
                  name,
                  'eval-agent',
                  '1700000000000000001',
                  String(1700000000000000001n + duration),
                  error ? 2 : 1,
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
        result: { rows, truncated: false },
      },
    })
  })
  await page.route('**/api/features', route =>
    route.fulfill({
      json: { evals: { enabled, label: 'Experimental and unsupported' } },
    })
  )
}

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

test('run selection, assessments, filtering and browser history stay connected', async ({
  page,
}) => {
  await enableEvals(page)
  await page.goto('/evals')
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
    page.getByRole('region', { name: 'Guide-assisted agent', exact: true })
  ).toBeVisible()
  await page
    .locator('[data-attempt-id]')
    .getByRole('button', { name: /Find the slow request/ })
    .first()
    .click()
  const detail = page.getByRole('region', {
    name: 'Attempt details',
    exact: true,
  })
  const bubbles = detail.locator('details.collapse')
  await expect(bubbles).toHaveCount(4)
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
  await page.locator('summary').filter({ hasText: 'Error' }).click()
  await expect(
    page.getByText('SQL binder error:', { exact: false })
  ).toBeVisible()
  await page.reload()
  await expect(bubbles.nth(3).locator(':scope > summary')).toHaveText(
    /Tool calls/
  )
  await expect(bubbles.nth(3)).toHaveAttribute('open')
  await page.getByRole('searchbox', { name: 'Search evals' }).fill('Mixed')
  await expect(
    page
      .getByRole('table', { name: 'Results for Guide-assisted agent' })
      .getByRole('row')
  ).toHaveCount(5)
  await page
    .getByRole('searchbox', { name: 'Search evals' })
    .fill('nonexistent-score')
  await expect(
    page.getByText('No matching attempts', { exact: true })
  ).toBeVisible()
  await toolbar.getByRole('button', { name: 'Clear search' }).click()
  await page.getByRole('button', { name: /Direct agent/ }).click()
  await expect(
    page.getByRole('region', { name: 'Direct agent', exact: true })
  ).toBeVisible()
  await page
    .locator('[data-attempt-id]')
    .getByRole('button', { name: /Compare CPU evidence/ })
    .first()
    .click()
  await expect(
    page.getByText('The investigator process exited', { exact: false })
  ).toBeVisible()
  await detail
    .locator('summary')
    .filter({ hasText: 'Execution details' })
    .click()
  const accounting = detail.getByRole('table', { name: 'Attempt accounting' })
  for (const key of ['investigator estimate (CAD)', 'grader estimate (CAD)']) {
    await expect(
      accounting.locator(`[data-field="${key}"] .detail-property__body`)
    ).toHaveText('Unknown')
  }
  await page.goBack()
  await expect(
    page.getByRole('table', { name: 'Grading for this run' })
  ).toBeVisible()
  await page.getByRole('button', { name: /Revised instructions/ }).click()
  const plannedResults = page.getByRole('table', {
    name: 'Results for Revised instructions',
  })
  await expect(
    plannedResults
      .locator('[data-attempt-id]')
      .getByText('Unscored', { exact: true })
  ).toHaveCount(48)
  await expect(
    page
      .getByRole('table', { name: 'Grading for this run' })
      .getByText('Unknown', { exact: true })
  ).toHaveCount(2)
})

test('Results, Time and Cost preserve selection and distinguish missing measurements and totals', async ({
  page,
}) => {
  await enableEvals(page)
  await page.goto('/evals?run=direct&attempt=cpu-evidence')
  const selected = page
    .getByRole('button', {
      name: 'Compare CPU evidence',
      exact: true,
    })
    .first()
  const url = page.url()
  await expect(selected).toHaveAttribute('aria-pressed', 'true')
  await expect(
    page.getByRole('tab', { name: /^(Results|Time|Cost)$/ })
  ).toHaveCount(3)
  await expect(
    page.getByRole('button', { name: 'Columns', exact: true })
  ).toHaveCount(0)
  await page.getByRole('tab', { name: 'Time', exact: true }).click()
  const time = page.getByRole('table', { name: 'Time for Direct agent' })
  await expect(time.locator('[data-attempt-id="cpu-evidence"]')).toContainText(
    '89.000 s'
  )
  await expect(
    time.locator('[data-attempt-id="checkout-latency"] td').first()
  ).toHaveText('')
  await expect(
    time.getByRole('columnheader', { name: 'Failures' })
  ).toBeVisible()
  const failedAttempt = time.locator('[data-attempt-id="request-latency"]')
  await expect(failedAttempt.locator('td').nth(1)).toHaveText('3')
  await expect(failedAttempt.locator('td').nth(2)).toHaveText('1')
  await expect(
    time
      .locator('.eval-results__case[data-case-id="request-latency"] td')
      .nth(2)
  ).toHaveText('3')
  const traceID = `eeee02${'2'.padStart(26, '0')}`
  const trace = failedAttempt.getByRole('link', {
    name: 'Open span Find the slow request, 37.000 s',
  })
  await expect(trace).toHaveAttribute(
    'href',
    `/traces/${traceID}?span=eeee000000000001`
  )
  await failedAttempt.getByRole('button', { name: /^Expand tools/ }).click()
  const toolRows = time.locator('[data-tool-attempt-id="request-latency"]')
  await expect(toolRows).toHaveCount(3)
  const toggle = failedAttempt.getByRole('button', { name: /^Collapse tools/ })
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
    toolRows.nth(1).getByRole('link', { name: 'Open span bash, 500.000 ms' })
  ).toHaveAttribute('href', `/traces/${traceID}?span=eeee000000000003`)
  expect(page.url()).toBe(url)
  await failedAttempt.getByRole('button', { name: /^Collapse tools/ }).focus()
  await page.keyboard.press('Space')
  await expect(toolRows).toHaveCount(0)
  await expect(
    time.locator('[data-attempt-id="checkout-latency"] td').first()
  ).toHaveText('')
  await expect(
    time.locator('[data-attempt-id="checkout-latency"]').getByRole('link')
  ).toHaveCount(0)
  await time
    .getByRole('button', { name: 'Sort by Failures', exact: true })
    .click()
  await time
    .getByRole('button', { name: 'Sort by Failures', exact: true })
    .click()
  await expect(time.locator('[data-attempt-id]').first()).toHaveAttribute(
    'data-case-id',
    'request-latency'
  )
  await expect(
    time.locator('[data-attempt-id="checkout-latency"] td').nth(2)
  ).toHaveText('0')
  await page.getByRole('tab', { name: 'Cost', exact: true }).click()
  const costs = page.getByRole('table', { name: 'Cost for Direct agent' })
  expect(
    await costs
      .locator('.eval-results__case')
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
    costs.getByRole('columnheader', { name: /Investigator/ })
  ).toBeVisible()
  await expect(
    costs.getByRole('columnheader', { name: /Grader/ })
  ).toBeVisible()
  await expect(
    costs
      .locator('[data-attempt-id="cpu-evidence"]')
      .getByRole('cell', { name: 'Unknown', exact: true })
  ).toHaveCount(2)
  const totals = costs.getByRole('row', {
    name: 'Estimated cost for shown attempts',
  })
  await expect(totals).toContainText('Partial estimate')
  const footer = costs.locator('tfoot')
  await expect(footer).toHaveCSS('opacity', '1')
  const divider = await footer.evaluate(element => {
    const style = getComputedStyle(element, '::before')
    return { opacity: style.opacity, height: style.height, top: style.top }
  })
  expect(divider).toEqual({ opacity: '0.6', height: '1px', top: '0px' })
  expect(
    await costs.evaluate(table => {
      const headers = [
        ...table.querySelectorAll(
          '.eval-results__case-heading, .eval-results__rep-heading, thead th[scope="col"]'
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
  await expect(selected).toHaveAttribute('aria-pressed', 'true')
  expect(page.url()).toBe(url)
  await page.getByRole('tab', { name: 'Results', exact: true }).click()
  const results = page.getByRole('table', { name: 'Results for Direct agent' })
  await expect(
    results
      .locator('[data-attempt-id="cpu-evidence"]')
      .getByText('Unscored', { exact: true })
  ).toHaveCount(3)
  await expect(results.locator('[data-attempt-id="trace-logs"]')).toContainText(
    '(0/1)'
  )
  await expect(selected).toHaveAttribute('aria-pressed', 'true')
  expect(page.url()).toBe(url)
  const parent = page.locator(
    '.eval-results__case[data-case-id="cpu-evidence"]'
  )
  await parent.getByRole('button').click()
  await page.getByRole('tab', { name: 'Time', exact: true }).click()
  await expect(parent.getByRole('button')).toHaveAttribute(
    'aria-expanded',
    'false'
  )
  await expect(
    time.locator('[data-case-id="cpu-evidence"][data-attempt-id]')
  ).toHaveCount(0)
  await page.getByRole('tab', { name: 'Cost', exact: true }).click()
  await expect(parent.getByRole('button')).toHaveAttribute(
    'aria-expanded',
    'false'
  )
  await parent.getByRole('button').click()
  await expect(selected).toHaveAttribute('aria-pressed', 'true')
  expect(page.url()).toBe(url)
  await page
    .getByRole('searchbox', { name: 'Search evals' })
    .fill('CPU evidence')
  await expect(costs.locator('[data-attempt-id]')).toHaveCount(2)
  await expect(
    totals.getByRole('cell', { name: 'Unknown', exact: true })
  ).toHaveCount(2)
  await expect(selected).toHaveAttribute('aria-pressed', 'true')
  await page.getByRole('button', { name: 'Clear search', exact: true }).click()
  await expect(costs.locator('[data-attempt-id]')).toHaveCount(16)
  expect(page.url()).toBe(url)
})

test('column sorting preserves selection and keeps unknown measurements last within cases', async ({
  page,
}) => {
  await enableEvals(page)
  await page.goto('/evals?run=direct&attempt=request-latency')
  const selected = page
    .getByRole('button', {
      name: 'Find the slow request',
      exact: true,
    })
    .first()
  const url = page.url()
  const results = page.getByRole('table', { name: 'Results for Direct agent' })
  const header = results.getByRole('columnheader', { name: /Task completion/ })
  await header.getByRole('button').click()
  await expect(header).toHaveAttribute('aria-sort', 'ascending')
  await expect(results.locator('tbody tr').first()).toContainText(
    'Correlate the trace and logs'
  )
  await header.getByRole('button').click()
  await expect(header).toHaveAttribute('aria-sort', 'descending')
  await expect(results.locator('tbody tr').first()).toContainText(
    'Explain the checkout failure'
  )
  await expect(results.locator('[data-attempt-id]').last()).toContainText(
    'Locate service errors'
  )
  await expect(selected).toHaveAttribute('aria-pressed', 'true')
  expect(page.url()).toBe(url)
  await page.getByRole('tab', { name: 'Time', exact: true }).click()
  const time = page.getByRole('table', { name: 'Time for Direct agent' })
  await time.getByRole('button', { name: 'Sort by Duration' }).click()
  await time.getByRole('button', { name: 'Sort by Duration' }).click()
  await expect(time.locator('tbody tr').first()).toContainText(
    'Compare CPU evidence'
  )
  await expect(time.locator('[data-attempt-id]').last()).toContainText(
    'Locate service errors'
  )
  await page.getByRole('tab', { name: 'Cost', exact: true }).click()
  const costs = page.getByRole('table', { name: 'Cost for Direct agent' })
  await costs.getByRole('button', { name: 'Sort by Investigator' }).click()
  await costs.getByRole('button', { name: 'Sort by Investigator' }).click()
  await expect(costs.locator('tbody tr').first()).toContainText(
    'Inspect sequence types'
  )
  await expect(costs.locator('[data-attempt-id]').last()).toContainText(
    'Locate service errors'
  )
  await page.getByRole('tab', { name: 'Results', exact: true }).click()
  await expect(header).toHaveAttribute('aria-sort', 'descending')
  await expect(selected).toHaveAttribute('aria-pressed', 'true')
  expect(page.url()).toBe(url)
})

test('column resizing uses shared handles and retains widths, sorting and selection', async ({
  page,
}) => {
  await enableEvals(page)
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto('/evals?run=direct&attempt=request-latency')
  const url = page.url()
  const table = page.getByRole('table', { name: 'Results for Direct agent' })
  const heading = table.getByRole('columnheader', {
    name: /Task completion/,
  })
  const handle = page.getByRole('separator', {
    name: 'Resize Task completion column',
    exact: true,
  })
  const width = () =>
    heading.evaluate(element => element.getBoundingClientRect().width)
  await expect(handle).toBeVisible()
  const before = await width()
  const hitAreas = await page.evaluate(() => {
    const handle = document.querySelector<HTMLElement>(
      '[aria-label="Resize Task completion column"]'
    )!
    const line = handle
      .querySelector('.col-resize-bar__line')!
      .getBoundingClientRect()
    const x = line.left + line.width / 2
    const parent = document
      .querySelector('.eval-results__case')!
      .getBoundingClientRect()
    const child = document
      .querySelector('[data-attempt-id]')!
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
  await page.keyboard.press('ArrowRight')
  await expect.poll(width).toBeCloseTo(before + 16, 0)
  const box = (await handle.boundingBox())!
  await page.mouse.move(box.x + box.width / 2, box.y + 20)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 + 24, box.y + 20, { steps: 4 })
  await page.mouse.up()
  await expect.poll(width).toBeCloseTo(before + 40, 0)
  await heading.getByRole('button').click()
  await expect(heading).toHaveAttribute('aria-sort', 'ascending')
  await expect(
    page
      .getByRole('button', { name: 'Find the slow request', exact: true })
      .first()
  ).toHaveAttribute('aria-pressed', 'true')
  expect(page.url()).toBe(url)
  await page.getByRole('tab', { name: 'Time', exact: true }).click()
  const duration = page
    .getByRole('table', { name: 'Time for Direct agent' })
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
  await page.getByRole('tab', { name: 'Results', exact: true }).click()
  await expect.poll(width).toBeCloseTo(before + 40, 0)
  await expect(heading).toHaveAttribute('aria-sort', 'ascending')
  await page.reload()
  await expect.poll(width).toBeCloseTo(before + 40, 0)
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
  await expect(table.locator('[data-attempt-id]')).toHaveCount(3)
  await expect
    .poll(async () => {
      const bottom = await table.evaluate(
        element => element.getBoundingClientRect().bottom
      )
      const bounds = (await handle.boundingBox())!
      return Math.abs(bottom - (bounds.y + bounds.height))
    })
    .toBeLessThan(1)
  await table.locator('.eval-results__case').getByRole('button').click()
  await expect(table.locator('[data-attempt-id]')).toHaveCount(0)
  await expect(handle).toHaveCSS('clip-path', 'none')
  const parent = (await table.locator('.eval-results__case').boundingBox())!
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
    expect(bounds.answers.every(width => width >= 160)).toBe(true)
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
  await page.addInitScript(() => localStorage.setItem('theme', 'rose-pine'))
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto('/evals?run=direct&attempt=request-latency-rep-2')
  const table = page.getByRole('table', { name: 'Results for Direct agent' })
  await expect(table.locator('.eval-results__case')).toHaveCount(8)
  await expect(table.locator('[data-attempt-id]')).toHaveCount(16)
  const parent = table.locator(
    '.eval-results__case[data-case-id="request-latency"]'
  )
  const children = table.locator(
    '[data-case-id="request-latency"][data-attempt-id]'
  )
  await expect(parent).toContainText('Find the slow request')
  await expect(
    parent.getByRole('img', { name: 'Complete', exact: true })
  ).toBeVisible()
  await expect(children.nth(1).getByRole('button')).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  await expect(
    table.locator('.eval-results__case[data-case-id="cpu-evidence"] .bg-papaya')
  ).toHaveCount(1)
  await expect(
    table.locator(
      '.eval-results__case[data-case-id="checkout-latency"] .tree-toggle .bg-base-300'
    )
  ).toHaveCount(1)
  const palette = await table.evaluate(element => {
    const dot = element.querySelector('.eval-results__case .bg-secondary')!
    const pass = element.querySelector('[data-attempt-id] .bg-accent')!
    return {
      completion: getComputedStyle(dot).backgroundColor,
      success: getComputedStyle(pass).backgroundColor,
    }
  })
  expect(palette).toEqual({
    completion: 'rgb(112, 179, 207)',
    success: 'rgb(156, 207, 216)',
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
          '[data-case-id="request-latency"][data-attempt-id]'
        ),
      ]
      const headers = [
        ...element.querySelectorAll(
          '.eval-results__case-heading, .eval-results__rep-heading, thead th[scope="col"]'
        ),
      ]
      return {
        stemOffset: Math.abs(dot.x + dot.width / 2 - stem.x - stem.width / 2),
        stemStart: Math.abs(dot.bottom - stem.top),
        join: Math.abs(
          stem.bottom - rect(children[0].querySelector('.seg')!).top
        ),
        rows: children.map(row => {
          const nameArm = rect(row.querySelector('td .seg__arm')!)
          const repArm = rect(row.querySelector('th .seg__arm')!)
          const button = rect(row.querySelector('button')!)
          return {
            gap: Math.abs(nameArm.right - repArm.left),
            level: Math.abs(nameArm.bottom - repArm.bottom),
            clearance: button.left - repArm.right,
            repAlignment: Math.abs(button.right - rect(headers[1]).right + 16),
            alignment: [...row.children].every(
              (cell, index) =>
                index === 1 ||
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
      expect(row.clearance).toBeCloseTo(8, 0)
      expect(row.repAlignment).toBeLessThan(1)
      expect(row.alignment).toBe(true)
    }
    expect(geometry.elbows).toEqual([0, 0, 1])
  }
  await checkGeometry()
  const toggle = parent.getByRole('button')
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
  await expect(children.nth(1).getByRole('button')).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  await expect(parent.locator('th')).toHaveAttribute('colspan', '2')
  const title = parent.locator('.eval-results__case-name > span:last-child')
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
  ).toBeGreaterThan(initialHeight)
  await checkGeometry()
  await table.getByRole('button', { name: 'Sort by Repetition' }).click()
  await table.getByRole('button', { name: 'Sort by Repetition' }).click()
  await expect(children.locator('button')).toHaveText(['3', '2', '1'])
  await page
    .getByRole('separator', {
      name: 'Resize Task completion column',
      exact: true,
    })
    .focus()
  await page.keyboard.press('ArrowRight')
  await table.locator('..').evaluate(element => (element.scrollLeft = 40))
  await checkGeometry()
  await children.first().getByRole('button').click()
  await expect(page).toHaveURL(/attempt=request-latency-rep-3/)
})

for (const theme of ['rose-pine', 'rose-pine-dawn']) {
  test(`Case and Rep stay pinned with shared row feedback in ${theme}`, async ({
    page,
  }) => {
    await enableEvals(page)
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.emulateMedia({
      reducedMotion: theme === 'rose-pine' ? 'reduce' : 'no-preference',
    })
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await page.goto('/evals?run=direct&attempt=request-latency')
    const table = page.getByRole('table', { name: 'Results for Direct agent' })
    const viewport = page.getByRole('region', {
      name: 'Results for Direct agent table viewport',
    })
    const selected = table.locator('[data-attempt-id="request-latency"]')
    await expect(table.locator('thead tr')).toHaveCount(1)
    await expect(
      table.getByRole('columnheader', { name: 'Case · Rep', exact: true })
    ).toBeVisible()
    for (const header of await table.locator('thead th').all())
      await expect(header).toHaveCSS('font-weight', '400')
    const measure = () =>
      table.evaluate(element => {
        const rect = (selector: string) =>
          element.querySelector(selector)!.getBoundingClientRect()
        const caseHeader = rect('.eval-results__case-heading')
        const repHeader = rect('.eval-results__rep-heading')
        const caseCell = rect(
          '[data-attempt-id="request-latency"] > :nth-child(1)'
        )
        const repCell = rect(
          '[data-attempt-id="request-latency"] > :nth-child(2)'
        )
        return {
          caseX: caseHeader.x,
          repX: repHeader.x,
          right: repHeader.right,
          caseCellX: caseCell.x,
          repCellX: repCell.x,
          titleX: rect('.eval-results__case-name').x,
          resultX: rect('thead th[scope="col"]').x,
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
      expect(after.repX).toBeCloseTo(before.repX, 0)
      expect(after.caseCellX).toBeCloseTo(before.caseX, 0)
      expect(after.repCellX).toBeCloseTo(before.repX, 0)
      expect(after.titleX).toBeCloseTo(before.titleX, 0)
      expect(after.resultX).toBeCloseTo(before.resultX - actualScroll, 0)
    }
    await expect(
      page.getByRole('separator', { name: /^Resize (Case|Rep) column$/ })
    ).toHaveCount(1)
    expect(before.repX - before.caseX).toBeLessThan(80)
    expect(before.right - before.repX).toBeLessThan(72)
    const rep = selected.getByRole('button')
    await rep.hover()
    const backgrounds = () =>
      selected.evaluate(row =>
        [...row.children].map(cell => getComputedStyle(cell).backgroundColor)
      )
    await expect.poll(async () => new Set(await backgrounds()).size).toBe(1)
    expect((await backgrounds())[0]).not.toBe('rgba(0, 0, 0, 0)')
    if (theme === 'rose-pine') {
      await expect(selected.locator('th')).toHaveCSS(
        'transition-duration',
        '0s'
      )
    }
    await rep.click()
    await expect(rep).toHaveAttribute('aria-pressed', 'true')
    await expect(page).toHaveURL(/attempt=request-latency/)
    await page.setViewportSize({ width: 390, height: 900 })
    await expect(table.locator('thead .table-cell--pinned')).toHaveCount(0)
  })
}

test('detail actions reopen the panel, including the selected attempt', async ({
  page,
}) => {
  await enableEvals(page)
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'false')
  )
  await page.goto('/evals')
  const detail = page.locator('#signal-drawer-detail')
  await expect(detail).toBeHidden()
  const attempt = page
    .locator('[data-attempt-id]')
    .getByRole('button', { name: /Find the slow request/ })
    .first()
  for (let selection = 0; selection < 2; selection++) {
    await attempt.click()
    await expect(detail).toBeVisible()
    await expect(page).toHaveURL(/attempt=request-latency/)
    await expect(
      detail.getByRole('region', { name: 'Attempt details', exact: true })
    ).toBeVisible()
    const fields = detail.locator('details.collapse').first()
    await expect(fields).toHaveAttribute('open')
    await fields.locator(':scope > summary').click()
    await expect(fields).not.toHaveAttribute('open')
    await expect(detail.locator('details.collapse').nth(1)).toHaveAttribute(
      'open'
    )
    await fields.locator(':scope > summary').click()
    await page.getByRole('button', { name: 'Collapse details' }).click()
    await expect(detail).toBeHidden()
  }
  await page.getByRole('button', { name: /Guide-assisted agent/ }).click()
  await expect(
    detail.getByRole('region', { name: 'Run details', exact: true })
  ).toBeVisible()
  expect(new URL(page.url()).searchParams.has('attempt')).toBe(false)
})

for (const theme of ['rose-pine', 'rose-pine-dawn']) {
  for (const width of [1440, 390]) {
    test(`Evals is accessible in ${theme} at ${width}px`, async ({ page }) => {
      await enableEvals(page)
      await page.setViewportSize({ width, height: 900 })
      await page.addInitScript(
        ({ theme, width }) => {
          localStorage.setItem('theme', theme)
          if (width < 600) localStorage.setItem('signal-drawer:open', 'false')
        },
        { theme, width }
      )
      await page.goto('/evals?run=guide-assisted&attempt=request-latency')
      await expect(
        page.getByRole('region', { name: 'Attempt details', exact: true })
      ).toBeVisible()
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      ).toBe(true)
      const scan = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'])
        .analyze()
      expect(scan.violations).toEqual([])
    })
  }
}
