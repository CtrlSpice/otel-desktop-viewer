import assert from 'node:assert/strict'
import { createServer, type ServerResponse } from 'node:http'
import { createRequire } from 'node:module'
import { aggregate } from './aggregate.ts'
import { render } from './render.ts'
import { liveClient } from './live-client.ts'
import { presentationAssets } from './assets.ts'

const require = createRequire(import.meta.url)
const { chromium } =
  require('../../../desktopexporter/internal/frontend/node_modules/@playwright/test') as typeof import('../../../desktopexporter/internal/frontend/node_modules/@playwright/test/index.js')

const investigator = 'synthetic/investigator'
const grader = 'synthetic/grader'
const timestamp = '2026-10-15T12:00:00.000Z'
function fixture(attempts: number, executionErrors = 0, toolCount = 2) {
  const rows = Array.from({ length: attempts }, (_, index) => ({
    provider: { label: investigator },
    vars: { task_id: 'checkout-failure' },
    success: index >= executionErrors,
    response: { ...(index < executionErrors ? { error: 'Synthetic execution failure' } : {}), metadata: { model: investigator, elapsedSeconds: 2, sessionIDs: [`fixture-${index}`], toolCalls: toolCount, failedToolCalls: toolCount ? 1 : 0,
      telemetry: {
        traceID: (index + 1).toString(16).padStart(32, '0'),
        spanID: 'c'.repeat(16),
        tools: toolCount ? [
          { spanID: 'a'.repeat(16), name: 'bash', error: false, startedAt: '2026-10-15T12:00:00.000Z' },
          { spanID: 'b'.repeat(16), name: 'bash', error: true, startedAt: '2026-10-15T12:00:01.000Z' },
        ] : [],
      },
    } },
    gradingResult: { score: 1 },
  }))
  const data = aggregate(
    { models: [investigator], tasks: 1, repetitions: 3, synthetic: true, telemetryViewer: '' },
    { results: { results: rows } }, undefined,
    {
      sessions: attempts, executionErrors,
      models: [{ model: grader, attempts, totalSeconds: attempts, medianSeconds: 1, p95Seconds: 1, maxSeconds: 1 }],
      judgments: rows.map((_, index) => ({
        taskID: 'checkout-failure', status: index < executionErrors ? 'execution-error' : 'pass',
        score: index < executionErrors ? null : 1,
        metadata: { model: grader, elapsedSeconds: 1 }, investigator: { model: investigator },
      })),
    }
  )
  return render(data).replace('</main>', '<p class="footnote" id="live-status" role="status"></p></main>')
}
const initial = fixture(1)
assert.ok(initial.includes(`data-live-key="${investigator}"`), 'Renderer must provide investigator live keys')
assert.ok(initial.includes(`data-live-key="grader:${grader}"`), 'Renderer must provide grader live keys')
const mainHtml = (html: string) => {
  const match = html.match(/<main>([\s\S]*?)<\/main>/)
  assert.ok(match)
  return match[1]
}
const clients = new Set<ServerResponse>()
let latest = { html: mainHtml(initial), state: 'running', updatedAt: timestamp }
function send(data: unknown) {
  for (const client of clients) client.write(`event: snapshot\ndata: ${JSON.stringify(data)}\n\n`)
}
const assets = presentationAssets()
const server = createServer((request, response) => {
  const asset = assets.get(request.url?.replace(/^\/assets\//, '') ?? '')
  if (request.url?.startsWith('/assets/') && asset) {
    response.writeHead(200, { 'Content-Type': asset.type }).end(asset.body)
  } else if (request.url === '/events') {
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })
    response.write(`retry: 50\nevent: snapshot\ndata: ${JSON.stringify(latest)}\n\n`)
    clients.add(response)
    request.on('close', () => clients.delete(response))
  } else if (request.url === '/') {
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
    response.end(initial.replace('</body>', `<script>${liveClient}</script></body>`))
  } else if (request.url === '/error-fixture') {
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
    response.end(fixture(3, 2))
  } else if (request.url === '/zero-fixture') {
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
    response.end(fixture(1, 0, 0))
  } else {
    response.writeHead(404)
    response.end()
  }
})
await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
const address = server.address()
assert.ok(address && typeof address !== 'string')
const origin = `http://127.0.0.1:${address.port}`
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined
try {
  browser = await chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 700, height: 400 }, colorScheme: 'dark', timezoneId: 'America/Toronto', locale: 'en-CA' })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('request', request => {
    if (!request.url().startsWith(origin + '/')) errors.push('Unexpected external request')
  })
  await page.goto(origin)
  await page.waitForFunction(() => document.querySelector('#live-status')?.textContent?.includes('Running'))
  const model = page.locator(`details[data-live-key="${investigator}"]`)
  const grading = page.locator(`section[data-live-key="grader:${grader}"]`)
  assert.equal(await model.locator('.result-mean strong').innerText(), '33.3%')
  assert.equal(await model.locator('.run-status-badge').innerText(), 'In Progress')
  assert.equal(await model.locator('summary .grade-strip').getAttribute('aria-label'), 'Results: 1 complete · 33.3%; 2 pending · 66.7%')
   await model.locator(':scope > summary').click()
   const tools = model.locator('details[data-live-key="tool-activity-checkout-failure"]')
   const failedTools = tools.locator('li').filter({ has: page.locator('.tool-error') })
   assert.equal(await model.locator('details.task-tool-metric').count(), 1)
   assert.deepEqual(await tools.locator('.activity-counts .result-badge').allTextContents(), ['Total: 2', 'Failed: 1'])
   assert.equal(await tools.locator('.activity-counts .result-tool-error').count(), 1)
   assert.equal(await tools.locator('.activity-counts .result-execution-error').count(), 0)
   await tools.locator('summary').focus()
   await page.keyboard.press('Enter')
   assert.equal(await tools.locator('li').count(), 2)
   assert.equal(await failedTools.count(), 1)
   assert.equal(await failedTools.locator('time').innerText(), '2026-10-15 08:00:01 EDT')
   assert.equal(await failedTools.locator('time').getAttribute('datetime'), '2026-10-15T12:00:01.000Z')
   assert.equal(await failedTools.locator('time').getAttribute('title'), '2026-10-15T12:00:01.000Z')
  assert.equal(await grading.locator('summary').count(), 0)
  assert.equal(await grading.locator('.grading-table-toggle').getAttribute('aria-expanded'), 'false')
  assert.ok(await grading.locator('.grading-model-total').isVisible())
  await grading.locator('.grading-table-toggle').click()

  async function checkUpdate(metric: string, attempts: number, focus: 'summary' | 'grading' | 'metric') {
    await page.locator(`[data-view="${metric}"]`).click()
    if (focus === 'summary') await model.locator(':scope > summary').focus()
    if (focus === 'grading') await grading.locator('.grading-output').focus()
    await grading.locator('.grading-output').evaluate(element => { element.scrollLeft = 120 })
    await page.evaluate(() => window.scrollTo(0, 180))
    const before = await page.evaluate(() => ({
      x: scrollX, y: scrollY,
      horizontal: document.querySelector('.grading-output')!.scrollLeft,
      focused: {
        tag: document.activeElement?.tagName,
        key: document.activeElement?.closest('[data-live-key]')?.getAttribute('data-live-key'),
        view: document.activeElement?.getAttribute('data-view'),
        className: document.activeElement?.className,
      },
    }))
    assert.ok(before.y > 0)
    assert.ok(before.horizontal > 0)
    latest = { html: mainHtml(fixture(attempts)), state: 'running', updatedAt: timestamp }
    send(latest)
    await page.waitForFunction(count => document.querySelector('.run-meta')?.textContent?.includes(count + '/3 recorded attempts'), attempts)
    assert.equal(await model.locator('.result-mean strong').innerText(), `${(attempts / 3 * 100).toFixed(1)}%`)
    assert.equal(await model.locator('.run-status-badge').innerText(), attempts === 3 ? 'Completed' : 'In Progress')
    assert.equal(await model.locator('summary .grade-strip .result-pending').count(), attempts === 3 ? 0 : 1)
    assert.equal(await page.locator('body').getAttribute('data-metric'), metric)
    assert.equal(await page.locator(`[data-view="${metric}"]`).getAttribute('aria-pressed'), 'true')
    assert.equal(await model.getAttribute('open'), '')
    assert.equal(await tools.getAttribute('open'), '')
    assert.equal(await tools.locator('li').count(), attempts * 2)
    assert.equal(await failedTools.count(), attempts)
    assert.deepEqual(await tools.locator('.activity-counts .result-badge').allTextContents(), [`Total: ${attempts * 2}`, `Failed: ${attempts}`])
    assert.ok((await failedTools.locator('time').allTextContents()).every(text => text === '2026-10-15 08:00:01 EDT'))
    assert.equal(await grading.locator('.grading-table-toggle').getAttribute('aria-expanded'), 'true')
    assert.ok(await grading.locator('.grading-questions').isVisible())
    assert.ok(await grading.locator('.grading-model-total').isVisible())
    const after = await page.evaluate(() => ({
      x: scrollX, y: scrollY,
      horizontal: document.querySelector('.grading-output')!.scrollLeft,
      focused: {
        tag: document.activeElement?.tagName,
        key: document.activeElement?.closest('[data-live-key]')?.getAttribute('data-live-key'),
        view: document.activeElement?.getAttribute('data-view'),
        className: document.activeElement?.className,
      },
    }))
    assert.deepEqual(after, before)
  }
  await checkUpdate('time', 2, 'summary')
  await checkUpdate('cost', 3, 'grading')
  await checkUpdate('result', 2, 'metric')

  // Identical HTML must not replace controls or force page scrolling.
  await page.evaluate(() => {
    const state = window as typeof window & { liveControl?: Element | null, liveScrollCalls?: number }
    state.liveControl = document.querySelector('[data-view="result"]')
    state.liveScrollCalls = 0
    const original = window.scrollTo.bind(window)
    window.scrollTo = ((...args: Parameters<typeof window.scrollTo>) => {
      state.liveScrollCalls!++
      original(...args)
    }) as typeof window.scrollTo
  })
  send({ ...latest, state: 'complete' })
  await page.waitForFunction(() => document.querySelector('#live-status')?.textContent?.includes('Complete'))
  assert.equal(await page.evaluate(() => {
    const state = window as typeof window & { liveControl?: Element | null, liveScrollCalls?: number }
    return state.liveControl === document.querySelector('[data-view="result"]') && state.liveScrollCalls === 0
  }), true)

  const previous = await page.locator('.run-meta').innerText()
  for (const bad of [null, { ...latest, html: 123 }, { ...latest, state: 'PRIVATE_ERROR' }, { ...latest, updatedAt: 'PRIVATE_PATH' }]) {
    send(bad)
    await page.waitForFunction(() => document.querySelector('#live-status')?.textContent?.includes('Updates unavailable'))
    assert.equal(await page.locator('.run-meta').innerText(), previous)
    assert.doesNotMatch(await page.locator('#live-status').innerText(), /PRIVATE/)
    send(latest)
    await page.waitForFunction(() => !document.querySelector('#live-status')?.textContent?.includes('Updates unavailable'))
  }
  // A script in a parsed fragment must remain inert and must not reach the page.
  send({ ...latest, state: 'stopped', html: latest.html + '<script>window.liveScriptExecuted = true</script>' })
  await page.waitForFunction(() => document.querySelector('#live-status')?.textContent?.includes('Stopped'))
  assert.equal(await page.locator('main script').count(), 0)
  assert.equal(await page.evaluate(() => 'liveScriptExecuted' in window), false)
  // Check actual rendered error counts, not only synthetic CSS probes. This
  // fixture includes one numeric grade and two unscored grading errors.
  await page.goto(origin + '/error-fixture')
  await model.locator(':scope > summary').click()
   const activity = model.locator('details[data-live-key="tool-activity-checkout-failure"]')
   const executionErrors = activity.locator('li').filter({ has: page.locator('.result-execution-error') })
   await activity.locator('summary').focus()
  await page.keyboard.press('Enter')
   assert.equal(await activity.locator('li').count(), 8)
   assert.equal(await executionErrors.count(), 2)
  assert.equal(await executionErrors.locator('a[href*="/traces/"]').count(), 2)
  for (const colorScheme of ['dark', 'light'] as const) {
    await page.emulateMedia({ colorScheme })
    await page.waitForFunction(theme => document.documentElement.dataset.theme === theme,
      colorScheme === 'dark' ? 'rose-pine' : 'rose-pine-dawn')
    const summaryError = grading.locator('.grader-summary .result-execution-error')
    const metricError = grading.locator('.grader-metrics dd .result-execution-error')
    const activityError = activity.locator('.activity-counts .result-execution-error')
    const failedCount = activity.locator('.activity-counts .result-tool-error')
    assert.equal(await activityError.textContent(), 'Run errors: 2')
    assert.equal(await failedCount.textContent(), 'Failed: 3')
    assert.equal(await failedCount.evaluate(element => getComputedStyle(element, '::before').backgroundColor), colorScheme === 'dark' ? 'rgb(235, 111, 146)' : 'rgb(180, 99, 122)')
    assert.equal(await summaryError.count(), 1)
    assert.equal(await summaryError.textContent(), '2 execution errors')
    assert.equal(await metricError.count(), 1)
    assert.equal(await metricError.textContent(), '2')
    for (const badge of [summaryError, metricError, activityError]) {
      assert.ok(await badge.isVisible())
      assert.equal(await badge.evaluate(element => getComputedStyle(element, '::before').backgroundColor), 'rgb(255, 128, 0)')
    }
    await grading.locator('.grading-table-toggle').click()
    assert.equal(await grading.locator('.grading-questions .result-execution-error').count(), 2)
    assert.equal((await grading.locator('.grading-model-total .grading-score-total').allTextContents()).join(','), '0/1,0/1,1/1')
    await grading.locator('.grading-table-toggle').click()
  }
   await page.goto(origin + '/zero-fixture')
   await model.locator(':scope > summary').click()
   assert.equal(await model.locator('details.task-tool-metric').count(), 0)
   const inactive = model.locator('.inactive-metric')
   assert.equal(await inactive.locator('.chevron svg').count(), 1)
   assert.equal(await inactive.locator('summary,button,a,[tabindex]').count(), 0)
   assert.equal(await inactive.locator('.result-badge').count(), 0)
   assert.deepEqual(errors, [])
  console.log('Live results: metrics, disclosures, focus, page/table scroll, counts, invalid-update retention and rendered grader-error badges in OG/Dawn passed.')
} finally {
  await browser?.close()
  for (const client of clients) client.end()
  server.closeAllConnections()
  await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
}
