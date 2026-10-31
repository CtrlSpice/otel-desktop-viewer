import { routeTelemetry } from './telemetry-fixtures'
import { expect, test, type Locator, type Page } from '@playwright/test'
import { projectPromptfooDataset } from '../src/components/evals/promptfoo-adapter'
import { exampleExperiments } from '../src/components/evals/examples'

declare global {
  interface Window {
    evalRequests?: Array<{ url: string; aborted: boolean }>
  }
}

test.use({ viewport: { width: 1600, height: 900 }, reducedMotion: 'reduce' })

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>(done => (resolve = done))
  return { promise, resolve }
}

function responseGate(status = 200) {
  const requested = deferred()
  const released = deferred()
  const finished = deferred()
  return { requested, released, finished, status }
}

async function openWorkspace(
  page: Page,
  legacyBookmark = false,
  pageSize = 100
) {
  const datasetID = 'd'.repeat(64)
  const rows = ['Alpha', 'Beta', 'Gamma'].flatMap(provider =>
    Array.from({ length: 32 }, (_, index) => ({
      id: `${provider.toLowerCase()}-result-${index}`,
      provider: { id: provider },
      promptId: 'mounted-state-prompt',
      testIdx: index,
      testCase: { description: `${provider} case ${index}` },
      prompt: { raw: `${provider} question ${index}` },
      response: {
        output:
          `${provider} answer ${index}\n` +
          // Exercise source-backed Copy, including values beyond the preview limit.
          (index < 2 ? 'Exact source 🙂\\\n"é '.repeat(2200) : 'Short answer'),
      },
      gradingResult: { pass: true, reason: `${provider} explanation ${index}` },
    }))
  )
  const envelope = {
    evalId: 'mounted-state-eval',
    config: { description: 'Mounted state regression' },
    results: { version: 3, results: rows },
  }
  const projection = projectPromptfooDataset({
    id: datasetID,
    source: 'local',
    name: 'mounted-state.json',
    format: 'promptfoo-v3',
    payload: JSON.stringify(envelope),
  })
  if (projection.status !== 'supported') throw new Error(projection.reason)
  const summaries = projection.experiments.map((experiment, index) => ({
    id: String(index + 1).padStart(64, '0'),
    adapterExperimentID: experiment.id,
    datasetID,
    name: experiment.name,
    evaluation: experiment.evaluation,
    source: experiment.source,
    format: 'promptfoo-v3' as const,
    state: 'Unknown' as const,
    stateReason: 'Received rows do not establish lifecycle.',
    pauseReason: null,
    startedAt: null,
    plannedCount: null,
    resultCount: experiment.results.length,
    assessmentName: 'Recorded outcome' as const,
    outcomes: [
      {
        key: 'success:Pass',
        label: 'Pass',
        tone: 'success',
        count: experiment.results.length,
      },
    ],
  }))
  const sourceRequests: string[] = []
  const listRequests: string[] = []
  const sourceGates = new Map<string, ReturnType<typeof responseGate>>()
  const listGates = new Map<string, ReturnType<typeof responseGate>>()
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: true } } })
  )
  await routeTelemetry(page, route => {
    const request = route.request().postDataJSON()
    return route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result:
          request.method === 'getStats'
            ? {
                traces: { count: 0 },
                logs: { count: 0 },
                metrics: { count: 0 },
                rejections: [],
              }
            : request.method === 'discoverTelemetry'
              ? { rows: [], truncated: false }
              : [],
      },
    })
  })
  await page.route('**/api/evals/experiments**', async route => {
    const url = new URL(route.request().url())
    const source = url.pathname.match(
      /^\/api\/evals\/experiments\/([^/]+)\/rows$/
    )
    const criteria = JSON.parse(url.searchParams.get('criteria') ?? 'null')
    const query = criteria?.type === 'text' ? criteria.query.value : ''
    const gate = source ? sourceGates.get(source[1]) : listGates.get(query)
    if (source) sourceRequests.push(source[1])
    else if (url.searchParams.get('limit') !== '1') listRequests.push(query)
    gate?.requested.resolve()
    if (gate) await gate.released.promise
    try {
      if (gate && gate.status !== 200) {
        await route.fulfill({
          status: gate.status,
          body: 'Replacement unavailable',
        })
        return
      }
      if (!source) {
        const experiments = summaries.filter(item =>
          item.name.toLowerCase().includes(query.toLowerCase())
        )
        const offset = Number(url.searchParams.get('offset') ?? 0)
        const limit = Math.min(
          pageSize,
          Number(url.searchParams.get('limit') ?? 100)
        )
        await route.fulfill({
          json: {
            experiments: experiments.slice(offset, offset + limit),
            total: experiments.length,
            hasMore: offset + limit < experiments.length,
            facets: summaries[0].outcomes,
            indexing: {
              remaining: 0,
              unsupported: 0,
              empty: 0,
              issues: [],
              hasMoreIssues: false,
            },
          },
        })
        return
      }
      const summary = summaries.find(item => item.id === source[1])
      const experiment = projection.experiments.find(
        item => item.id === summary?.adapterExperimentID
      )
      if (!summary || !experiment) {
        await route.fulfill({ status: 404 })
        return
      }
      const resultIDs = new Set(experiment.results.map(item => item.id))
      const sourceRows = projection.rows.flatMap((entry, ordinal) =>
        resultIDs.has(entry.resultID)
          ? [{ ordinal, json: JSON.stringify(rows[ordinal]) }]
          : []
      )
      await route.fulfill({
        json: {
          summary,
          contextJSON: JSON.stringify({
            ...envelope,
            results: { version: 3, results: [] },
          }),
          rows: sourceRows,
          nextAfter: sourceRows.at(-1)?.ordinal ?? -1,
          hasMore: false,
        },
      })
    } finally {
      gate?.finished.resolve()
    }
  })
  const alpha = summaries.find(item => item.name === 'Alpha')!
  const alphaResult = projection.experiments.find(
    item => item.id === alpha.adapterExperimentID
  )!.results[0]
  const query = new URLSearchParams(
    legacyBookmark
      ? { run: alpha.id, rep: alphaResult.id }
      : { experiment: alpha.id, result: alphaResult.id }
  )
  await page.goto(`/evals?${query}`)
  await expect(
    page.getByRole('table', { name: 'Results for Alpha', exact: true })
  ).toBeVisible()
  await expect(page.locator('#signal-drawer-detail')).toContainText(
    'Alpha answer 0'
  )
  await expect.poll(() => listRequests.length).toBe(1)
  return {
    sourceRequests,
    listRequests,
    alpha,
    alphaResult,
    output: (provider: string, index: number) =>
      rows.find(
        item => item.id === `${provider.toLowerCase()}-result-${index}`
      )!.response.output,
    holdSource(name: string, status = 200) {
      const gate = responseGate(status)
      sourceGates.set(summaries.find(item => item.name === name)!.id, gate)
      return gate
    },
    holdList(query: string, status = 200) {
      const gate = responseGate(status)
      listGates.set(query, gate)
      return gate
    },
  }
}

