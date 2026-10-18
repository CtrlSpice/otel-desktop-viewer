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
    attributes: [],
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
  await expect(card).toHaveClass(/signal-row/)
  await expect(card).toHaveAttribute('aria-pressed', 'true')
  await expect(card).toContainText('checkout-api')
  await expect(card).toContainText('WARN (13)')
  await expect(card.locator('.signal-row__badge')).toHaveText('Log WARN (13)')
  await expect(card).toContainText('Associated warning')
  const fields = page.getByRole('table', { name: 'Span fields' })
  const key = fields.locator('.detail-property__key').first()
  const muted = await key.evaluate(element => {
    const probe = document.createElement('span')
    probe.style.color = 'var(--color-muted)'
    element.append(probe)
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  })
  await expect(key).toHaveCSS('color', muted)
  await expect(fields.locator('.detail-property__value').first()).not.toHaveCSS(
    'color',
    muted
  )
  await expect(
    associated.getByText('order.delayed', { exact: true })
  ).toBeVisible()
  await expect(
    card
      .locator('.signal-row__time')
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
  await expect(eventCard.locator('.signal-row__badge')).toHaveText('Span event')
  await expect(eventCard).toHaveAttribute('aria-expanded', 'false')
  await eventCard.click()
  await expect(
    page.getByRole('table', { name: 'Event target event' })
  ).toBeVisible()
  const linkCard = page.getByRole('button', {
    name: 'consume order (worker) Link 3 fields',
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
  await page.goBack()
  await expect(page).toHaveURL(tracePath)
  await expect(card).toHaveAttribute('aria-pressed', 'true')
})

test('detail targets reopen the panel with independent native disclosures', async ({
  page,
}) => {
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
    await expect(detail.getByRole('tablist')).toHaveCount(0)
    await expect(fields.locator(':scope > summary')).toHaveText('Span')
    await expect(fields.locator('details')).toHaveCount(0)
    const table = fields.getByRole('table', { name: 'Span fields' })
    const resource = table.locator('[data-field="service.name"]')
    await expect(resource.locator('.detail-property__key')).toHaveText(
      'service.name'
    )
    await expect(resource.locator('.detail-property__origin')).toHaveText(
      'resource'
    )
    const scopeName = table
      .getByRole('button', { name: 'Copy name (scope)', exact: true })
      .locator('xpath=ancestor::tr[1]')
    await expect(scopeName.locator('.detail-property__key')).toHaveText('name')
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
    await page
      .getByRole('button', { name: 'Select 1 event', exact: true })
      .click()
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
})
