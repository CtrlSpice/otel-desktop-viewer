import type { Page, Route, Request } from '@playwright/test'
import { tableToIPC } from 'apache-arrow'
import { telemetryDataset } from '../src/test/native-telemetry-fixtures'

export function telemetryRequest(request: Request) {
  const method = new URL(request.url()).pathname.match(
    /^\/api\/data\/(.+)$/
  )?.[1]
  return method
    ? {
        id: 0,
        method: method === 'getSpanLogs' ? 'getSpan' : method,
        params: request.postDataJSON(),
      }
    : request.postDataJSON()
}

export type FixtureRoute = Pick<Route, 'fallback' | 'continue' | 'abort'> & {
  originalRequest(): Request
  request(): { postDataJSON(): ReturnType<typeof telemetryRequest> }
  fulfill(options: Parameters<Route['fulfill']>[0]): Promise<void>
}

export async function routeTelemetry(
  page: Page,
  handler: (route: FixtureRoute) => Promise<void> | void
) {
  await page.route(/\/(?:rpc|api\/data\/[^/]+)$/, async route => {
    const nativeMethod = new URL(route.request().url()).pathname.match(
      /^\/api\/data\/(.+)$/
    )?.[1]
    return handler({
      originalRequest: () => route.request(),
      request: () => ({
        postDataJSON: () => telemetryRequest(route.request()),
      }),
      fallback: options => route.fallback(options),
      continue: options => route.continue(options),
      abort: code => route.abort(code),
      fulfill: async options => {
        if (!nativeMethod) return route.fulfill(options)
        const response = options?.json ?? JSON.parse(String(options?.body))
        if (response.error)
          return route.fulfill({ status: 400, json: response.error })
        const table = telemetryDataset(nativeMethod, response.result)
        return route.fulfill({
          status: options?.status ?? 200,
          contentType: 'application/vnd.apache.arrow.stream',
          headers: { 'X-Arrow-Rows': String(table.numRows) },
          body: Buffer.from(tableToIPC(table, 'stream')),
        })
      },
    })
  })
}