// ElementHandles deliberately retain the original nodes; locators alone would
// silently find replacement nodes after a destructive rerender.
async function rememberNode(locator: Locator) {
  const node = await locator.elementHandle()
  if (!node) throw new Error(`Missing mounted node: ${locator}`)
  return async (current = locator) => {
    expect(
      await node.evaluate(element => element.isConnected),
      'original node remains connected'
    ).toBe(true)
    expect(
      await current.evaluate((element, original) => element === original, node),
      'same DOM node is reused'
    ).toBe(true)
  }
}

async function selectExperiment(page: Page, name: string) {
  await page
    .getByRole('button', { name: new RegExp(`^${name}`) })
    .locator('.signal-row__title')
    .click()
}

test('row selection reuses Details sections and Copy controls while updating title and values', async ({
  page,
}) => {
  const fixture = await openWorkspace(page, true)
  await expect
    .poll(() => new URL(page.url()).searchParams.get('experiment'))
    .toBe(fixture.alpha.id)
  await expect
    .poll(() => new URL(page.url()).searchParams.get('result'))
    .toBe(fixture.alphaResult.id)
  expect(new URL(page.url()).searchParams.has('run')).toBe(false)
  expect(new URL(page.url()).searchParams.has('rep')).toBe(false)

  const detail = page.locator('#signal-drawer-detail')
  const stack = detail.locator('.detail-stack:visible')
  const primary = stack.locator('details').first()
  const assessments = stack
    .locator('details')
    .filter({ has: page.locator('summary', { hasText: /^\s*Assessments/ }) })
  await assessments.locator('summary').click()
  await expect(assessments).not.toHaveAttribute('open')
  const sameStack = await rememberNode(stack)
  const samePrimary = await rememberNode(primary)
  const sameAssessments = await rememberNode(assessments)
  const copy = detail.getByRole('button', { name: 'Copy output', exact: true })
  const sameCopy = await rememberNode(copy)

  const selected = page
    .getByRole('table', { name: 'Results for Alpha', exact: true })
    .getByRole('button', { name: 'Alpha case 1', exact: true })
  await selected.click()
  await expect(stack).toHaveAccessibleName('Alpha case 1 details')
  await expect(primary.locator('summary')).toContainText('Alpha case 1')
  await expect(
    detail.getByRole('group', { name: 'output', exact: true })
  ).toContainText('Alpha answer 1')
  await expect(
    detail.getByRole('group', { name: 'result ID', exact: true })
  ).toContainText('alpha-result-1')
  await sameStack()
  await samePrimary()
  await sameAssessments()
  await sameCopy()
  await expect(primary).toHaveAttribute('open')
  await expect(assessments).not.toHaveAttribute('open')
  await expect(selected).toBeFocused()
  await copy.focus()
  await page.keyboard.press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(fixture.output('Alpha', 1))
  expect(fixture.sourceRequests).toEqual([fixture.alpha.id])
})

