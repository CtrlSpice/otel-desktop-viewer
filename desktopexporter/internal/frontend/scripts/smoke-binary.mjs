import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { createServer } from 'node:net'
import { resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { chromium, expect } from '@playwright/test'

// Exercise the shipped public executable, with fresh in-memory storage and no
// Vite server or API mocks. Reserve separate ports from interactive previews.
assert.ok(process.argv[2], 'Usage: node scripts/smoke-binary.mjs <executable>')
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
  assert.equal((await json(`${baseURL}/api/features`)).evals.enabled, false)
  assert.deepEqual(await json(`${baseURL}/build-features.json`), {
    evals: false,
  })

  const traceId = '0123456789abcdef0123456789abcdef'
  const spanId = '0123456789abcdef'
  const timestamp = String(BigInt(Date.now()) * 1000000n + 123n)
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
                  body: { stringValue: 'ci-smoke-log' },
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
                        asInt: '9007199254740993',
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

  browser = await chromium.launch()
  const page = await browser.newPage()
  const errors = []
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
  assert.deepEqual(errors, [])
  assertRunning()
  console.log(
    'Embedded public smoke passed: UI, feature exclusion, all-signal ingestion, service identity and exact trace export'
  )
} catch (error) {
  console.error(output)
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
