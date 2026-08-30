import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'

type RpcRequest = {
  id: number
  method: string
}

const EMPTY_STATS = {
  storage: { sizeBytes: 0, maxSizeBytes: 0 },
  traces: {
    traceCount: 0,
    spanCount: 0,
    serviceCount: 0,
    errorCount: 0,
    lastReceived: null,
  },
  logs: { logCount: 0, errorCount: 0, lastReceived: null },
  metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
  rejections: [],
}

async function interceptRpc(page: Page): Promise<void> {
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as RpcRequest
    const response =
      request.method === 'getStats'
        ? { jsonrpc: '2.0', id: request.id, result: EMPTY_STATS }
        : {
            jsonrpc: '2.0',
            id: request.id,
            error: {
              code: -32601,
              message: `No accessibility-test fixture for ${request.method}`,
            },
          }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(response),
    })
  })
}

test.describe('home page accessibility', () => {
  test.beforeEach(async ({ page }) => {
    await interceptRpc(page)
    await page.goto('/')
    await expect(
      page.getByRole('heading', {
        level: 1,
        name: 'OpenTelemetry Desktop Viewer',
      })
    ).toBeVisible()
  })

  test('has no automatically detectable Home page WCAG violations', async ({
    page,
  }) => {
    const scan = await new AxeBuilder({ page })
      .withTags([
        'wcag2a',
        'wcag2aa',
        'wcag21a',
        'wcag21aa',
        'wcag22a',
        'wcag22aa',
      ])
      .analyze()

    expect(scan.violations).toEqual([])
  })

  test('starts keyboard traversal in primary navigation', async ({ page }) => {
    await page.keyboard.press('Tab')

    const tracesLink = page.getByRole('link', { name: 'Traces', exact: true })
    await expect(tracesLink).toBeFocused()

    const shape = await tracesLink.evaluate(element => {
      const styles = getComputedStyle(element)
      return {
        radius: Number.parseFloat(styles.borderTopLeftRadius),
        width: element.getBoundingClientRect().width,
        zIndex: Number.parseFloat(styles.zIndex),
      }
    })
    expect(shape.radius).toBeGreaterThanOrEqual(shape.width / 2)
    expect(shape.zIndex).toBeGreaterThan(0)
  })

  test('keeps visible endpoint labels in their accessible names', async ({
    page,
  }) => {
    const protocolTabs = page.getByRole('tablist', {
      name: 'OTLP endpoint protocol',
    })

    await expect(protocolTabs).toMatchAriaSnapshot(`
      - tablist "OTLP endpoint protocol":
        - tab "HTTP" [selected]
        - tab "gRPC"
    `)

    for (const label of ['HTTP', 'gRPC'] as const) {
      const tab = protocolTabs.getByRole('tab', { name: label, exact: true })
      await expect(tab).toHaveText(label)
      await expect(tab).toHaveAccessibleName(label)
    }
  })

  test('supports keyboard focus and activation for endpoint tabs', async ({
    page,
  }) => {
    const protocolTabs = page.getByRole('tablist', {
      name: 'OTLP endpoint protocol',
    })
    const httpTab = protocolTabs.getByRole('tab', {
      name: 'HTTP',
      exact: true,
    })
    const grpcTab = protocolTabs.getByRole('tab', {
      name: 'gRPC',
      exact: true,
    })

    await httpTab.focus()
    await expect(httpTab).toBeFocused()

    await page.keyboard.press('ArrowRight')
    await expect(grpcTab).toBeFocused()
    await expect(grpcTab).toHaveAttribute('aria-selected', 'true')
    await expect(grpcTab).toHaveAttribute('tabindex', '0')
    await expect(httpTab).toHaveAttribute('aria-selected', 'false')
    await expect(httpTab).toHaveAttribute('tabindex', '-1')

    await page.keyboard.press('Tab')
    await expect(
      page.getByRole('button', { name: 'Copy snippet' })
    ).toBeFocused()
  })

  test('keeps focus while keyboard-resizing the Home panels', async ({
    page,
  }) => {
    const separator = page.getByRole('separator', {
      name: 'Resize the panels',
    })
    const valueBefore = Number(await separator.getAttribute('aria-valuenow'))

    await separator.focus()
    await page.keyboard.press('ArrowRight')

    await expect(separator).toBeFocused()
    await expect
      .poll(async () => Number(await separator.getAttribute('aria-valuenow')))
      .toBeGreaterThan(valueBefore)
  })
})