for (const outcome of ['pending', 'failed']) {
  test(`retained Evals controls and exact Copy remain usable after a ${outcome} replacement`, async ({
    page,
  }) => {
    const fixture = await openWorkspace(page)
    const detail = page.locator('#signal-drawer-detail')
    const copy = detail.getByRole('button', {
      name: 'Copy output',
      exact: true,
    })
    const sameCopy = await rememberNode(copy)
    const gate = fixture.holdSource('Beta', outcome === 'failed' ? 500 : 200)
    try {
      await selectExperiment(page, 'Beta')
      await gate.requested.promise
      if (outcome === 'failed') {
        gate.released.resolve()
        await expect(page.getByRole('alert')).toContainText(
          'Replacement unavailable'
        )
      } else
        await expect(
          page.getByRole('status').filter({ hasText: 'Loading experiment…' })
        ).toBeVisible()
      await sameCopy()
      await copy.focus()
      await page.keyboard.press('Enter')
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()))
        .toBe(fixture.output('Alpha', 0))
      const assessments = detail
        .locator('.detail-stack:visible details')
        .filter({
          has: page.locator('summary', { hasText: /^\s*Assessments/ }),
        })
      await assessments.locator('summary').click()
      await expect(assessments).not.toHaveAttribute('open')
      await page
        .getByRole('table', { name: 'Results for Alpha', exact: true })
        .getByRole('button', { name: 'Alpha case 1', exact: true })
        .click()
      await expect
        .poll(() => new URL(page.url()).searchParams.get('experiment'))
        .toBe(fixture.alpha.id)
      await expect(detail).toContainText('Alpha answer 1')
      gate.released.resolve()
      await gate.finished.promise
      await detail.getByRole('group', { name: 'output', exact: true }).hover()
      await copy.click()
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()))
        .toBe(fixture.output('Alpha', 1))
      await expect(
        page.getByRole('table', { name: 'Results for Alpha', exact: true })
      ).toBeVisible()
    } finally {
      gate.released.resolve()
    }
  })
}

