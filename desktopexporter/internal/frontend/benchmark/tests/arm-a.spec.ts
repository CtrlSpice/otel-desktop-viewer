import { readFileSync } from 'node:fs'
import { expect, test, type Page, type Request } from '@playwright/test'
import type { ArmARunContract, ArmARunResult } from '../api-types'

type FixtureManifestEntry = {
  name: string
  filename: string
  sha256: string
  bytes: number
  traceId: string
  spanCount: number
  expectedDisplayedSpanCount: number
  expectedMaximumDisplayedDepth: number
  expectedFirstSpanId: string
  topology: 'rooted-tree' | 'multiple-roots' | 'orphan' | 'cycle'
}

type FixtureManifest = {
  schemaVersion: number
  fixtures: FixtureManifestEntry[]
}

type JsonRpcResponse = {
  jsonrpc: unknown
  result?: unknown
  error?: unknown
  id: unknown
}

const manifestURL = new URL(
  '../../../cmd/waterfallbench/testdata/manifest.json',
  import.meta.url
)
const manifest = JSON.parse(
  readFileSync(manifestURL, 'utf8')
) as FixtureManifest

if (manifest.schemaVersion !== 1 || manifest.fixtures.length !== 7) {
  throw new Error(
    `unexpected checked fixture manifest: schema=${manifest.schemaVersion}, ` +
      `fixtures=${manifest.fixtures.length}`
  )
}

const SPAN_1 = '0000000000000001'
const SPAN_2 = '0000000000000002'
const SPAN_3 = '0000000000000003'
const SPAN_4 = '0000000000000004'
const MISSING_PARENT = '00000000000003e7'

function asRecord(
  value: unknown,
  description: string
): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new TypeError(`${description} must be an object`)
  }
  return value as Record<string, unknown>
}

function isRPCRequest(request: Request): boolean {
  const url = new URL(request.url())
  return url.port === '4174' && url.pathname === '/rpc'
}

function collectBrowserErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error') {
      errors.push(`console: ${message.text()}`)
    }
  })
  return errors
}

function contractFor(fixture: FixtureManifestEntry): ArmARunContract {
  return {
    fixtureName: fixture.name,
    traceID: fixture.traceId,
    expectedDisplayedSpanCount: fixture.expectedDisplayedSpanCount,
    expectedMaximumDisplayedDepth: fixture.expectedMaximumDisplayedDepth,
    expectedFirstSpanID: fixture.expectedFirstSpanId,
  }
}

function assertProductionRequest(
  requests: Request[],
  fixture: FixtureManifestEntry
): number {
  expect(requests).toHaveLength(1)
  const request = requests[0]!
  expect(request.method()).toBe('POST')
  expect(request.headers()['content-type']).toBe('application/json')
  expect(request.postDataJSON()).toEqual({
    method: 'searchSpans',
    params: { traceID: fixture.traceId },
    id: expect.any(Number),
    jsonrpc: '2.0',
  })

  const body = asRecord(request.postDataJSON(), 'JSON-RPC request body')
  expect(Number.isInteger(body.id)).toBe(true)
  return body.id as number
}

function assertBigIntProof(result: ArmARunResult): void {
  const proof = result.bigintRehydration
  expect(proof.allSpanStartAndEndTimesAreBigInts).toBe(true)
  expect(proof.allEventTimestampsAreBigInts).toBe(true)
  expect(proof.sampleExceedsNumberSafeInteger).toBe(true)

  const samples = [
    proof.firstSpanStartTime,
    proof.firstSpanEndTime,
    proof.firstEventTimestamp,
  ]
  for (const sample of samples) {
    expect(sample).not.toBeNull()
    if (sample === null) throw new Error('fixture is missing a bigint sample')
    expect(sample.type).toBe('bigint')
    expect(sample.decimal).toMatch(/^\d+$/)
    expect(BigInt(sample.decimal)).toBeGreaterThan(
      BigInt(Number.MAX_SAFE_INTEGER)
    )
  }
}

