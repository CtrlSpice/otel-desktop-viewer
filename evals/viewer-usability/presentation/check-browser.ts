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
  await page.goto(pathToFileURL(file).href)
  await page.locator('.model summary').first().focus()
  await page.keyboard.press('Enter')
  assert.equal(await page.locator('.model').first().getAttribute('open'), '')
  await page.keyboard.press('Space')
  assert.equal(await page.locator('.model').first().getAttribute('open'), null)
  for (const metric of ['duration', 'cost', 'result']) {
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
  }
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await page.locator('.model summary').first().click()
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      ),
      true
    )
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
  }
  assert.deepEqual(errors, [])
  console.log(
    'Dark-mode desktop/mobile: keyboard expansion, collapse, metric controls, overflow and axe checks passed.'
  )
} finally {
  await browser.close()
}
