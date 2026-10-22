import { expect, test } from '@playwright/test'
import type { JsonTraceData } from '../src/types/wire-types'

const traceID = 'faca0000000000000000000000000001'
const spanID = 'faca000000000002'
const trace: JsonTraceData = {
  traceID,
  traceStart: '1700000000000000000',
  unplacedSpanCount: 0,
  resources: {
    0: { attributes: [], droppedAttributesCount: 0 },
  },
  scopes: {
    0: { name: 'test', version: '', attributes: [], droppedAttributesCount: 0 },
  },
  spans: [
    {
      depth: 0,
      matched: true,
      spanData: {
        spanID,
        parentSpanID: null,
        name: '800ms span',
        traceState: '',
        flags: 1,
        kindCode: 1,
        kind: 'Internal',
        start: '0',
        dur: '800000000',
        r: 0,
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
    },
  ],
}

test('every labelled waterfall tick has an unclipped guide, including the end', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result:
          method === 'getTraceView'
            ? trace
            : method === 'searchTraceSummaries'
              ? [
                  {
                    traceID,
                    hasRootSpan: true,
                    rootSpan: { serviceName: '', name: '800ms span' },
                    startTime: trace.traceStart,
                    durationNs: '800000000',
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
  await page.goto(`/traces/${traceID}?span=${spanID}&time=all`)
  const ticks = page.locator(
    'th.waterfall-time-axis-header__th-ruler .waterfall-time-ruler__tick'
  )
  await expect(ticks.last()).toHaveText('800ms')

  // Exercise an exact endpoint and a partial final interval at narrower widths.
  for (const width of [1920, 1440]) {
    await page.setViewportSize({ width, height: 1080 })
    await expect(async () => {
      const geometry = await page.evaluate(() => {
        const ruler = document.querySelector(
          '.waterfall-time-axis-header__th-ruler .waterfall-time-ruler'
        )!
        const guides = document.querySelector('.waterfall-time-guides')!
        const ticks = [...ruler.children].map(
          tick => tick.getBoundingClientRect().left
        )
        const lines = [...guides.children].map(line => {
          const rect = line.getBoundingClientRect()
          let left = rect.left
          let right = rect.right
          for (
            let parent = line.parentElement;
            parent;
            parent = parent.parentElement
          ) {
            if (getComputedStyle(parent).overflowX !== 'visible') {
              const clip = parent.getBoundingClientRect()
              left = Math.max(left, clip.left)
              right = Math.min(right, clip.right)
            }
          }
          const style = getComputedStyle(line)
          return {
            x: rect.left,
            visibleWidth: Math.max(0, right - left),
            color: style.backgroundColor,
            opacity: style.opacity,
            pointerEvents: style.pointerEvents,
          }
        })
        return {
          ticks,
          lines,
          end: ruler.getBoundingClientRect().right,
          barEnd: document
            .querySelector('.waterfall-row__bar')!
            .getBoundingClientRect().right,
        }
      })
      expect(geometry.lines).toHaveLength(geometry.ticks.length)
      for (const [index, line] of geometry.lines.entries()) {
        expect(line.x).toBeCloseTo(geometry.ticks[index], 1)
        expect(line.visibleWidth).toBe(1)
        expect(line.color).not.toBe('rgba(0, 0, 0, 0)')
        expect(line.opacity).toBe('0.18')
        expect(line.pointerEvents).toBe('none')
      }
      if (width === 1920) {
        expect(geometry.lines.at(-1)!.x).toBeCloseTo(geometry.end, 1)
      }
      expect(geometry.barEnd).toBeCloseTo(geometry.end, 1)
    }).toPass()
  }
})
