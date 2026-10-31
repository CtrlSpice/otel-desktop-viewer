import { routeTelemetry } from './telemetry-fixtures'
import { expect, test, type Page } from '@playwright/test'

async function openSavedLayout(
  page: Page,
  path: string,
  saved: Record<string, string>,
  width = 2200
) {
  await page.setViewportSize({ width, height: 1100 })
  await page.addInitScript(values => {
    if (sessionStorage.getItem('layout-seeded')) return
    sessionStorage.setItem('layout-seeded', 'true')
    localStorage.setItem('signal-detail:open', 'true')
    for (const [key, value] of Object.entries(values)) {
      localStorage.setItem(key, value)
    }
  }, saved)
  await page.route('**/api/features', route => route.fulfill({ json: {} }))
  await routeTelemetry(page, async route => {
    const { id, method } = route.request().postDataJSON()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result:
          method === 'getStats'
            ? {
                storage: { sizeBytes: 0, maxSizeBytes: 0 },
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
            : method === 'discoverTelemetry'
              ? { rows: [], truncated: false }
              : [],
      },
    })
  })
  await page.goto(path)
  await expect(
    page.locator('.page-layout__region > .panel-split')
  ).toBeVisible()
}

function split(page: Page) {
  return page.locator('.page-layout__region > .panel-split')
}

async function geometry(page: Page) {
  return split(page).evaluate(element => {
    const rootPx = parseFloat(
      getComputedStyle(document.documentElement).fontSize
    )
    const gutter =
      parseFloat(
        getComputedStyle(element).getPropertyValue('--panel-split-total')
      ) * rootPx
    return {
      space: element.getBoundingClientRect().width - gutter,
      rootPx,
      detail: element.querySelector('.panel-detail')!.getBoundingClientRect()
        .width,
      main: element.querySelector('.panel-shell')!.getBoundingClientRect()
        .width,
    }
  })
}

for (const [path, key, otherKey] of [
  ['/traces', 'trace-detail-panels', 'metric-detail-panels'],
  ['/metrics', 'metric-detail-panels', 'trace-detail-panels'],
]) {
  test(`${path} migrates its legacy fraction on mount and preserves the shared width across routes and reload`, async ({
    page,
  }) => {
    await openSavedLayout(page, path!, { [key!]: '0.4', [otherKey!]: '0.75' })
    await expect
      .poll(() =>
        page.evaluate(() => Number(localStorage.getItem('signal-detail-width')))
      )
      .toBeGreaterThan(40)
    const initial = await geometry(page)
    const saved = await page.evaluate(() =>
      Number(localStorage.getItem('signal-detail-width'))
    )
    expect(initial.detail).toBeCloseTo(initial.space * 0.6, 0)
    expect(saved).toBeCloseTo(initial.detail / initial.rootPx, 1)
    // Migration must finish during initial sizing, before resize transitions begin.
    await expect(split(page)).not.toHaveClass(/panel-split--initializing/)
    await page
      .getByRole('link', { name: path === '/traces' ? /^Metrics/ : /^Traces/ })
      .click()
    await expect
      .poll(async () => (await geometry(page)).detail)
      .toBeCloseTo(initial.detail, 0)
    await page.reload()
    await expect
      .poll(async () => (await geometry(page)).detail)
      .toBeCloseTo(initial.detail, 0)
    await page.setViewportSize({ width: 1500, height: 1100 })
    await expect
      .poll(async () => (await geometry(page)).detail)
      .toBeLessThan(initial.detail)
    await expect
      .poll(async () => {
        const narrow = await geometry(page)
        return narrow.main / narrow.rootPx
      })
      .toBeGreaterThanOrEqual(27.99)
    expect(
      await page.evaluate(() =>
        Number(localStorage.getItem('signal-detail-width'))
      )
    ).toBe(saved)
    await page.setViewportSize({ width: 2200, height: 1100 })
    await expect
      .poll(async () => (await geometry(page)).detail)
      .toBeCloseTo(initial.detail, 0)
  })
}

test('a valid shared width wins over legacy splits on the first mount', async ({
  page,
}) => {
  await openSavedLayout(page, '/metrics', {
    'signal-detail-width': '60',
    'metric-detail-panels': '0.75',
    'workspace-panel-split': '0.7',
  })
  await expect
    .poll(async () => {
      const { detail, rootPx } = await geometry(page)
      return detail / rootPx
    })
    .toBeCloseTo(60, 1)
  expect(
    await page.evaluate(() => localStorage.getItem('signal-detail-width'))
  ).toBe('60')
})

test('a stacked first mount preserves the legacy height split and waits for horizontal space to migrate width', async ({
  page,
}) => {
  await openSavedLayout(page, '/traces', { 'trace-detail-panels': '0.6' }, 1000)
  const divider = split(page).getByRole('separator', {
    name: 'Resize the panels',
    exact: true,
  })
  await expect(divider).toHaveAttribute('aria-orientation', 'horizontal')
  await expect
    .poll(() =>
      page.evaluate(() => localStorage.getItem('workspace-panel-split'))
    )
    .toBe('0.6')
  expect(
    await page.evaluate(() => localStorage.getItem('signal-detail-width'))
  ).toBeNull()
  await page.setViewportSize({ width: 2200, height: 1100 })
  await expect(divider).toHaveAttribute('aria-orientation', 'vertical')
  await expect
    .poll(() =>
      page.evaluate(() => Number(localStorage.getItem('signal-detail-width')))
    )
    .toBeGreaterThan(40)
  const wide = await geometry(page)
  expect(wide.detail).toBeCloseTo(wide.space * 0.4, 0)
  await page.setViewportSize({ width: 1000, height: 1100 })
  await expect(divider).toHaveAttribute('aria-orientation', 'horizontal')
  expect(
    await page.evaluate(() => localStorage.getItem('workspace-panel-split'))
  ).toBe('0.6')
})

test('malformed shared and legacy values retain the default and allow resizing', async ({
  page,
}) => {
  await openSavedLayout(page, '/traces', {
    'signal-detail-width': 'Infinity',
    'trace-detail-panels': '0.4oops',
  })
  await expect
    .poll(async () => {
      const { detail, rootPx } = await geometry(page)
      return detail / rootPx
    })
    .toBeCloseTo(28, 1)
  await split(page)
    .getByRole('separator', { name: 'Resize the panels', exact: true })
    .press('ArrowLeft')
  await expect
    .poll(() =>
      page.evaluate(() => Number(localStorage.getItem('signal-detail-width')))
    )
    .toBeGreaterThan(28)
})