for (const action of ['search', 'replacement', 'unmount']) {
  test(`cancels Evals pagination on ${action} and rejects its late response`, async ({
    page,
  }) => {
    await page.addInitScript(() => {
      const fetch = window.fetch
      const requests: Array<{ url: string; aborted: boolean }> = []
      Object.assign(window, { evalRequests: requests })
      window.fetch = (input, init) => {
        const request = {
          url: input instanceof Request ? input.url : String(input),
          aborted: false,
        }
        requests.push(request)
        init?.signal?.addEventListener('abort', () => {
          request.aborted = true
        })
        return fetch(input, init)
      }
    })
    const fixture = await openWorkspace(page, false, 1)
    const gate = fixture.holdList('')
    const replacement =
      action === 'replacement' ? fixture.holdList('Beta') : undefined
    try {
      if (replacement) {
        const search = page.getByRole('textbox', { name: 'Search evals' })
        await search.fill('Beta')
        await search.press('Enter')
        await replacement.requested.promise
      }
      await page
        .getByRole('button', { name: 'Load more experiments', exact: true })
        .click()
      await gate.requested.promise
      if (action === 'search') {
        const search = page.getByRole('textbox', { name: 'Search evals' })
        await search.fill('Beta')
        await search.press('Enter')
        await expect(page.getByRole('button', { name: /^Beta/ })).toBeVisible()
      } else if (replacement) {
        expect(fixture.listRequests.at(-1)).toBe('')
        replacement.released.resolve()
        await expect(page.getByRole('button', { name: /^Beta/ })).toBeVisible()
      } else {
        await page.evaluate(() => {
          window.history.pushState(null, '', '/traces')
          window.dispatchEvent(new PopStateEvent('popstate'))
        })
        await expect(
          page.getByRole('table', { name: 'Results for Alpha', exact: true })
        ).toHaveCount(0)
      }
      await expect
        .poll(() =>
          page.evaluate(() => {
            return window.evalRequests?.find(
              request =>
                new URL(request.url, location.href).searchParams.get(
                  'offset'
                ) === '1'
            )?.aborted
          })
        )
        .toBe(true)
      gate.released.resolve()
      await gate.finished.promise
      if (action !== 'unmount') {
        await expect(
          page
            .getByRole('region', { name: 'Evals list', exact: true })
            .getByRole('button', { name: /^Alpha/ })
        ).toHaveCount(0)
        await expect(
          page.getByRole('button', {
            name: 'Load more experiments',
            exact: true,
          })
        ).toHaveCount(0)
      }
    } finally {
      gate.released.resolve()
      replacement?.released.resolve()
    }
  })
}

test('retained experiment pagination uses the displayed list query after a failed replacement', async ({
  page,
}) => {
  const fixture = await openWorkspace(page, false, 1)
  const gate = fixture.holdList('Beta', 500)
  const search = page.getByRole('textbox', { name: 'Search evals' })
  await search.fill('Beta')
  await search.press('Enter')
  await gate.requested.promise
  gate.released.resolve()
  await expect(page.getByRole('alert')).toContainText('Replacement unavailable')
  const list = page.getByRole('region', { name: 'Evals list', exact: true })
  await expect(list.getByRole('button', { name: /^Alpha/ })).toBeVisible()
  await page
    .getByRole('button', { name: 'Load more experiments', exact: true })
    .click()
  await expect(list.getByRole('button', { name: /^Beta/ })).toBeVisible()
  await expect(list.getByRole('button', { name: /^Alpha/ })).toBeVisible()
  expect(fixture.listRequests.at(-1)).toBe('')
  expect(fixture.sourceRequests).toEqual([fixture.alpha.id])
  await expect(page.getByRole('alert')).toContainText('Replacement unavailable')
})

