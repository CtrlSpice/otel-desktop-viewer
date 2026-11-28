import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { createServer, type Server } from 'node:net'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { DataType, tableFromIPC, type Table } from 'apache-arrow'
import { chromium, expect, type Browser, type Request } from '@playwright/test'

interface RpcError {
  code: number
  message: string
}

interface RpcParams {
  options?: { kind: string }
  sql?: string
  limit?: number
}

interface ArrowSearchAttributeMatchesParams {
  term: string
  signal: string
  owner: string
  path: string[]
  type: string
}

interface OtlpIngestResponse {
  partialSuccess?: {
    rejectedSpans?: number
    rejectedLogRecords?: number
    rejectedDataPoints?: number
  }
}

interface SmokeLogRecord {
  timeUnixNano?: string
  body?: OtlpValue
}

interface RpcResponse<T> {
  result?: T
  error?: RpcError
}

interface FeaturesResponse {
  evals: { enabled: boolean }
}

interface BuildFeaturesResponse {
  evals: boolean
}

interface StatsResponse {
  traces: { spanCount: number }
  logs: { logCount: number }
  metrics: { dataPointCount: number }
}

interface QueryResponse {
  rows: unknown[][]
  truncated?: boolean
}

interface DiscoverServiceRow {
  serviceName: string
  serviceIdentity: string
}

interface DiscoverResponse {
  rows: [DiscoverServiceRow][]
}

interface OtlpValue {
  stringValue?: string
  intValue?: string
  bytesValue?: string
  arrayValue?: { values: OtlpValue[] }
  kvlistValue?: { values: OtlpAttribute[] }
}

interface OtlpAttribute {
  key: string
  value: OtlpValue
}

interface OtlpSpan {
  traceId: string
  spanId: string
  parentSpanId?: string
  name: string
  kind: number
  startTimeUnixNano: string
  endTimeUnixNano: string
  attributes: OtlpAttribute[]
}

interface OtlpResource {
  attributes: OtlpAttribute[]
}

interface OtlpScope {
  name: string
}

interface OtlpTracesPayload {
  resourceSpans: Array<{
    resource: OtlpResource
    scopeSpans: Array<{
      scope: OtlpScope
      spans: OtlpSpan[]
    }>
  }>
}

interface ExportedTrace {
  resourceSpans: Array<{
    scopeSpans: Array<{
      spans: OtlpSpan[]
    }>
  }>
}

interface ExportedLog {
  resourceLogs: Array<{
    scopeLogs: Array<{
      logRecords: Array<{
        attributes: OtlpAttribute[]
      }>
    }>
  }>
}

interface VariantSample {
  metadata: Uint8Array
  value: Uint8Array
}

interface ArrowListVector<T> {
  length: number
  get: (index: number) => T | null
}

// Exercise the embedded executable with an isolated temporary DuckDB file and
// no Vite server or API mocks. Reserve separate ports from interactive previews.
const executableArg = process.argv[2]
assert.ok(executableArg, 'Usage: node scripts/smoke-binary.ts <executable>')
const mode = process.env.EVALS ?? '0'
assert.ok(mode === '0' || mode === '1', 'EVALS must be 0 or 1')
const evals = mode === '1'
const reservations: Server[] = []
for (let index = 0; index < 3; index++) {
  const server = createServer()
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  reservations.push(server)
}
function boundPort(server: Server): number {
  const address = server.address()
  assert.ok(address instanceof Object && Number.isInteger(address.port))
  return address.port
}

const [viewerPort, httpPort, grpcPort] = reservations.map(boundPort)
await Promise.all(
  reservations.map(
    server =>
      new Promise<void>(resolveClose => {
        server.close(() => resolveClose())
      })
  )
)
const baseURL = `http://127.0.0.1:${viewerPort}`
const smokeDirectory = await mkdtemp(join(tmpdir(), 'otel-smoke-'))
const child = spawn(resolve(executableArg), [
  '--open-browser=false',
  '--host',
  '127.0.0.1',
  '--browser-port',
  String(viewerPort),
  '--http',
  String(httpPort),
  '--grpc',
  String(grpcPort),
  '--db',
  join(smokeDirectory, 'smoke.duckdb'),
])
let output = ''
let spawnError: Error | undefined
child.on('error', error => {
  spawnError = error
})
child.stdout.on('data', chunk => {
  output += chunk
})
child.stderr.on('data', chunk => {
  output += chunk
})
let browser: Browser | undefined
let importDirectory: string | undefined
const errors: string[] = []

