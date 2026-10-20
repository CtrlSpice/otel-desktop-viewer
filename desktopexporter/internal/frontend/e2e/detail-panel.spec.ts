import { expect, test } from '@playwright/test'
import type { JsonLogData, JsonTraceData } from '../src/types/wire-types'

const traceID = '12345678901234567890123456789012'
const spanID = '1234567890123456'
const trace: JsonTraceData = {
  traceID,
  traceStart: '1700000000000000000',
  resources: {
    0: {
      attributes: [
        {
          key: 'service.name',
          value: { kind: 'string', value: 'checkout-api' },
        },
      ],
      droppedAttributesCount: 0,
    },
  },
  scopes: {
    0: {
      name: 'test-scope',
      version: '1.2',
      attributes: [],
      droppedAttributesCount: 0,
    },
  },
  unplacedSpanCount: 0,
  spans: [
    {
      depth: 0,
      matched: true,
      spanData: {
        spanID,
        parentSpanID: null,
        traceState: '',
        flags: 0,
        name: 'detail target',
        kindCode: 1,
        kind: 'Internal',
        start: '0',
        dur: '1000000000',
        r: 0,
        s: 0,
        attributes: [],
        links: [],
        events: [
          {
            name: 'target event',
            timestamp: '1700000000500000000',
            attributes: [],
            droppedAttributesCount: 0,
          },
        ],
        droppedAttributesCount: 0,
        droppedEventsCount: 0,
        droppedLinksCount: 0,
        statusCodeValue: 0,
        statusCode: 'Unset',
        statusMessage: '',
      },
    },
  ],
}

for (const kind of ['event', 'log'] as const) {
  test(`${kind} marker reveals collapsed Events without reopening manually closed Span`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    const linked = structuredClone(trace)
    linked.spans[0]!.spanData.links.push({
      traceID,
      spanID,
      traceState: '',
      flags: 0,
      attributes: [],
      droppedAttributesCount: 0,
    })
    const logRef = '00000000-0000-0000-0000-000000000001'
    await page.route('**/rpc', async route => {
      const { id, method } = route.request().postDataJSON()
      const result =
        method === 'getTraceView'
          ? linked
          : method === 'searchTraceSummaries'
            ? [
                {
                  traceID,
                  hasRootSpan: true,
                  rootSpan: {
                    serviceName: 'checkout-api',
                    name: 'detail target',
                  },
                  startTime: trace.traceStart,
                  durationNs: '1000000000',
                  spanCount: 1,
                  errorCount: 0,
                },
              ]
            : method === 'getTraceLogSummaries'
              ? [
                  {
                    logRef,
                    spanID,
                    timestamp: '1700000000800000000',
                    severityText: 'WARN',
                    severityNumber: 13,
                    serviceName: 'checkout-api',
                    bodyPreview: 'Selected warning',
                    eventName: '',
                  },
                ]
              : method === 'getStats'
                ? {
                    storage: { sizeBytes: 0, maxSizeBytes: 0 },
                    traces: {
                      traceCount: 1,
                      spanCount: 1,
                      serviceCount: 1,
                      errorCount: 0,
                      lastReceived: null,
                    },
                    logs: { logCount: 1, errorCount: 0, lastReceived: null },
                    metrics: {
                      metricCount: 0,
                      dataPointCount: 0,
                      lastReceived: null,
                    },
                    rejections: [],
                  }
                : []
      await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
    })
    await page.goto(`/traces/${traceID}?span=${spanID}`)
    await page.evaluate(() => {
      const states: boolean[] = []
      Object.assign(window, { revealScrollStates: states })
      const scroll = Element.prototype.scrollIntoView
      Element.prototype.scrollIntoView = function (options) {
        if (
          this.id.startsWith('span-event-') ||
          this.id.startsWith('span-log-')
        ) {
          states.push(this.closest('details')?.open ?? false)
        }
        scroll.call(this, options)
      }
    })
    const stack = page.getByRole('region', {
      name: 'Span details',
      exact: true,
    })
    const sections = stack.locator('.detail-bubble__disclosure')
    const span = sections.nth(0)
    const events = sections.nth(1)
    const links = sections.nth(2)
    await span.locator(':scope > summary').press('Enter')
    await expect(span).not.toHaveAttribute('open')
    for (let attempt = 0; attempt < 2; attempt++) {
      await events.locator(':scope > summary').click()
      await expect(events).not.toHaveAttribute('open')
      await page
        .getByRole('button', { name: `Select 1 ${kind}`, exact: true })
        .click()
      await expect(events).toHaveAttribute('open')
      await expect(span).not.toHaveAttribute('open')
      await expect(links).toHaveAttribute('open')
      const target =
        kind === 'event'
          ? events.getByRole('table', {
              name: 'Event target event',
              exact: true,
            })
          : events.locator(`#span-log-${logRef}`)
      await expect(target).toBeVisible()
      await expect(target).toBeInViewport()
      const scrollStates = await page.evaluate(
        () =>
          (window as Window & { revealScrollStates?: boolean[] })
            .revealScrollStates ?? []
      )
      expect(scrollStates.length).toBeGreaterThan(0)
      expect(scrollStates.every(Boolean)).toBe(true)
      // The native disclosure's queued event must not undo the stack reveal.
      await expect(events.locator(':scope > summary')).not.toHaveAttribute(
        'aria-disabled',
        'true'
      )
    }
  })
}

