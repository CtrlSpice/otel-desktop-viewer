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
    assert.deepEqual((await page.locator('.metrics button').allTextContents()).map(text => text.trim()), ['Results', 'Time', 'Cost (est., CAD)'])
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
  const checkErrorColours = async () => {
    for (const status of ['harness-error', 'execution-error']) {
      assert.deepEqual(await page.evaluate(status => {
      const badge = document.createElement('span')
      badge.className = `result-badge result-${status}`
      document.body.append(badge)
      const colour = getComputedStyle(badge, '::before').backgroundColor
      badge.remove()
      const strip = document.createElement('div')
      strip.className = 'grade-strip'
      const segment = document.createElement('span')
      segment.className = `result-${status}`
      strip.append(segment)
      document.body.append(strip)
      const barColour = getComputedStyle(segment).backgroundColor
      strip.remove()
      return [colour, barColour]
      }, status), ['rgb(255, 128, 0)', 'rgb(255, 128, 0)'])
    }
  }
  const checkModelDividers = async () => {
    const rows = await page.locator('.model').count()
    assert.deepEqual(await page.locator('.model').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderBottomWidth)), Array(rows).fill('1px'))
    assert.deepEqual(await page.locator('.model>summary').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderBottomWidth)), Array(await page.locator('.model>summary').count()).fill('0px'))
  }
  await checkResultColours({ complete: 'rgb(49, 116, 143)', partial: 'rgb(246, 193, 119)', 'no-credit': 'rgb(235, 111, 146)' })
  await checkErrorColours()
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
    const headerLabelColour = await page.locator('.board-label').evaluate(element => getComputedStyle(element).color)
    assert.ok((await page.locator('.grading-table-caret').evaluateAll(elements => elements.map(element => getComputedStyle(element).color))).every(colour => colour === headerLabelColour))
    assert.ok((await page.locator('.grading-model-header th:first-child').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderRadius))).every(radius => radius === '8px 0px 0px 8px'))
    assert.ok((await page.locator('.grading-model-header th:last-child').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderRadius))).every(radius => radius === '0px 8px 8px 0px'))
    assert.ok((await page.locator('.grading-status-badge').evaluateAll(elements => elements.map(element => getComputedStyle(element).color))).every(colour => colour === headerLabelColour))
    const panelColour = await page.evaluate(() => {
      const probe = document.createElement('span')
      probe.style.backgroundColor = 'var(--panel)'
      document.body.append(probe)
      const colour = getComputedStyle(probe).backgroundColor
      probe.remove()
      return colour
    })
    assert.ok((await page.locator('.grading-status-badge').evaluateAll(elements => elements.map(element => getComputedStyle(element).backgroundColor))).every(colour => colour === panelColour))
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
      await page.evaluate(() => scrollTo(0, 0))
      await page.screenshot({
        path: `${process.env.EVAL_PRESENTATION_SCREENSHOT_DIR}/dark-${width}.png`,
      })
    }
    await page.locator('.model summary').first().click()
    if (await page.locator('.grader-row').count()) {
      assert.equal(await page.locator('.grader-row summary').count(), 0)
      const summaryLines = await page.locator('.grader-metrics>div').evaluateAll(elements => elements.map(element => element.getBoundingClientRect().y))
      assert.ok(summaryLines.every(y => Math.abs(y - summaryLines[0]) < 1))
      assert.ok(await page.locator('.grading-output table').first().isVisible())
      assert.equal(await page.locator('.grading-output table table').count(), 0)
      for (const grader of await page.locator('.grader-row').all()) {
        assert.equal(await grader.locator('.grading-output>table').count(), 1)
      }
      for (const header of await page.locator('.grading-model-header').all()) {
        const questionsBody = header.locator('xpath=following-sibling::tbody[1]')
        const total = header.locator('xpath=following-sibling::tbody[2]')
        assert.equal(await header.locator('.grading-table-toggle').getAttribute('aria-expanded'), 'false')
        assert.equal(await questionsBody.isVisible(), false)
        assert.ok(await total.isVisible())
        assert.equal(await total.locator('tr').count(), 1)
        assert.equal(await total.locator('tr>th').textContent(), 'Total')
        assert.equal(await total.locator('tr>th').evaluate(element => getComputedStyle(element).borderTopWidth), '0px')
        assert.equal(await total.evaluate(element => getComputedStyle(element).backgroundColor), 'rgba(0, 0, 0, 0)')
        assert.equal(await total.locator('tr>th').evaluate(element => getComputedStyle(element).fontWeight), '400')
        const statusBadge = await header.locator('.grading-status-badge').boundingBox()
        assert.notEqual(await header.locator('.grading-status-badge').evaluate(element => getComputedStyle(element).backgroundColor), await header.locator('th').first().evaluate(element => getComputedStyle(element).backgroundColor))
        assert.equal(await total.locator('.grading-status-badge').count(), 0)
        assert.equal(await questionsBody.locator('.grading-status-badge').count(), 0)
        const columnCount = await header.locator('tr>th').count()
        assert.equal(await total.locator('tr>*').count(), columnCount)
        for (const row of await questionsBody.locator('tr').all()) assert.equal(await row.locator('th,td').count(), columnCount)
        assert.deepEqual((await header.locator('tr>th').allTextContents()).slice(-4), ['Grading Time', 'Cost (est., CAD)', 'Input Tokens', 'Output Tokens'])
        assert.ok(await header.locator('tr>th').first().textContent())
        assert.notEqual(await header.locator('tr>th').first().textContent(), 'Question')
        const tableCaret = await header.locator('.grading-table-caret').boundingBox()
        const tableName = await header.locator('.grading-model-name').boundingBox()
        assert.ok(tableCaret && tableName && tableCaret.x + tableCaret.width <= tableName.x)
        assert.ok(statusBadge && statusBadge.x >= tableName.x + tableName.width)
        assert.ok(Math.abs(statusBadge.y + statusBadge.height / 2 - tableName.y - tableName.height / 2) < 1)
        const headerCell = await header.locator('th').first().boundingBox()
        assert.ok(headerCell && statusBadge.x + statusBadge.width <= headerCell.x + headerCell.width)
        const totalStart = await total.locator('tr>th').evaluate(element => element.getBoundingClientRect().x + parseFloat(getComputedStyle(element).paddingLeft))
        assert.ok(Math.abs(totalStart - tableName.x) < 1)
        const headerBox = await header.boundingBox()
        const totalBox = await total.boundingBox()
        assert.ok(headerBox && totalBox && Math.abs(headerBox.y + headerBox.height - totalBox.y) < 1)
        const nextHeader = total.locator('xpath=following-sibling::tbody[1]')
        if (await nextHeader.count()) {
          const nextBox = await nextHeader.boundingBox()
          assert.ok(nextBox && Math.abs(totalBox.y + totalBox.height - nextBox.y) < 1)
        }
        const questions = await questionsBody.locator('tr>th').allTextContents()
        assert.equal(new Set(questions).size, questions.length)
      }
      const toggle = page.locator('.grading-table-toggle').first()
      const scoreHeading = page.locator('.grading-model-header th.grading-run').first()
      await scoreHeading.click()
      assert.equal(await toggle.getAttribute('aria-expanded'), 'true')
      await scoreHeading.click()
      assert.equal(await toggle.getAttribute('aria-expanded'), 'false')
      await toggle.focus()
      await page.keyboard.press('Enter')
      assert.equal(await toggle.getAttribute('aria-expanded'), 'true')
      assert.ok(await page.locator('.grading-questions').first().isVisible())
      const questionStart = await page.locator('.grading-questions th').first().evaluate(element => element.getBoundingClientRect().x + parseFloat(getComputedStyle(element).paddingLeft))
      const headingName = await toggle.locator('.grading-model-name').boundingBox()
      assert.ok(headingName && Math.abs(questionStart - headingName.x) < 1)
      assert.ok(await page.locator('.grading-model-total').first().isVisible())
      const layouts = await page.locator('.grading-model-header').evaluateAll(headers => headers.map(header => {
        const start = header.getBoundingClientRect().x
        return [...header.querySelectorAll('th')].map(cell => ({ x: cell.getBoundingClientRect().x - start, width: cell.getBoundingClientRect().width }))
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
      assert.equal(await page.locator('.grading-questions').first().isVisible(), false)
      if (await page.locator('.grading-table-toggle').count() > 1) {
        const second = page.locator('.grading-table-toggle').nth(1)
        await second.click()
        await toggle.click()
        assert.ok(await page.locator('.grading-questions').first().isVisible())
        assert.ok(await page.locator('.grading-questions').nth(1).isVisible())
        await toggle.click()
        assert.ok(await page.locator('.grading-questions').nth(1).isVisible())
        await second.click()
      }
    }
  }
  await page.emulateMedia({ colorScheme: 'light' })
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'rose-pine-dawn')
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor), 'rgb(242, 233, 225)')
  await checkMetricControls()
  await checkHeaderLayout()
  await checkResultColours({ complete: 'rgb(40, 105, 131)', partial: 'rgb(234, 157, 52)', 'no-credit': 'rgb(180, 99, 122)' })
  await checkErrorColours()
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
