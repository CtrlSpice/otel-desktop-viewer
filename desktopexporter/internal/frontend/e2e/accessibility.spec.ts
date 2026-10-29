import { routeTelemetry } from './telemetry-fixtures'
import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Locator, type Page } from '@playwright/test'
import { THEMES, themeRgb } from './theme-colours'
import type { JsonTraceData } from '../src/types/wire-types'
import { SEASONS } from '../src/themes'

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

const EMPTY_LIST_METHODS = new Set([
  'getTraceAttributeDefinitions',
  'getTraceAttributeDefinitionsByTraceID',
  'getLogAttributeDefinitions',
  'getMetricAttributeDefinitions',
  'searchTraceSummaries',
  'searchMetricSummaries',
  'getTraceLogSummaries',
])

const HIGHLIGHT_THEMES = THEMES.map(theme => ({
  theme: theme.id,
  low: themeRgb(theme.id, 'highlightLow'),
  selected: themeRgb(theme.id, 'highlightMedium'),
  high: theme.colors.highlightHigh,
}))

async function expectDropdownSurface(surface: Locator, theme: string) {
  await expect(surface).toHaveCSS('background-color', themeRgb(theme, 'panel'))
  await expect(surface).toHaveCSS('color', themeRgb(theme, 'text'))
  await expect(surface).toHaveCSS('border-width', '1px')
  await expect(surface).toHaveCSS(
    'border-color',
    themeRgb(theme, 'highlightLow')
  )
  await expect(surface).toHaveCSS('border-radius', '12px')
  const shadow = await surface.evaluate(element => {
    const probe = document.createElement('span')
    probe.style.boxShadow = 'var(--popover-shadow)'
    document.body.append(probe)
    const expected = getComputedStyle(probe).boxShadow
    probe.remove()
    return { actual: getComputedStyle(element).boxShadow, expected }
  })
  expect(shadow.actual).not.toBe('none')
  expect(shadow.actual).toBe(shadow.expected)
  await expect(surface).toHaveCSS('backdrop-filter', 'none')
  await expect(surface).toHaveCSS('font-weight', '400')
}

// Pinned upstream Usuzumi 1 / Kasure 1, deliberately not contrast-corrected.
const NATIVE_NEUTRALS = {
  'madoromi-haru-hiru': ['#6b5d70', '#998d9e'],
  'madoromi-haru-yoru': ['#cdbed3', '#8a7e8f'],
  'madoromi-natsu-hiru': ['#4e6872', '#80969f'],
  'madoromi-natsu-yoru': ['#adcad4', '#728790'],
  'madoromi-aki-hiru': ['#715f4f', '#9f8f81'],
  'madoromi-aki-yoru': ['#d4c0ae', '#908072'],
  'madoromi-fuyu-hiru': ['#5e6277', '#8d91a4'],
  'madoromi-fuyu-yoru': ['#bec3db', '#7e8294'],
} as const

async function interceptRpc(page: Page): Promise<void> {
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await routeTelemetry(page, async route => {
    // SAFETY: This page's /rpc requests come from callRPC, which supplies a
    // numeric request-correlation id and a string method.
    const request = route.request().postDataJSON() as RpcRequest
    const response =
      request.method === 'getStats'
        ? { jsonrpc: '2.0', id: request.id, result: EMPTY_STATS }
        : request.method === 'discoverTelemetry'
          ? {
              jsonrpc: '2.0',
              id: request.id,
              result: { rows: [], truncated: false },
            }
          : request.method === 'getImportConfig'
            ? { jsonrpc: '2.0', id: request.id, result: { otlpHttpPort: 4318 } }
            : EMPTY_LIST_METHODS.has(request.method)
              ? { jsonrpc: '2.0', id: request.id, result: [] }
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

const TRACE_IDS = ['a'.repeat(32), 'b'.repeat(32)]
const SPAN_IDS = ['1'.repeat(16), '2'.repeat(16), '3'.repeat(16)]
const TRACE_NAMES = ['Checkout trace', 'Inventory trace']
const TRACE_SERVICES = ['checkout', 'payments', 'database']

/** Exercise the shared cards, rows and Details with the Evals flag off. */
async function interceptTraces(page: Page): Promise<void> {
  await routeTelemetry(page, async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'searchTraceSummaries') {
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: TRACE_IDS.map((traceID, index) => ({
            traceID,
            hasRootSpan: true,
            rootSpan: { name: TRACE_NAMES[index], serviceName: 'checkout' },
            startTime: '1700000000000000000',
            durationNs: '1000000000',
            spanCount: 3,
            errorCount: 0,
            serviceBreakdown: TRACE_SERVICES.map(label => ({
              key: `service:${label}`,
              label,
              count: 1,
            })),
          })),
        },
      })
    }
    if (method !== 'getTraceView') return route.fallback()
    const traceID: string = params.traceID
    const trace: JsonTraceData = {
      traceID,
      traceStart: '1700000000000000000',
      unplacedSpanCount: 0,
      resources: Object.fromEntries(
        TRACE_SERVICES.map((service, index) => [
          index,
          {
            attributes: [
              {
                key: 'service.name',
                value: { kind: 'string', value: service },
              },
            ],
            droppedAttributesCount: 0,
          },
        ])
      ),
      scopes: {
        0: {
          name: 'a11y',
          version: '',
          attributes: [],
          droppedAttributesCount: 0,
        },
      },
      spans: SPAN_IDS.map((spanID, index) => ({
        depth: index === 0 ? 0 : 1,
        matched: true,
        spanData: {
          spanID,
          parentSpanID: index === 0 ? null : SPAN_IDS[0],
          name:
            index === 0
              ? TRACE_NAMES[TRACE_IDS.indexOf(traceID)]
              : TRACE_SERVICES[index],
          traceState: '',
          flags: 1,
          kindCode: 2,
          kind: 'Server',
          start: '0',
          dur: '1000000000',
          r: index,
          s: 0,
          attributes: [],
          links: [],
          events: [],
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCodeValue: 1,
          statusCode: 'Ok',
          statusMessage: '',
        },
      })),
    }
    return route.fulfill({ json: { jsonrpc: '2.0', id, result: trace } })
  })
}

async function showMetadataExamples(page: Page, theme: string) {
  await interceptRpc(page)
  await page.addInitScript(theme => localStorage.setItem('theme', theme), theme)
  await page.goto('/')
  await expect(
    page.getByRole('region', { name: 'Service map', exact: true })
  ).toBeVisible()
  await page.evaluate(() => {
    const examples = document.createElement('section')
    examples.id = 'metadata-a11y-fixture'
    examples.setAttribute('aria-label', 'Metadata examples')
    examples.style.cssText =
      'display:flex;flex-wrap:wrap;gap:8px;padding:16px;background:var(--color-base-100)'
    // AGENTS.md: counts are Subtle, provenance is plain Secondary, and
    // status labels are neutral Text with decorative palette dots, not pills.
    // Exercise every approved base/interaction surface without changing tokens.
    for (const surface of [
      'base-100',
      'base-200',
      'base-300',
      'highlight-low',
      'highlight-med',
    ]) {
      const row = document.createElement('div')
      row.style.cssText = `display:flex;gap:16px;padding:8px;background:var(--color-${surface})`
      for (const [className, text] of [
        ['badge-count', '[42 spans]'],
        ['text-muted', 'Unavailable'],
        ['metadata-origin', 'Resource'],
        ['text-primary', 'Primary action'],
        ['text-accent', 'Accent action'],
      ]) {
        const label = document.createElement('span')
        label.className = className
        label.textContent = text
        row.append(label)
      }
      for (const [tone, text] of [
        ['warning', 'WARN (13)'],
        ['error', 'ERROR (17)'],
        ['papaya', 'Internal fault'],
      ]) {
        const label = document.createElement('span')
        label.className = 'text-base-content text-sm font-normal'
        const dot = document.createElement('span')
        dot.setAttribute('aria-hidden', 'true')
        dot.style.cssText = `display:inline-block;width:8px;height:8px;margin-right:8px;border-radius:50%;background:var(--color-${tone})`
        label.append(dot, text)
        row.append(label)
      }
      examples.append(row)
    }
    document.querySelector('main')!.prepend(examples)
  })
}