test('detail dividers collapse, reopen and redistribute space across collapsed neighbours', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'true')
  )
  const linked = structuredClone(trace)
  linked.spans[0]!.spanData.links.push({
    traceID,
    spanID,
    traceState: '',
    flags: 0,
    attributes: [],
    droppedAttributesCount: 0,
  })
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result:
          method === 'getTraceView'
            ? linked
            : method === 'searchTraceSummaries'
              ? [
                  {
                    traceID,
                    hasRootSpan: true,
                    rootSpan: {
                      serviceName: 'checkout-api',
                      name: 'detail target',
                    },
                    startTime: trace.traceStart,
                    durationNs: '1000000000',
                    spanCount: 1,
                    errorCount: 0,
                  },
                ]
              : method === 'getStats'
                ? {
                    storage: { sizeBytes: 0, maxSizeBytes: 0 },
                    traces: {
                      traceCount: 1,
                      spanCount: 1,
                      serviceCount: 1,
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
                : [],
      },
    })
  })
  await page.goto(`/traces/${traceID}?span=${spanID}`)
  const stack = page.getByRole('region', { name: 'Span details', exact: true })
  const sections = stack.locator('.detail-bubble--resizable')
  const span = sections.nth(0)
  const events = sections.nth(1)
  const links = sections.nth(2)
  const top = stack.getByRole('separator', {
    name: 'Resize Span and Events sections',
  })
  const bottom = stack.getByRole('separator', {
    name: 'Resize Events and Links sections',
  })
  const open = (section: typeof span) => section.locator(':scope > details')
  const height = (section: typeof span) =>
    section.evaluate(element => element.getBoundingClientRect().height)
  const heading = (section: typeof span) =>
    section.locator(':scope > details > summary')
  async function drag(handle: typeof top, delta: number, reverse = false) {
    const rect = (await handle.boundingBox())!
    const x = rect.x + rect.width / 2
    const y = rect.y + rect.height / 2
    await page.mouse.move(x, y)
    await page.mouse.down()
    await page.mouse.move(x, y + delta, { steps: 8 })
    if (reverse) await page.mouse.move(x, y, { steps: 8 })
    await page.mouse.up()
  }
  async function anchored() {
    const bounds = (await stack.boundingBox())!
    const first = (await span.boundingBox())!
    const last = (await links.boundingBox())!
    expect(first.y).toBeCloseTo(bounds.y, 0)
    expect(last.y + last.height).toBeCloseTo(bounds.y + bounds.height, 0)
    expect(
      await stack.locator('.detail-bubble__disclosure[open]').count()
    ).toBeGreaterThan(0)
  }
  await expect(sections).toHaveCount(3)
  await anchored()
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  // Events must track the pointer one-for-one while pushing Links down,
  // including a reversal before the previous release animation settles.
  for (const delta of [80, -80, 80, -80]) {
    const before = (await bottom.boundingBox())!
    const beforeHeights = await Promise.all([
      height(span),
      height(events),
      height(links),
    ])
    const x = before.x + before.width / 2
    const y = before.y + before.height / 2
    await page.mouse.move(x, y)
    await page.mouse.down()
    await page.mouse.move(x, y + delta, { steps: 5 })
    expect((await bottom.boundingBox())!.y).toBeCloseTo(before.y + delta, 0)
    expect(await height(span)).toBeCloseTo(beforeHeights[0]!, 0)
    expect(await height(events)).toBeCloseTo(beforeHeights[1]! + delta, 0)
    expect(await height(links)).toBeCloseTo(beforeHeights[2]! - delta, 0)
    await page.mouse.up()
  }
  await page.emulateMedia({ reducedMotion: 'reduce' })
  // One continuous Links drag exhausts Events, then takes space from Span.
  // Reversing that same gesture restores both donors without a second grab.
  const cascadeHeights = await Promise.all([
    height(span),
    height(events),
    height(links),
  ])
  const cascadeHandle = (await bottom.boundingBox())!
  const cascadeX = cascadeHandle.x + cascadeHandle.width / 2
  const cascadeY = cascadeHandle.y + cascadeHandle.height / 2
  await page.mouse.move(cascadeX, cascadeY)
  await page.mouse.down()
  await page.mouse.move(cascadeX, cascadeY - cascadeHeights[1]! - 180, {
    steps: 12,
  })
  await expect(open(events)).not.toHaveAttribute('open')
  expect(await height(span)).toBeLessThan(cascadeHeights[0]! - 80)
  expect(await height(links)).toBeGreaterThan(
    cascadeHeights[2]! + cascadeHeights[1]!
  )
  await anchored()
  await page.mouse.move(cascadeX, cascadeY, { steps: 12 })
  await page.mouse.up()
  await expect(open(events)).toHaveAttribute('open')
  for (const [index, section] of [span, events, links].entries()) {
    expect(await height(section)).toBeCloseTo(cascadeHeights[index]!, 0)
  }
  // Growing Span first compresses Events through its resistance region.
  // Links must not also shrink while Events still has content space.
  const lowerHeight = await height(links)
  const eventHeight = await height(events)
  const topRect = (await top.boundingBox())!
  await page.mouse.move(
    topRect.x + topRect.width / 2,
    topRect.y + topRect.height / 2
  )
  await page.mouse.down()
  await page.mouse.move(
    topRect.x + topRect.width / 2,
    topRect.y + topRect.height / 2 + eventHeight,
    { steps: 8 }
  )
  expect(await height(events)).toBeGreaterThan(40)
  expect(await height(links)).toBeCloseTo(lowerHeight, 0)
  await page.mouse.up()
  await expect(open(events)).not.toHaveAttribute('open')
  expect(await height(links)).toBeCloseTo(lowerHeight, 0)
  await heading(events).click()
  const initial = await height(span)
  await drag(top, -initial - 80, true)
  await expect(open(span)).toHaveAttribute('open')
  expect(await height(span)).toBeCloseTo(initial, 0)
  await drag(top, -initial - 80)
  await expect(open(span)).not.toHaveAttribute('open')
  await anchored()
  await drag(top, 120)
  await expect(open(span)).toHaveAttribute('open')
  expect(await height(span)).toBeGreaterThan(140)

  await top.focus()
  await top.press('End')
  await expect(open(links)).not.toHaveAttribute('open')
  await expect(open(events)).not.toHaveAttribute('open')
  await anchored()
  expect(await height(span)).toBeGreaterThan(700)
  await heading(span).focus()
  await heading(span).press('Enter')
  await expect(open(span)).not.toHaveAttribute('open')
  await expect(open(events)).toHaveAttribute('open')
  await expect(open(links)).not.toHaveAttribute('open')
  expect(await height(events)).toBeGreaterThan(700)
  await anchored()
  await heading(events).click()
  await expect(open(events)).not.toHaveAttribute('open')
  await expect(open(links)).toHaveAttribute('open')
  await heading(links).click()
  await expect(open(links)).not.toHaveAttribute('open')
  await expect(open(events)).toHaveAttribute('open')
  await top.focus()
  await top.press('End')
  await expect(open(span)).toHaveAttribute('open')
  await expect(open(events)).not.toHaveAttribute('open')

  // Both neighbours of this divider are collapsed; borrow from Span above them.
  await expect(bottom).toHaveAttribute('aria-disabled', 'false')
  await drag(bottom, -120)
  await expect(open(links)).toHaveAttribute('open')
  await expect(open(events)).not.toHaveAttribute('open')
  await anchored()
  await drag(top, -120)
  await expect(open(events)).toHaveAttribute('open')
  await anchored()
  await bottom.focus()
  await bottom.press('End')
  await expect(open(links)).not.toHaveAttribute('open')
  await bottom.press('ArrowUp')
  await expect(open(links)).toHaveAttribute('open')
  await anchored()
  await page.setViewportSize({ width: 1440, height: 650 })
  await anchored()
  const sizes = await page.evaluate(() =>
    localStorage.getItem('trace-detail-section-sizes')
  )
  await page.reload()
  await expect(sections).toHaveCount(3)
  expect(
    await page.evaluate(() =>
      localStorage.getItem('trace-detail-section-sizes')
    )
  ).toBe(sizes)
  await anchored()
})