test('retained list loading feedback resets after 200ms for each submitted query', async ({
  page,
}) => {
  const fixture = await openWorkspace(page)
  const beta = fixture.holdList('Beta')
  const gamma = fixture.holdList('Gamma')
  const search = page.getByRole('textbox', { name: 'Search evals' })
  const notice = page.getByText('Loading experiments…', { exact: true })
  await page.clock.install({ time: 0 })
  await page.clock.pauseAt(1000)
  try {
    await search.fill('Beta')
    await search.press('Enter')
    await beta.requested.promise
    await page.clock.runFor(199)
    await expect(notice).toHaveCount(0)
    await page.clock.runFor(1)
    await expect(notice).toBeVisible()
    await search.fill('Gamma')
    await search.press('Enter')
    await gamma.requested.promise
    await expect(notice).toHaveCount(0)
    await page.clock.runFor(199)
    await expect(notice).toHaveCount(0)
    await expect(
      page.getByRole('table', { name: 'Results for Alpha', exact: true })
    ).toBeVisible()
    gamma.released.resolve()
    await gamma.finished.promise
    await page.clock.resume()
    await expect(page.getByRole('button', { name: /^Gamma/ })).toBeVisible()
    await expect(notice).toHaveCount(0)
    expect(fixture.sourceRequests).toEqual([fixture.alpha.id])
  } finally {
    beta.released.resolve()
    gamma.released.resolve()
  }
})