function assertProductionResponse(
  response: JsonRpcResponse,
  requestID: number
): void {
  expect(response.error).toBeUndefined()
  expect(response.jsonrpc).toBe('2.0')
  expect(response.id).toBe(requestID)
}

function assertHeadlineWireShape(response: JsonRpcResponse): void {
  const result = asRecord(response.result, 'JSON-RPC result')

  expect(result.traceStart).toEqual(expect.any(String))
  expect(
    Object.keys(asRecord(result.resources, 'result.resources')).length
  ).toBeGreaterThan(0)
  expect(
    Object.keys(asRecord(result.scopes, 'result.scopes')).length
  ).toBeGreaterThan(0)
  expect(Array.isArray(result.spans)).toBe(true)

  const firstNode = asRecord((result.spans as unknown[])[0], 'result.spans[0]')
  const firstSpan = asRecord(firstNode.spanData, 'result.spans[0].spanData')
  expect(firstSpan).toEqual(
    expect.objectContaining({
      r: expect.any(Number),
      s: expect.any(Number),
      start: expect.any(Number),
      dur: expect.any(Number),
    })
  )
  expect(firstSpan).not.toHaveProperty('traceID')
  expect(firstSpan).not.toHaveProperty('resource')
  expect(firstSpan).not.toHaveProperty('scope')
  expect(firstSpan).not.toHaveProperty('startTime')
  expect(firstSpan).not.toHaveProperty('endTime')

  const events = firstSpan.events
  expect(Array.isArray(events)).toBe(true)
  const firstEvent = asRecord(
    (events as unknown[])[0],
    'result.spans[0].spanData.events[0]'
  )
  expect(firstEvent.timestamp).toEqual(expect.any(String))
}

function assertMalformedTopology(
  fixture: FixtureManifestEntry,
  result: ArmARunResult
): void {
  if (fixture.topology === 'orphan') {
    expect(result.unplacedSpanCount).toBe(0)
    expect(result.displayRootSpanIDs).toEqual([SPAN_1, SPAN_3])
    expect(result.topology).toEqual([
      {
        spanID: SPAN_1,
        parentSpanID: null,
        depth: 0,
        salvaged: false,
        cyclePoint: false,
      },
      {
        spanID: SPAN_2,
        parentSpanID: SPAN_1,
        depth: 1,
        salvaged: false,
        cyclePoint: false,
      },
      {
        spanID: SPAN_3,
        parentSpanID: MISSING_PARENT,
        depth: 0,
        salvaged: false,
        cyclePoint: false,
      },
      {
        spanID: SPAN_4,
        parentSpanID: SPAN_3,
        depth: 1,
        salvaged: false,
        cyclePoint: false,
      },
    ])
  }

  if (fixture.topology === 'cycle') {
    expect(result.unplacedSpanCount).toBe(0)
    expect(result.displayRootSpanIDs).toEqual([SPAN_1])
    expect(result.topology).toEqual([
      {
        spanID: SPAN_1,
        parentSpanID: SPAN_3,
        depth: 0,
        salvaged: true,
        cyclePoint: true,
      },
      {
        spanID: SPAN_2,
        parentSpanID: SPAN_1,
        depth: 1,
        salvaged: true,
        cyclePoint: false,
      },
      {
        spanID: SPAN_3,
        parentSpanID: SPAN_2,
        depth: 2,
        salvaged: true,
        cyclePoint: false,
      },
    ])
  }
}

