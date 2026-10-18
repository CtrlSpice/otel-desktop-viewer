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

  test('restores the original Dawn palette', async ({ page }) => {
    await page.evaluate(() => localStorage.setItem('theme', 'rose-pine-dawn'))
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute(
      'data-theme',
      'rose-pine-dawn'
    )
    // Preserve the palette from before the Dawn colour experiment.
    const expected = {
      '--color-base-100': '#f2e9e1',
      '--color-base-200': '#f8f1ea',
      '--color-base-300': '#fffff9',
      '--color-base-content': '#575279',
      '--color-on-light': '#575279',
      '--color-on-dark': '#faf4ed',
      '--color-muted': '#9893a5',
      '--color-subtle': '#797593',
      '--color-rose': '#d7827e',
      '--color-primary': '#907aa9',
      '--color-secondary': '#286983',
      '--color-accent': '#56949f',
      '--color-neutral': '#9893a5',
      '--color-neutral-content': '#575279',
      '--color-info': '#56949f',
      '--color-success': '#286983',
      '--color-warning': '#ea9d34',
      '--color-error': '#b4637a',
    }
    const actual = await page.evaluate(tokens => {
      const style = getComputedStyle(document.documentElement)
      return Object.fromEntries(
        tokens.map(token => [token, style.getPropertyValue(token).trim()])
      )
    }, Object.keys(expected))
    expect(actual).toEqual(expected)
  })

  for (const [theme, canvas, panel, elevated] of [
    ['rose-pine', 'rgb(25, 23, 36)', 'rgb(31, 29, 46)', 'rgb(38, 35, 58)'],
    ['rose-pine-moon', 'rgb(35, 33, 54)', 'rgb(42, 39, 63)', 'rgb(57, 53, 82)'],
  ]) {
    test(`preserves ${theme} surfaces after CSS cleanup`, async ({ page }) => {
      await page.evaluate(theme => localStorage.setItem('theme', theme), theme)
      await page.goto('/traces')
      await expect(page.getByRole('main')).toHaveCSS('background-color', canvas)
      await expect(page.locator('.search-bar')).toHaveCSS(
        'background-color',
        elevated
      )
      await expect(page.locator('.signal-drawer__controls')).toHaveCSS(
        'background-color',
        panel
      )
      await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
        'background-color',
        panel
      )
      await page.route('**/api/features', route =>
        route.fulfill({ json: { evals: { enabled: true } } })
      )
      await page.goto('/evals')
      await expect(
        page.locator('.page-layout__main-chrome .pane-header').first()
      ).toHaveCSS('background-color', elevated)
    })
  }

  test('restores the original Dawn surface styling', async ({ page }) => {
    await page.evaluate(() => localStorage.setItem('theme', 'rose-pine-dawn'))
    await page.goto('/traces')
    await expect(page.getByRole('main')).toHaveCSS(
      'background-color',
      'rgb(242, 233, 225)'
    )
    await expect(page.locator('.signal-drawer__panel')).toHaveCSS(
      'background-color',
      'rgb(242, 233, 225)'
    )
    await expect(page.locator('.search-bar')).toHaveCSS(
      'background-color',
      'rgb(255, 255, 249)'
    )
    await expect(page.locator('.signal-drawer__controls')).toHaveCSS(
      'background-color',
      'rgb(248, 241, 234)'
    )
    await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
      'background-color',
      'rgb(248, 241, 234)'
    )

    await page.route('**/api/features', route =>
      route.fulfill({ json: { evals: { enabled: true } } })
    )
    await page.goto('/evals')
    await expect(page.locator('.signal-row--selected')).toHaveCSS(
      'background-color',
      'rgb(255, 255, 249)'
    )
    await expect(
      page.locator('.signal-row:not(.signal-row--selected)').first()
    ).toHaveCSS('background-color', 'rgb(242, 233, 225)')
    await expect(page.locator('.signal-row--selected')).toHaveCSS(
      'outline-style',
      'none'
    )
    await page.locator('.signal-row--selected').hover()
    await expect(page.locator('.signal-row--selected')).toHaveCSS(
      'background-color',
      'rgb(255, 255, 249)'
    )
  })

  for (const theme of ['rose-pine', 'rose-pine-moon', 'rose-pine-dawn']) {
    test(`shares item hover and selection states in ${theme} without changing tabs`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 1920, height: 1000 })
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await page.route('**/api/features', route =>
        route.fulfill({ json: { evals: { enabled: true } } })
      )
      await page.evaluate(theme => localStorage.setItem('theme', theme), theme)
      await page.goto('/evals')
      const colors = await page.evaluate(() => {
        const probe = document.createElement('span')
        document.body.append(probe)
        probe.style.backgroundColor = 'var(--item-hover-bg)'
        const hover = getComputedStyle(probe).backgroundColor
        probe.style.backgroundColor = 'var(--item-selected-bg)'
        const selected = getComputedStyle(probe).backgroundColor
        probe.remove()
        return { hover, selected }
      })
      const selectedCard = page.locator('.signal-row--selected')
      const idleCard = page
        .locator('.signal-row:not(.signal-row--selected)')
        .first()
      await idleCard.hover()
      await expect(idleCard).toHaveCSS('background-color', colors.hover)
      await selectedCard.hover()
      await expect(selectedCard).toHaveCSS('background-color', colors.selected)

      const row = page
        .getByRole('table', { name: /^Results for/ })
        .locator('tbody tr')
        .first()
      await row.hover()
      await expect(row).toHaveCSS('background-color', colors.hover)
      await row.getByRole('button').click()
      await expect(row).toHaveCSS('background-color', colors.selected)
      await row.hover()
      await expect(row).toHaveCSS('background-color', colors.selected)
      await expect(row).toHaveCSS('outline-style', 'none')
      await expect(
        page.getByRole('navigation', { name: 'Primary' }).locator('.tab-active')
      ).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      const paneTab = page.locator('.pane-header__tab.tab-active').first()
      await paneTab.hover()
      await expect(paneTab).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(paneTab).toHaveCSS('box-shadow', 'none')
      const underline = await paneTab.evaluate(element => {
        const style = getComputedStyle(element, '::before')
        return { height: style.height, border: style.borderTopWidth }
      })
      expect(underline).toEqual({ height: '1px', border: '1px' })

      await page.goto('/traces')
      await page.getByRole('button', { name: /^Sort By/ }).click()
      const menu = page.getByRole('group', { name: 'Sort By', exact: true })
      const selectedOption = menu.locator('button[aria-pressed="true"]')
      await selectedOption.hover()
      await expect(selectedOption).toHaveCSS(
        'background-color',
        colors.selected
      )
      const idleOption = menu.locator('button[aria-pressed="false"]').first()
      await idleOption.hover()
      await expect(idleOption).toHaveCSS('background-color', colors.hover)
      await page.keyboard.press('Escape')
      await page.getByRole('button', { name: /Change time range/i }).click()
      await expect(page.locator('.preset-time-ranges .tab-active')).toHaveCSS(
        'background-color',
        'rgba(0, 0, 0, 0)'
      )
      await page.getByText('Timezone:', { exact: true }).click()
      const timezone = page.locator('.tz-option[aria-pressed="true"]')
      await timezone.hover()
      await expect(timezone).toHaveCSS('background-color', colors.selected)
    })
  }

  test('starts keyboard traversal with the drawer toggle and primary navigation', async ({
    page,
  }) => {
    await page.keyboard.press('Tab')

    await expect(
      page.getByRole('button', { name: 'Collapse sidebar' })
    ).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(
      page.getByRole('link', { name: 'Home', exact: true })
    ).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(
      page.getByRole('link', { name: 'Traces', exact: true })
    ).toBeFocused()
  })

  test('mutes the sidebar toggle when the drawer is dragged closed', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    const toggle = page.getByRole('button', { name: 'Collapse sidebar' })
    const activeColour = await toggle.evaluate(
      element => getComputedStyle(element).color
    )
    const handle = await page
      .getByRole('separator', { name: 'Resize the list' })
      .boundingBox()
    await page.mouse.move(handle!.x + handle!.width / 2, handle!.y + 100)
    await page.mouse.down()
    await page.mouse.move(0, handle!.y + 100, { steps: 5 })
    await page.mouse.up()
    const reopen = page.getByRole('button', { name: 'Open sidebar' })
    await expect(reopen).toHaveAttribute('aria-expanded', 'false')
    await expect(reopen).not.toHaveCSS('color', activeColour)
    await reopen.focus()
    await page.keyboard.press('Enter')
    await expect(toggle).toHaveClass(/btn-soft/)
    await toggle.blur()
    await expect(toggle).toHaveCSS('color', activeColour)
  })

  test('collapses and reopens details without losing the split or keyboard focus', async ({
    page,
  }) => {
    for (const width of [1440, 700]) {
      await page.setViewportSize({ width, height: 900 })
      const toggle = page.getByRole('button', { name: 'Collapse details' })
      const detail = page.locator('#signal-drawer-detail')
      const main = page.locator('.page-layout__main-chrome')
      const separator = page.getByRole('separator', {
        name: 'Resize the panels',
      })
      await expect(detail).toBeVisible()
      await expect(separator).toHaveAttribute(
        'aria-orientation',
        width === 700 ? 'horizontal' : 'vertical'
      )
      await expect
        .poll(() => detail.evaluate(element => element.getAnimations().length))
        .toBe(0)
      const orientation = await separator.getAttribute('aria-orientation')
      const dimension = orientation === 'vertical' ? 'width' : 'height'
      const initial = await main.boundingBox()
      const detailSize = await detail.boundingBox()
      const url = page.url()

      await toggle.focus()
      await page.keyboard.press('Enter')
      const reopen = page.getByRole('button', { name: 'Open details' })
      await expect(reopen).toBeFocused()
      await expect(detail).toBeHidden()
      await expect(separator).toBeHidden()
      await expect
        .poll(async () => (await main.boundingBox())![dimension])
        .toBeGreaterThan(initial![dimension])
      expect(page.url()).toBe(url)

      await page.keyboard.press('Enter')
      await expect(toggle).toBeFocused()
      await expect(detail).toBeVisible()
      await expect
        .poll(async () =>
          Math.abs(
            (await detail.boundingBox())![dimension] - detailSize![dimension]
          )
        )
        .toBeLessThan(1)
    }

    await page.getByRole('button', { name: 'Collapse details' }).click()
    await page.reload()
    await expect(
      page.getByRole('button', { name: 'Open details' })
    ).toHaveAttribute('aria-expanded', 'false')
    await expect(page.locator('#signal-drawer-detail')).toBeHidden()
  })

  test('shares one set of panel widths across navigation and reloads', async ({
    page,
  }) => {
    await page.route('**/api/features', route =>
      route.fulfill({ json: { evals: { enabled: true } } })
    )
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.goto('/traces')
    const detail = page.locator('#signal-drawer-detail')
    const navigator = page.locator('.signal-drawer__panel')
    const detailHandle = page.getByRole('separator', {
      name: 'Resize the panels',
    })
    const navigatorHandle = page.getByRole('separator', {
      name: 'Resize the list',
    })
    await expect
      .poll(async () => Math.round((await detail.boundingBox())!.width))
      .toBe(448)
    await detailHandle.focus()
    await page.keyboard.press('ArrowLeft')
    await page.keyboard.press('ArrowLeft')
    const detailWidth = (await detail.boundingBox())!.width
    expect(detailWidth).toBeGreaterThan(448)
    await navigatorHandle.focus()
    await page.keyboard.press('ArrowRight')
    await page.keyboard.press('ArrowRight')
    const navigatorWidth = (await navigator.boundingBox())!.width
    expect(navigatorWidth).toBeGreaterThan(448)

    async function expectWidths(hasDetail = true) {
      await expect
        .poll(async () =>
          Math.abs((await navigator.boundingBox())!.width - navigatorWidth)
        )
        .toBeLessThan(1)
      if (hasDetail) {
        await expect
          .poll(async () =>
            Math.abs((await detail.boundingBox())!.width - detailWidth)
          )
          .toBeLessThan(1)
      }
    }

    for (const name of ['Metrics', 'Evals', 'Home', 'Logs', 'Traces']) {
      await page
        .getByRole('navigation', { name: 'Primary' })
        .getByRole('link', { name, exact: true })
        .click()
      await expectWidths(name !== 'Logs')
    }
    await page.reload()
    await expectWidths()
    await page.setViewportSize({ width: 2100, height: 1000 })
    await expectWidths()
    await page.setViewportSize({ width: 1920, height: 1000 })
    await expectWidths()

    await page.getByRole('button', { name: 'Collapse details' }).click()
    await page
      .getByRole('navigation', { name: 'Primary' })
      .getByRole('link', { name: 'Metrics', exact: true })
      .click()
    await expect(detail).toBeHidden()
    await page.getByRole('button', { name: 'Open details' }).click()
    await expectWidths()
  })

  test('slides details through the right edge without changing the content width', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.emulateMedia({ reducedMotion: 'no-preference' })
    await page.goto('/traces')
    const detail = page.locator('#signal-drawer-detail')
    await expect
      .poll(async () => Math.round((await detail.boundingBox())!.width))
      .toBe(448)
    for (const label of ['Collapse details', 'Open details']) {
      const frames = await page.evaluate(async label => {
        const panel = document.querySelector('#signal-drawer-detail')!
        const content = panel.querySelector('.panel-clip__content')!
        const split = panel.parentElement!
        document
          .querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!
          .click()
        const frames = []
        for (let frame = 0; frame < 24; frame++) {
          await new Promise<void>(resolve =>
            requestAnimationFrame(() => resolve())
          )
          const bounds = panel.getBoundingClientRect()
          const inner = content.getBoundingClientRect()
          frames.push({
            width: bounds.width,
            right: bounds.right,
            left: inner.left,
            contentWidth: inner.width,
            edge: split.getBoundingClientRect().right,
          })
        }
        return frames
      }, label)
      const moving = frames.filter(
        frame => frame.width > 8 && frame.width < 440
      )
      expect(moving.length).toBeGreaterThan(0)
      for (const frame of moving) {
        expect(Math.abs(frame.right - frame.edge)).toBeLessThan(1)
        expect(Math.abs(frame.contentWidth - 448)).toBeLessThan(1)
        expect(frame.left).toBeGreaterThan(frame.edge - 448)
      }
    }
  })

  test('clips compressed panels without squeezing content and respects reduced motion', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.goto('/traces')

    for (const reducedMotion of ['no-preference', 'reduce'] as const) {
      await page.emulateMedia({ reducedMotion })
      for (const side of ['left', 'right'] as const) {
        const panel = page.locator(
          side === 'left' ? '.signal-drawer__panel' : '#signal-drawer-detail'
        )
        const handle = page.getByRole('separator', {
          name: side === 'left' ? 'Resize the list' : 'Resize the panels',
        })
        const toggle = page.getByRole('button', {
          name: side === 'left' ? 'Collapse sidebar' : 'Collapse details',
        })
        const content = panel.locator('.panel-clip__content')
        await expect
          .poll(async () => Math.round((await panel.boundingBox())!.width))
          .toBe(448)
        const before = (await panel.boundingBox())!.width
        const box = (await handle.boundingBox())!
        const x = box.x + box.width / 2
        const y = box.y + 100
        const direction = side === 'left' ? -1 : 1
        const minimum = 352

        await page.mouse.move(x, y)
        await page.mouse.down()
        await page.mouse.move(x + direction * (before - minimum + 32), y, {
          steps: 5,
        })
        await expect(content).toBeVisible()
        expect((await content.boundingBox())!.width).toBeGreaterThanOrEqual(
          minimum - 1
        )
        await expect(panel.locator('.panel-clip')).toHaveCSS(
          'overflow',
          'hidden'
        )
        await expect(toggle).toHaveAttribute('aria-expanded', 'true')
        const resisted = (await panel.boundingBox())!.width
        expect(resisted).toBeGreaterThan(minimum - 32)
        expect(resisted).toBeLessThan(minimum)

        await page.mouse.move(x, y, { steps: 5 })
        await expect(content).toBeVisible()
        await page.mouse.move(x + direction * (before - minimum + 80), y, {
          steps: 5,
        })
        await page.mouse.up()
        const reopen = page.getByRole('button', {
          name: side === 'left' ? 'Open sidebar' : 'Open details',
        })
        await expect(reopen).toHaveAttribute('aria-expanded', 'false')
        await expect
          .poll(async () => (await panel.boundingBox())?.width ?? 0)
          .toBeLessThan(1)

        if (reducedMotion === 'reduce') {
          expect(
            await panel.evaluate(element =>
              getComputedStyle(element)
                .transitionDuration.split(',')
                .every(duration => parseFloat(duration) === 0)
            )
          ).toBe(true)
        }
        await reopen.click()
        await expect
          .poll(async () =>
            Math.abs((await panel.boundingBox())!.width - before)
          )
          .toBeLessThan(1)
        await expect(content).toBeVisible()
      }
    }
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

  test('keeps the icon-only time range control accessible in the workspace toolbar', async ({
    page,
  }) => {
    await page.goto('/traces')
    const trigger = page.getByRole('button', { name: /^Change time range/ })
    await expect(trigger).toHaveText('')
    await expect(trigger).toHaveCSS('font-size', '14px')

    await expect(
      page
        .getByRole('banner', { name: 'Workspace navigation and search' })
        .getByRole('button', { name: /^Sort By/ })
    ).toHaveCount(0)
    await trigger.focus()
    await page.keyboard.press('Enter')
    await expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await expect(
      page.getByRole('button', { name: 'Last 5 min', exact: true })
    ).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(trigger).toBeFocused()
    await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  test('changes the drawer sort target and direction independently', async ({
    page,
  }) => {
    await page.goto('/traces')
    const controls = page.getByRole('group', { name: 'Traces list controls' })
    const trigger = controls.getByRole('button', { name: /^Sort By/ })
    await expect(trigger).toHaveText('Start Time')
    await expect(trigger).toHaveCSS('font-size', '14px')
    const spacing = await controls.evaluate(element => {
      const target = element.querySelector('button[popovertarget]')!
      const icon = target.querySelector('svg')!.getBoundingClientRect()
      const label = target.querySelector('span')!.getBoundingClientRect()
      const arrow = element
        .querySelector('button[title]')!
        .getBoundingClientRect()
      return {
        iconGap: label.left - icon.right,
        spacer: arrow.left - label.right,
      }
    })
    expect(spacing.iconGap).toBeGreaterThanOrEqual(0)
    expect(spacing.iconGap).toBeLessThanOrEqual(9)
    expect(spacing.spacer).toBeGreaterThan(16)
    const directionIcon = controls.locator('button[title] svg')
    await expect(directionIcon).toHaveCSS('rotate', 'none')

    await controls
      .getByRole('button', { name: 'Sort ascending', exact: true })
      .click()
    await expect(trigger).toHaveAttribute('aria-expanded', 'false')
    await expect(
      controls.getByRole('button', { name: 'Sort descending', exact: true })
    ).toBeVisible()
    await expect(directionIcon).toHaveCSS('rotate', '180deg')

    await trigger.click()
    const popup = page.getByRole('group', { name: 'Sort By', exact: true })
    await popup.getByRole('button', { name: 'Duration', exact: true }).click()
    await expect(trigger).toHaveText('Duration')
    await expect(trigger).toBeFocused()
    await expect(popup).not.toBeVisible()
    await expect(
      controls.getByRole('button', { name: 'Sort descending', exact: true })
    ).toBeVisible()

    await trigger.click()
    await popup.getByRole('button', { name: 'Duration', exact: true }).click()
    await expect(
      controls.getByRole('button', { name: 'Sort descending', exact: true })
    ).toBeVisible()
    await trigger.click()
    await page.keyboard.press('Escape')
    await expect(trigger).toBeFocused()
    await expect(trigger).toHaveAttribute('aria-expanded', 'false')
    await controls
      .getByRole('button', { name: 'Sort descending', exact: true })
      .click()
    await expect(directionIcon).toHaveCSS('rotate', 'none')
    await expect(
      controls.getByRole('button', { name: 'Sort ascending', exact: true })
    ).toBeVisible()
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
        ).toBeVisible()
        await expect(editor).toBeVisible()
        await expect(editor).toHaveText('service.name = "checkout"')
        await expect(
          page
            .getByRole('banner', { name: 'Workspace navigation and search' })
            .getByRole('button', { name: /^Sort By/ })
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
      const underline = getComputedStyle(element, '::before')
      return {
        borderWidth: styles.borderTopWidth,
        boxShadow: styles.boxShadow,
        outlineStyle: styles.outlineStyle,
        background: styles.backgroundColor,
        underlineWidth: underline.borderTopWidth,
      }
    })
    expect(selectedStyle.borderWidth).toBe('0px')
    expect(selectedStyle.boxShadow).toBe('none')
    expect(selectedStyle.outlineStyle).toBe('solid')
    expect(selectedStyle.background).toBe('rgba(0, 0, 0, 0)')
    expect(selectedStyle.underlineWidth).toBe('1px')

    await page.keyboard.press('Tab')
    await expect(
      page.getByRole('button', { name: 'Copy snippet' })
    ).toBeFocused()
  })

  test('keeps focus while keyboard-resizing the Home panels', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1000 })
    const separator = page.getByRole('separator', {
      name: 'Resize the panels',
    })
    await expect(separator).toHaveAttribute('aria-orientation', 'vertical')
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

  test('balances preset label spacing and outer edges', async ({ page }) => {
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
      return {
        tabs,
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
      expect(tab.textCenter).toBeCloseTo(tab.center, 0)
      expect(tab.textLeft - tab.left).toBeCloseTo(
        alignment.tabs[0].textLeft - alignment.tabs[0].left,
        0
      )
    }
    expect(alignment.tabs[0].textLeft - alignment.tabs[0].left).toBeCloseTo(
      alignment.tabs.at(-1)!.right - alignment.tabs.at(-1)!.textRight,
      0
    )
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
    const timezoneSummary = page.getByText('Timezone:', { exact: true })
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
