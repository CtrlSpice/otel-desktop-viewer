import assert from 'node:assert/strict'
import { pathToFileURL } from 'node:url'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const { chromium } =
  require('../../../desktopexporter/internal/frontend/node_modules/@playwright/test') as typeof import('../../../desktopexporter/internal/frontend/node_modules/@playwright/test/index.js')
const { AxeBuilder } =
  require('../../../desktopexporter/internal/frontend/node_modules/@axe-core/playwright') as typeof import('../../../desktopexporter/internal/frontend/node_modules/@axe-core/playwright/dist/index.js')

const file = process.argv[2]
if (!file)
  throw new Error('Usage: node presentation/check-browser.ts PREVIEW.html')
const browser = await chromium.launch({ headless: true })
try {
  const context = await browser.newContext({
    colorScheme: 'dark',
    viewport: { width: 1280, height: 900 },
  })
  const page = await context.newPage()
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('request', request => {
    if (/^https?:/.test(request.url())) errors.push('Unexpected external request')
  })
  await page.goto(pathToFileURL(file).href)
  await page.locator('.model').evaluateAll(elements => elements.forEach(element => element.removeAttribute('open')))
  assert.ok(await page.evaluate(async () => {
    await document.fonts.ready
    return [...document.fonts].some(font => font.family.replaceAll('"', '') === 'Atkinson Hyperlegible Next' && font.status === 'loaded')
  }))
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor), 'rgb(25, 23, 36)')
  const checkMetricControls = async () => {
    assert.equal(await page.locator('.metrics').evaluate(element => getComputedStyle(element).backgroundColor), 'rgba(0, 0, 0, 0)')
    assert.equal(await page.locator('.board-label>.metric-heading .metrics').count(), 1)
    assert.deepEqual(await page.locator('.metrics button').allTextContents(), ['Results', 'Time', 'Cost (est., CAD)'])
    await page.locator('[data-view="result"]').focus()
    await page.keyboard.press('Enter')
    const selectedColour = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--text').trim())
    const selected = page.locator('[data-view="result"]')
    assert.equal(await selected.evaluate(element => getComputedStyle(element).boxShadow), 'none')
    assert.equal(await selected.evaluate(element => getComputedStyle(element).outlineStyle), 'solid')
    await page.waitForFunction(colour => {
      const element = document.querySelector('[data-view="result"]')!
      const expected = document.createElement('span')
      expected.style.color = colour
      document.body.append(expected)
      const equal = getComputedStyle(element).color === getComputedStyle(expected).color
      expected.remove()
      return equal
    }, selectedColour)
    assert.equal(await page.locator('#metric-label').count(), 0)
    assert.deepEqual(await page.locator('.metric-separator').allTextContents(), ['|', '|'])
  }
  await checkMetricControls()
  const costLabels = await page.locator('main').evaluate(element => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT)
    const labels: string[] = []
    while (walker.nextNode()) {
      const text = walker.currentNode.textContent?.trim() ?? ''
      if (/\bcost\b/i.test(text)) labels.push(text)
    }
    return labels
  })
  assert.ok(costLabels.length > 0)
  assert.ok(costLabels.every(label => label === 'Cost/run' || label.includes('(est., CAD)')), JSON.stringify(costLabels))
  assert.equal(await page.locator('.cost-heading').textContent(), 'Cost/run')
  const checkResultColours = async (colours: Record<string, string>) => {
    for (const [status, colour] of Object.entries(colours)) {
      const badges = page.locator(`.result-badge.result-${status}`)
      const count = await badges.count()
      assert.deepEqual(await badges.evaluateAll(elements => elements.map(element => getComputedStyle(element, '::before').backgroundColor)), Array(count).fill(colour))
      assert.deepEqual(await badges.evaluateAll(elements => elements.map(element => getComputedStyle(element).fontSize)), Array(count).fill('14px'))
    }
  }
  const checkHarnessColour = async () => {
    assert.equal(await page.evaluate(() => {
      const badge = document.createElement('span')
      badge.className = 'result-badge result-harness-error'
      document.body.append(badge)
      const colour = getComputedStyle(badge, '::before').backgroundColor
      badge.remove()
      return colour
    }), 'rgb(255, 128, 0)')
  }
  const checkModelDividers = async () => {
    const rows = await page.locator('.model').count()
    assert.deepEqual(await page.locator('.model').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderBottomWidth)), Array(rows).fill('1px'))
    assert.deepEqual(await page.locator('.model>summary').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderBottomWidth)), Array(await page.locator('.model>summary').count()).fill('0px'))
  }
  await checkResultColours({ complete: 'rgb(49, 116, 143)', partial: 'rgb(246, 193, 119)', 'no-credit': 'rgb(235, 111, 146)' })
  await checkHarnessColour()
  await page.locator('.model summary').first().focus()
  await page.keyboard.press('Enter')
  assert.equal(await page.locator('.model').first().getAttribute('open'), '')
  await page.keyboard.press('Space')
  assert.equal(await page.locator('.model').first().getAttribute('open'), null)
  for (const metric of ['time', 'cost', 'result']) {
    await page.locator(`[data-view="${metric}"]`).focus()
    await page.keyboard.press('Enter')
    assert.equal(await page.locator('body').getAttribute('data-metric'), metric)
    assert.equal(
      await page
        .locator(`[data-view="${metric}"]`)
        .getAttribute('aria-pressed'),
      'true'
    )
    assert.ok(
      await page.locator(`.model .metric.${metric}`).first().isVisible()
    )
    assert.ok(await page.locator('.metric-selector').isVisible())
    assert.equal(await page.locator('#metric-label').count(), 0)
    {
      const heading = await page.locator(metric === 'cost' ? '.cost-heading' : metric === 'time' ? '.total-heading' : '.mean-heading').boundingBox()
      const value = await page.locator(metric === 'cost' ? '.model .metric.cost strong' : metric === 'time' ? '.model .time-total strong' : '.model .result-mean strong').first().boundingBox()
      assert.ok(heading && value)
      assert.ok(Math.abs(heading.x + heading.width - value.x - value.width) < 1)
    }
  }
  const checkTimeAndProvider = async () => {
    await page.locator('[data-view="time"]').click()
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
    const ranges = await page.locator('.time-track').evaluateAll(elements => elements.filter(element => element.children.length).map(element => [...element.children].map(child => child.className)))
    assert.ok(ranges.length > 0)
    assert.ok(ranges.every(range => JSON.stringify(range) === JSON.stringify(['time-median', 'time-p95', 'time-tail'])))
    const provider = await page.locator('.model-name>span').first().boundingBox()
    const modelID = await page.locator('.model-name>.model-id').first().boundingBox()
    assert.ok(provider && modelID && modelID.y >= provider.y + provider.height)
    const caret = await page.locator('.model-heading .chevron').first().boundingBox()
    const name = await page.locator('.model-heading .model-name').first().boundingBox()
    assert.ok(caret && name && caret.x + caret.width <= name.x)
    assert.deepEqual((await new AxeBuilder({ page }).analyze()).violations.map(value => value.id), [])
    await page.locator('[data-view="result"]').click()
  }
  const checkHeaderLayout = async () => {
    for (const width of [390, 700, 701, 1024, 1600]) {
      await page.setViewportSize({ width, height: 900 })
      for (const metric of ['result', 'time', 'cost']) {
        await page.locator(`[data-view="${metric}"]`).click()
        for (const control of ['result', 'time', 'cost']) assert.ok(await page.locator(`[data-view="${control}"]`).isVisible())
        const heading = await page.locator(metric === 'cost' ? '.cost-heading' : metric === 'time' ? '.total-heading' : '.mean-heading').boundingBox()
        const column = await page.locator('.metric-heading').boundingBox()
        const value = await page.locator(`.model .metric.${metric}`).first().boundingBox()
        assert.ok(heading && column && value)
        assert.ok(Math.abs(column.x - value.x) < 1, `${width}px ${metric}: header belongs to the metric column`)
        for (const control of ['result', 'time', 'cost']) {
          const box = await page.locator(`[data-view="${control}"]`).boundingBox()
          assert.ok(box)
          assert.ok(box.x + box.width <= heading.x || box.y + box.height <= heading.y || box.y >= heading.y + heading.height, `${width}px ${metric}: controls must not overlap the value heading`)
        }
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      }
    }
    await page.locator('[data-view="result"]').click()
  }
  await checkHeaderLayout()
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await checkTimeAndProvider()
    const collapsed = await page.locator('.model').first().boundingBox()
    await page.locator('.model summary').first().click()
    const expanded = await page.locator('.model').first().boundingBox()
    assert.ok(collapsed && expanded && expanded.height > collapsed.height)
    await checkModelDividers()
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      ),
      true
    )
    assert.equal(await page.locator('.task h4').count(), 0)
    assert.equal(await page.locator('.task-group').first().evaluate(element => getComputedStyle(element).borderTopWidth), '0px')
    assert.equal(await page.locator('.task dd').first().evaluate(element => getComputedStyle(element).fontSize), '14px')
    const results = await new AxeBuilder({ page }).analyze()
    assert.deepEqual(
      results.violations.map((value) => ({
        id: value.id,
        impact: value.impact,
        count: value.nodes.length,
      })),
      []
    )
    if (process.env.EVAL_PRESENTATION_SCREENSHOT_DIR) {
      await page
        .locator('.model')
        .first()
        .evaluate((element) => {
          const target = element as HTMLElement
          target.style.outline = '5px solid #ff69b4'
          target.style.borderRadius = '12px'
        })
      await page.evaluate(() => scrollTo(0, 0))
      await page.screenshot({
        path: `${process.env.EVAL_PRESENTATION_SCREENSHOT_DIR}/dark-${width}.png`,
      })
      await page
        .locator('.model')
        .first()
        .evaluate((element) => element.removeAttribute('style'))
    }
    await page.locator('.model summary').first().click()
    if (await page.locator('.grader-row').count()) {
      assert.equal(await page.locator('.grader-row summary').count(), 0)
      assert.ok(await page.locator('.grading-output table').first().isVisible())
      assert.equal(await page.locator('.grading-output table table').count(), 0)
      for (const table of await page.locator('.grading-output>table').all()) {
        assert.equal(await table.locator('.grading-table-toggle').getAttribute('aria-expanded'), 'false')
        assert.equal(await table.locator('tbody').isVisible(), false)
        assert.ok(await table.locator('tfoot').isVisible())
        assert.equal(await table.locator('tfoot>tr').count(), 1)
        assert.equal(await table.locator('tfoot>tr>th').textContent(), 'Total')
        assert.equal(await table.locator('tfoot>tr>th').evaluate(element => getComputedStyle(element).borderTopWidth), '0px')
        assert.equal(await table.locator('tfoot').evaluate(element => getComputedStyle(element).backgroundColor), 'rgba(0, 0, 0, 0)')
        assert.equal(await table.locator('tfoot>tr>th').evaluate(element => getComputedStyle(element).fontWeight), '400')
        assert.deepEqual((await table.locator('thead>tr>th').allTextContents()).slice(-4), ['Grading Time', 'Cost (est., CAD)', 'Input Tokens', 'Output Tokens'])
        assert.ok(await table.locator('thead>tr>th').first().textContent())
        assert.notEqual(await table.locator('thead>tr>th').first().textContent(), 'Question')
        assert.equal(await table.locator('caption:not(.sr-only)').count(), 0)
        const tableCaret = await table.locator('.grading-table-caret').boundingBox()
        const tableName = await table.locator('.grading-table-toggle>span').last().boundingBox()
        assert.ok(tableCaret && tableName && tableCaret.x + tableCaret.width <= tableName.x)
        const totalStart = await table.locator('tfoot>tr>th').evaluate(element => element.getBoundingClientRect().x + parseFloat(getComputedStyle(element).paddingLeft))
        assert.ok(Math.abs(totalStart - tableName.x) < 1)
        const questions = await table.locator('tbody>tr>th').allTextContents()
        assert.equal(new Set(questions).size, questions.length)
      }
      const toggle = page.locator('.grading-table-toggle').first()
      const scoreHeading = page.locator('.grading-model-table thead th').nth(1)
      await scoreHeading.click()
      assert.equal(await toggle.getAttribute('aria-expanded'), 'true')
      await scoreHeading.click()
      assert.equal(await toggle.getAttribute('aria-expanded'), 'false')
      await toggle.focus()
      await page.keyboard.press('Enter')
      assert.equal(await toggle.getAttribute('aria-expanded'), 'true')
      assert.ok(await page.locator('.grading-model-table tbody').first().isVisible())
      const questionStart = await page.locator('.grading-model-table tbody th').first().evaluate(element => element.getBoundingClientRect().x + parseFloat(getComputedStyle(element).paddingLeft))
      const headingName = await toggle.locator('span').last().boundingBox()
      assert.ok(headingName && Math.abs(questionStart - headingName.x) < 1)
      assert.ok(await page.locator('.grading-model-table tfoot').first().isVisible())
      const layouts = await page.locator('.grading-model-table').evaluateAll(tables => tables.map(table => {
        const start = table.getBoundingClientRect().x
        return [...table.querySelectorAll('thead th')].map(cell => ({ x: cell.getBoundingClientRect().x - start, width: cell.getBoundingClientRect().width }))
      }))
      for (const layout of layouts.slice(1)) {
        assert.equal(layout.length, layouts[0].length)
        layout.forEach((cell, index) => {
          assert.ok(Math.abs(cell.x - layouts[0][index].x) < 1)
          assert.ok(Math.abs(cell.width - layouts[0][index].width) < 1)
        })
      }
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      const grading = await new AxeBuilder({ page }).analyze()
      assert.deepEqual(grading.violations.map(value => ({ id: value.id, impact: value.impact, count: value.nodes.length })), [])
      await page.keyboard.press('Space')
      assert.equal(await toggle.getAttribute('aria-expanded'), 'false')
      assert.equal(await page.locator('.grading-model-table tbody').first().isVisible(), false)
    }
  }
  await page.emulateMedia({ colorScheme: 'light' })
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'rose-pine-dawn')
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor), 'rgb(242, 233, 225)')
  await checkMetricControls()
  await checkHeaderLayout()
  await checkResultColours({ complete: 'rgb(40, 105, 131)', partial: 'rgb(234, 157, 52)', 'no-credit': 'rgb(180, 99, 122)' })
  await checkHarnessColour()
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await checkTimeAndProvider()
    await page.locator('.model summary').first().click()
    await checkModelDividers()
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
    const results = await new AxeBuilder({ page }).analyze()
    assert.deepEqual(results.violations.map(value => ({ id: value.id, impact: value.impact, count: value.nodes.length })), [])
    await page.locator('.model summary').first().click()
  }
  if (await page.locator('.grader-row').count()) {
    await page.locator('.grading-table-toggle').first().click()
    const grading = await new AxeBuilder({ page }).analyze()
    assert.deepEqual(grading.violations.map(value => ({ id: value.id, impact: value.impact, count: value.nodes.length })), [])
    await page.locator('.grading-table-toggle').first().click()
  }
  assert.deepEqual(errors, [])
  console.log(
    'Rosé Pine OG/Dawn desktop/mobile: keyboard controls, overflow and axe checks passed.'
  )
} finally {
  await browser.close()
}