function assertRunning(): void {
  if (spawnError) throw spawnError
  assert.equal(child.exitCode, null, `Viewer exited: ${output}`)
  assert.equal(child.signalCode, null, `Viewer was terminated: ${output}`)
}

async function json<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...options,
    signal: AbortSignal.timeout(2000),
  })
  assert.equal(response.status, 200, `${url}: ${await response.clone().text()}`)
  const parsed: T = await response.json()
  return parsed
}

async function rpc<T>(method: string, params: RpcParams = {}): Promise<T> {
  const response = await json<RpcResponse<T>>(`${baseURL}/rpc`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params }),
  })
  assert.equal(response.error, undefined, JSON.stringify(response.error))
  assert.ok(response.result !== undefined, `${method}: missing result`)
  return response.result
}

async function arrow(
  method: string,
  params: ArrowSearchAttributeMatchesParams
): Promise<Table> {
  const response = await fetch(`${baseURL}/api/data/${method}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
    signal: AbortSignal.timeout(2000),
  })
  assert.equal(response.status, 200)
  assert.equal(
    response.headers.get('Content-Type'),
    'application/vnd.apache.arrow.stream'
  )
  assert.equal(response.headers.get('Cache-Control'), 'no-store')
  const bytes = new Uint8Array(await response.arrayBuffer())
  assert.deepEqual(
    bytes.subarray(-8),
    Uint8Array.of(255, 255, 255, 255, 0, 0, 0, 0)
  )
  const table = tableFromIPC(bytes)
  assert.equal(response.headers.get('X-Arrow-Rows'), String(table.numRows))
  return table
}

function findAttribute(
  attributes: OtlpAttribute[],
  key: string
): OtlpAttribute {
  const attribute = attributes.find(item => item.key === key)
  assert.ok(attribute, `Missing attribute ${key}`)
  return attribute
}

try {
  await expect
    .poll(
      async () => {
        assertRunning()
        try {
          return (
            await fetch(`${baseURL}/api/features`, {
              signal: AbortSignal.timeout(1000),
            })
          ).status
        } catch {
          return 0
        }
      },
      { timeout: 30000 }
    )
    .toBe(200)
  assert.equal(
    (await json<FeaturesResponse>(`${baseURL}/api/features`)).evals.enabled,
    evals
  )
  assert.deepEqual(
    await json<BuildFeaturesResponse>(`${baseURL}/build-features.json`),
    { evals }
  )

  const traceId = '0123456789abcdef0123456789abcdef'
  const spanId = '0123456789abcdef'
  const timestamp = String(BigInt(Date.now()) * 1000000n + 123n)
  const exactInteger = '9007199254740993'
  let deep: OtlpValue = {
    kvlistValue: {
      values: [
        { key: 'bytes', value: { bytesValue: 'AP8=' } },
        { key: 'exact', value: { intValue: exactInteger } },
        { key: 'marker', value: { stringValue: 'native-deep-leaf' } },
        { key: 'text', value: { stringValue: exactInteger } },
      ],
    },
  }
  for (let level = 0; level < 200; level++) {
    deep =
      level % 2
        ? { kvlistValue: { values: [{ key: 'child', value: deep }] } }
        : { arrayValue: { values: [deep] } }
  }
  const resource = {
    attributes: [{ key: 'service.name', value: { stringValue: 'ci-smoke' } }],
  }
  const scope = { name: 'embedded-smoke' }
  const initialTraces: OtlpTracesPayload = {
    resourceSpans: [
      {
        resource,
        scopeSpans: [
          {
            scope,
            spans: [
              {
                traceId,
                spanId,
                name: 'ci-smoke-span',
                kind: 2,
                startTimeUnixNano: timestamp,
                endTimeUnixNano: String(BigInt(timestamp) + 1000000n),
                attributes: [
                  { key: 'smoke.exact', value: { intValue: exactInteger } },
                  { key: 'smoke.text', value: { stringValue: exactInteger } },
                ],
              },
            ],
          },
        ],
      },
    ],
  }
  const payloads = {
    traces: initialTraces,
    logs: {
      resourceLogs: [
        {
          resource,
          scopeLogs: [
            {
              scope,
              logRecords: [
                {
                  traceId,
                  spanId,
                  timeUnixNano: timestamp,
                  severityNumber: 9,
                  body: {
                    kvlistValue: {
                      values: [
                        { key: 'deep', value: deep },
                        {
                          key: 'items',
                          value: {
                            arrayValue: {
                              values: Array.from(
                                { length: 250 },
                                (_, index) => ({
                                  stringValue:
                                    index === 249
                                      ? 'last-hidden-value'
                                      : `item-${index}`,
                                })
                              ),
                            },
                          },
                        },
                        {
                          key: 'message',
                          value: { stringValue: 'ci-smoke-log' },
                        },
                      ],
                    },
                  },
                },
              ],
            },
          ],
        },
      ],
    },
    metrics: {
      resourceMetrics: [
        {
          resource,
          scopeMetrics: [
            {
              scope,
              metrics: [
                {
                  name: 'ci.smoke.gauge',
                  gauge: {
                    dataPoints: [
                      {
                        timeUnixNano: timestamp,
                        asInt: exactInteger,
                      },
                    ],
                  },
                },
              ],
            },
          ],
        },
      ],
    },
  }
  for (const [signal, payload] of Object.entries(payloads)) {
    await json<OtlpIngestResponse>(
      `http://127.0.0.1:${httpPort}/v1/${signal}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      }
    )
  }
  await expect
    .poll(
      async () => {
        assertRunning()
        const stats = await rpc<StatsResponse>('getStats')
        return [
          stats.traces.spanCount,
          stats.logs.logCount,
          stats.metrics.dataPointCount,
        ]
      },
      { timeout: 30000 }
    )
    .toEqual([1, 1, 1])
  const services = await rpc<DiscoverResponse>('discoverTelemetry', {
    options: { kind: 'services' },
  })
  assert.equal(services.rows.length, 1)
  assert.equal(services.rows[0][0].serviceName, 'ci-smoke')
  assert.ok(services.rows[0][0].serviceIdentity)
  const exported = await json<ExportedTrace>(
    `${baseURL}/export/traces/${traceId}`
  )
  const span = exported.resourceSpans[0].scopeSpans[0].spans[0]
  assert.equal(span.traceId, traceId)
  assert.equal(span.spanId, spanId)
  assert.equal(span.startTimeUnixNano, timestamp)
  assert.deepEqual(findAttribute(span.attributes, 'smoke.exact').value, {
    intValue: exactInteger,
  })
  assert.deepEqual(findAttribute(span.attributes, 'smoke.text').value, {
    stringValue: exactInteger,
  })

  const stored = await rpc<QueryResponse>('query', {
    sql: `SELECT (SELECT version FROM schema_meta), typeof(attributes),
      variant_typeof(attributes['smoke.exact']),
      attributes['smoke.exact']::bigint::varchar,
      variant_typeof(attributes['smoke.text']) FROM spans`,
  })
  assert.deepEqual(stored.rows, [
    [27, 'VARIANT', 'INT64', exactInteger, 'VARCHAR'],
  ])
  assert.equal(stored.truncated, false)

  // Exercise the shipped browser transport with an integer that JSON.parse
  // would round. The numeric and string attributes must remain distinct.
  const integerBytes = new Uint8Array(9)
  integerBytes[0] = 0x18
  new DataView(integerBytes.buffer).setBigInt64(1, BigInt(exactInteger), true)
  const stringBytes = Uint8Array.of(
    (exactInteger.length << 2) | 1,
    ...new TextEncoder().encode(exactInteger)
  )
  const variantChecks: Array<[string, string, Uint8Array]> = [
    ['smoke.exact', 'int64', integerBytes],
    ['smoke.text', 'string', stringBytes],
  ]
  for (const [key, type, expected] of variantChecks) {
    const matches = await arrow('searchAttributeMatches', {
      term: '',
      signal: 'traces',
      owner: 'span',
      path: ['attributes', key],
      type,
    })
    assert.equal(matches.numRows, 1)
    const sampleField = matches.schema.fields.find(
      field => field.name === 'sampleValues'
    )
    assert.ok(sampleField, 'Missing sampleValues field')
    const sampleType = sampleField.type
    assert.ok(DataType.isList(sampleType))
    assert.equal(
      sampleType.children[0].metadata.get('ARROW:extension:name'),
      'arrow.parquet.variant'
    )
    const sampleColumn = matches.getChild('sampleValues')
    assert.ok(sampleColumn, 'Missing sampleValues column')
    const samples: ArrowListVector<VariantSample> | null = sampleColumn.get(0)
    assert.ok(samples, 'Missing sampleValues row')
    assert.equal(samples.length, 1)
    const firstSample = samples.get(0)
    assert.ok(firstSample, 'Missing sampleValues element')
    assert.equal(firstSample.metadata[0] & 0x0f, 1)
    assert.deepEqual(firstSample.value, expected)
  }

  browser = await chromium.launch()
  const page = await browser.newPage({
    permissions: ['clipboard-read', 'clipboard-write'],
  })
  page.on('pageerror', error => errors.push(error.message))
  page.on('response', response => {
    if (response.url().startsWith(baseURL) && response.status() >= 400) {
      errors.push(`${response.status()} ${response.url()}`)
    }
  })
  await page.goto(baseURL)
  await expect(
    page.getByRole('button', { name: /ci-smoke/ }).first()
  ).toBeVisible({ timeout: 15000 })
  const evalsLink = page.getByRole('link', { name: /Evals/ })
  if (evals) await expect(evalsLink).toBeVisible()
  else await expect(evalsLink).toHaveCount(0)

  const logRows = await rpc<QueryResponse>('query', {
    sql: 'SELECT id::varchar FROM logs',
  })
  assert.equal(logRows.rows.length, 1)
  const firstLogID = String(logRows.rows[0][0])
  await page.addInitScript(() => {
    localStorage.setItem('signal-detail:open', 'true')
    localStorage.setItem('signal-drawer:open', 'false')
  })
  await page.goto(`${baseURL}/logs/${firstLogID}?time=all`)
  const body = page
    .getByRole('table', { name: 'Log fields' })
    .locator('[data-field="body"]')
  await expect(
    body.getByRole('button', { name: 'Expand deep', exact: true })
  ).toBeVisible()
  await expect(body.locator('.cm-editor')).toHaveCount(0)
  await body.getByRole('button', { name: 'Expand items', exact: true }).click()
  await expect(body).toContainText('item-99')
  await expect(body).not.toContainText('item-100')
  await body.getByRole('button', { name: 'Show next 100 of 250' }).click()
  await expect(body).toContainText('item-199')
  await expect(body).not.toContainText('last-hidden-value')
  await body
    .getByRole('button', { name: 'Copy body', exact: true })
    .press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain('last-hidden-value')
  const copied = await page.evaluate(() => navigator.clipboard.readText())
  assert.equal(copied.match(/"child":/g)?.length, 100)
  assert.ok(copied.includes(`"exact": ${exactInteger}`))
  assert.ok(copied.includes(`"text": "${exactInteger}"`))
  assert.ok(copied.includes('"bytes": "AP8="'))
  assert.ok(copied.includes('native-deep-leaf'))
  await body.getByRole('button', { name: 'Full document / Find' }).click()
  const search = body.locator('.cm-search input[name="search"]')
  await expect(search).toBeFocused()
  await search.pressSequentially('native-deep-leaf')
  await search.press('Enter')
  await expect(body.locator('.cm-searchMatch-selected')).toHaveText(
    'native-deep-leaf'
  )
  await body
    .getByRole('button', { name: 'Expand all body JSON', exact: true })
    .click()
  await body
    .getByRole('button', { name: 'Collapse all body JSON', exact: true })
    .click()
  await body
    .getByRole('button', { name: 'Copy body', exact: true })
    .press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(copied)

  const selectedURL = page.url()
  let detailRequests = 0
  page.on('request', request => {
    if (request.url() === `${baseURL}/api/data/getLog`) detailRequests++
  })
  const live = page.getByRole('button', { name: 'Live', exact: true })
  await expect(live).toHaveAttribute('aria-pressed', 'false')
  const receivedLogCount = (count: number) =>
    page.waitForResponse(async response => {
      if (
        response.url() !== `${baseURL}/rpc` ||
        response.request().postDataJSON()?.method !== 'getStats'
      ) {
        return false
      }
      const body: RpcResponse<StatsResponse> = await response.json()
      return body.result?.logs?.logCount === count
    })
  const appendLog = async (marker: string): Promise<void> => {
    const response = await fetch(`http://127.0.0.1:${httpPort}/v1/logs`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        resourceLogs: [
          {
            resource,
            scopeLogs: [
              {
                scope,
                logRecords: [
                  {
                    timeUnixNano: timestamp,
                    body: { stringValue: marker },
                  },
                ],
              },
            ],
          },
        ],
      }),
    })
    assert.equal(response.status, 200)
  }
  const offCommit = receivedLogCount(2)
  await appendLog('live-off-arrival')
  await offCommit
  assert.equal(detailRequests, 0)
  assert.equal(page.url(), selectedURL)

  const enabled = page.waitForResponse(`${baseURL}/api/data/getLog`)
  await live.click()
  await enabled
  await expect(live).toHaveAttribute('aria-pressed', 'true')
  const onCommit = receivedLogCount(3)
  const refreshed = page.waitForResponse(`${baseURL}/api/data/getLog`)
  await appendLog('live-on-arrival')
  await Promise.all([onCommit, refreshed])
  assert.equal(page.url(), selectedURL)
  await body
    .getByRole('button', { name: 'Copy body', exact: true })
    .press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(copied)

  await live.click()
  await expect(live).toHaveAttribute('aria-pressed', 'false')
  await page.goto(baseURL)
  await page.getByLabel('Choose telemetry JSON files').setInputFiles({
    name: 'redacted-first-log.json',
    mimeType: 'application/json',
    buffer: Buffer.from(
      JSON.stringify({
        resourceLogs: [
          {
            resource,
            scopeLogs: [
              {
                scope,
                logRecords: [
                  {
                    timeUnixNano: timestamp,
                    body: { stringValue: 'browser-import-destination' },
                    attributes: [
                      {
                        key: 'password',
                        value: { stringValue: 'synthetic-import-secret' },
                      },
                      {
                        key: 'exact',
                        value: { intValue: exactInteger },
                      },
                    ],
                  },
                ],
              },
            ],
          },
        ],
      })
    ),
  })
  await expect(page).not.toHaveURL(selectedURL, { timeout: 15000 })
  await expect(page).toHaveURL(/\/logs\/[a-f0-9-]+\?time=all$/)
  await expect(body).toContainText('browser-import-destination')
  const importedID = new URL(page.url()).pathname.split('/').at(-1)
  assert.ok(importedID, 'Missing imported log ID in URL')
  const imported = await json<ExportedLog>(
    `${baseURL}/export/logs/${importedID}`
  )
  const importedLog = imported.resourceLogs[0].scopeLogs[0].logRecords[0]
  assert.match(
    findAttribute(importedLog.attributes, 'password').value.stringValue ?? '',
    /\*{4}/
  )
  assert.deepEqual(findAttribute(importedLog.attributes, 'exact').value, {
    intValue: exactInteger,
  })

  await mkdir(join(tmpdir(), 'opencode'), { recursive: true })
  importDirectory = await mkdtemp(join(tmpdir(), 'opencode', 'cli-import-'))
  const importFiles = async (
    files: string[]
  ): Promise<{ code: number | null; text: string }> => {
    const command = spawn(resolve(executableArg), [
      'import',
      ...files,
      '--endpoint',
      baseURL,
    ])
    let text = ''
    command.stdout.on('data', chunk => {
      text += chunk
    })
    command.stderr.on('data', chunk => {
      text += chunk
    })
    const exitEvent = await once(command, 'exit')
    const code: number | null = exitEvent[0]
    return { code, text }
  }
  const metricIDs = await rpc<QueryResponse>('query', {
    sql: `select metric_id::varchar, series_id::varchar, id::varchar
      from metric_datapoints order by timestamp limit 1`,
  })
  const [metricID, seriesID, datapointID] = metricIDs.rows[0].map(String)
  const filter = 'service.name = "ci-smoke"'
  const reads = new Map<string, number>()
  const pending = new Set<Request>()
  let importLookups = 0
  page.on('request', request => {
    if (request.url().startsWith(`${baseURL}/api/import-target/`)) {
      importLookups++
    }
    if (request.url().startsWith(`${baseURL}/api/data/`)) {
      pending.add(request)
      const method = new URL(request.url()).pathname.split('/').at(-1)
      if (method) reads.set(method, (reads.get(method) ?? 0) + 1)
    }
  })
  page.on('requestfinished', request => pending.delete(request))
  page.on('requestfailed', request => pending.delete(request))
  const settled = () => expect.poll(() => pending.size).toBe(0)
  await page.setViewportSize({ width: 1920, height: 1080 })
  let sequence = 0
  const signalCases: Array<
    ['logs' | 'traces' | 'metrics', string, Record<string, string>, string]
  > = [
    ['logs', firstLogID, {}, 'getLog'],
    ['traces', traceId, { span: spanId }, 'getTraceView'],
    [
      'metrics',
      metricID,
      { series: seriesID, dp: datapointID, agg: 'raw', visible: 'none' },
      'getMetricView',
    ],
  ]
  for (const [signal, id, selection, method] of signalCases) {
    for (const liveEnabled of [false, true]) {
      const params = new URLSearchParams({
        time: 'all',
        q: filter,
        ...selection,
      })
      await page.goto(`${baseURL}/${signal}/${id}?${params}`)
      const searchBox = page.getByRole('textbox', {
        name: `Search ${signal}`,
        exact: true,
      })
      const selectedDetail = async (): Promise<void> => {
        await expect(searchBox).toHaveText(filter)
        if (signal === 'logs') {
          await expect(
            page.getByRole('table', { name: 'Log fields' })
          ).toContainText('ci-smoke-log')
        } else if (signal === 'traces') {
          await expect(
            page.getByRole('table', { name: 'Span fields' })
          ).toContainText('ci-smoke-span')
        } else {
          await expect(
            page.locator(`[data-dp-id="${datapointID}"]`)
          ).toHaveAttribute('aria-expanded', 'true')
          await expect(
            page.getByRole('checkbox', { name: / in chart$/ })
          ).not.toBeChecked()
        }
      }
      await selectedDetail()
      await settled()
      assert.ok(reads.has(method), `${signal} did not request ${method}`)
      if (liveEnabled) {
        const before = reads.get(method) ?? 0
        await live.click()
        await expect.poll(() => reads.get(method) ?? 0).toBeGreaterThan(before)
        await settled()
      }
      await expect(live).toHaveAttribute('aria-pressed', String(liveEnabled))
      const selected = page.url()
      const beforeReads = reads.get(method) ?? 0
      const before = await rpc<StatsResponse>('getStats')
      const marker = `cli-import-${++sequence}`
      const newSpanID = sequence.toString(16).padStart(16, '0')
      const traces = structuredClone(payloads.traces)
      traces.resourceSpans[0].scopeSpans[0].spans = [
        {
          ...span,
          traceId: sequence.toString(16).padStart(32, '0'),
          spanId: newSpanID,
          name: marker,
        },
        {
          ...span,
          spanId: newSpanID,
          parentSpanId: spanId,
          name: `${marker}-child`,
        },
      ]
      const metrics = structuredClone(payloads.metrics)
      const metric = metrics.resourceMetrics[0].scopeMetrics[0].metrics[0]
      metric.gauge.dataPoints[0].timeUnixNano = String(
        BigInt(timestamp) + BigInt(sequence) * 1000000n
      )
      metrics.resourceMetrics[0].scopeMetrics[0].metrics.push({
        ...structuredClone(metric),
        name: marker,
      })
      const contents = JSON.stringify({
        ...traces,
        resourceLogs: [
          {
            resource,
            scopeLogs: [
              {
                scope,
                logRecords: [
                  {
                    timeUnixNano: timestamp,
                    body: { stringValue: marker },
                    attributes: [
                      {
                        key: 'password',
                        value: { stringValue: 'synthetic-cli-secret' },
                      },
                      { key: 'exact', value: { intValue: exactInteger } },
                    ],
                  },
                ],
              },
            ],
          },
        ],
        ...metrics,
      })
      const file = join(importDirectory, `${marker}.json`)
      await writeFile(file, contents)
      const notified = receivedLogCount(before.logs.logCount + 1)
      const result = await importFiles([file])
      assert.equal(result.code, 0, result.text)
      assert.ok(result.text.includes('Received'), result.text)
      assert.equal(await readFile(file, 'utf8'), contents)
      // Receiver acceptance alone does not prove that these writes committed.
      await expect
        .poll(async () => {
          const stats = await rpc<StatsResponse>('getStats')
          return [
            stats.traces.spanCount,
            stats.logs.logCount,
            stats.metrics.dataPointCount,
          ]
        })
        .toEqual([
          before.traces.spanCount + 2,
          before.logs.logCount + 1,
          before.metrics.dataPointCount + 2,
        ])
      await notified
      if (liveEnabled) {
        await expect
          .poll(() => reads.get(method) ?? 0)
          .toBeGreaterThan(beforeReads)
      } else {
        assert.equal(reads.get(method), beforeReads)
        await page.getByRole('button', { name: /^Refresh(?:$|:)/ }).click()
        await expect
          .poll(() => reads.get(method) ?? 0)
          .toBeGreaterThan(beforeReads)
      }
      await settled()
      assert.equal(page.url(), selected)
      await selectedDetail()
      assert.equal(importLookups, 0)
      const storedLog = await rpc<QueryResponse>('query', {
        sql: `select id::varchar from logs where body::varchar = '${marker}'`,
      })
      assert.equal(storedLog.rows.length, 1)
      const cliLog = await json<ExportedLog>(
        `${baseURL}/export/logs/${String(storedLog.rows[0][0])}`
      )
      const attrs = cliLog.resourceLogs[0].scopeLogs[0].logRecords[0].attributes
      assert.match(
        findAttribute(attrs, 'password').value.stringValue ?? '',
        /\*{4}/
      )
      assert.deepEqual(findAttribute(attrs, 'exact').value, {
        intValue: exactInteger,
      })
      const download = page.waitForEvent('download')
      const exportRequest = page.waitForRequest(
        request =>
          request.method() === 'GET' &&
          request.url().startsWith(`${baseURL}/export/${signal}/`)
      )
      await page
        .getByRole('button', {
          name: `Download ${signal.slice(0, -1)}`,
          exact: true,
        })
        .click()
      await download
      assert.equal(
        new URL((await exportRequest).url()).pathname,
        `/export/${signal}/${id}`
      )
      console.log(
        `CLI import retained ${signal} selection with Live ${liveEnabled ? 'on' : 'off'}`
      )
    }
  }

  // A malformed later JSONL object sends none of that file. Receiver errors
  // still allow a later file to commit, without changing the inspected item.
  const selected = page.url()
  const bad = join(importDirectory, 'malformed.jsonl')
  const rejected = join(importDirectory, 'receiver-rejected.json')
  const good = join(importDirectory, 'after-errors.json')
  const logPayload = (record: SmokeLogRecord): string =>
    JSON.stringify({
      resourceLogs: [
        { resource, scopeLogs: [{ scope, logRecords: [record] }] },
      ],
    })
  await writeFile(
    bad,
    logPayload({ body: { stringValue: 'must-not-be-sent' } }) + '\n{'
  )
  await writeFile(rejected, logPayload({ timeUnixNano: 'invalid-timestamp' }))
  await writeFile(
    good,
    logPayload({
      timeUnixNano: timestamp,
      body: { stringValue: 'after-cli-errors' },
    })
  )
  const beforeErrors = await rpc<StatsResponse>('getStats')
  const notified = receivedLogCount(beforeErrors.logs.logCount + 1)
  const failedImport = await importFiles([bad, rejected, good])
  assert.equal(failedImport.code, 1, failedImport.text)
  assert.ok(failedImport.text.includes('invalid JSON'), failedImport.text)
  assert.ok(failedImport.text.includes('Rejected:'), failedImport.text)
  assert.ok(failedImport.text.includes('Received'), failedImport.text)
  await notified
  await settled()
  assert.equal(page.url(), selected)
  assert.equal(importLookups, 0)
  assert.deepEqual(
    (
      await rpc<QueryResponse>('query', {
        sql: `select body::varchar from logs
      where body::varchar in ('must-not-be-sent', 'after-cli-errors')`,
      })
    ).rows,
    [['after-cli-errors']]
  )

  // Large direct links must survive the actual HTTP server and a fresh browser.
  const linkedMetrics = (['histogram', 'exponentialHistogram'] as const).map(
    kind => ({
      name: `url-smoke-${kind}`,
      [kind]: {
        aggregationTemporality: 1,
        dataPoints: Array.from({ length: 1000 }, (_, index) => ({
          attributes: [{ key: 'index', value: { intValue: String(index) } }],
          startTimeUnixNano: String(BigInt(timestamp) - 1000000000n),
          timeUnixNano: timestamp,
          count: '1',
          sum: 1.5,
          min: 1,
          max: 2,
          ...(kind === 'histogram'
            ? { explicitBounds: [2], bucketCounts: ['1', '0'] }
            : {
                scale: 0,
                zeroCount: '0',
                positive: { offset: 0, bucketCounts: ['1'] },
              }),
        })),
      },
    })
  )
  const longLinkFile = join(importDirectory, 'long-metric-links.json')
  await writeFile(
    longLinkFile,
    JSON.stringify({
      resourceMetrics: [
        { resource, scopeMetrics: [{ scope, metrics: linkedMetrics }] },
      ],
    })
  )
  const longLinkImport = await importFiles([longLinkFile])
  assert.equal(longLinkImport.code, 0, longLinkImport.text)
  const longLinkRows = () =>
    rpc<QueryResponse>('query', {
      sql: "select id::varchar, name from metrics where name like 'url-smoke-%' order by name",
    })
  await expect.poll(async () => (await longLinkRows()).rows.length).toBe(2)
  for (const row of (await longLinkRows()).rows) {
    const id = String(row[0])
    const name = String(row[1])
    const rows = await rpc<QueryResponse>('query', {
      sql: `select series_id::varchar from metric_datapoints where metric_id = '${id}' order by series_id`,
      limit: 1000,
    })
    const keys = rows.rows.map(item => String(item[0]))
    assert.equal(keys.length, 1000)
    const url = `${baseURL}/metrics/${id}?${new URLSearchParams({ time: 'all', htab: 'histogram', visible: keys.join(',') })}`
    assert.ok(Buffer.byteLength(url) > 39000)
    const fresh = await browser.newContext({
      viewport: { width: 1920, height: 1080 },
    })
    try {
      const linked = await fresh.newPage()
      linked.on('pageerror', error => errors.push(error.message))
      const requested: string[][] = []
      linked.on('request', request => {
        if (request.url().endsWith('/api/data/getMetricAggregateView')) {
          const payload: { seriesRefs?: string[] } | null =
            request.postDataJSON()
          assert.ok(Array.isArray(payload?.seriesRefs))
          requested.push(payload.seriesRefs)
        }
      })
      const response = await linked.goto(url)
      assert.ok(response, 'Missing navigation response')
      assert.equal(response.status(), 200)
      await expect(linked.locator('.lc-bars-bar:visible').first()).toBeVisible()
      await expect(
        linked.getByRole('list', { name: 'Histogram count scale' })
      ).toContainText('1,000')
      await expect.poll(() => requested.length).toBe(2)
      assert.ok(
        requested.every(
          selection => JSON.stringify(selection) === JSON.stringify(keys)
        )
      )
      const copied = linked.url()
      assert.equal(new URL(copied).searchParams.get('visible'), keys.join(','))
      await linked.reload()
      await expect(linked.locator('.lc-bars-bar:visible').first()).toBeVisible()
      await expect(
        linked.getByRole('list', { name: 'Histogram count scale' })
      ).toContainText('1,000')
      await expect.poll(() => requested.length).toBe(4)
      assert.deepEqual(requested.at(-1), keys)
      assert.equal(linked.url(), copied)
      console.log(
        `${name}: 1000 selected series restored from ${Buffer.byteLength(copied)}-byte URL and reload`
      )
    } finally {
      await fresh.close()
    }
  }
  assert.deepEqual(errors, [])
  assertRunning()
  console.log(
    `Embedded smoke passed (EVALS=${mode}): UI, feature flag, all-signal ingestion, schema 27, native Variant, 200-level display/Find/Copy, bounded children, exact Arrow values, service identity, trace export, Live off/on selection, redacted browser import and actual CLI imports retaining all three inspected signals`
  )
} catch (error) {
  console.error(output)
  if (errors.length) console.error('Browser errors:', errors)
  throw error
} finally {
  try {
    await browser?.close()
  } finally {
    if (child.exitCode === null && child.signalCode === null && !spawnError) {
      const stopped = once(child, 'exit')
      child.kill('SIGTERM')
      await Promise.race([stopped, delay(5000)])
      if (child.exitCode === null && child.signalCode === null) {
        child.kill('SIGKILL')
        await stopped
      }
    }
    if (importDirectory) {
      await rm(importDirectory, { recursive: true, force: true })
    }
    await rm(smokeDirectory, { recursive: true, force: true })
  }
}
