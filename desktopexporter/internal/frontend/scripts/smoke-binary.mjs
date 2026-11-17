import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { createServer } from 'node:net'
import { resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { DataType, tableFromIPC } from 'apache-arrow'
import { chromium, expect } from '@playwright/test'

// Exercise the embedded executable, with fresh in-memory storage and no
// Vite server or API mocks. Reserve separate ports from interactive previews.
assert.ok(process.argv[2], 'Usage: node scripts/smoke-binary.mjs <executable>')
const mode = process.env.EVALS ?? '0'
assert.ok(mode === '0' || mode === '1', 'EVALS must be 0 or 1')
const evals = mode === '1'
const reservations = []
for (let index = 0; index < 3; index++) {
  const server = createServer()
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  reservations.push(server)
}
const [viewerPort, httpPort, grpcPort] = reservations.map(
  server => server.address().port
)
await Promise.all(
  reservations.map(
    server => new Promise(resolveClose => server.close(resolveClose))
  )
)
const baseURL = `http://127.0.0.1:${viewerPort}`
const child = spawn(resolve(process.argv[2]), [
  '--open-browser=false',
  '--host',
  '127.0.0.1',
  '--browser-port',
  String(viewerPort),
  '--http',
  String(httpPort),
  '--grpc',
  String(grpcPort),
])
let output = ''
let spawnError
child.on('error', error => {
  spawnError = error
})
child.stdout.on('data', chunk => {
  output += chunk
})
child.stderr.on('data', chunk => {
  output += chunk
})
let browser
const errors = []

function assertRunning() {
  if (spawnError) throw spawnError
  assert.equal(child.exitCode, null, `Viewer exited: ${output}`)
  assert.equal(child.signalCode, null, `Viewer was terminated: ${output}`)
}

async function json(url, options) {
  const response = await fetch(url, {
    ...options,
    signal: AbortSignal.timeout(2000),
  })
  assert.equal(response.status, 200, `${url}: ${await response.clone().text()}`)
  return response.json()
}

async function rpc(method, params = {}) {
  const response = await json(`${baseURL}/rpc`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params }),
  })
  assert.equal(response.error, undefined, JSON.stringify(response.error))
  return response.result
}