for (const { id: theme, colors, syntax } of THEMES) {
  test(`plain metadata preserves the approved styling in ${theme}`, async ({
    page,
  }) => {
    await showMetadataExamples(page, theme)
    const fixture = page.locator('#metadata-a11y-fixture')
    const tokens = await page.locator('html').evaluate(element => {
      const style = getComputedStyle(element)
      return ['--color-subtle', '--color-muted', '--color-highlight-med'].map(
        token => style.getPropertyValue(token).trim()
      )
    })
    const medium = `#${[1, 3, 5]
      .map(index =>
        Math.round(
          parseInt(syntax.nijimi2.slice(index, index + 2), 16) * 0.45 +
            parseInt(colors.panel.slice(index, index + 2), 16) * 0.55
        )
          .toString(16)
          .padStart(2, '0')
      )
      .join('')}`
    expect(tokens).toEqual([...NATIVE_NEUTRALS[theme], medium])
    const appearance = await fixture.evaluate(element => {
      const probe = document.createElement('span')
      element.append(probe)
      const results = [
        ...element.querySelectorAll<HTMLElement>(
          '.badge-count, .text-muted, .metadata-origin, .text-primary, .text-accent, .text-base-content'
        ),
      ].map(badge => {
        const style = getComputedStyle(badge)
        probe.style.color = badge.classList.contains('badge-count')
          ? 'var(--color-subtle)'
          : badge.classList.contains('text-muted')
            ? 'var(--color-muted)'
            : badge.classList.contains('metadata-origin')
              ? 'var(--color-secondary)'
              : badge.classList.contains('text-primary')
                ? 'var(--color-primary)'
                : badge.classList.contains('text-accent')
                  ? 'var(--color-accent)'
                  : 'var(--color-base-content)'
        const expected = getComputedStyle(probe)
        return {
          kind: badge.className,
          background: style.backgroundColor,
          border: style.borderWidth,
          semanticText: style.color === expected.color,
          fontSize: style.fontSize,
          fontWeight: style.fontWeight,
          lineHeight: style.lineHeight,
        }
      })
      probe.remove()
      return results
    })
    expect(appearance).toHaveLength(40)
    for (const badge of appearance) {
      expect(badge.background, badge.kind).toBe('rgba(0, 0, 0, 0)')
      expect(badge.border, badge.kind).toBe('0px')
      expect(badge.semanticText, badge.kind).toBe(true)
      expect(badge.fontSize, badge.kind).toBe('14px')
      expect(badge.fontWeight, badge.kind).toBe('400')
      expect(badge.lineHeight, badge.kind).toBe('21px')
    }
  })

  test(`Text and accent contrast on all five surfaces, with native Subtle/Muted in ${theme}`, async ({
    page,
  }) => {
    await showMetadataExamples(page, theme)
    const fixture = page.locator('#metadata-a11y-fixture')
    // AGENTS.md deliberately retains Usuzumi 1 and Kasure 1, including on
    // interaction surfaces. Check those exact roles, not a 4.5 adjustment.
    for (const [selector, role] of [
      ['.badge-count', 'subtle'],
      ['.text-muted', 'muted'],
    ] as const) {
      const labels = fixture.locator(selector)
      await expect(labels).toHaveCount(5)
      for (const label of await labels.all()) {
        await expect(label).toHaveCSS('color', themeRgb(theme, role))
        await expect(label).toHaveCSS('opacity', '1')
      }
    }
    const scan = await new AxeBuilder({ page })
      .include('#metadata-a11y-fixture')
      // Only these two documented native roles are outside the contrast target.
      // Text, Primary, Secondary and Accent still pass axe on every surface;
      // the full Home WCAG scan below retains all rules.
      .exclude('#metadata-a11y-fixture .badge-count')
      .exclude('#metadata-a11y-fixture .text-muted')
      .withRules(['color-contrast'])
      .analyze()
    expect(scan.violations).toEqual([])
  })
}

for (const colorScheme of ['light', 'dark'] as const) {
  test(`defaults to Autumn with the system ${colorScheme} preference`, async ({
    page,
  }) => {
    await interceptRpc(page)
    await page.emulateMedia({ colorScheme })
    await page.goto('/')
    await expect(page.locator('html')).toHaveAttribute(
      'data-theme',
      `madoromi-aki-${colorScheme === 'light' ? 'hiru' : 'yoru'}`
    )
    await expect(page.getByRole('button', { name: /^Season:/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /Evals/ })).toHaveCount(0)
  })
}

for (const { id: theme, scheme, name } of THEMES) {
  test(`appearance controls preserve saved ${theme}, keyboard focus and shared ghost states`, async ({
    page,
  }) => {
    await interceptRpc(page)
    await page.emulateMedia({
      colorScheme: scheme === 'light' ? 'dark' : 'light',
      reducedMotion: 'reduce',
    })
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await page.goto('/')
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    const appearance = page.getByRole('group', {
      name: 'Appearance',
      exact: true,
    })
    const ink = appearance.getByRole('button', { name: /^Season:/ })
    await expect(appearance).toHaveCSS('border-width', '0px')
    const mode = appearance.getByRole('button', {
      name: `${name} theme active`,
    })
    await expect(ink).toHaveCSS('border-top-right-radius', '0px')
    await expect(mode).toHaveCSS('border-top-left-radius', '0px')
    for (const control of [ink, mode]) {
      await expect(control).toHaveCSS('font-size', '14px')
      await expect(control).toHaveCSS('font-weight', '400')
    }
    await expect(ink.locator('svg')).toHaveCount(1)
    await expect(ink).toHaveText(name.split(' ')[0]!)
    const geometry = await appearance.evaluate(element => {
      const buttons = [...element.children].filter(
        child => child.tagName === 'BUTTON'
      )
      return buttons.map(button => button.getBoundingClientRect().toJSON())
    })
    expect(geometry).toHaveLength(2)
    expect(geometry[1].left).toBeCloseTo(geometry[0].right, 0)
    await ink.hover()
    await expect(ink).toHaveCSS(
      'background-color',
      themeRgb(theme, 'highlightLow')
    )
    const resting = await ink.boundingBox()
    await page.mouse.down()
    await expect(ink).toHaveCSS(
      'background-color',
      themeRgb(theme, 'highlightLow')
    )
    expect(await ink.boundingBox()).toEqual(resting)
    // Release outside the button, then activate without pointer hover.
    await page.mouse.move(0, 0)
    await page.mouse.up()
    await ink.focus()
    await page.keyboard.down('Space')
    await expect(ink).toHaveCSS(
      'background-color',
      themeRgb(theme, 'highlightMedium')
    )
    expect(await ink.boundingBox()).toEqual(resting)
    await page.keyboard.up('Space')
    const seasons = page.getByRole('radiogroup', {
      name: 'Season',
      exact: true,
    })
    await expect(seasons).toBeVisible()
    const selectedSeason = SEASONS.find(
      season => season.name === name.split(' ')[0]
    )!
    await expect(
      seasons.getByRole('radio', { checked: true })
    ).toHaveAccessibleName(selectedSeason.name)
    await expect(seasons.getByRole('radio')).toHaveCount(4)
    for (const choice of await seasons.getByRole('radio').all()) {
      await expect(choice.locator('svg')).toHaveCount(1)
    }
    const themePicker = page.locator('.theme-picker-popover:popover-open')
    await expect(themePicker).toHaveCSS('border-width', '1px')
    await expect(themePicker).toHaveRole('dialog')
    await expect(themePicker.getByRole('searchbox')).toHaveCount(0)
    await expect(themePicker.locator('[data-ink-choice]')).toHaveCount(0)
    await page.keyboard.press('Escape')
    await expect(ink).toBeFocused()
    await expect(ink).toHaveAttribute('aria-expanded', 'false')
    await expect(seasons).not.toBeVisible()
    await ink.focus()
    await page.keyboard.press('Enter')
    const selectedInk = seasons.getByRole('radio', { checked: true })
    await expect(selectedInk).toBeFocused()
    await expect(selectedInk).toHaveAttribute('aria-checked', 'true')
    await page.keyboard.press('Escape')
    await expect(ink).toBeFocused()
    for (const pane of ['sidebar', 'details']) {
      const toggle = page.getByRole('button', {
        name: `Collapse ${pane}`,
        exact: true,
      })
      await expect(toggle).toHaveCSS('color', themeRgb(theme, 'text'))
      await expect(toggle.locator('svg')).toHaveCSS(
        'color',
        themeRgb(theme, 'text')
      )
      await expect(toggle).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      const box = (await toggle.boundingBox())!
      expect(box.width).toBe(box.height)
      expect(
        await toggle.evaluate(element =>
          parseFloat(getComputedStyle(element).borderTopLeftRadius)
        )
      ).toBeGreaterThanOrEqual(box.width / 2)
      await toggle.hover()
      await expect(toggle).toHaveCSS(
        'background-color',
        themeRgb(theme, 'highlightLow')
      )
      await toggle.click()
      const reopen = page.getByRole('button', {
        name: `Open ${pane}`,
        exact: true,
      })
      await expect(reopen).toHaveAttribute('aria-expanded', 'false')
      await expect(reopen).toHaveCSS(
        'background-color',
        themeRgb(theme, 'highlightLow')
      )
      await page.mouse.move(0, 0)
      await expect(reopen).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(reopen.locator('svg')).toHaveCSS(
        'color',
        themeRgb(theme, 'subtle')
      )
      await reopen.focus()
      await page.keyboard.press('Enter')
      await expect(toggle).toBeFocused()
      await expect(toggle).toHaveAttribute('aria-expanded', 'true')
      await expect(toggle).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    }
  })
}

