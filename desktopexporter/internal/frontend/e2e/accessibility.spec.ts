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
    // SAFETY: This page's /rpc requests come from callRPC, which supplies a
    // numeric request-correlation id and a string method.
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

    await expect(
      page.getByRole('link', { name: 'Home', exact: true })
    ).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(
      page.getByRole('link', { name: 'Traces', exact: true })
    ).toBeFocused()
  })

  test('uses DaisyUI navigation tabs with a primary selected icon and underline', async ({
    page,
  }) => {
    const navigation = page.getByRole('navigation', { name: 'Primary' })
    const tabs = await navigation.evaluate(element => {
      const reference = element.cloneNode(true) as HTMLElement
      document.body.append(reference)
      const primary = document.createElement('span')
      primary.className = 'text-primary'
      reference.append(primary)
      const primaryColour = getComputedStyle(primary).color
      const referenceLinks = reference.querySelectorAll('a')
      const centreY = (element: Element) => {
        const bounds = element.getBoundingClientRect()
        return bounds.top + bounds.height / 2
      }
      try {
        return [...element.querySelectorAll('a')].map((link, index) => {
          const style = getComputedStyle(link)
          const plainStyle = getComputedStyle(referenceLinks[index])
          const underline = getComputedStyle(link, '::before')
          const selected = link.getAttribute('aria-current') === 'page'
          return {
            fontSize: style.fontSize,
            plainColour: style.color === plainStyle.color,
            plainBackground:
              style.backgroundColor === plainStyle.backgroundColor,
            plainShadow: style.boxShadow === plainStyle.boxShadow,
            labelColour:
              getComputedStyle(link.querySelector('span')!).color ===
              style.color,
            iconColour:
              getComputedStyle(link.querySelector('svg')!).color ===
              (selected ? primaryColour : style.color),
            underlineMatchesSelection: selected
              ? underline.borderTopColor === primaryColour &&
                underline.borderTopWidth === '1px' &&
                underline.height === '1px' &&
                underline.left === '0px' &&
                Math.abs(
                  Number.parseFloat(underline.width) -
                    link.getBoundingClientRect().width
                ) < 0.5
              : underline.borderTopWidth === '0px',
            centred:
              Math.abs(
                centreY(link.querySelector('svg')!) -
                  centreY(link.querySelector('span')!)
              ) < 0.5,
          }
        })
      } finally {
        reference.remove()
      }
    })

    expect(tabs).toHaveLength(4)
    for (const tab of tabs) {
      expect(tab).toEqual({
        fontSize: '14px',
        plainColour: true,
        plainBackground: true,
        plainShadow: true,
        labelColour: true,
        iconColour: true,
        underlineMatchesSelection: true,
        centred: true,
      })
    }
  })

  test('keeps the icon-only time range control accessible without a sort menu', async ({
    page,
  }) => {
    await page.goto('/traces')
    const trigger = page.getByRole('button', { name: /^Change time range/ })
    await expect(trigger).toHaveText('')
    await expect(trigger).toHaveCSS('font-size', '14px')
    await expect(page.getByRole('button', { name: /^Sort by/ })).toHaveCount(0)
    await expect(page.getByRole('menu', { name: 'Sort by' })).toHaveCount(0)
    await trigger.focus()
    await page.keyboard.press('Enter')
    await expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await expect(
      page.getByRole('button', { name: 'Last 5m', exact: true })
    ).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(trigger).toBeFocused()
    await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  test('keeps search drafts and controls accessible above signal drawers', async ({
    page,
  }) => {
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      for (const signal of ['traces', 'metrics', 'logs']) {
        await page.evaluate(() =>
          localStorage.setItem('signal-drawer:open', 'true')
        )
        await page.goto(`/${signal}`)
        const editor = page.locator('.cm-content[contenteditable="true"]')
        await editor.pressSequentially('service.name = "checkout"')
        await expect(
          page.getByRole('button', { name: /^(Open|Collapse) sidebar$/ })
        ).toHaveCount(0)
        await expect(editor).toBeVisible()
        await expect(editor).toHaveText('service.name = "checkout"')
        await expect(
          page.getByRole('button', { name: /^Sort by/ })
        ).toHaveCount(0)
        await expect(
          page.getByRole('button', { name: /^Change time range/ })
        ).toHaveText('')
        await expect(
          page.getByRole('button', { name: 'Refresh', exact: true })
        ).toBeVisible()
        const searchBar = page.getByRole('search', { name: 'Workspace search' })
        await expect(searchBar.locator('.search-bar__query')).not.toHaveCSS(
          'background-color',
          'rgba(0, 0, 0, 0)'
        )
        for (const name of ['Refresh', 'Clear search', 'Search query help']) {
          const button = searchBar.getByRole('button', { name, exact: true })
          await expect(button).toBeVisible()
          await expect(button).toHaveCSS('width', '32px')
          await expect(button).toHaveCSS('height', '32px')
        }
        await expect(
          searchBar.getByRole('combobox', { name: 'Result limit' })
        ).toHaveCount(0)
        const timeButton = searchBar.getByRole('button', {
          name: /^Change time range/,
        })
        await expect(timeButton).toBeVisible()
        await expect(timeButton).toHaveCSS('width', '32px')
        await expect(timeButton).toHaveCSS('height', '32px')
        const help = searchBar.getByRole('button', {
          name: 'Search query help',
        })
        await expect(help).toBeVisible()
        await expect
          .poll(() =>
            searchBar.evaluate(element => {
              const help = element.querySelector(
                'button[aria-label="Search query help"]'
              )!
              const clear = element.querySelector(
                'button[aria-label="Clear search"]'
              )!
              return (
                help.getBoundingClientRect().left >=
                clear.getBoundingClientRect().right
              )
            })
          )
          .toBe(true)
        await expect
          .poll(() =>
            searchBar.evaluate(element => {
              const colour = getComputedStyle(element).color
              return [...element.querySelectorAll('button')]
                .filter(button => !button.closest('[popover], dialog'))
                .every(button => getComputedStyle(button).color === colour)
            })
          )
          .toBe(true)
        await expect(editor).toBeFocused()
        const geometry = await editor.evaluate(element => {
          const editorBox = element.getBoundingClientRect()
          const navigation = document.querySelector('[aria-label="Primary"]')!
          const navigationBox = navigation.getBoundingClientRect()
          const refresh = document.querySelector(
            'button[aria-label="Refresh"]'
          )!
          const refreshBox = refresh.getBoundingClientRect()
          const searchBox = element
            .closest('.search-editor-container')!
            .getBoundingClientRect()
          return {
            outsideDrawer: !element.closest('.drawer-side'),
            noOverlap:
              editorBox.left >= navigationBox.right ||
              editorBox.top >= navigationBox.bottom,
            noPageOverflow:
              document.documentElement.scrollWidth <= window.innerWidth,
            refreshBeforeSearch:
              Boolean(
                refresh.compareDocumentPosition(element) &
                Node.DOCUMENT_POSITION_FOLLOWING
              ) &&
              refreshBox.right <= searchBox.left &&
              refreshBox.top < searchBox.bottom &&
              refreshBox.bottom > searchBox.top,
          }
        })
        expect(geometry).toEqual({
          outsideDrawer: true,
          noOverlap: true,
          noPageOverflow: true,
          refreshBeforeSearch: true,
        })
        await page.getByRole('button', { name: 'Refresh', exact: true }).click()
        await expect(editor).toHaveText('service.name = "checkout"')
      }
    }
  })

  test('keeps the search bar visible without an active outline or collapse control', async ({
    page,
  }) => {
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      await page.goto('/logs')
      const bar = page.getByRole('search', { name: 'Workspace search' })
      await expect(
        bar.getByRole('button', { name: /Expand search|Collapse search/ })
      ).toHaveCount(0)
      const surface = bar.locator('.search-bar__query')
      const background = await surface.evaluate(
        element => getComputedStyle(element).backgroundColor
      )
      await expect(bar).toHaveCSS('background-color', background)
      const editor = bar.locator('.cm-content[contenteditable="true"]')
      await editor.pressSequentially('service.name = "checkout"')
      await expect(surface).toHaveCSS('background-color', background)
      await expect(surface).toHaveCSS('outline-style', 'none')
      await expect(editor).toHaveText('service.name = "checkout"')
      await bar.getByRole('button', { name: 'Clear search' }).click()
      await expect(editor).toBeVisible()
      await expect(editor.locator('.cm-placeholder')).toBeVisible()
    }
  })

  test('opens query help after Clear and restores focus on close', async ({
    page,
  }) => {
    await page.goto('/logs')
    const help = page.getByRole('button', { name: 'Search query help' })
    await help.click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(dialog).not.toBeVisible()
    await expect(help).toBeFocused()
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

  test('uses the main navigation color rules for preset labels', async ({
    page,
  }) => {
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()

    const presetColors = await page
      .locator('.preset-time-ranges')
      .evaluate(element => {
        const active = element.querySelector('.tab-active')
        const inactive = element.querySelector('.tab:not(.tab-active)')
        if (!active || !inactive) throw new Error('Preset state is missing')

        const probe = document.createElement('span')
        document.body.append(probe)
        probe.style.color = 'var(--color-primary)'
        const primary = getComputedStyle(probe).color
        probe.style.color = 'var(--color-base-content)'
        const baseContent = getComputedStyle(probe).color
        probe.remove()
        return {
          active: getComputedStyle(active).color,
          inactive: getComputedStyle(inactive).color,
          navInactive: getComputedStyle(
            document.querySelector(
              'nav[aria-label="Primary"] .tab:not(.tab-active)'
            )!
          ).color,
          underline: getComputedStyle(active, '::before').color,
          background: getComputedStyle(active).backgroundColor,
          primary,
          baseContent,
        }
      })
    expect(presetColors.inactive).toBe(presetColors.navInactive)
    expect(presetColors.active).toBe(presetColors.baseContent)
    expect(presetColors.underline).toBe(presetColors.primary)
    expect(presetColors.background).toBe('rgba(0, 0, 0, 0)')
  })

  test('aligns preset edge labels with the range labels and timezone value', async ({
    page,
  }) => {
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()

    const popup = page.locator('[popover]:popover-open')
    const alignment = await popup.evaluate(element => {
      const presets = element.querySelectorAll('.preset-time-ranges .tab')
      const tabs = [...presets].map(tab => {
        const range = document.createRange()
        range.selectNodeContents(tab)
        const text = range.getBoundingClientRect()
        const bounds = tab.getBoundingClientRect()
        return {
          left: bounds.left,
          right: bounds.right,
          width: bounds.width,
          center: (bounds.left + bounds.right) / 2,
          textCenter: (text.left + text.right) / 2,
          textLeft: text.left,
          textRight: text.right,
        }
      })
      const popupBounds = element.getBoundingClientRect()
      const popupStyle = getComputedStyle(element)
      const row = element.querySelector('.range-editor-row')!
      const border = Number.parseFloat(popupStyle.borderLeftWidth)
      const labelRange = document.createRange()
      labelRange.selectNodeContents(element.querySelector('.endpoint-label')!)
      const timezoneRange = document.createRange()
      timezoneRange.selectNodeContents(
        element.querySelector('.timezone-summary')!
      )
      return {
        tabs,
        labelLeft: labelRange.getBoundingClientRect().left,
        timezoneRight: timezoneRange.getBoundingClientRect().right,
        leftInset: tabs[0].left - popupBounds.left - border,
        rightInset: popupBounds.right - border - tabs[tabs.length - 1].right,
        rowInset: row.getBoundingClientRect().left - popupBounds.left - border,
        contentInset: Number.parseFloat(popupStyle.paddingLeft),
      }
    })
    expect(alignment.leftInset).toBeCloseTo(alignment.contentInset, 0)
    expect(alignment.rightInset).toBeCloseTo(alignment.contentInset, 0)
    expect(alignment.rowInset).toBeCloseTo(alignment.contentInset, 0)
    for (const tab of alignment.tabs) {
      expect(tab.width).toBeCloseTo(alignment.tabs[0].width, 0)
    }
    expect(alignment.tabs[0].textLeft).toBeCloseTo(alignment.labelLeft, 0)
    expect(alignment.tabs.at(-1)!.textRight).toBeCloseTo(
      alignment.timezoneRight,
      0
    )
    for (const tab of alignment.tabs.slice(1, -1)) {
      expect(tab.textCenter).toBeCloseTo(tab.center, 0)
    }
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
    const focusTreatment = await startDate
      .locator('xpath=ancestor::div[contains(@class, "range-editor-row")]')
      .evaluate(element => {
        const styles = getComputedStyle(element)
        return {
          boxShadow: styles.boxShadow,
          outlineStyle: styles.outlineStyle,
        }
      })
    expect(focusTreatment.boxShadow).toContain('inset')
    expect(focusTreatment.outlineStyle).toBe('none')

    await startCalendar.hover()
    const triggerMetrics = await startCalendar.evaluate(element => {
      const styles = getComputedStyle(element)
      const bounds = element.getBoundingClientRect()
      return {
        radius: Number.parseFloat(styles.borderTopLeftRadius),
        width: bounds.width,
        height: bounds.height,
      }
    })
    expect(triggerMetrics.width).toBe(triggerMetrics.height)
    expect(triggerMetrics.radius).toBeGreaterThanOrEqual(
      triggerMetrics.width / 2
    )

    await startCalendar.click()
    await expect(startCalendar).toHaveAttribute('aria-expanded', 'true')
    const activeTreatment = await startCalendar.evaluate(element => ({
      backgroundColor: getComputedStyle(element).backgroundColor,
      borderColor: getComputedStyle(element).borderTopColor,
      rowBoxShadow: getComputedStyle(element.parentElement!).boxShadow,
    }))
    expect(activeTreatment.backgroundColor).not.toBe('rgba(0, 0, 0, 0)')
    expect(activeTreatment.borderColor).not.toBe('rgba(0, 0, 0, 0)')
    expect(activeTreatment.rowBoxShadow).toBe('none')

    for (const name of ['Previous', 'Next'] as const) {
      const navigationButton = page.getByRole('button', { name, exact: true })
      const navigationMetrics = await navigationButton.evaluate(element => {
        const styles = getComputedStyle(element)
        const bounds = element.getBoundingClientRect()
        return {
          radius: Number.parseFloat(styles.borderTopLeftRadius),
          width: bounds.width,
          height: bounds.height,
        }
      })
      expect(navigationMetrics.width).toBe(navigationMetrics.height)
      expect(navigationMetrics.radius).toBeGreaterThanOrEqual(
        navigationMetrics.width / 2
      )
    }

    const dayMetrics = await page
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
    expect(dayMetrics.width).toBe(dayMetrics.height)
    expect(dayMetrics.radius).toBeGreaterThanOrEqual(dayMetrics.width / 2)

    await startCalendar.click()
    await startDate.focus()
    await page.keyboard.press('Tab')
    await page.keyboard.press('Tab')
    await expect(startCalendar).toBeFocused()
  })

  test('omits recent ranges even when saved ranges exist', async ({ page }) => {
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

    const popup = page.locator('[popover]:popover-open')
    await expect(popup.getByText('Recent', { exact: true })).toHaveCount(0)
    await expect(
      popup.getByRole('list', { name: 'Recent time ranges' })
    ).toHaveCount(0)
    await expect(
      popup.getByRole('button', { name: 'Apply range' })
    ).toBeVisible()
  })

  test('keeps timezone controls full width with optically inset option text', async ({
    page,
  }) => {
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()
    const timezoneSummary = page.getByText('Timezone', { exact: true })
    const timezoneSection = timezoneSummary.locator('xpath=ancestor::details')
    await timezoneSummary.click()

    const timezoneList = timezoneSection.locator('.timezone-list')
    const timezoneOption = timezoneList.locator('.tz-option').first()
    const timezoneFilter = page.getByRole('searchbox', {
      name: 'Filter timezones',
    })
    const bounds = await Promise.all([
      timezoneSection.boundingBox(),
      timezoneList.boundingBox(),
      timezoneOption.boundingBox(),
      timezoneFilter.boundingBox(),
      timezoneOption.locator('.tz-name').boundingBox(),
      timezoneOption.locator('.tz-badge').boundingBox(),
    ])
    expect(bounds.every(Boolean)).toBe(true)
    const [
      timezoneSectionBounds,
      timezoneListBounds,
      timezoneOptionBounds,
      timezoneFilterBounds,
      timezoneNameBounds,
      timezoneBadgeBounds,
    ] = bounds
    expect(timezoneListBounds!.x).toBeCloseTo(timezoneSectionBounds!.x, 0)
    expect(timezoneListBounds!.x + timezoneListBounds!.width).toBeCloseTo(
      timezoneSectionBounds!.x + timezoneSectionBounds!.width,
      0
    )
    expect(timezoneOptionBounds!.x).toBeCloseTo(timezoneListBounds!.x, 0)
    expect(timezoneOptionBounds!.width).toBeCloseTo(
      timezoneListBounds!.width,
      0
    )
    expect(timezoneFilterBounds!.x).toBeCloseTo(timezoneSectionBounds!.x, 0)
    expect(timezoneFilterBounds!.x + timezoneFilterBounds!.width).toBeCloseTo(
      timezoneSectionBounds!.x + timezoneSectionBounds!.width,
      0
    )
    const timezoneLeftInset = timezoneNameBounds!.x - timezoneListBounds!.x
    const timezoneRightInset =
      timezoneListBounds!.x +
      timezoneListBounds!.width -
      timezoneBadgeBounds!.x -
      timezoneBadgeBounds!.width
    expect(timezoneLeftInset).toBeCloseTo(12, 0)
    expect(timezoneRightInset).toBeCloseTo(timezoneLeftInset, 0)
  })
})