test('associated logs reuse summary cards and open the full log without losing the range', async ({
  page,
}) => {
  const logRef = '00000000-0000-0000-0000-000000000001'
  const log: JsonLogData = {
    logRef,
    timestamp: '1700000000500000000',
    observedTimestamp: '1700000000500000000',
    traceID,
    spanID,
    severityText: 'WARN',
    severityNumber: 13,
    body: { kind: 'string', value: 'Associated warning' },
    resource: trace.resources[0]!,
    scope: trace.scopes[0]!,
    attributes: Array.from({ length: 20 }, (_, index) => ({
      key: `sample.${index}`,
      value: {
        kind: 'string' as const,
        value: `Retained log attribute ${index}`,
      },
    })),
    droppedAttributesCount: 0,
    flags: 0,
    eventName: 'order.delayed',
  }
  const summary = {
    logRef,
    timestamp: log.timestamp,
    spanID,
    severityText: log.severityText,
    severityNumber: log.severityNumber,
    serviceName: 'checkout-api',
    bodyPreview: 'Associated warning',
    eventName: log.eventName,
  }
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'true')
  )
  function fixture(method: string) {
    if (method === 'getTraceView')
      return {
        ...trace,
        spans: trace.spans.map(span => ({
          ...span,
          spanData: {
            ...span.spanData,
            links: [
              {
                traceID: 'linked-trace',
                spanID: 'linked-span',
                traceState: '',
                attributes: [],
                droppedAttributesCount: 0,
                flags: 0,
                summary: {
                  target: { name: 'consume order', serviceName: 'worker' },
                },
              },
            ],
          },
        })),
      }
    if (method === 'getTraceLogSummaries' || method === 'searchLogSummaries')
      return [summary]
    if (method === 'searchLogPage') return { total: 1, logs: [summary] }
    if (method === 'getLogErrorNavigation')
      return { count: 0, index: null, previous: null, next: null }
    if (method === 'getLog') return log
    if (method === 'searchTraceSummaries')
      return [
        {
          traceID,
          hasRootSpan: true,
          rootSpan: { serviceName: 'checkout-api', name: 'detail target' },
          startTime: trace.traceStart,
          durationNs: '1000000000',
          spanCount: 1,
          errorCount: 0,
        },
      ]
    if (method === 'getStats')
      return {
        storage: { sizeBytes: 0, maxSizeBytes: 0 },
        traces: {
          traceCount: 1,
          spanCount: 1,
          serviceCount: 1,
          errorCount: 0,
          lastReceived: null,
        },
        logs: { logCount: 1, errorCount: 0, lastReceived: null },
        metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
        rejections: [],
      }
    return []
  }
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result: fixture(request.method),
      },
    })
  })
  const tracePath = `/traces/${traceID}?span=${spanID}&log=${logRef}&start=0&end=2000000000000`
  await page.goto(tracePath)
  const associated = page.getByRole('region', { name: 'Log summary' })
  const card = associated.getByRole('button')
  await expect(card).toBeVisible()
  const selectedRow = page.locator('.waterfall-row[aria-selected="true"]')
  await expect(selectedRow).toHaveCSS('border-radius', '12px')
  await expect(selectedRow).toHaveCSS('overflow', 'visible')
  const mainHeader = page.getByRole('region', {
    name: 'Trace waterfall',
    exact: true,
  })
  const darkest = await page.evaluate(() => {
    const probe = document.createElement('span')
    probe.style.backgroundColor = 'var(--color-base-100)'
    document.body.append(probe)
    const color = getComputedStyle(probe).backgroundColor
    probe.remove()
    return color
  })
  await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
    'background-color',
    darkest
  )
  await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
    'border-width',
    '0px'
  )
  await expect(page.locator('.waterfall-view')).toHaveCSS(
    'background-color',
    darkest
  )
  await expect(mainHeader).toHaveCSS('border-radius', '12px')
  await expect(mainHeader).toHaveCSS(
    'background-color',
    await page
      .locator('.detail-bubble__heading')
      .first()
      .evaluate(element => getComputedStyle(element).backgroundColor)
  )
  await expect(card).toHaveClass(/signal-row/)
  await expect(card).toHaveAttribute('aria-pressed', 'true')
  await expect(card).toContainText('checkout-api')
  await expect(card).toContainText('WARN (13)')
  await expect(card.locator('.signal-row__badge')).toHaveText('WARN (13)')
  await expect(card.locator('.signal-row__title')).toHaveText(
    'checkout-api · Log'
  )
  await expect(card).toContainText('Associated warning')
  const fields = page.getByRole('table', { name: 'Span fields' })
  const key = fields.locator('.detail-property__key').first()
  const subtle = await key.evaluate(element => {
    const probe = document.createElement('span')
    probe.style.color = 'var(--color-subtle)'
    element.append(probe)
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  })
  await expect(key).toHaveCSS('color', subtle)
  await expect(fields.locator('.detail-property__value').first()).not.toHaveCSS(
    'color',
    subtle
  )
  await expect(
    associated.getByText('order.delayed', { exact: true })
  ).toBeVisible()
  await expect(
    card
      .locator('.signal-row__date')
      .getByText('(+500.000 ms)', { exact: true })
  ).toBeVisible()
  await expect(associated.locator('table, details, [data-field]')).toHaveCount(
    0
  )
  const eventCard = page
    .getByRole('region', { name: 'Event target event' })
    .locator('button[aria-expanded]')
  const eventsHeading = page
    .locator('summary.detail-bubble__heading')
    .filter({ has: page.getByText('Events', { exact: true }) })
  await expect(eventsHeading).toContainText('2 events')
  await expect(eventsHeading.locator('svg')).toHaveCount(1)
  await expect(eventCard.locator('.signal-row__title-cluster')).toHaveText(
    /^\s*target event\s*·\s*Span event\s*$/
  )
  await expect(eventCard).toHaveAttribute('aria-expanded', 'false')
  await eventCard.click()
  await expect(
    page.getByRole('table', { name: 'Event target event' })
  ).toBeVisible()
  const linkCard = page.getByRole('button', {
    name: 'consume order (worker) · Link 3 fields',
  })
  await expect(linkCard).toHaveAttribute('aria-expanded', 'false')
  await linkCard.focus()
  await page.keyboard.press('Enter')
  await expect(
    page.getByRole('table', { name: 'Link linked-trace' })
  ).toBeVisible()
  await expect(eventCard).toHaveAttribute('aria-expanded', 'true')
  await expect(
    page.getByRole('link', { name: 'linked-span', exact: true })
  ).toHaveAttribute(
    'href',
    '/traces/linked-trace?start=0&end=2000000000000&span=linked-span'
  )
  await expect(associated.getByRole('link', { name: 'Open log' })).toHaveCount(
    0
  )
  await card.focus()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(`/logs/${logRef}?start=0&end=2000000000000`)
  await expect(page.getByRole('table', { name: 'Log fields' })).toBeVisible()
  const logScroll = page.locator('.log-detail-panel__scroll')
  await logScroll.evaluate(element => {
    element.scrollTop = element.scrollHeight
  })
  await expect
    .poll(() => logScroll.evaluate(element => element.scrollTop))
    .toBeGreaterThan(0)
  await page.getByRole('button', { name: 'Scroll log to top' }).click()
  await expect
    .poll(() => logScroll.evaluate(element => element.scrollTop))
    .toBe(0)
  await expect(page).toHaveURL(`/logs/${logRef}?start=0&end=2000000000000`)
  await page.goBack()
  await expect(page).toHaveURL(tracePath)
  await expect(card).toHaveAttribute('aria-pressed', 'true')
})