test('season trigger updates its drawing while keeping the picker open and keyboard focus', async ({
  page,
}) => {
  await interceptRpc(page)
  await page.addInitScript(() =>
    localStorage.setItem('theme', 'madoromi-aki-yoru')
  )
  await page.goto('/')
  const trigger = page.getByRole('button', { name: /^Season:/ })
  await trigger.click()
  const seasons = page.getByRole('radiogroup', { name: 'Season', exact: true })
  const originalButton = await trigger.elementHandle()
  for (const name of ['Winter', 'Spring', 'Summer', 'Autumn']) {
    await page.keyboard.press('ArrowDown')
    const chosen = seasons.getByRole('radio', { name, exact: true })
    await expect(chosen).toBeFocused()
    await expect(chosen).toHaveAttribute('aria-checked', 'true')
    await expect(trigger).toHaveAccessibleName(`Season: ${name}`)
    await expect(trigger).toHaveAttribute('aria-expanded', 'true')
    const expected = await chosen.locator('svg').innerHTML()
    await expect.poll(() => trigger.locator('svg').innerHTML()).toBe(expected)
    expect(
      await trigger.evaluate(
        (element, original) => element === original,
        originalButton
      )
    ).toBe(true)
  }
  await page.keyboard.press('Escape')
  await expect(trigger).toBeFocused()
})

test('season picker stays compact and viewport-clamped without swatch controls', async ({
  page,
}) => {
  await interceptRpc(page)
  await page.addInitScript(() =>
    localStorage.setItem('theme', 'madoromi-aki-yoru')
  )
  for (const viewport of [
    { width: 320, height: 720 },
    { width: 390, height: 720 },
    { width: 520, height: 720 },
    { width: 1024, height: 768 },
    { width: 1440, height: 900 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize({
      width: viewport.width,
      height: viewport.height,
    })
    await page.goto('/')
    const trigger = page.getByRole('button', { name: /^Season:/ })
    await expect(trigger.locator('svg')).toHaveCount(1)
    const savedAssignments = await page.evaluate(() =>
      Object.entries(localStorage).filter(([key]) =>
        key.startsWith('graph-colour-')
      )
    )
    await trigger.click()
    const picker = page.locator('.theme-picker-popover:popover-open')
    await expectDropdownSurface(picker, 'madoromi-aki-yoru')
    await expect(
      picker.getByRole('heading', { name: /^(Theme|Preview)$/ })
    ).toHaveCount(0)
    await expect(
      picker.getByText('Trace preview', { exact: true })
    ).toHaveCount(0)
    await expect(picker.getByRole('table')).toHaveCount(0)
    await expect(
      picker.getByRole('button', { name: 'Close theme picker' })
    ).toHaveCount(0)
    await expect(picker.getByRole('searchbox')).toHaveCount(0)
    const seasons = picker.getByRole('radiogroup', {
      name: 'Season',
      exact: true,
    })
    await expect(seasons.getByRole('radio')).toHaveCount(4)
    await expect(seasons.locator('svg')).toHaveCount(4)
    await expect(picker.locator('[data-ink-choice]')).toHaveCount(0)
    const selected = seasons.getByRole('radio', { name: 'Autumn' })
    await expect(selected).toBeFocused()
    const selectedBox = (await selected.boundingBox())!
    await expect(seasons.locator('[tabindex="0"]')).toHaveCount(1)
    expect(
      await page.evaluate(() =>
        Object.entries(localStorage).filter(([key]) =>
          key.startsWith('graph-colour-')
        )
      )
    ).toEqual(savedAssignments)
    const box = (await picker.boundingBox())!
    const triggerBox = (await trigger.boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(8)
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width - 8)
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height - 8)
    await expect(picker).toHaveCSS('transform', 'none')
    if (viewport.width >= 1024) {
      expect(box.y).toBeCloseTo(triggerBox.y + triggerBox.height + 8, 0)
      expect(box.x + box.width).toBeCloseTo(triggerBox.x + triggerBox.width, 0)
    }
    expect(selectedBox.y).toBeGreaterThanOrEqual(box.y)
    expect(selectedBox.y + selectedBox.height).toBeLessThanOrEqual(
      box.y + box.height
    )
    expect(box.width).toBeLessThanOrEqual(288)
    await expect(selected).toHaveAttribute('aria-checked', 'true')
    await page.keyboard.press('Escape')
    await expect(trigger).toBeFocused()
  }
})

test('persists season and Day/Night with one fixed interface palette', async ({
  page,
}) => {
  await interceptRpc(page)
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
  await page.goto('/')
  const ink = page.getByRole('button', { name: /^Season:/ })
  await ink.click()
  await expect(page.getByRole('radio', { name: 'Autumn' })).toBeFocused()
  await expect(ink).toHaveAttribute('aria-expanded', 'true')
  const winter = page
    .getByRole('radiogroup', { name: 'Season', exact: true })
    .getByRole('radio', { name: 'Winter', exact: true })
  await winter.focus()
  await page.keyboard.press('Enter')
  await expect(winter).toBeFocused()
  await expect(ink).toHaveAttribute('aria-expanded', 'true')
  await expect(page.locator('html')).toHaveAttribute(
    'data-theme',
    'madoromi-fuyu-hiru'
  )
  const picker = page.locator('.theme-picker-popover:popover-open')
  await expect(picker).toHaveCSS(
    'background-color',
    themeRgb('madoromi-fuyu-hiru', 'panel')
  )
  const selectedInk = winter
  await expect(selectedInk).toHaveAttribute('aria-checked', 'true')
  await expect(selectedInk).toHaveCSS(
    'background-color',
    themeRgb('madoromi-fuyu-hiru', 'highlightMedium')
  )
  await selectedInk.hover()
  await expect(selectedInk).toHaveCSS(
    'background-color',
    themeRgb('madoromi-fuyu-hiru', 'highlightLow')
  )
  await page.keyboard.press('Escape')
  await expect(ink).toBeFocused()
  const mode = page.getByRole('button', { name: 'Winter Day theme active' })
  await mode.focus()
  await page.keyboard.press('Enter')
  await expect(page.locator('html')).toHaveAttribute(
    'data-theme',
    'madoromi-fuyu-yoru'
  )
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute(
    'data-theme',
    'madoromi-fuyu-yoru'
  )
  await expect(page.locator('.primary-navigation .tab-active svg')).toHaveCSS(
    'color',
    themeRgb('madoromi-fuyu-yoru', 'primary')
  )
  expect(
    await page.evaluate(() => ({
      theme: localStorage.getItem('theme'),
    }))
  ).toEqual({ theme: 'madoromi-fuyu-yoru' })
  await ink.click()
  await expect(winter).toBeFocused()
  await ink.click()
  await expect(ink).toBeFocused()
  await expect(picker).not.toBeVisible()
  await ink.click()
  await expect(winter).toBeFocused()
  await page.mouse.click(8, 8)
  await expect(picker).not.toBeVisible()
})

test('oversized service tooltips remain scrollable and selectable on a narrow viewport', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await interceptRpc(page)
  const label = `service-${'recorded-service-segment-'.repeat(240)}end`
  await routeTelemetry(page, async route => {
    const { id, method } = route.request().postDataJSON()
    if (method === 'getTraceLogSummaries')
      return route.fulfill({ json: { jsonrpc: '2.0', id, result: [] } })
    if (method === 'getTraceView')
      return route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: {
            traceID: 'a'.repeat(32),
            traceStart: '1700000000000000000',
            unplacedSpanCount: 0,
            resources: {},
            scopes: {},
            spans: [],
          },
        },
      })
    if (method !== 'searchTraceSummaries') return route.fallback()
    return route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result: [
          {
            traceID: 'a'.repeat(32),
            hasRootSpan: true,
            rootSpan: { name: 'Long service trace', serviceName: label },
            startTime: '1700000000000000000',
            durationNs: '1000000000',
            spanCount: 8,
            errorCount: 0,
            serviceBreakdown: [
              { key: `service:${label}`, label, count: 7 },
              { key: 'service:other', label: 'other', count: 1 },
            ],
          },
        ],
      },
    })
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/traces')
  const segment = page.locator('.breakdown-segment').first()
  await segment.hover()
  const tooltip = page.locator('.breakdown-tooltip:popover-open')
  await expect(tooltip).toBeVisible()
  await expect(tooltip).toHaveCSS('overflow-y', 'auto')
  const bounds = (await tooltip.boundingBox())!
  expect(bounds.x).toBeGreaterThanOrEqual(8)
  expect(bounds.y).toBeGreaterThanOrEqual(8)
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(382)
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(836)
  expect(
    await tooltip.evaluate(
      element => element.scrollHeight > element.clientHeight
    )
  ).toBe(true)

  await tooltip.hover()
  await page.mouse.wheel(0, 100000)
  await expect
    .poll(() =>
      tooltip.evaluate(
        element =>
          element.scrollTop + element.clientHeight >= element.scrollHeight - 1
      )
    )
    .toBe(true)
  const labelNode = tooltip.locator('.breakdown-tooltip__label')
  const tailVisible = await labelNode.evaluate(element => {
    const range = document.createRange()
    const text = element.firstChild!
    range.setStart(text, text.textContent!.length - 3)
    range.setEnd(text, text.textContent!.length)
    const rect = range.getBoundingClientRect()
    return element.contains(
      document.elementFromPoint(
        rect.x + rect.width / 2,
        rect.y + rect.height / 2
      )
    )
  })
  expect(tailVisible).toBe(true)
  const count = tooltip.getByText('[7]', { exact: true })
  await count.scrollIntoViewIfNeeded()
  expect(
    await count.evaluate(element => {
      const rect = element.getBoundingClientRect()
      return element.contains(
        document.elementFromPoint(
          rect.x + rect.width / 2,
          rect.y + rect.height / 2
        )
      )
    })
  ).toBe(true)
  expect(
    await labelNode.evaluate(element => {
      const range = document.createRange()
      range.selectNodeContents(element)
      const selection = window.getSelection()!
      selection.removeAllRanges()
      selection.addRange(range)
      return selection.toString()
    })
  ).toBe(label)
  await page.keyboard.press('Escape')
  await expect(tooltip).toHaveCount(0)
  expect(errors).toEqual([])
})

