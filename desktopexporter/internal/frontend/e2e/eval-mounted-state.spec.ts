import { expect, test, type Locator, type Page } from '@playwright/test'
import { projectPromptfooDataset } from '../src/components/evals/promptfoo-adapter'

test.use({ viewport: { width: 1600, height: 900 }, reducedMotion: 'reduce' })

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>(done => (resolve = done))
  return { promise, resolve }
}

function responseGate() {
  const requested = deferred()
  const released = deferred()
  const finished = deferred()
  return { requested, released, finished }
}

async function openWorkspace(page: Page, legacyBookmark = false) {
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
  await page.route('**/rpc', route => {
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
      if (!source) {
        const experiments = summaries.filter(item =>
          item.name.toLowerCase().includes(query.toLowerCase())
        )
        await route.fulfill({
          json: {
            experiments,
            total: experiments.length,
            hasMore: false,
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
    holdSource(name: string) {
      const gate = responseGate()
      sourceGates.set(summaries.find(item => item.name === name)!.id, gate)
      return gate
    },
    holdList(query: string) {
      const gate = responseGate()
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
  await page.clock.install({ time: 0 })
  await page.clock.pauseAt(1000)
  try {
    await selectExperiment(page, 'Beta')
    await gate.requested.promise
    const loading = page
      .getByRole('status')
      .filter({ hasText: 'Loading experiment…' })
    await expect(loading).toHaveCount(0)
    await page.clock.runFor(199)
    await expect(loading).toHaveCount(0)
    await page.clock.runFor(1)
    await expect(loading).toBeVisible()
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