for (const fixture of manifest.fixtures) {
  test(`Arm A renders checked fixture: ${fixture.name}`, async ({ page }) => {
    const browserErrors = collectBrowserErrors(page)
    const rpcRequests: Request[] = []
    page.on('request', request => {
      if (isRPCRequest(request)) rpcRequests.push(request)
    })

    await page.goto('/benchmark/')
    expect(new URL(page.url()).port).toBe('4174')

    const responsePromise = page.waitForResponse(response =>
      isRPCRequest(response.request())
    )
    const result = await page.evaluate(
      async (contract): Promise<ArmARunResult> =>
        window.__TRACE_WATERFALL_BENCHMARK__.runArmA(contract),
      contractFor(fixture)
    )
    const rpcResponse = await responsePromise
    const rawResponse = (await rpcResponse.json()) as JsonRpcResponse

    expect(rpcResponse.status()).toBe(200)
    const requestID = assertProductionRequest(rpcRequests, fixture)
    assertProductionResponse(rawResponse, requestID)
    expect(result).toMatchObject({
      arm: 'A',
      fixtureName: fixture.name,
      traceID: fixture.traceId,
      displayedSpanCount: fixture.expectedDisplayedSpanCount,
      maximumDisplayedDepth: fixture.expectedMaximumDisplayedDepth,
      firstDisplayedSpanID: fixture.expectedFirstSpanId,
      unplacedSpanCount: 0,
    })
    expect(result.displayedSpanCount).toBe(fixture.spanCount)
    expect(result.topology).toHaveLength(fixture.expectedDisplayedSpanCount)
    expect(result.displayRootSpanIDs[0]).toBe(fixture.expectedFirstSpanId)
    expect(result.viewportGeometry.width).toBeGreaterThan(0)
    expect(result.viewportGeometry.height).toBeGreaterThan(0)
    expect(result.mountedRowCount).toBeGreaterThan(0)
    expect(result.mountedRowCount).toBeLessThanOrEqual(
      fixture.expectedDisplayedSpanCount
    )
    assertBigIntProof(result)
    expect(result.semanticHash.format).toBe(
      'odv.trace-waterfall.semantic.v1+jcs'
    )
    expect(result.semanticHash.hash).toMatch(/^[0-9a-f]{64}$/)
    expect(() => JSON.stringify(result)).not.toThrow()

    if (fixture.name === 'realistic-159') {
      assertHeadlineWireShape(rawResponse)
      expect(result.mountedRowCount).toBeLessThan(159)
    }
    assertMalformedTopology(fixture, result)
    expect(browserErrors).toEqual([])
  })
}

test('Arm A resets same-page state and rejects overlapping runs', async ({
  page,
}) => {
  const browserErrors = collectBrowserErrors(page)
  const single = manifest.fixtures.find(
    fixture => fixture.name === 'single-span'
  )
  const wide = manifest.fixtures.find(fixture => fixture.name === 'wide')
  if (!single || !wide) throw new Error('lifecycle fixtures are missing')

  await page.goto('/benchmark/')

  const overlap = await page.evaluate(
    async ({ first, second }) => {
      const firstRun = window.__TRACE_WATERFALL_BENCHMARK__.runArmA(first)
      let overlappingError = ''
      try {
        await window.__TRACE_WATERFALL_BENCHMARK__.runArmA(second)
      } catch (error) {
        overlappingError = String(error)
      }
      return { firstResult: await firstRun, overlappingError }
    },
    { first: contractFor(single), second: contractFor(wide) }
  )
  expect(overlap.firstResult.fixtureName).toBe(single.name)
  expect(overlap.overlappingError).toContain('does not permit concurrent runs')

  const invalidContract = {
    ...contractFor(wide),
    expectedMaximumDisplayedDepth: wide.expectedMaximumDisplayedDepth + 1,
  }
  await expect(
    page.evaluate(
      contract => window.__TRACE_WATERFALL_BENCHMARK__.runArmA(contract),
      invalidContract
    )
  ).rejects.toThrow(/violated its contract/)

  const firstWide = await page.evaluate(
    contract => window.__TRACE_WATERFALL_BENCHMARK__.runArmA(contract),
    contractFor(wide)
  )
  const secondWide = await page.evaluate(
    contract => window.__TRACE_WATERFALL_BENCHMARK__.runArmA(contract),
    contractFor(wide)
  )
  expect(secondWide.semanticHash).toEqual(firstWide.semanticHash)
  await expect(
    page.locator('[role="grid"][aria-label="Span waterfall"]')
  ).toHaveCount(1)
  await expect(
    page.locator('[role="grid"][aria-label="Span waterfall"]')
  ).toHaveAttribute('aria-rowcount', String(wide.expectedDisplayedSpanCount))
  expect(browserErrors).toEqual([])
})