test('starts known native activity before span views and retains identified exact evidence after a failed next span', async ({
  page,
}) => {
  const experiment = exampleExperiments[0]
  const selected = experiment.results[4]
  const selectedTrace = selected.telemetry!.traceID
  const tool = selected.tools[0].telemetry!
  const references = experiment.results.flatMap(result =>
    [
      result.telemetry,
      ...result.tools.map(tool => tool.telemetry),
      ...result.assessments.map(assessment => assessment.telemetry),
    ].flatMap(reference => (reference?.spanID ? [reference] : []))
  )
  const traceIDs = [...new Set(references.map(ref => ref.traceID))]
  const views = new Map(traceIDs.map(id => [id, deferred()]))
  const summaries = new Map(traceIDs.map(id => [id, deferred()]))
  const requestedViews = new Set<string>()
  const requestedSummaries = new Set<string>()
  const requestedOwners = new Set<string>()
  const body =
    'Retained complete browser body 🙂\\\n' +
    'Exact native log evidence '.repeat(3000)
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: true } } })
  )
  await page.route('**/api/evals/experiments?*', route =>
    route.fulfill({
      json: {
        experiments: [],
        total: 0,
        hasMore: false,
        facets: [],
        indexing: {
          remaining: 0,
          unsupported: 0,
          empty: 0,
          issues: [],
          hasMoreIssues: false,
        },
      },
    })
  )
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON()
    const nativeMethod = new URL(route.originalRequest().url()).pathname
      .split('/')
      .at(-1)
    const traceID = request.params?.traceID
    if (nativeMethod === 'getTraceView') {
      requestedViews.add(traceID)
      if (views.has(traceID)) await views.get(traceID)!.promise
      else
        return route.fulfill({
          json: { error: { message: 'Next span lookup failed' } },
        })
      return route.fulfill({
        json: {
          result: {
            traceID,
            traceStart: '1700000000000000000',
            unplacedSpanCount: 0,
            resources: {
              1: {
                attributes: [
                  {
                    key: 'service.name',
                    value: { kind: 'string', value: 'browser-runner' },
                  },
                ],
                droppedAttributesCount: 0,
              },
            },
            scopes: {
              1: {
                name: 'browser',
                version: '',
                attributes: [],
                droppedAttributesCount: 0,
              },
            },
            spans: request.params.spanIDs.map((spanID: string) => ({
              depth: 0,
              matched: true,
              spanData: {
                spanID,
                parentSpanID: null,
                name:
                  spanID === tool.spanID
                    ? 'Received native tool'
                    : 'Received native span',
                traceState: '',
                flags: 0,
                start: '0',
                dur: '1000000000',
                r: 1,
                s: 1,
                kind: 'Internal',
                kindCode: 1,
                statusCode: 'Ok',
                statusCodeValue: 1,
                statusMessage: '',
                attributes: [],
                events: [],
                links: [],
                droppedAttributesCount: 0,
                droppedEventsCount: 0,
                droppedLinksCount: 0,
              },
            })),
          },
        },
      })
    }
    if (nativeMethod === 'getTraceLogSummaries') {
      requestedSummaries.add(traceID)
      if (summaries.has(traceID)) await summaries.get(traceID)!.promise
      return route.fulfill({
        json: {
          result:
            traceID === selectedTrace
              ? [
                  {
                    logRef: '1'.repeat(64),
                    spanID: tool.spanID,
                    timestamp: '1700000000500000000',
                    eventName: 'tool.output',
                    severityNumber: 0,
                    severityText: '',
                    serviceName: 'browser-runner',
                    bodyPreview: 'Ready activity',
                  },
                ]
              : [],
        },
      })
    }
    if (nativeMethod === 'getSpanLogs') {
      requestedOwners.add(`${traceID}/${request.params.spanID}`)
      return route.fulfill({
        json: {
          result: {
            logs:
              traceID === selectedTrace && request.params.spanID === tool.spanID
                ? [
                    {
                      logRef: '1'.repeat(64),
                      traceID,
                      spanID: tool.spanID,
                      timestamp: '1700000000500000000',
                      observedTimestamp: '1700000000500000000',
                      severityNumber: 0,
                      severityText: '',
                      eventName: 'tool.output',
                      body: { kind: 'string', value: body },
                      attributes: [],
                      resource: { attributes: [], droppedAttributesCount: 0 },
                      scope: {
                        name: 'browser',
                        version: '',
                        attributes: [],
                        droppedAttributesCount: 0,
                      },
                      flags: 0,
                      droppedAttributesCount: 0,
                    },
                  ]
                : [],
          },
        },
      })
    }
    if (nativeMethod === 'searchTraceSummaries')
      return route.fulfill({
        json: { error: { message: 'Summary lookup failed' } },
      })
    return route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result:
          request.method === 'getStats'
            ? {
                traces: { count: 0 },
                logs: { count: 0 },
                metrics: { count: 0 },
                rejections: [],
              }
            : [],
      },
    })
  })
  try {
    await page.goto(
      `/evals?preview=examples&experiment=${experiment.id}&result=${selected.id}&row=${encodeURIComponent(`${selected.id}/tool/0`)}`
    )
    await expect.poll(() => requestedViews.size).toBe(traceIDs.length)
    await expect.poll(() => requestedSummaries.size).toBe(traceIDs.length)
    await expect.poll(() => requestedOwners.size).toBe(references.length)
    // The fifth trace can publish without the first four, using the real Arrow decoder.
    expect(traceIDs[4]).toBe(selectedTrace)
    views.get(selectedTrace)!.resolve()
    summaries.get(selectedTrace)!.resolve()
    const detail = page.locator('#signal-drawer-detail')
    const evidence = detail.getByRole('group', { name: 'body', exact: true })
    await expect(evidence).toContainText('Retained complete browser body')
    await page.getByRole('radio', { name: 'Tool calls', exact: true }).click()
    await expect(page.locator('.timeline-marker').first()).toBeVisible()
    const copy = evidence.getByRole('button', {
      name: 'Copy body',
      exact: true,
    })
    const sameEvidence = await rememberNode(evidence)
    await copy.focus()
    await page.keyboard.press('Enter')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(body)
    const next = exampleExperiments[1]
    await page.evaluate(
      ({ id, result }) => {
        history.pushState(
          null,
          '',
          `/evals?preview=examples&experiment=${id}&result=${result}&row=${encodeURIComponent(`${result}/tool/0`)}`
        )
        dispatchEvent(new PopStateEvent('popstate'))
      },
      { id: next.id, result: next.results[0].id }
    )
    await expect(
      detail.getByText('Referenced span could not be loaded.', { exact: true })
    ).toBeVisible()
    await expect(
      detail
        .locator('.detail-stack:visible')
        .getByText('Span lookup failed', { exact: true })
    ).toBeVisible()
    await sameEvidence()
    await expect(
      evidence.locator('xpath=ancestor::table[1]').locator('caption')
    ).toContainText(tool.traceID)
    await expect(
      detail.getByRole('link').filter({ hasText: tool.traceID })
    ).toHaveCount(0)
    const nextRef = next.results[0].tools[0].telemetry!
    await expect(
      detail.locator(
        `a[href="/traces/${nextRef.traceID}?span=${nextRef.spanID}"]`
      )
    ).toBeVisible()
    await copy.focus()
    await page.keyboard.press('Enter')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(body)
    await expect(
      detail.locator('.detail-stack:visible').getByText(/0 logs/)
    ).toHaveCount(0)
  } finally {
    for (const gate of views.values()) gate.resolve()
    for (const gate of summaries.values()) gate.resolve()
  }
})

