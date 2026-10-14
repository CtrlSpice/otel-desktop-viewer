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
  assert.ok(await page.evaluate(async () => {
    await document.fonts.ready
    return [...document.fonts].some(font => font.family.replaceAll('"', '') === 'Atkinson Hyperlegible Next' && font.status === 'loaded')
  }))
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor), 'rgb(25, 23, 36)')
  const checkTabSurface = async () => {
    assert.equal(await page.locator('.metrics').evaluate(element => getComputedStyle(element).backgroundColor), 'rgba(0, 0, 0, 0)')
    const textColour = await page.evaluate(() => getComputedStyle(document.documentElement).color)
    assert.deepEqual(await page.locator('.metrics button').evaluateAll(elements => elements.map(element => getComputedStyle(element).color)), Array(4).fill(textColour))
    await page.locator('[data-view="result"]').focus()
    await page.keyboard.press('Enter')
    await page.waitForFunction(() => {
      const tab = document.querySelector('[data-view="result"]')!
      return getComputedStyle(tab).backgroundColor === 'rgba(0, 0, 0, 0)'
    })
    assert.match(await page.locator('[data-view="result"]').evaluate(element => getComputedStyle(element).boxShadow), /inset/)
  }
  await checkTabSurface()
  const checkResultColours = async (colours: Record<string, string>) => {
    for (const [status, colour] of Object.entries(colours)) {
      const badges = page.locator(`.result-${status}`)
      const count = await badges.count()
      assert.deepEqual(await badges.evaluateAll(elements => elements.map(element => getComputedStyle(element, '::before').backgroundColor)), Array(count).fill(colour))
      assert.deepEqual(await badges.evaluateAll(elements => elements.map(element => getComputedStyle(element).fontSize)), Array(count).fill('14px'))
    }
  }
  const checkModelDividers = async () => {
    const rows = await page.locator('.model').count()
    assert.deepEqual(await page.locator('.model').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderBottomWidth)), Array(rows).fill('1px'))
    assert.deepEqual(await page.locator('.model>summary').evaluateAll(elements => elements.map(element => getComputedStyle(element).borderBottomWidth)), Array(rows).fill('0px'))
  }
  await checkResultColours({ complete: 'rgb(156, 207, 216)', partial: 'rgb(246, 193, 119)', 'no-credit': 'rgb(235, 111, 146)' })
  await page.locator('.model summary').first().focus()
  await page.keyboard.press('Enter')
  assert.equal(await page.locator('.model').first().getAttribute('open'), '')
  await page.keyboard.press('Space')
  assert.equal(await page.locator('.model').first().getAttribute('open'), null)
  for (const metric of ['duration', 'p95', 'cost', 'result']) {
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
    const heading = await page.locator('#metric-label').boundingBox()
    const value = await page.locator(`.model .metric.${metric} strong`).first().boundingBox()
    assert.ok(heading && value)
    assert.ok(Math.abs(heading.x + heading.width - value.x - value.width) < 1)
  }
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 })
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
      await page.locator('.grader-row summary').first().click()
      assert.ok(await page.locator('.grading-output table').first().isVisible())
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      const grading = await new AxeBuilder({ page }).analyze()
      assert.deepEqual(grading.violations.map(value => ({ id: value.id, impact: value.impact, count: value.nodes.length })), [])
      await page.locator('.grader-row summary').first().click()
    }
  }
  await page.emulateMedia({ colorScheme: 'light' })
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'rose-pine-dawn')
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor), 'rgb(242, 233, 225)')
  await checkTabSurface()
  await checkResultColours({ complete: 'rgb(86, 148, 159)', partial: 'rgb(234, 157, 52)', 'no-credit': 'rgb(180, 99, 122)' })
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await page.locator('.model summary').first().click()
    await checkModelDividers()
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
    const results = await new AxeBuilder({ page }).analyze()
    assert.deepEqual(results.violations.map(value => ({ id: value.id, impact: value.impact, count: value.nodes.length })), [])
    await page.locator('.model summary').first().click()
  }
  if (await page.locator('.grader-row').count()) {
    await page.locator('.grader-row summary').first().click()
    const grading = await new AxeBuilder({ page }).analyze()
    assert.deepEqual(grading.violations.map(value => ({ id: value.id, impact: value.impact, count: value.nodes.length })), [])
    await page.locator('.grader-row summary').first().click()
  }
  assert.deepEqual(errors, [])
  console.log(
    'Rosé Pine OG/Dawn desktop/mobile: keyboard controls, overflow and axe checks passed.'
  )
} finally {
  await browser.close()
}
