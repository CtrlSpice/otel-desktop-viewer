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

    const selectedStyle = await grpcTab.evaluate(element => {
      const styles = getComputedStyle(element)
      return {
        borderWidth: styles.borderTopWidth,
        boxShadow: styles.boxShadow,
        outlineStyle: styles.outlineStyle,
      }
    })
    expect(selectedStyle.borderWidth).toBe('0px')
    expect(selectedStyle.boxShadow).toContain('inset')
    expect(selectedStyle.outlineStyle).toBe('none')

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

    await expect
      .poll(() =>
        separator.evaluate(element => {
          const handle = element.querySelector<HTMLElement>(
            '.col-resize-bar__line'
          )
          if (!handle) {
            throw new Error('Resize handle is missing its visible line')
          }
          return Number(getComputedStyle(handle).opacity)
        })
      )
      .toBeGreaterThan(0.8)

    const handleSize = await separator.evaluate(element => {
      const handle = element.querySelector<HTMLElement>('.col-resize-bar__line')
      if (!handle) throw new Error('Resize handle is missing its visible line')
      const handleRect = handle.getBoundingClientRect()
      return {
        height: handleRect.height,
        separatorHeight: element.getBoundingClientRect().height,
      }
    })
    expect(handleSize.height).toBeLessThan(handleSize.separatorHeight)
  })

  test('keeps custom range controls circular and focus treatment inset', async ({
    page,
  }) => {
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()

    const startDate = page.getByRole('textbox', { name: 'Start', exact: true })
    const startCalendar = page.getByRole('button', {
      name: 'Choose start date',
    })

    await startDate.focus()
    const focusTreatment = await startDate.locator('..').evaluate(element => {
      const styles = getComputedStyle(element)
      return {
        boxShadow: styles.boxShadow,
        outlineStyle: styles.outlineStyle,
      }
    })
    expect(focusTreatment.boxShadow).toContain('inset')
    expect(focusTreatment.outlineStyle).toBe('none')

    await startCalendar.hover()
    const triggerShape = await startCalendar.evaluate(element => {
      const styles = getComputedStyle(element)
      const bounds = element.getBoundingClientRect()
      return {
        radius: Number.parseFloat(styles.borderTopLeftRadius),
        width: bounds.width,
        height: bounds.height,
      }
    })
    expect(triggerShape.width).toBe(triggerShape.height)
    expect(triggerShape.radius).toBeGreaterThanOrEqual(triggerShape.width / 2)

    await startCalendar.click()
    await expect(startCalendar).toHaveAttribute('aria-expanded', 'true')

    for (const name of ['Previous', 'Next'] as const) {
      const navigationButton = page.getByRole('button', { name, exact: true })
      const shape = await navigationButton.evaluate(element => {
        const styles = getComputedStyle(element)
        const bounds = element.getBoundingClientRect()
        return {
          radius: Number.parseFloat(styles.borderTopLeftRadius),
          width: bounds.width,
          height: bounds.height,
        }
      })
      expect(shape.width).toBe(shape.height)
      expect(shape.radius).toBeGreaterThanOrEqual(shape.width / 2)
    }

    const dayShape = await page
      .locator('.calendar-day')
      .first()
      .evaluate(element => {
        const styles = getComputedStyle(element)
        const bounds = element.getBoundingClientRect()
        return {
          radius: Number.parseFloat(styles.borderTopLeftRadius),
          width: bounds.width,
          height: bounds.height,
        }
      })
    expect(dayShape.width).toBe(dayShape.height)
    expect(dayShape.radius).toBeGreaterThanOrEqual(dayShape.width / 2)

    await startCalendar.click()
    await startDate.focus()
    await page.keyboard.press('Tab')
    await page.keyboard.press('Tab')
    await expect(startCalendar).toBeFocused()
  })

  test('extends recent range selection to the section edges', async ({
    page,
  }) => {
    const end = Date.UTC(2026, 0, 15, 12)
    await page.evaluate(
      ({ ranges, selection }) => {
        localStorage.setItem('datetime-filter-recent', JSON.stringify(ranges))
        localStorage.setItem('time-selection', JSON.stringify(selection))
      },
      {
        ranges: [
          { start: end - 600_000, end, usedAt: end },
          {
            start: end - 1_200_000,
            end: end - 600_000,
            usedAt: end - 1_000,
          },
        ],
        selection: { type: 'recent', start: end - 600_000, end },
      }
    )
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()

    const list = page.getByRole('list', {
      name: 'Recently used time ranges',
    })
    const firstRange = list.getByRole('button').first()
    const section = list.locator('xpath=ancestor::details')
    const bounds = await Promise.all([
      section.boundingBox(),
      list.boundingBox(),
      firstRange.boundingBox(),
    ])

    expect(bounds.every(Boolean)).toBe(true)
    const [sectionBounds, listBounds, buttonBounds] = bounds
    expect(listBounds!.x).toBeCloseTo(sectionBounds!.x, 0)
    expect(listBounds!.x + listBounds!.width).toBeCloseTo(
      sectionBounds!.x + sectionBounds!.width,
      0
    )
    expect(buttonBounds!.x).toBeCloseTo(listBounds!.x, 0)
    expect(buttonBounds!.x + buttonBounds!.width).toBeCloseTo(
      listBounds!.x + listBounds!.width,
      0
    )

    await expect(firstRange).toHaveAttribute('aria-pressed', 'true')
    const backgroundColor = await firstRange.evaluate(
      element => getComputedStyle(element).backgroundColor
    )
    expect(backgroundColor).not.toBe('rgba(0, 0, 0, 0)')
  })
})
