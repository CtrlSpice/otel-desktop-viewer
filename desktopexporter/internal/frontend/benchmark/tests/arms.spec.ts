import { readFileSync } from 'node:fs'
import { expect, test, type Page, type Request } from '@playwright/test'
import { ARM_C_FLAT_FORMAT, ARM_C_FLAT_PATH } from '../api-types'
import type {
  ArmRunContract,
  ArmRunResult,
  FixtureTopology,
} from '../api-types'

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
  topology: FixtureTopology
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

function isArmCRequest(request: Request): boolean {
  const url = new URL(request.url())
  return url.port === '4174' && url.pathname === ARM_C_FLAT_PATH
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

function contractFor(fixture: FixtureManifestEntry): ArmRunContract {
  return {
    fixtureName: fixture.name,
    fixtureSHA256: fixture.sha256,
    fixtureBytes: fixture.bytes,
    inputSpanCount: fixture.spanCount,
    fixtureTopology: fixture.topology,
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

function assertArmCRequest(
  requests: Request[],
  fixture: FixtureManifestEntry
): void {
  expect(requests).toHaveLength(1)
  const request = requests[0]!
  expect(request.method()).toBe('POST')
  expect(request.headers()['content-type']).toBe('application/json')
  expect(request.postDataJSON()).toEqual({ traceID: fixture.traceId })
}

function assertBigIntProof(result: ArmRunResult): void {
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

function exactKeys(
  value: unknown,
  description: string,
  expected: string[]
): Record<string, unknown> {
  const record = asRecord(value, description)
  expect(Object.keys(record).sort()).toEqual([...expected].sort())
  return record
}

function assertArmCWireShape(
  response: unknown,
  fixture: FixtureManifestEntry
): void {
  const trace = exactKeys(response, 'Arm C response', [
    'format',
    'resources',
    'rows',
    'scopes',
    'traceID',
  ])
  expect(trace.format).toBe(ARM_C_FLAT_FORMAT)
  expect(trace.traceID).toBe(fixture.traceId)
  const resources = asRecord(trace.resources, 'Arm C resources')
  const scopes = asRecord(trace.scopes, 'Arm C scopes')
  expect(Object.keys(resources).length).toBeGreaterThan(0)
  expect(Object.keys(scopes).length).toBeGreaterThan(0)
  expect(Array.isArray(trace.rows)).toBe(true)
  const rows = trace.rows as unknown[]
  expect(rows).toHaveLength(fixture.spanCount)

  for (const [index, candidate] of rows.entries()) {
    const row = exactKeys(candidate, `Arm C rows[${index}]`, [
      'attributes',
      'droppedAttributesCount',
      'droppedEventsCount',
      'droppedLinksCount',
      'endTime',
      'events',
      'flags',
      'kind',
      'links',
      'name',
      'parentSpanID',
      'resourceRef',
      'scopeRef',
      'spanID',
      'startTime',
      'statusCode',
      'statusMessage',
      'traceState',
    ])
    expect(row.startTime).toEqual(expect.any(String))
    expect(row.endTime).toEqual(expect.any(String))
    expect(resources).toHaveProperty(String(row.resourceRef))
    expect(scopes).toHaveProperty(String(row.scopeRef))
    expect(Array.isArray(row.attributes)).toBe(true)
    expect(Array.isArray(row.events)).toBe(true)
    expect(Array.isArray(row.links)).toBe(true)
    for (const event of row.events as unknown[]) {
      expect(asRecord(event, 'Arm C event').timestamp).toEqual(
        expect.any(String)
      )
    }
  }
}

function assertArmResult(
  result: ArmRunResult,
  fixture: FixtureManifestEntry,
  arm: 'A' | 'C'
): void {
  expect(result).toMatchObject({
    arm,
    fixtureName: fixture.name,
    fixtureSHA256: fixture.sha256,
    fixtureBytes: fixture.bytes,
    inputSpanCount: fixture.spanCount,
    fixtureTopology: fixture.topology,
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
  expect(result.semanticHash.format).toBe('odv.trace-waterfall.semantic.v1+jcs')
  expect(result.semanticHash.hash).toMatch(/^[0-9a-f]{64}$/)
  expect(() => JSON.stringify(result)).not.toThrow()
  if (fixture.name === 'realistic-159') {
    expect(result.mountedRowCount).toBeLessThan(159)
  }
  assertMalformedTopology(fixture, result)
}

function assertMalformedTopology(
  fixture: FixtureManifestEntry,
  result: ArmRunResult
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

function runArm(
  page: Page,
  arm: 'A' | 'C',
  contract: ArmRunContract
): Promise<ArmRunResult> {
  return page.evaluate(
    async ({ selectedArm, runContract }): Promise<ArmRunResult> =>
      selectedArm === 'A'
        ? window.__TRACE_WATERFALL_BENCHMARK__.runArmA(runContract)
        : window.__TRACE_WATERFALL_BENCHMARK__.runArmC(runContract),
    { selectedArm: arm, runContract: contract }
  )
}

for (const fixture of manifest.fixtures) {
  test(`Arms A and C agree for checked fixture: ${fixture.name}`, async ({
    page,
  }) => {
    const browserErrors = collectBrowserErrors(page)
    const rpcRequests: Request[] = []
    const armCRequests: Request[] = []
    page.on('request', request => {
      if (isRPCRequest(request)) rpcRequests.push(request)
      if (isArmCRequest(request)) armCRequests.push(request)
    })

    await page.goto('/benchmark/')
    expect(new URL(page.url()).port).toBe('4174')

    const armAResponsePromise = page.waitForResponse(response =>
      isRPCRequest(response.request())
    )
    const armA = await runArm(page, 'A', contractFor(fixture))
    const armAResponse = await armAResponsePromise
    const rawArmA = (await armAResponse.json()) as JsonRpcResponse

    const armCResponsePromise = page.waitForResponse(response =>
      isArmCRequest(response.request())
    )
    const armC = await runArm(page, 'C', contractFor(fixture))
    const armCResponse = await armCResponsePromise
    const rawArmC: unknown = await armCResponse.json()

    expect(armAResponse.status()).toBe(200)
    const requestID = assertProductionRequest(rpcRequests, fixture)
    assertProductionResponse(rawArmA, requestID)
    assertArmResult(armA, fixture, 'A')
    if (fixture.name === 'realistic-159') assertHeadlineWireShape(rawArmA)

    expect(armCResponse.status()).toBe(200)
    assertArmCRequest(armCRequests, fixture)
    assertArmCWireShape(rawArmC, fixture)
    assertArmResult(armC, fixture, 'C')

    expect(armC.semanticHash).toEqual(armA.semanticHash)
    expect(armC.topology).toEqual(armA.topology)
    expect(armC.displayRootSpanIDs).toEqual(armA.displayRootSpanIDs)
    expect(armC.displayedSpanCount).toBe(armA.displayedSpanCount)
    expect(armC.maximumDisplayedDepth).toBe(armA.maximumDisplayedDepth)
    expect(browserErrors).toEqual([])
  })
}

test('both arms share same-page lifecycle and overlap protection', async ({
  page,
}) => {
  const browserErrors = collectBrowserErrors(page)
  const single = manifest.fixtures.find(
    fixture => fixture.name === 'single-span'
  )
  const wide = manifest.fixtures.find(fixture => fixture.name === 'wide')
  if (!single || !wide) throw new Error('lifecycle fixtures are missing')

  await page.goto('/benchmark/')

  const aThenCOverlap = await page.evaluate(
    async ({ first, second }) => {
      const firstRun = window.__TRACE_WATERFALL_BENCHMARK__.runArmA(first)
      let overlappingError = ''
      try {
        await window.__TRACE_WATERFALL_BENCHMARK__.runArmC(second)
      } catch (error) {
        overlappingError = String(error)
      }
      return { firstResult: await firstRun, overlappingError }
    },
    { first: contractFor(single), second: contractFor(wide) }
  )
  expect(aThenCOverlap.firstResult.fixtureName).toBe(single.name)
  expect(aThenCOverlap.overlappingError).toContain(
    'does not permit concurrent runs'
  )

  const cThenAOverlap = await page.evaluate(
    async ({ first, second }) => {
      const firstRun = window.__TRACE_WATERFALL_BENCHMARK__.runArmC(first)
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
  expect(cThenAOverlap.firstResult.fixtureName).toBe(single.name)
  expect(cThenAOverlap.overlappingError).toContain(
    'does not permit concurrent runs'
  )

  const failedRoute = `**${ARM_C_FLAT_PATH}`
  await page.route(failedRoute, route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
  )
  await expect(runArm(page, 'C', contractFor(wide))).rejects.toThrow(
    /flatTrace must contain exactly/
  )
  await page.unroute(failedRoute)

  const invalidContract = {
    ...contractFor(wide),
    expectedMaximumDisplayedDepth: wide.expectedMaximumDisplayedDepth + 1,
  }
  await expect(
    page.evaluate(
      contract => window.__TRACE_WATERFALL_BENCHMARK__.runArmC(contract),
      invalidContract
    )
  ).rejects.toThrow(/violated its contract/)

  const firstA = await runArm(page, 'A', contractFor(wide))
  const firstC = await runArm(page, 'C', contractFor(wide))
  const secondA = await runArm(page, 'A', contractFor(wide))
  const secondC = await runArm(page, 'C', contractFor(wide))
  expect(secondA.semanticHash).toEqual(firstA.semanticHash)
  expect(secondC.semanticHash).toEqual(firstC.semanticHash)
  expect(firstC.semanticHash).toEqual(firstA.semanticHash)
  expect(secondC.semanticHash).toEqual(secondA.semanticHash)
  await expect(
    page.locator('[role="grid"][aria-label="Span waterfall"]')
  ).toHaveCount(1)
  await expect(
    page.locator('[role="grid"][aria-label="Span waterfall"]')
  ).toHaveAttribute('aria-rowcount', String(wide.expectedDisplayedSpanCount))
  expect(browserErrors).toEqual([])
})
