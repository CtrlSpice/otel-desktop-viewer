import type { Page, Route, Request } from '@playwright/test'
import { tableToIPC } from 'apache-arrow'
import { telemetryDataset } from '../src/test/native-telemetry-fixtures'
import { ArrowFrame } from '../src/services/arrow-data'
import { traceData } from '../src/services/arrow-traces'
import { tableValuePreview } from '../src/components/shared/table-value-preview'
import type { JsonTraceData } from '../src/types/wire-types'
import type { TraceColumn } from '../src/types/api-types'

function tracePreviews(result: JsonTraceData, columns: TraceColumn[]) {
  const full = traceData(
    new ArrowFrame(telemetryDataset('getTraceView', result))
  )
  return full.spans.flatMap(({ spanData: span }) =>
    columns.map(column => ({
      ...column,
      spanID: span.spanID,
      values: (column.source === 'span'
        ? span.attributes
        : span[column.source].attributes
      )
        .filter(a => a.key === column.key)
        .map(a => ({
          valueType: a.value.kind,
          preview: tableValuePreview(a.value),
        })),
    }))
  )
}

export function telemetryRequest(request: Request) {
  const method = new URL(request.url()).pathname.match(
    /^\/api\/data\/(.+)$/
  )?.[1]
  return method
    ? {
        id: 0,
        // Legacy trace fixtures describe full received traces. Project their
        // exact span and preview datasets independently, as the native API does.
        method:
          method === 'getSpanLogs'
            ? 'getSpan'
            : method === 'getSpan' || method === 'getTraceColumnPreviews'
              ? 'getTraceView'
              : method,
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
        const params = route.request().postDataJSON()
        let result = response.result
        if (nativeMethod === 'getSpan' && result) {
          result = {
            ...result,
            spans: result.spans.filter(
              (node: JsonTraceData['spans'][number]) =>
                node.spanData.spanID === params.spanID
            ),
          }
        } else if (nativeMethod === 'getTraceColumnPreviews') {
          result = tracePreviews(result, params.columns ?? [])
        } else if (nativeMethod === 'getTraceView' && params.columns?.length) {
          const previews = tracePreviews(result, params.columns)
          result = {
            ...result,
            spans: result.spans.map((node: JsonTraceData['spans'][number]) => ({
              ...node,
              spanData: {
                ...node.spanData,
                columnValues: previews.filter(
                  cell => cell.spanID === node.spanData.spanID
                ),
              },
            })),
          }
        }
        const table = telemetryDataset(nativeMethod, result)
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