test('detail targets reopen the panel with independent native disclosures', async ({
  page,
}) => {
  const pageErrors: string[] = []
  page.on('pageerror', error => pageErrors.push(error.message))
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'false')
  )
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result:
          request.method === 'getTraceView'
            ? trace
            : request.method === 'searchTraceSummaries'
              ? [
                  {
                    traceID,
                    hasRootSpan: true,
                    rootSpan: { serviceName: null, name: 'detail target' },
                    startTime: trace.traceStart,
                    durationNs: '1000000000',
                    spanCount: 1,
                    errorCount: 0,
                  },
                ]
              : request.method === 'getStats'
                ? {
                    storage: { sizeBytes: 0, maxSizeBytes: 0 },
                    traces: {
                      traceCount: 1,
                      spanCount: 1,
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
                : [],
      },
    })
  })
  await page.goto(`/traces/${traceID}?span=${spanID}`)
  const detail = page.locator('#signal-drawer-detail')
  const span = page
    .locator('.waterfall-row')
    .filter({ hasText: 'detail target' })
  await expect(span).toBeVisible()
  // Loading a selected trace must not override the persisted collapsed state.
  await expect(detail).toBeHidden()
  const fields = detail.locator('details.collapse').nth(0)
  const activity = detail.locator('details.collapse').nth(1)
  const links = detail.getByRole('region', { name: 'Links', exact: true })
  const darkestSurface = await page.evaluate(() => {
    const probe = document.createElement('div')
    probe.style.backgroundColor = 'var(--color-base-100)'
    document.body.append(probe)
    const colour = getComputedStyle(probe).backgroundColor
    probe.remove()
    return colour
  })
  await expect(fields).toHaveCSS('background-color', darkestSurface)
  await expect(fields).toHaveCSS('border-top-width', '0px')
  expect(
    await fields.evaluate(element =>
      parseFloat(getComputedStyle(element).borderTopLeftRadius)
    )
  ).toBeGreaterThan(0)
  for (let selection = 0; selection < 2; selection++) {
    await span.click()
    await expect(detail).toBeVisible()
    const views = detail.getByRole('tablist', { name: 'Detail view' })
    await expect(views.getByRole('tab')).toHaveCount(2)
    await expect(
      views.getByRole('tab', { name: 'Properties', exact: true })
    ).toHaveAttribute('aria-selected', 'true')
    await expect(
      views.getByRole('tab', { name: 'JSON', exact: true })
    ).toHaveAttribute('aria-selected', 'false')
    await expect(
      detail.getByRole('tabpanel', { name: 'Properties', exact: true })
    ).toBeVisible()
    await expect(fields.locator(':scope > summary')).toHaveText('Span * root')
    await expect(fields.locator('details')).toHaveCount(0)
    const table = fields.getByRole('table', { name: 'Span fields' })
    const resource = table.locator('[data-field="service.name"]')
    await expect(resource.locator('.detail-property__key')).toHaveText(
      'service.name · string'
    )
    await expect(resource.locator('.detail-property__origin')).toHaveText(
      'resource'
    )
    const scopeName = table
      .getByRole('button', { name: 'Copy name (scope)', exact: true })
      .locator('xpath=ancestor::tr[1]')
    await expect(scopeName.locator('.detail-property__key')).toHaveText(
      'name · string'
    )
    await expect(scopeName.locator('.detail-property__origin')).toHaveText(
      'scope'
    )
    await expect(
      detail.getByRole('region', { name: 'Span details' })
    ).toHaveCSS('padding-left', '0px')
    await expect(table.locator('td').first()).toHaveCSS('padding-left', '0px')
    await expect(fields).toHaveAttribute('open')
    await expect(activity).toHaveAttribute('open')
    await expect(activity.locator('details')).toHaveCount(0)
    await expect(links).toContainText('0 links')
    await expect(
      links.locator('details, summary, .collapse-content')
    ).toHaveCount(0)
    for (const section of [fields, activity, links]) {
      await expect(
        section.locator(':scope > .detail-bubble__heading')
      ).toBeInViewport({
        ratio: 1,
      })
    }
    await expect(fields).toHaveCSS('overflow-y', 'auto')
    await fields.evaluate(element => {
      element.scrollTop = element.scrollHeight
    })
    await expect(fields.locator(':scope > summary')).toBeInViewport({
      ratio: 1,
    })
    await fields.locator(':scope > summary').focus()
    await page.keyboard.press('Enter')
    await expect(fields).not.toHaveAttribute('open')
    await expect(activity).toHaveAttribute('open')
    const extraHeight = await links.evaluate(element => {
      const heading = element.querySelector('.detail-bubble__heading')!
      return (
        element.getBoundingClientRect().height -
        heading.getBoundingClientRect().height
      )
    })
    expect(Math.abs(extraHeight)).toBeLessThan(1)
    await page.keyboard.press('Enter')
    await expect(fields).toHaveAttribute('open')
    await page.getByRole('button', { name: 'Collapse details' }).click()
    await expect(detail).toBeHidden()
  }
  for (let selection = 0; selection < 2; selection++) {
    const marker = page.getByRole('button', {
      name: 'Select 1 event',
      exact: true,
    })
    const beforeHover = page.url()
    await marker.hover()
    const preview = page.getByRole('dialog', { name: 'Span activity' })
    await expect(preview).toBeVisible()
    await expect(preview.getByText('Span event', { exact: true })).toBeVisible()
    await expect(detail).toBeHidden()
    await expect(page).toHaveURL(beforeHover)
    await marker.click()
    await expect(detail).toBeVisible()
    await expect(activity).toHaveAttribute('open')
    await expect(fields).toHaveAttribute('open')
    await expect(
      detail.getByRole('table', { name: 'Event target event' })
    ).toBeVisible()
    await activity.locator(':scope > summary').click()
    await expect(activity).not.toHaveAttribute('open')
    await expect(fields).toHaveAttribute('open')
    await activity.locator(':scope > summary').click()
    await page.getByRole('button', { name: 'Collapse details' }).click()
    await expect(detail).toBeHidden()
  }
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(span).toBeVisible()
  await expect(detail).toBeHidden()
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    if (request.method !== 'getTraceView') return route.fallback()
    const clustered = structuredClone(trace)
    const events = clustered.spans[0]!.spanData.events
    events.push({ ...events[0]!, name: 'second event' })
    await route.fulfill({
      json: { jsonrpc: '2.0', id: request.id, result: clustered },
    })
  })
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  const cluster = page.getByRole('button', {
    name: 'Open 2 events',
    exact: true,
  })
  const preview = page.getByRole('dialog', { name: 'Span activity' })
  const beforeHover = page.url()
  await cluster.hover()
  await expect(preview).toBeVisible()
  await expect(preview.getByText('Span event', { exact: true })).toHaveCount(2)
  await expect(detail).toBeHidden()
  await expect(page).toHaveURL(beforeHover)
  await preview.hover()
  await expect(preview).toBeVisible()
  await page.locator('.pane-header__title').first().hover()
  await expect(preview).toBeHidden()
  await cluster.hover()
  await cluster.click()
  await page.locator('.pane-header__title').first().hover()
  await expect(preview).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(preview).toBeHidden()
  await expect(cluster).toBeFocused()
  await cluster.hover()
  await preview.getByRole('button', { name: /second event/ }).click()
  await expect(
    detail.getByRole('table', { name: 'Event second event' })
  ).toBeVisible()
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  const resize = detail.getByRole('separator', {
    name: 'Resize Span and Events sections',
    exact: true,
  })
  const sectionHeight = () =>
    fields.evaluate(
      element => element.parentElement!.getBoundingClientRect().height
    )
  const originalHeight = await sectionHeight()
  const handle = (await resize.boundingBox())!
  await page.mouse.move(
    handle.x + handle.width / 2,
    handle.y + handle.height / 2
  )
  await page.mouse.down()
  await page.mouse.move(
    handle.x + handle.width / 2,
    handle.y + handle.height / 2 - 50,
    { steps: 5 }
  )
  await page.mouse.up()
  await expect.poll(sectionHeight).toBeCloseTo(originalHeight - 50, 0)
  const pairHeight =
    (await sectionHeight()) +
    (await activity.evaluate(
      element => element.parentElement!.getBoundingClientRect().height
    ))
  const headerHeight = await fields
    .locator(':scope > summary')
    .evaluate(element => element.getBoundingClientRect().height)
  const savedHeight =
    originalHeight - 50 + (pairHeight - headerHeight * 2) * 0.05
  await resize.focus()
  await resize.press('Shift+ArrowDown')
  await expect.poll(sectionHeight).toBeCloseTo(savedHeight, 0)
  await fields
    .locator(':scope > summary')
    .getByText('Span', { exact: true })
    .click()
  await expect(resize).toHaveAttribute('aria-disabled', 'false')
  await expect.poll(sectionHeight).toBeLessThan(60)
  await fields
    .locator(':scope > summary')
    .getByText('Span', { exact: true })
    .click()
  await expect(resize).toHaveAttribute('aria-disabled', 'false')
  await fields.evaluate(async element => {
    await new Promise(requestAnimationFrame)
    await Promise.all(
      element
        .parentElement!.getAnimations()
        .map(animation => animation.finished)
    )
  })
  const reopenedHeight = await sectionHeight()
  expect(reopenedHeight).toBeGreaterThan(60)
  await page.reload()
  await span.click()
  await expect.poll(sectionHeight).toBeCloseTo(reopenedHeight, 0)
  await resize.dblclick()
  await expect
    .poll(() =>
      page.evaluate(() => localStorage.getItem('trace-detail-section-sizes'))
    )
    .toBe('{}')
  expect(pageErrors).toEqual([])
})