test('a delayed experiment source keeps Results mounted through loading and replacement', async ({
  page,
}) => {
  const fixture = await openWorkspace(page)
  const table = page.getByRole('table', {
    name: 'Results for Alpha',
    exact: true,
  })
  const sameTable = await rememberNode(table)
  const viewport = page.getByRole('region', {
    name: 'Results for Alpha table viewport',
    exact: true,
  })
  const sameViewport = await rememberNode(viewport)
  await viewport.evaluate(element => (element.scrollTop = 240))
  const scrollTop = await viewport.evaluate(element => element.scrollTop)
  expect(scrollTop).toBeGreaterThan(0)
  const gate = fixture.holdSource('Beta')
  const header = page.getByRole('region', {
    name: 'Evaluation results',
    exact: true,
  })
  const headerBox = await header.boundingBox()
  const viewportBox = await viewport.boundingBox()
  await page.clock.install({ time: 0 })
  await page.clock.pauseAt(1000)
  try {
    await selectExperiment(page, 'Beta')
    await gate.requested.promise
    const loading = page
      .getByRole('status')
      .filter({ hasText: 'Loading experiment…' })
    await expect(loading).toHaveCount(0)
    expect(await header.boundingBox()).toEqual(headerBox)
    expect(await viewport.boundingBox()).toEqual(viewportBox)
    await page.clock.runFor(199)
    await expect(loading).toHaveCount(0)
    await page.clock.runFor(1)
    await expect(loading).toBeVisible()
    expect(await header.boundingBox()).toEqual(headerBox)
    expect(await viewport.boundingBox()).toEqual(viewportBox)
    await page.clock.resume()
    await sameTable()
    await sameViewport()
    await expect(table).toBeVisible()
    expect(await viewport.evaluate(element => element.scrollTop)).toBe(
      scrollTop
    )
    const search = page.getByRole('textbox', { name: 'Search evals' })
    await search.focus()
    gate.released.resolve()
    const nextTable = page.getByRole('table', {
      name: 'Results for Beta',
      exact: true,
    })
    await expect(nextTable).toBeVisible()
    expect(await header.boundingBox()).toEqual(headerBox)
    expect(
      await page
        .getByRole('region', {
          name: 'Results for Beta table viewport',
          exact: true,
        })
        .boundingBox()
    ).toEqual(viewportBox)
    await sameTable(nextTable)
    await sameViewport(
      page.getByRole('region', {
        name: 'Results for Beta table viewport',
        exact: true,
      })
    )
    await expect(
      page.getByRole('region', { name: 'Evaluation results', exact: true })
    ).toContainText('Beta')
    await expect(nextTable).toContainText('Beta case 0')
    await expect(nextTable).not.toContainText('Alpha case 0')
    await expect(search).toBeFocused()
  } finally {
    gate.released.resolve()
  }
})