async function arrow(method, params) {
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
  // Evals is disabled by default even when its implementation is included.
  assert.equal((await json(`${baseURL}/api/features`)).evals.enabled, false)
  assert.deepEqual(await json(`${baseURL}/build-features.json`), {
    evals,
  })

  const traceId = '0123456789abcdef0123456789abcdef'
  const spanId = '0123456789abcdef'
  const timestamp = String(BigInt(Date.now()) * 1000000n + 123n)
  const exactInteger = '9007199254740993'
  let deep = {
    kvlistValue: {
      values: [
        { key: 'bytes', value: { bytesValue: 'AP8=' } },
        { key: 'exact', value: { intValue: exactInteger } },
        { key: 'marker', value: { stringValue: 'native-deep-leaf' } },
        { key: 'text', value: { stringValue: exactInteger } },
      ],
    },
  }
  for (let level = 0; level < 200; level++)
    deep =
      level % 2
        ? { kvlistValue: { values: [{ key: 'child', value: deep }] } }
        : { arrayValue: { values: [deep] } }
  const resource = {
    attributes: [{ key: 'service.name', value: { stringValue: 'ci-smoke' } }],
  }
  const scope = { name: 'embedded-smoke' }
  const payloads = {
    traces: {
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
    },
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
    await json(`http://127.0.0.1:${httpPort}/v1/${signal}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
  }
  await expect
    .poll(
      async () => {
        assertRunning()
        const stats = await rpc('getStats')
        return [
          stats.traces.spanCount,
          stats.logs.logCount,
          stats.metrics.dataPointCount,
        ]
      },
      { timeout: 30000 }
    )
    .toEqual([1, 1, 1])
  const services = await rpc('discoverTelemetry', {
    options: { kind: 'services' },
  })
  assert.equal(services.rows.length, 1)
  assert.equal(services.rows[0][0].serviceName, 'ci-smoke')
  assert.ok(services.rows[0][0].serviceIdentity)
  const exported = await json(`${baseURL}/export/traces/${traceId}`)
  const span = exported.resourceSpans[0].scopeSpans[0].spans[0]
  assert.equal(span.traceId, traceId)
  assert.equal(span.spanId, spanId)
  assert.equal(span.startTimeUnixNano, timestamp)
  assert.deepEqual(
    span.attributes.find(attribute => attribute.key === 'smoke.exact').value,
    { intValue: exactInteger }
  )
  assert.deepEqual(
    span.attributes.find(attribute => attribute.key === 'smoke.text').value,
    { stringValue: exactInteger }
  )

  const stored = await rpc('query', {
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
  for (const [key, type, expected] of [
    ['smoke.exact', 'int64', integerBytes],
    ['smoke.text', 'string', stringBytes],
  ]) {
    const matches = await arrow('searchAttributeMatches', {
      term: '',
      signal: 'traces',
      owner: 'span',
      path: ['attributes', key],
      type,
    })
    assert.equal(matches.numRows, 1)
    const sampleType = matches.schema.fields.find(
      field => field.name === 'sampleValues'
    ).type
    assert.ok(DataType.isList(sampleType))
    assert.equal(
      sampleType.children[0].metadata.get('ARROW:extension:name'),
      'arrow.parquet.variant'
    )
    const samples = matches.getChild('sampleValues').get(0)
    assert.equal(samples.length, 1)
    assert.equal(samples.get(0).metadata[0] & 0x0f, 1)
    assert.deepEqual(samples.get(0).value, expected)
  }

  browser = await chromium.launch()
  const page = await browser.newPage({
    permissions: ['clipboard-read', 'clipboard-write'],
  })
  page.on('pageerror', error => errors.push(error.message))
  page.on('response', response => {
    if (response.url().startsWith(baseURL) && response.status() >= 400)
      errors.push(`${response.status()} ${response.url()}`)
  })
  await page.goto(baseURL)
  await expect(
    page.getByRole('button', { name: /ci-smoke/ }).first()
  ).toBeVisible({ timeout: 15000 })
  await expect(page.getByRole('link', { name: /Evals/ })).toHaveCount(0)

  const logRows = await rpc('query', { sql: 'SELECT id::varchar FROM logs' })
  assert.equal(logRows.rows.length, 1)
  await page.addInitScript(() => {
    localStorage.setItem('signal-detail:open', 'true')
    localStorage.setItem('signal-drawer:open', 'false')
  })
  await page.goto(`${baseURL}/logs/${logRows.rows[0][0]}?time=all`)
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
  assert.equal(copied.match(/"child":/g).length, 100)
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
  const receivedLogCount = count =>
    page.waitForResponse(async response => {
      if (
        response.url() !== `${baseURL}/rpc` ||
        response.request().postDataJSON()?.method !== 'getStats'
      )
        return false
      return (await response.json()).result?.logs?.logCount === count
    })
  const appendLog = async marker => {
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
  const imported = await json(`${baseURL}/export/logs/${importedID}`)
  const importedLog = imported.resourceLogs[0].scopeLogs[0].logRecords[0]
  assert.match(
    importedLog.attributes.find(attribute => attribute.key === 'password').value
      .stringValue,
    /\*{4}/
  )
  assert.deepEqual(
    importedLog.attributes.find(attribute => attribute.key === 'exact').value,
    { intValue: exactInteger }
  )
  assert.deepEqual(errors, [])
  assertRunning()
  console.log(
    `Embedded smoke passed (EVALS=${mode}): UI, feature flag, all-signal ingestion, schema 27, native Variant, 200-level display/Find/Copy, bounded children, exact Arrow values, service identity, trace export, Live off/on selection and redacted browser import`
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
  }
}