for (const { theme, low, selected: selectedColor } of HIGHLIGHT_THEMES) {
  test(`card service bars preserve native selection and readable tooltips in ${theme}`, async ({
    page,
  }) => {
    await interceptRpc(page)
    await interceptTraces(page)
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.goto(`/traces/${TRACE_IDS[0]}?span=${SPAN_IDS[0]}&time=all`)
    const card = page.getByRole('button', {
      name: /^Inventory trace/,
    })
    const bar = card.getByRole('group', {
      name: /Service breakdown by span count/,
    })
    await expect(card).toHaveAttribute('aria-pressed', 'false')
    const segment = bar.locator('.breakdown-segment').first()
    const beforeHover = page.url()
    await segment.hover()
    const tooltip = card.locator('.breakdown-tooltip:popover-open')
    await expect(tooltip).toHaveText('checkout [1]')
    await expect(tooltip).toHaveCSS('font-size', '14px')
    await expect(tooltip).toHaveCSS('line-height', '21px')
    await expect(tooltip).toHaveCSS('font-weight', '400')
    await expect(tooltip).toHaveCSS(
      'background-color',
      themeRgb(theme, 'panel')
    )
    await expect(tooltip).toHaveCSS('color', themeRgb(theme, 'text'))
    await expect(card).toHaveCSS('background-color', low)
    expect(page.url()).toBe(beforeHover)
    await page.mouse.down()
    await expect(card).toHaveAttribute('aria-pressed', 'false')
    expect(page.url()).toBe(beforeHover)
    await page.mouse.up()
    await expect(card).toHaveAttribute('aria-pressed', 'true')
    await expect(card).toHaveCSS('background-color', low)
    expect(new URL(page.url()).pathname).toBe(`/traces/${TRACE_IDS[1]}`)
    await page.mouse.move(0, 0)
    await expect(card).toHaveCSS('background-color', selectedColor)
    await expect(tooltip).toHaveCount(0)
    await expect(card.locator('.signal-row__title')).toHaveText(
      'Inventory trace'
    )
    await expect(card.locator('.signal-row__title')).toHaveCSS(
      'font-size',
      '17px'
    )
    await expect(card.locator('.signal-row__title')).toHaveCSS(
      'font-weight',
      '400'
    )
    await expect(card.locator('.signal-row__title')).toHaveCSS(
      'font-family',
      /Atkinson Hyperlegible Next/
    )
    await expect(card.locator('.signal-row__title')).toHaveCSS(
      'line-height',
      '25.5px'
    )
    await expect(card.locator('.signal-date__label')).toHaveText('Start:')
    const dateSpacing = await card.locator('.signal-date').evaluate(element => {
      const label = element.querySelector('.signal-date__label')!
      const text = document.createRange()
      text.selectNodeContents(label)
      const labelBounds = label.getBoundingClientRect()
      const valueBounds = label.nextElementSibling!.getBoundingClientRect()
      return {
        unusedLabelWidth:
          labelBounds.width - text.getBoundingClientRect().width,
        gap: valueBounds.left - labelBounds.right,
      }
    })
    expect(Math.abs(dateSpacing.unusedLabelWidth)).toBeLessThan(1)
    expect(dateSpacing.gap).toBeCloseTo(8, 0)
    const datePlacement = await card.evaluate(element => {
      const date = element
        .querySelector('.signal-date')!
        .getBoundingClientRect()
      const header = element
        .querySelector('.signal-row__header')!
        .getBoundingClientRect()
      const body = element
        .querySelector('.signal-row__body')!
        .getBoundingClientRect()
      const bar = element
        .querySelector('.breakdown-bar')!
        .getBoundingClientRect()
      return {
        leftOffset: date.left - header.left,
        bottomOffset: body.bottom - date.bottom,
        followsBar: date.top >= bar.bottom,
      }
    })
    expect(Math.abs(datePlacement.leftOffset)).toBeLessThan(1)
    expect(Math.abs(datePlacement.bottomOffset)).toBeLessThan(1)
    expect(datePlacement.followsBar).toBe(true)
    await expect(card).toHaveAccessibleName(
      /checkout: 1 span.*payments: 1 span.*database: 1 span/
    )
    await expect(
      card.locator('button, a, [tabindex], .legend-rows')
    ).toHaveCount(0)
    await expect(card.locator('.breakdown-segment')).toHaveCount(3)
    const other = page
      .locator('button.signal-row')
      .filter({ hasText: 'Checkout trace' })
    await other.focus()
    await page.keyboard.press('Space')
    await expect(other).toHaveAttribute('aria-pressed', 'true')
    await card.focus()
    await page.keyboard.press('Enter')
    await expect(card).toHaveAttribute('aria-pressed', 'true')
    await expect(card).toBeFocused()
    const scan = await new AxeBuilder({ page })
      .include('.signal-row--selected')
      .withRules(['nested-interactive'])
      .analyze()
    expect(scan.violations).toEqual([])
  })

  test(`sort target uses full text contrast in ${theme}`, async ({ page }) => {
    await interceptRpc(page)
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await page.goto('/traces')
    const trigger = page.getByRole('button', { name: /^Sort by:/ })
    await expect(trigger).toHaveAccessibleName('Sort by: Start Time')
    const textColor = themeRgb(theme, 'text')
    await expect(trigger.getByText('Start Time', { exact: true })).toHaveCSS(
      'color',
      textColor
    )
    await expect(trigger.getByText('Sort by:', { exact: true })).toHaveCount(0)
    await trigger.click()
    await expectDropdownSurface(
      page.getByRole('group', { name: 'Sort by', exact: true }),
      theme
    )
    await page
      .getByRole('group', { name: 'Sort by', exact: true })
      .getByRole('button', { name: 'Duration', exact: true })
      .click()
    await expect(trigger.getByText('Duration', { exact: true })).toHaveCSS(
      'color',
      textColor
    )
  })

  test(`enabled tabs stay full-strength when inactive in ${theme}`, async ({
    page,
  }) => {
    await interceptRpc(page)
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await page.goto('/')
    const textColor = themeRgb(theme, 'text')
    const navigation = page.getByRole('navigation', { name: 'Primary' })
    const inactive = navigation.locator('.tab:not(.tab-active)')
    await expect(inactive).toHaveCount(3)
    for (const tab of await inactive.all()) {
      await expect(tab).toHaveCSS('color', textColor)
      await expect(tab).toHaveCSS('opacity', '1')
      await expect(tab.locator('svg')).toHaveCSS('color', textColor)
    }
    const paneTabs = page.locator(
      '.pane-header .header-tab:not(:disabled, [aria-disabled="true"])'
    )
    expect(await paneTabs.count()).toBeGreaterThan(1)
    for (const tab of await paneTabs.all()) {
      await expect(tab).toHaveCSS('color', textColor)
    }
    // Disabled local tabs retain DaisyUI's attenuation and cannot be activated.
    const disabledOpacity = await navigation.evaluate(element => {
      const disabled = document.createElement('button')
      disabled.className = 'tab'
      disabled.disabled = true
      disabled.textContent = 'Disabled test tab'
      element.append(disabled)
      const opacity = getComputedStyle(disabled).opacity
      disabled.remove()
      return opacity
    })
    expect(disabledOpacity).toBe('0.4')
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()
    const presets = page.locator('.preset-time-ranges .header-tab')
    expect(await presets.count()).toBeGreaterThan(1)
    for (const tab of await presets.all()) {
      await expect(tab).toHaveCSS('color', textColor)
      await expect(tab).toHaveCSS('opacity', '1')
    }
  })

  test(`search completions use rounded Low hover and Medium selection in ${theme}`, async ({
    page,
  }) => {
    await interceptRpc(page)
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await page.goto('/traces')
    const editor = page.locator('.search-bar .cm-content')
    await editor.click()
    await page.keyboard.press('Control+Space')
    const selected = page.locator(
      '.cm-tooltip-autocomplete li[aria-selected="true"]'
    )
    await expect(selected).toBeVisible()
    await expectDropdownSurface(page.locator('.cm-tooltip-autocomplete'), theme)
    await expect(selected).toHaveCSS('border-radius', '12px')
    await expect(selected).toHaveCSS('background-color', selectedColor)
    const idle = page
      .locator('.cm-tooltip-autocomplete li:not([aria-selected])')
      .first()
    await idle.hover()
    await expect(idle).toHaveCSS('border-radius', '12px')
    await expect(idle).toHaveCSS('background-color', low)
    await selected.hover()
    await expect(selected).toHaveCSS('background-color', low)
    await editor.hover()
    await expect(selected).toHaveCSS('background-color', selectedColor)
    const selection = await editor.evaluate(
      element => getComputedStyle(element, '::selection').backgroundColor
    )
    expect(selection).toBe(selectedColor)
  })
}