test('typing waits for submission and sends no source loads, preserving nodes, scroll and focus', async ({
  page,
}) => {
  const fixture = await openWorkspace(page)
  const table = page.getByRole('table', {
    name: 'Results for Alpha',
    exact: true,
  })
  const viewport = page.getByRole('region', {
    name: 'Results for Alpha table viewport',
    exact: true,
  })
  const sameTable = await rememberNode(table)
  const sameStack = await rememberNode(
    page.locator('#signal-drawer-detail .detail-stack:visible')
  )
  await viewport.evaluate(element => (element.scrollTop = 240))
  const scrollTop = await viewport.evaluate(element => element.scrollTop)
  expect(scrollTop).toBeGreaterThan(0)
  const search = page.getByRole('textbox', { name: 'Search evals' })
  const sameSearch = await rememberNode(search)
  const initialSourceRequests = [...fixture.sourceRequests]
  fixture.listRequests.length = 0
  const gate = fixture.holdList('Alpha')
  await page.clock.install({ time: new Date('2026-10-21T12:00:00Z') })
  await page.clock.pauseAt(new Date('2026-10-21T12:00:01Z'))
  try {
    await search.focus()
    await search.pressSequentially('Alpha')
    await page.clock.fastForward(500)
    expect(fixture.listRequests).toEqual([])
    await search.press('Enter')
    await gate.requested.promise
    expect(fixture.listRequests).toEqual(['Alpha'])
    await sameTable()
    await sameStack()
    await sameSearch()
    await expect(search).toBeFocused()
    expect(await viewport.evaluate(element => element.scrollTop)).toBe(
      scrollTop
    )
    gate.released.resolve()
    await gate.finished.promise
    await expect(
      page.getByRole('button', { name: /^Beta/ }).locator('.signal-row__title')
    ).toHaveCount(0)
    await page.clock.fastForward(500)
    expect(fixture.listRequests).toEqual(['Alpha'])
    expect(fixture.sourceRequests).toEqual(initialSourceRequests)
    await sameTable()
    await sameStack()
    await expect(search).toBeFocused()
    expect(await viewport.evaluate(element => element.scrollTop)).toBe(
      scrollTop
    )

    // Enter and Clear submit immediately, without loading experiment sources.
    await search.fill('Al')
    await search.press('Enter')
    await expect.poll(() => fixture.listRequests).toEqual(['Alpha', 'Al'])
    await page.clock.fastForward(500)
    expect(fixture.listRequests).toEqual(['Alpha', 'Al'])
    await page
      .getByRole('button', { name: 'Clear search', exact: true })
      .click()
    await expect.poll(() => fixture.listRequests).toEqual(['Alpha', 'Al', ''])
    await page.clock.fastForward(500)
    expect(fixture.listRequests).toEqual(['Alpha', 'Al', ''])
    expect(fixture.sourceRequests).toEqual(initialSourceRequests)
    await expect(search.locator('.cm-placeholder')).toBeVisible()
    await sameTable()
    await sameStack()
  } finally {
    gate.released.resolve()
  }
})

test('a superseded source response cannot replace the latest experiment or its Copy value', async ({
  page,
}) => {
  const fixture = await openWorkspace(page)
  const gate = fixture.holdSource('Beta')
  try {
    await selectExperiment(page, 'Beta')
    await gate.requested.promise
    await selectExperiment(page, 'Gamma')
    const table = page.getByRole('table', {
      name: 'Results for Gamma',
      exact: true,
    })
    await expect(table).toBeVisible()
    const sameTable = await rememberNode(table)
    gate.released.resolve()
    await gate.finished.promise
    await table
      .getByRole('button', { name: 'Gamma case 1', exact: true })
      .click()
    const detail = page.locator('#signal-drawer-detail')
    await expect(detail).toContainText('Gamma answer 1')
    const copy = detail.getByRole('button', {
      name: 'Copy output',
      exact: true,
    })
    await copy.focus()
    await page.keyboard.press('Enter')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(fixture.output('Gamma', 1))
    await expect(
      detail.getByRole('group', { name: 'result ID', exact: true })
    ).toContainText('gamma-result-1')
    await expect(
      page.getByRole('region', { name: 'Evaluation results', exact: true })
    ).toContainText('Gamma')
    await sameTable()
    expect(fixture.sourceRequests).toHaveLength(3)
  } finally {
    gate.released.resolve()
  }
})
