import { expect, test } from '@playwright/test'

const shellSelectors = [
  '.page-layout',
  'header[aria-label="Workspace navigation and search"]',
  'nav[aria-label="Primary"]',
  '#signal-drawer-panel',
  '.page-layout__main-chrome',
  '#signal-drawer-detail',
]

for (const reducedMotion of ['no-preference', 'reduce'] as const) {
  test(`navigation preserves shell identity and saved geometry (${reducedMotion})`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.emulateMedia({ reducedMotion })
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.addInitScript(() => {
      localStorage.setItem('signal-drawer:open', 'true')
      localStorage.setItem('signal-detail:open', 'true')
      localStorage.setItem('signal-drawer-width', '24')
      localStorage.setItem('signal-detail-width', '30')
    })
    let evalsEnabled = true
    await page.route('**/api/features', route =>
      route.fulfill({ json: { evals: { enabled: evalsEnabled } } })
    )
    await page.route('**/api/evals/experiments**', route =>
      route.fulfill({
        json: {
          experiments: [],
          total: 0,
          hasMore: false,
          facets: [],
          indexing: {
            remaining: 0,
            unsupported: 0,
            empty: 0,
            issues: [],
            hasMoreIssues: false,
          },
        },
      })
    )
    let releaseLogs!: () => void
    const logsGate = new Promise<void>(resolve => {
      releaseLogs = resolve
    })
    await page.route('**/rpc', async route => {
      const { id, method } = route.request().postDataJSON()
      if (method === 'searchLogPage') {
        await logsGate
        await route.fulfill({
          json: {
            jsonrpc: '2.0',
            id,
            error: { code: -32000, message: 'shell test failure' },
          },
        })
        return
      }
      const result =
        method === 'getStats'
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
          : method === 'discoverTelemetry'
            ? { rows: [], truncated: false }
            : method === 'getImportConfig'
              ? { otlpHttpPort: 4318 }
              : []
      await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
    })
    await page.goto('/?time=all')
    await expect(
      page.getByRole('button', { name: 'Import files' })
    ).toBeVisible()
    const nodes = await page.evaluateHandle(
      selectors => selectors.map(selector => document.querySelector(selector)),
      shellSelectors
    )
    const main = page.locator('.page-layout__main-chrome')
    const details = page.locator('#signal-drawer-detail')
    const list = page.locator('#signal-drawer-panel')
    await expect
      .poll(async () => (await details.boundingBox())?.width)
      .toBe(480)
    const homeBounds = (await main.boundingBox())!

    async function assertShell(listOpen = 'true') {
      expect(
        await nodes.evaluate(
          (original, selectors) =>
            original.every(
              (node, index) =>
                node?.isConnected &&
                node === document.querySelector(selectors[index])
            ),
          shellSelectors
        )
      ).toBe(true)
      await expect(page.locator('.workspace-shell')).toHaveCount(1)
      expect(
        await page.evaluate(() => [
          localStorage.getItem('signal-drawer-width'),
          localStorage.getItem('signal-detail-width'),
          localStorage.getItem('signal-drawer:open'),
        ])
      ).toEqual(['24', '30', listOpen])
      expect(
        await page
          .locator('.page-layout')
          .evaluate(
            node =>
              node
                .getAnimations({ subtree: true })
                .filter(
                  animation =>
                    animation instanceof CSSTransition &&
                    [
                      'width',
                      'height',
                      'flex-basis',
                      'flex-grow',
                      'gap',
                    ].includes(animation.transitionProperty)
                ).length
          )
      ).toBe(0)
    }

    const nav = page.getByRole('navigation', { name: 'Primary' })
    for (const name of ['Traces', 'Metrics']) {
      const link = nav.getByRole('link', { name, exact: true })
      await link.focus()
      await link.press('Enter')
      await expect(link).toHaveAttribute('aria-current', 'page')
      await expect(link).toBeFocused()
      await assertShell()
      await expect
        .poll(async () => (await main.boundingBox())?.width)
        .toBe(homeBounds.width)
    }

    await nav.getByRole('link', { name: 'Logs', exact: true }).click()
    await expect(list).toBeHidden()
    await assertShell()
    await expect
      .poll(async () => (await details.boundingBox())?.width)
      .toBe(480)
    const logsBounds = (await main.boundingBox())!
    expect(logsBounds.width).toBeGreaterThan(homeBounds.width)
    releaseLogs()
    await expect(page.getByText('Error: shell test failure')).toBeVisible()
    await assertShell()
    expect((await main.boundingBox())!.width).toBe(logsBounds.width)

    let releaseModule!: () => void
    const moduleGate = new Promise<void>(resolve => {
      releaseModule = resolve
    })
    await page.route('**/src/pages/EvalsWorkspace.svelte*', async route => {
      await moduleGate
      await route.continue()
    })
    await nav
      .getByRole('link', { name: 'Experimental Evals', exact: true })
      .click()
    await expect(page.getByText('Opening Evals…')).toBeVisible()
    await assertShell()
    await expect
      .poll(async () => (await main.boundingBox())?.width)
      .toBe(homeBounds.width)
    releaseModule()
    await expect(page.getByText('Opening Evals…')).toHaveCount(0)
    await assertShell()

    // A destination without Details gives its space to Main and retains
    // both the Details node and its saved open/width preferences.
    evalsEnabled = false
    await expect(page.getByText('Evals is not enabled')).toBeVisible({
      timeout: 10000,
    })
    await expect(details).toHaveAttribute('aria-hidden', 'true')
    await assertShell()
    expect((await main.boundingBox())!.width).toBeGreaterThan(homeBounds.width)
    expect(
      await page.evaluate(() => localStorage.getItem('signal-detail:open'))
    ).toBe('true')

    await nav.getByRole('link', { name: 'Home', exact: true }).click()
    await expect(
      page.getByRole('button', { name: 'Import files' })
    ).toBeVisible()
    await assertShell()
    await expect.poll(async () => (await list.boundingBox())?.width).toBe(384)
    await expect
      .poll(async () => (await main.boundingBox())?.width)
      .toBe(homeBounds.width)

    // Responsive direction changes must retain the same Main and Details nodes.
    await page.setViewportSize({ width: 1000, height: 1000 })
    await expect(
      page.getByRole('separator', { name: 'Resize the panels', exact: true })
    ).toHaveAttribute('aria-orientation', 'horizontal')
    await assertShell()
    await page.setViewportSize({ width: 1920, height: 1000 })
    await expect
      .poll(async () => (await details.boundingBox())?.width)
      .toBe(480)
    await assertShell()

    await page
      .getByRole('button', { name: 'Collapse sidebar', exact: true })
      .press('Enter')
    await page
      .getByRole('button', { name: 'Collapse details', exact: true })
      .press('Enter')
    await nav.getByRole('link', { name: 'Logs', exact: true }).click()
    await nav.getByRole('link', { name: 'Home', exact: true }).click()
    await expect(
      page.getByRole('button', { name: 'Open sidebar', exact: true })
    ).toBeVisible()
    await expect(
      page.getByRole('button', { name: 'Open details', exact: true })
    ).toBeVisible()
    await assertShell('false')
    await page
      .getByRole('button', { name: 'Open sidebar', exact: true })
      .press('Enter')
    await page
      .getByRole('button', { name: 'Open details', exact: true })
      .press('Enter')
    await expect.poll(async () => (await list.boundingBox())?.width).toBe(384)
    await expect
      .poll(async () => (await details.boundingBox())?.width)
      .toBe(480)
    await assertShell()
    expect(errors).toEqual([])
  })
}