test.describe('home page accessibility', () => {
  test.beforeEach(async ({ page }) => {
    await interceptRpc(page)
    await page.goto('/')
    await expect(
      page.getByRole('region', { name: 'Service map', exact: true })
    ).toBeVisible()
  })

  test('has no automatically detectable Home page WCAG violations', async ({
    page,
  }) => {
    // Theme hydration transitions the canvas. Scan its settled surface, not
    // an intermediate colour frozen by axe while it measures descendants.
    await page.getByRole('main').evaluate(async element => {
      await Promise.all(
        element.getAnimations().map(animation => animation.finished)
      )
    })
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

  test('keeps Home import and setup guide accessible', async ({ page }) => {
    const imports = page.getByRole('button', { name: /^Import files\./ })
    await expect(imports).toBeVisible()
    await expect(imports).toHaveText(
      'Drop telemetry JSON files here or anywhere on the screen.'
    )
    await expect(imports).toHaveCSS('border-style', 'dotted')
    await expect(
      page.getByRole('region', { name: 'Getting started', exact: true })
    ).toBeVisible()
    await expect(page.locator('.guide-name')).toHaveText('OTel Desktop Viewer')
    await expect(
      page.getByText('Connect an exporter', { exact: true })
    ).toBeVisible()
    await expect(
      page.getByText('Send a test trace', { exact: true })
    ).toBeVisible()
    await expect(
      page.locator('.page-layout__main-chrome > footer')
    ).toHaveCount(0)
  })

  test('uses the approved 20% Winter Day surfaces and mapped foregrounds', async ({
    page,
  }) => {
    await page.evaluate(() =>
      localStorage.setItem('theme', 'madoromi-fuyu-hiru')
    )
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute(
      'data-theme',
      'madoromi-fuyu-hiru'
    )
    // 20% surfaces, 45% selection, native neutrals and the shared six-role palette.
    const expected = {
      '--color-base-100': '#f1f2f3',
      '--color-base-200': '#e7e8ea',
      '--color-base-300': '#dddee0',
      '--color-highlight-low': '#d0d1d5',
      '--color-highlight-med': '#cecfd2',
      '--color-highlight-high': '#c0c1c5',
      '--color-base-content': '#4a4e63',
      '--color-on-light': '#4a4e63',
      '--color-on-dark': '#f1f2f3',
      '--color-muted': '#8d91a4',
      '--color-subtle': '#5e6277',
      '--color-sakura': '#aa6e89',
      '--color-primary': '#694d80',
      '--color-secondary': '#1e5d88',
      '--color-accent': '#18644a',
      '--color-neutral': '#8d91a4',
      '--color-neutral-content': '#4a4e63',
      '--color-info': '#1e5d88',
      '--color-success': '#18644a',
      '--color-warning': '#7a5000',
      '--color-error': '#9b334e',
    }
    const actual = await page.evaluate(tokens => {
      const style = getComputedStyle(document.documentElement)
      return Object.fromEntries(
        tokens.map(token => [token, style.getPropertyValue(token).trim()])
      )
    }, Object.keys(expected))
    expect(actual).toEqual(expected)
  })

  for (const { id: theme } of THEMES) {
    const canvas = themeRgb(theme, 'canvas')
    const panel = themeRgb(theme, 'panel')
    test(`uses Madoromi paper roles for fields and panes in ${theme}`, async ({
      page,
    }) => {
      await page.evaluate(theme => localStorage.setItem('theme', theme), theme)
      await page.goto('/traces')
      await expect(page.getByRole('main')).toHaveCSS('background-color', canvas)
      await expect(page.locator('.search-bar')).toHaveCSS(
        'background-color',
        panel
      )
      await expect(
        page.locator('.signal-drawer__controls > .pane-header')
      ).toHaveCSS('background-color', panel)
      await expect(page.locator('.signal-drawer__panel')).toHaveCSS(
        'background-color',
        canvas
      )
      await expect(
        page.locator('header[aria-label="Workspace navigation and search"]')
      ).toHaveCSS('background-color', canvas)
      await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
        'background-color',
        canvas
      )
      await interceptTraces(page)
      await page.goto(`/traces/${TRACE_IDS[0]}?span=${SPAN_IDS[0]}&time=all`)
      await expect(
        page.locator('.page-layout__main-chrome .pane-header').first()
      ).toHaveCSS('background-color', panel)
      const detailHeadings = page.locator(
        '.detail-stack:visible > .detail-bubble > :is(details, section) > .detail-bubble__heading'
      )
      await expect(detailHeadings).toHaveCount(3)
      for (const heading of await detailHeadings.all()) {
        await expect(heading).toHaveCSS('background-color', panel)
        await expect(heading).toHaveCSS('font-weight', '400')
        await expect(heading).toHaveCSS('height', '48px')
        await expect(heading).toHaveCSS('border-radius', '12px')
      }
    })
  }

  test('preserves Winter Day surfaces and independent highlights', async ({
    page,
  }) => {
    await page.evaluate(() =>
      localStorage.setItem('theme', 'madoromi-fuyu-hiru')
    )
    await page.goto('/traces')
    await expect(page.getByRole('main')).toHaveCSS(
      'background-color',
      'rgb(241, 242, 243)'
    )
    await expect(page.locator('.signal-drawer__panel')).toHaveCSS(
      'background-color',
      'rgb(241, 242, 243)'
    )
    await expect(page.locator('.search-bar')).toHaveCSS(
      'background-color',
      'rgb(231, 232, 234)'
    )
    await expect(
      page.locator('.signal-drawer__controls .pane-header')
    ).toHaveCSS('background-color', 'rgb(231, 232, 234)')
    await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
      'background-color',
      'rgb(241, 242, 243)'
    )

    await interceptTraces(page)
    await page.goto(`/traces/${TRACE_IDS[0]}?span=${SPAN_IDS[0]}&time=all`)
    await expect(page.locator('.signal-row--selected')).toHaveCSS(
      'background-color',
      'rgb(206, 207, 210)'
    )
    await expect(
      page.locator('.signal-row:not(.signal-row--selected)').first()
    ).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(page.locator('.detail-bubble__heading').first()).toHaveCSS(
      'background-color',
      'rgb(231, 232, 234)'
    )
    await expect(page.locator('.signal-row--selected')).toHaveCSS(
      'outline-style',
      'none'
    )
    await page.locator('.signal-row--selected .signal-row__header').hover()
    await expect(page.locator('.signal-row--selected')).toHaveCSS(
      'background-color',
      'rgb(208, 209, 213)'
    )
    await page.mouse.move(0, 0)
    await expect(page.locator('.signal-row--selected')).toHaveCSS(
      'background-color',
      'rgb(206, 207, 210)'
    )
  })

  for (const {
    theme,
    low,
    selected: selectedColor,
    high,
  } of HIGHLIGHT_THEMES) {
    test(`shares theme hover and selection colours across items and header tabs in ${theme}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 1920, height: 1000 })
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await interceptTraces(page)
      await page.evaluate(theme => localStorage.setItem('theme', theme), theme)
      await page.goto(`/traces/${TRACE_IDS[0]}?span=${SPAN_IDS[0]}&time=all`)
      expect(
        await page
          .locator('html')
          .evaluate(element =>
            getComputedStyle(element)
              .getPropertyValue('--color-highlight-high')
              .trim()
          )
      ).toBe(high)
      const selectedCard = page.locator('.signal-row--selected')
      const idleCard = page
        .locator('.signal-row:not(.signal-row--selected)')
        .first()
      await expect(idleCard).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await idleCard.locator('.signal-row__header').hover()
      await expect(idleCard).toHaveCSS('background-color', low)
      await selectedCard.locator('.signal-row__header').hover()
      await expect(idleCard).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(selectedCard).toHaveCSS('background-color', low)
      await page.mouse.move(0, 0)
      await expect(selectedCard).toHaveCSS('background-color', selectedColor)
      await expect(
        page.locator('.detail-bubble__heading').first()
      ).not.toHaveCSS('background-color', selectedColor)

      const row = page.locator(`.waterfall-row[data-span-id="${SPAN_IDS[1]}"]`)
      await row.hover()
      const rowCell = row.locator('th, td').first()
      await expect(rowCell).toHaveCSS('background-color', low)
      await expect(rowCell).toHaveCSS('border-top-left-radius', '12px')
      await expect(row.locator('th, td').last()).toHaveCSS(
        'border-bottom-right-radius',
        '12px'
      )
      await row.getByRole('button', { name: /span field: name/ }).click()
      await expect(row).toHaveAttribute('aria-selected', 'true')
      await expect(rowCell).toHaveCSS('background-color', low)
      await page.mouse.move(0, 0)
      await expect(rowCell).toHaveCSS('background-color', selectedColor)
      await row.hover()
      await expect(rowCell).toHaveCSS('background-color', low)
      await expect(row).toHaveCSS('outline-style', 'none')
      await expect(
        page.getByRole('navigation', { name: 'Primary' }).locator('.tab-active')
      ).toHaveCSS('background-color', selectedColor)
      const paneTab = page
        .getByRole('tablist', { name: 'Detail view' })
        .first()
        .getByRole('tab', { selected: true })
      await paneTab.hover()
      await expect(paneTab).toHaveCSS('background-color', low)
      await page.mouse.move(0, 0)
      await expect(paneTab).toHaveCSS('background-color', selectedColor)
      await expect(paneTab).toHaveCSS('box-shadow', 'none')
      const highlight = await paneTab.evaluate(element => {
        const style = getComputedStyle(
          element.closest('.header-tabs')!,
          '::before'
        )
        return style.content
      })
      expect(highlight).toBe('none')

      await page.goto('/traces')
      await page.getByRole('button', { name: /^Sort by:/ }).click()
      const menu = page.getByRole('group', { name: 'Sort by', exact: true })
      const selectedOption = menu.locator('button[aria-pressed="true"]')
      await expect(selectedOption).toHaveCSS('border-radius', '12px')
      await selectedOption.hover()
      await expect(selectedOption).toHaveCSS('background-color', low)
      await page.mouse.move(0, 0)
      await expect(selectedOption).toHaveCSS('background-color', selectedColor)
      const idleOption = menu.locator('button[aria-pressed="false"]').first()
      await expect(idleOption).toHaveCSS('border-radius', '12px')
      await idleOption.hover()
      await expect(idleOption).toHaveCSS('background-color', low)
      await idleOption.focus()
      await page.keyboard.press('Tab')
      await page.keyboard.press('Shift+Tab')
      await expect(idleOption).toBeFocused()
      await expect(idleOption).toHaveCSS(
        'outline-color',
        themeRgb(theme, 'primary')
      )
      await page.keyboard.press('Escape')
      await page.getByRole('button', { name: /Change time range/i }).click()
      await expect(
        page.locator('.preset-time-ranges [aria-pressed="true"]')
      ).toHaveCSS('background-color', selectedColor)
      await page.getByText('Timezone:', { exact: true }).click()
      const timezone = page.locator('.tz-option[aria-pressed="true"]')
      await expect(timezone).toHaveCSS('border-radius', '12px')
      await timezone.hover()
      await expect(timezone).toHaveCSS('background-color', low)
      await page.mouse.move(0, 0)
      await expect(timezone).toHaveCSS('background-color', selectedColor)
      const idleTimezone = page
        .locator('.tz-option[aria-pressed="false"]')
        .first()
      await idleTimezone.hover()
      await expect(idleTimezone).toHaveCSS('background-color', low)
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
      page.getByRole('link', { name: /^Home(?: \[|$)/ })
    ).toBeFocused()
    await page.keyboard.press('Tab')
    await expect(
      page.getByRole('link', { name: /^Traces(?: \[|$)/ })
    ).toBeFocused()
  })

  test('keeps the sidebar toggle transparent and distinguishes its state with icon colour', async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.setViewportSize({ width: 1440, height: 900 })
    const toggle = page.getByRole('button', { name: 'Collapse sidebar' })
    const activeColour = await toggle.evaluate(
      element => getComputedStyle(element).color
    )
    const theme = (await page.locator('html').getAttribute('data-theme'))!
    expect(activeColour).toBe(themeRgb(theme, 'text'))
    await expect(toggle).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    const handle = await page
      .getByRole('separator', { name: 'Resize the list' })
      .boundingBox()
    await page.mouse.move(handle!.x + handle!.width / 2, handle!.y + 100)
    await page.mouse.down()
    await page.mouse.move(0, handle!.y + 100, { steps: 5 })
    await page.mouse.up()
    const reopen = page.getByRole('button', { name: 'Open sidebar' })
    await expect(reopen).toHaveAttribute('aria-expanded', 'false')
    await expect(reopen).toHaveCSS('color', themeRgb(theme, 'subtle'))
    await expect(reopen.locator('svg')).toHaveCSS(
      'color',
      themeRgb(theme, 'subtle')
    )
    await expect(reopen).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await reopen.focus()
    await page.keyboard.press('Enter')
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await toggle.blur()
    await expect(toggle).toHaveCSS('color', activeColour)
    await expect(toggle).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await toggle.hover()
    await expect(toggle).toHaveCSS(
      'background-color',
      themeRgb(theme, 'highlightLow')
    )
  })

  test('collapses and reopens details without losing the split or keyboard focus', async ({
    page,
  }) => {
    for (const width of [1440, 700]) {
      await page.setViewportSize({ width, height: 900 })
      const toggle = page.getByRole('button', { name: 'Collapse details' })
      const theme = (await page.locator('html').getAttribute('data-theme'))!
      await expect(toggle).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(toggle).toHaveCSS('color', themeRgb(theme, 'text'))
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
      await expect(reopen).toHaveCSS('color', themeRgb(theme, 'subtle'))
      await expect(reopen).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(reopen).toHaveCSS('outline-style', 'solid')
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
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.setViewportSize({ width: 3000, height: 1000 })
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

    for (const name of ['Metrics', 'Home', 'Logs', 'Traces']) {
      await page
        .getByRole('navigation', { name: 'Primary' })
        .getByRole('link', {
          name: new RegExp(`^${name}(?: \\[|$)`),
        })
        .click()
      if (name === 'Logs') {
        await expect(navigator).toBeHidden()
        await expect(detail).toBeVisible()
        expect((await detail.boundingBox())!.width).toBeCloseTo(detailWidth, 0)
      } else {
        await expectWidths()
      }
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
      .getByRole('link', { name: /^Metrics(?: \[|$)/ })
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
          .dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
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

  test('uses navigation tabs with a primary selected icon, Medium selection and plain bracketed counts', async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    const navigation = page.getByRole('navigation', { name: 'Primary' })
    await navigation.evaluate(async element => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .map(animation => animation.finished)
      )
    })
    const tabs = await navigation.evaluate(element => {
      const reference = element.cloneNode(true) as HTMLElement
      document.body.append(reference)
      const primary = document.createElement('span')
      primary.className = 'text-primary'
      reference.append(primary)
      const primaryColour = getComputedStyle(primary).color
      primary.style.backgroundColor = 'var(--item-selected-bg)'
      const selectedBackground = getComputedStyle(primary).backgroundColor
      const referenceLinks = reference.querySelectorAll('a')
      const centreY = (element: Element) => {
        const bounds = element.getBoundingClientRect()
        return bounds.top + bounds.height / 2
      }
      try {
        return [...element.querySelectorAll('a')].map((link, index) => {
          const style = getComputedStyle(link)
          const plainStyle = getComputedStyle(referenceLinks[index])
          const counter = link.querySelector('.primary-navigation__count')!
          const counterStyle = getComputedStyle(counter)
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
            backgroundMatchesSelection:
              style.backgroundColor ===
              (selected ? selectedBackground : 'rgba(0, 0, 0, 0)'),
            noUnderline: getComputedStyle(link, '::before').display === 'none',
            plainCounter:
              counterStyle.color === style.color &&
              counterStyle.fontSize === '14px' &&
              counterStyle.fontWeight === '400' &&
              counterStyle.backgroundColor === 'rgba(0, 0, 0, 0)' &&
              counterStyle.borderWidth === '0px' &&
              /^Stored (services|traces|metrics|logs): \[0\]$/.test(
                counter.textContent?.trim() ?? ''
              ),
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
        backgroundMatchesSelection: true,
        noUnderline: true,
        plainCounter: true,
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
        .getByRole('button', { name: /^Sort by:/ })
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
    const trigger = controls.getByRole('button', { name: /^Sort by:/ })
    await expect(trigger).toHaveText('Start Time')
    await expect(trigger).toHaveAccessibleName('Sort by: Start Time')
    await expect(trigger).toHaveAttribute('data-tip', 'Sort by: Start Time')
    await expect(trigger).toHaveClass(/header-icon-button/)
    expect((await trigger.boundingBox())!.width).toBeGreaterThan(28)
    await expect(trigger).toHaveCSS('height', '32px')
    await expect(controls).toHaveCSS('box-shadow', 'none')
    await expect(trigger).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(trigger).toHaveCSS('font-size', '14px')
    const spacing = await controls.evaluate(element => {
      const target = element.querySelector('button[popovertarget]')!
      const button = target.getBoundingClientRect()
      const arrow = element
        .querySelector('button[title]')!
        .getBoundingClientRect()
      return {
        centreOffset:
          arrow.top + arrow.height / 2 - button.top - button.height / 2,
        spacer: arrow.left - button.right,
      }
    })
    expect(Math.abs(spacing.centreOffset)).toBeLessThan(1)
    expect(spacing.spacer).toBeCloseTo(2, 0)
    const directionIcon = controls.locator('button[title] svg')
    await expect(controls.locator('button[title]')).toHaveClass(
      /header-icon-button/
    )
    await expect(controls.locator('button[title]')).toHaveCSS('width', '32px')
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
    const popup = page.getByRole('group', { name: 'Sort by', exact: true })
    await popup.getByRole('button', { name: 'Duration', exact: true }).click()
    await expect(trigger).toHaveText('Duration')
    await expect(trigger).toHaveAccessibleName('Sort by: Duration')
    await expect(trigger).toHaveAttribute('data-tip', 'Sort by: Duration')
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

  test('keeps labelled sort targets and the direction control reachable in narrow drawers', async ({
    page,
  }) => {
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      for (const [signal, target] of [
        ['traces', 'Root Span Name'],
        ['metrics', 'Timeseries Count'],
      ]) {
        await page.evaluate(() =>
          localStorage.setItem('signal-drawer:open', 'true')
        )
        await page.goto(`/${signal}`)
        const trigger = page.getByRole('button', { name: /^Sort by:/ })
        await trigger.click()
        await page
          .getByRole('group', { name: 'Sort by', exact: true })
          .getByRole('button', { name: target, exact: true })
          .click()
        await expect(trigger).toHaveAccessibleName(`Sort by: ${target}`)
        await expect(trigger.getByText(target, { exact: true })).toBeVisible()
        const direction = page.getByRole('button', {
          name: /^Sort (ascending|descending)$/,
        })
        await expect(direction).toBeVisible()
        const triggerBox = (await trigger.boundingBox())!
        const directionBox = (await direction.boundingBox())!
        expect(directionBox.x).toBeGreaterThanOrEqual(
          triggerBox.x + triggerBox.width
        )
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth
          )
        ).toBe(true)
      }
    }
  })

  for (const width of [1440, 390]) {
    for (const signal of ['traces', 'metrics', 'logs']) {
      test(`keeps search drafts and controls accessible above ${signal} drawers at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 900 })
        await page.evaluate(() =>
          localStorage.setItem('signal-drawer:open', 'true')
        )
        await page.goto(`/${signal}`)
        const editor = page.locator('.cm-content[contenteditable="true"]')
        await expect(editor).toHaveCSS(
          'font-family',
          /Atkinson Hyperlegible Next/
        )
        await expect(editor.locator('.cm-placeholder')).toHaveCSS(
          'font-family',
          /Atkinson Hyperlegible Next/
        )
        await editor.pressSequentially('service.name = "checkout"')
        const sidebar = page.getByRole('button', {
          name: /^(Open|Collapse) sidebar$/,
        })
        if (signal === 'logs') await expect(sidebar).toHaveCount(0)
        else await expect(sidebar).toBeVisible()
        await expect(editor).toBeVisible()
        await expect(editor).toHaveText('service.name = "checkout"')
        await expect(
          page
            .getByRole('banner', { name: 'Workspace navigation and search' })
            .getByRole('button', { name: /^Sort by:/ })
        ).toHaveCount(0)
        await expect(
          page.getByRole('button', { name: /^Change time range/ })
        ).toHaveText('')
        await expect(
          page.getByRole('button', { name: 'Refresh', exact: true })
        ).toBeVisible()
        const searchBar = page.getByRole('search', { name: 'Workspace search' })
        await expect(searchBar).not.toHaveCSS(
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
      })
    }
  }

  for (const width of [1440, 390]) {
    test(`keeps workspace tooltips inward, visible and unclipped at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 })
      await page.goto('/traces')
      const header = page.getByRole('toolbar', {
        name: 'Workspace controls',
      })
      const editor = header.locator('.cm-content[contenteditable="true"]')
      const draft = `service.name = "${'long-service-name-'.repeat(30)}"`
      await editor.fill(draft)
      await expect(editor).toHaveText(draft)
      const controls = [
        { name: /^(Open|Collapse) sidebar$/, direction: 'right' },
        { name: /^(Open|Collapse) details$/, direction: 'left' },
        {
          name: /^Season:/,
          direction: width < 768 ? 'bottom' : 'left',
          themeInk: true,
        },
        { name: /theme active$/, direction: 'left' },
        { name: /^Refresh$/, direction: 'bottom' },
        { name: /^Clear search$/, direction: 'bottom' },
        { name: /^Search query help$/, direction: 'bottom' },
        { name: /^Change time range/, direction: 'bottom' },
      ]
      for (const { name, direction, themeInk } of controls) {
        const button = header.getByRole('button', { name })
        await expect(button).toHaveAttribute('data-tip', /\S/)
        await button.hover()
        await expect
          .poll(
            () =>
              button.evaluate(
                element => getComputedStyle(element, '::before').opacity
              ),
            { message: `${name} tooltip at ${width}px` }
          )
          .toBe('1')
        const bounds = await button.evaluate(element => {
          const trigger = element.getBoundingClientRect()
          const host = getComputedStyle(element)
          const style = getComputedStyle(element, '::before')
          const transform = new DOMMatrixReadOnly(style.transform)
          const width = parseFloat(style.width)
          const height = parseFloat(style.height)
          const left =
            trigger.left +
            parseFloat(host.borderLeftWidth) +
            (style.left === 'auto'
              ? element.clientWidth - parseFloat(style.right) - width
              : parseFloat(style.left)) +
            transform.m41
          const top =
            trigger.top +
            parseFloat(host.borderTopWidth) +
            parseFloat(style.top) +
            transform.m42
          const bubble = {
            left,
            top,
            right: left + width,
            bottom: top + height,
          }
          const clippedBy: string[] = []
          for (
            let parent = element.parentElement;
            parent;
            parent = parent.parentElement
          ) {
            const css = getComputedStyle(parent)
            const box = parent.getBoundingClientRect()
            if (
              (/(hidden|clip|auto|scroll)/.test(css.overflowX) &&
                (bubble.left < box.left - 1 || bubble.right > box.right + 1)) ||
              (/(hidden|clip|auto|scroll)/.test(css.overflowY) &&
                (bubble.top < box.top - 1 || bubble.bottom > box.bottom + 1))
            ) {
              clippedBy.push(parent.className)
            }
          }
          const probe = document.createElement('span')
          probe.style.cssText =
            'color:var(--color-base-content);background:var(--color-base-200)'
          element.append(probe)
          const reference = getComputedStyle(probe)
          const palette = {
            panel: style.backgroundColor === reference.backgroundColor,
            text: style.color === reference.color,
            weight: style.fontWeight,
            size: style.fontSize,
          }
          probe.remove()
          return {
            bubble,
            trigger: trigger.toJSON(),
            clippedBy,
            palette,
          }
        })
        expect(bounds.palette).toEqual({
          panel: true,
          text: true,
          weight: '400',
          size: '14px',
        })
        expect(bounds.clippedBy, `${name} at ${width}px`).toEqual([])
        expect(bounds.bubble.left).toBeGreaterThanOrEqual(0)
        expect(bounds.bubble.right).toBeLessThanOrEqual(width)
        expect(bounds.bubble.top).toBeGreaterThanOrEqual(0)
        expect(bounds.bubble.bottom).toBeLessThanOrEqual(900)
        if (themeInk) {
          await expect(button).toHaveAttribute('data-tip', 'Autumn')
        }
        if (direction === 'right')
          expect(bounds.bubble.left).toBeGreaterThan(bounds.trigger.right)
        else if (direction === 'left')
          expect(bounds.bubble.right).toBeLessThan(bounds.trigger.left)
        else expect(bounds.bubble.top).toBeGreaterThan(bounds.trigger.bottom)
      }
      const help = header.getByRole('button', { name: 'Search query help' })
      await page.mouse.move(0, 899)
      await help.focus()
      await page.keyboard.press('Tab')
      await page.keyboard.press('Shift+Tab')
      await expect(help).toBeFocused()
      await expect
        .poll(() =>
          help.evaluate(
            element => getComputedStyle(element, '::before').opacity
          )
        )
        .toBe('1')
      await page.keyboard.press('Escape')
      await expect
        .poll(() =>
          help.evaluate(
            element => getComputedStyle(element, '::before').opacity
          )
        )
        .toBe('0')
      await expect(editor).toHaveText(draft)
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth
        )
      ).toBe(true)
    })
  }

  test('gives workspace search one field boundary and one focus ring at desktop and mobile widths', async ({
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
      const background = await bar.evaluate(
        element => getComputedStyle(element).backgroundColor
      )
      await expect(bar).toHaveCSS('background-color', background)
      await expect(bar).toHaveCSS('border-width', '1px')
      await expect(bar).toHaveCSS('outline-style', 'none')
      const editor = bar.locator('.cm-content[contenteditable="true"]')
      await editor.pressSequentially('service.name = "checkout"')
      await expect(bar).toHaveCSS('background-color', background)
      await expect(surface).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
      await expect(surface).toHaveCSS('outline-style', 'none')
      await expect(bar).toHaveCSS('outline-style', 'solid')
      await expect(bar.locator('.typed-field')).toHaveCSS('border-width', '0px')
      await expect(bar.locator('.typed-field')).toHaveCSS(
        'outline-style',
        'none'
      )
      await expect(editor).toHaveText('service.name = "checkout"')
      await bar.getByRole('button', { name: 'Clear search' }).click()
      const bounds = (await bar.boundingBox())!
      const fieldBounds = (await bar.locator('.typed-field').boundingBox())!
      expect(fieldBounds.y).toBeGreaterThanOrEqual(bounds.y)
      expect(fieldBounds.y + fieldBounds.height).toBeLessThanOrEqual(
        bounds.y + bounds.height
      )
      await expect(editor).toBeVisible()
      await expect(editor.locator('.cm-placeholder')).toBeVisible()
      await page.getByRole('button', { name: /^Season:/ }).focus()
      await expect(bar).toHaveCSS('outline-style', 'none')
    }
  })

  for (const signal of ['logs', 'traces', 'metrics']) {
    test(`centres ${signal} query help and restores focus on close`, async ({
      page,
    }) => {
      for (const width of [1440, 390]) {
        await page.setViewportSize({ width, height: 900 })
        await page.goto(`/${signal}`)
        const help = page.getByRole('button', { name: 'Search query help' })
        await help.click()
        const dialog = page.getByRole('dialog', { name: `Search ${signal}` })
        await expect(dialog).toBeVisible()
        const bounds = (await dialog.boundingBox())!
        expect(bounds.width).toBeCloseTo(Math.min(768, width - 32), 0)
        expect(bounds.x + bounds.width / 2).toBeCloseTo(width / 2, 0)
        expect(bounds.y + bounds.height / 2).toBeCloseTo(450, 0)
        await expect(dialog.locator('table')).toBeVisible()
        expect(
          await dialog.evaluate(
            element => element.scrollWidth <= element.clientWidth
          )
        ).toBe(true)
        await page.keyboard.press('Escape')
        await expect(dialog).not.toBeVisible()
        await expect(help).toBeFocused()
        await help.click()
        await dialog.getByRole('button', { name: 'Close search help' }).click()
        await expect(dialog).not.toBeVisible()
        await expect(help).toBeFocused()
      }
    })
  }

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

  test('uses full-strength preset labels and the shared selection highlight', async ({
    page,
  }) => {
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()

    const presetColors = await page
      .locator('.preset-time-ranges')
      .evaluate(element => {
        const active = element.querySelector('[aria-pressed="true"]')
        const inactive = element.querySelector('[aria-pressed="false"]')
        if (!active || !inactive) throw new Error('Preset state is missing')

        const probe = document.createElement('span')
        document.body.append(probe)
        probe.style.backgroundColor = 'var(--item-selected-bg)'
        const selected = getComputedStyle(probe).backgroundColor
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
          highlight: getComputedStyle(element, '::before').backgroundColor,
          background: getComputedStyle(active).backgroundColor,
          selected,
          baseContent,
        }
      })
    expect(presetColors.inactive).toBe(presetColors.navInactive)
    expect(presetColors.inactive).toBe(presetColors.baseContent)
    expect(presetColors.active).toBe(presetColors.baseContent)
    expect(presetColors.highlight).toBe('rgba(0, 0, 0, 0)')
    expect(presetColors.background).toBe(presetColors.selected)
  })

  test('balances preset label spacing and outer edges', async ({ page }) => {
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()

    const popup = page.locator('[popover]:popover-open')
    const alignment = await popup.evaluate(element => {
      const presets = element.querySelectorAll(
        '.preset-time-ranges .header-tab'
      )
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
      expect(tab.width).toBeCloseTo(alignment.tabs[0].width, 0)
    }
    expect(alignment.tabs).toHaveLength(8)
  })

  for (const width of [1920, 900, 390, 320]) {
    test(`time-range menu has room for dates and uses Next throughout at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 1000 })
      await page.goto('/traces')
      await page.getByRole('button', { name: /Change time range/i }).click()
      const popup = page.locator('.datetime-popover:popover-open')
      await expect(popup).toBeVisible()
      await page.evaluate(() => document.fonts.ready)
      const box = (await popup.boundingBox())!
      expect(box.x).toBeGreaterThanOrEqual(8)
      expect(box.x + box.width).toBeLessThanOrEqual(width - 8)
      if (width >= 900) expect(box.width).toBe(512)
      await popup.getByText('Timezone:', { exact: true }).click()
      const toggle = popup.getByRole('button', {
        name: /^Filter timezones/,
      })
      const filter = popup.getByRole('searchbox', { name: 'Filter timezones' })
      await expect(filter).toBeHidden()
      await toggle.click()
      await expect(filter).toBeFocused()
      await filter.fill('los angeles')
      await filter.press('Escape')
      await expect(popup).toBeVisible()
      await expect(filter).toBeHidden()
      await expect(toggle).toBeFocused()
      await expect(toggle).toHaveAccessibleName('Filter timezones (active)')
      await toggle.click()
      await expect(filter).toHaveValue('los angeles')
      const layout = await popup.evaluate(element => ({
        mono: [...element.querySelectorAll('*')].flatMap(child =>
          getComputedStyle(child).fontFamily.includes(
            'Atkinson Hyperlegible Mono'
          )
            ? [child.className]
            : []
        ),
        rows: [...element.querySelectorAll('.range-editor-row')].map(row => ({
          width: row.clientWidth,
          scroll: row.scrollWidth,
        })),
        clippedInputs: [...element.querySelectorAll('input')].flatMap(input =>
          input.getClientRects().length > 0 &&
          input.scrollWidth > input.clientWidth
            ? [input.getAttribute('aria-label')]
            : []
        ),
      }))
      expect(layout.mono).toEqual([])
      expect(layout.clippedInputs).toEqual([])
      for (const row of layout.rows)
        expect(row.scroll).toBeLessThanOrEqual(row.width)
      await expect(popup).not.toHaveCSS('box-shadow', 'none')
      await page.keyboard.press('Escape')
    })
  }

  for (const signal of ['traces', 'logs', 'metrics']) {
    test(`${signal} service filter uses the shared header icon without clearing on collapse`, async ({
      page,
    }) => {
      await page.goto(`/${signal}`)
      await page.getByRole('button', { name: 'Filters', exact: true }).click()
      const popup = page.getByRole('group', {
        name: 'Filters options',
        exact: true,
      })
      const toggle = popup.getByRole('button', {
        name: /^Find services/,
      })
      const filter = popup.getByRole('searchbox', { name: 'Find services' })
      await expect(filter).toBeHidden()
      await toggle.focus()
      await toggle.press('Enter')
      await expect(filter).toBeFocused()
      await expect(filter).toHaveCSS(
        'font-family',
        /Atkinson Hyperlegible Next/
      )
      await filter.fill('not-a-service')
      await filter.press('Escape')
      await expect(popup).toBeVisible()
      await expect(filter).toBeHidden()
      await expect(toggle).toBeFocused()
      await expect(toggle).toHaveAccessibleName('Find services (active)')
      await toggle.press('Space')
      await expect(filter).toHaveValue('not-a-service')
      await filter.fill('')
      await filter.press('Escape')
      await expect(toggle).toHaveAccessibleName('Find services')
      await toggle.press('Escape')
      await expect(popup).toBeHidden()
    })
  }

  test('keeps custom range controls circular with one shared field focus ring', async ({
    page,
  }) => {
    await page.goto('/traces')
    await page.getByRole('button', { name: /Change time range/i }).click()

    const startDate = page.getByRole('textbox', { name: 'Start', exact: true })
    const startCalendar = page.getByRole('button', {
      name: 'Choose start date',
    })
    // The start calendar is expanded when the range editor opens.
    await expect(startCalendar).toHaveAttribute('aria-expanded', 'true')
    await startCalendar.click()
    await expect(startCalendar).toHaveAttribute('aria-expanded', 'false')

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
    expect(focusTreatment.boxShadow).toBe('none')
    expect(focusTreatment.outlineStyle).toBe('solid')
    await expect(startDate).toHaveCSS('outline-style', 'none')

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
      iconColor: getComputedStyle(element.querySelector('svg')!).color,
      rowBoxShadow: getComputedStyle(element.parentElement!).boxShadow,
    }))
    const primary = await page
      .locator('.primary-navigation .tab-active svg')
      .evaluate(element => getComputedStyle(element).color)
    expect(activeTreatment.iconColor).toBe(primary)
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
    await page
      .getByRole('button', { name: 'Filter timezones', exact: true })
      .click()

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
    expect(timezoneFilterBounds!.x).toBeCloseTo(
      timezoneSectionBounds!.x + 12,
      0
    )
    expect(timezoneFilterBounds!.x + timezoneFilterBounds!.width).toBeCloseTo(
      timezoneSectionBounds!.x + timezoneSectionBounds!.width - 12,
      0
    )
    const timezoneLeftInset = timezoneNameBounds!.x - timezoneListBounds!.x
    const timezoneRightInset =
      timezoneListBounds!.x +
      timezoneListBounds!.width -
      timezoneBadgeBounds!.x -
      timezoneBadgeBounds!.width
    expect(timezoneLeftInset).toBeCloseTo(8, 0)
    // The shared choice row reserves a 16px selection mark and an 8px gap.
    expect(timezoneRightInset).toBeCloseTo(timezoneLeftInset + 24, 0)
  })
})
