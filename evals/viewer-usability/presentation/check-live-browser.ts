import assert from 'node:assert/strict'
import { createServer, type ServerResponse } from 'node:http'
import { createRequire } from 'node:module'
import { aggregate } from './aggregate.ts'
import { render } from './render.ts'
import { liveClient } from './live-client.ts'

const require = createRequire(import.meta.url)
const { chromium } =
  require('../../../desktopexporter/internal/frontend/node_modules/@playwright/test') as typeof import('../../../desktopexporter/internal/frontend/node_modules/@playwright/test/index.js')

const investigator = 'synthetic/investigator'
const grader = 'synthetic/grader'
const timestamp = '2026-10-15T12:00:00.000Z'
function fixture(attempts: number) {
  const rows = Array.from({ length: attempts }, (_, index) => ({
    provider: { label: investigator },
    vars: { task_id: 'checkout-failure' },
    success: true,
    response: { metadata: { model: investigator, elapsedSeconds: 2, sessionIDs: [`fixture-${index}`] } },
    gradingResult: { score: 1 },
  }))
  const data = aggregate(
    { models: [investigator], tasks: 1, repetitions: 3, synthetic: true },
    { results: { results: rows } }, undefined,
    {
      sessions: attempts, executionErrors: 0,
      models: [{ model: grader, attempts, totalSeconds: attempts, medianSeconds: 1, p95Seconds: 1, maxSeconds: 1 }],
      judgments: rows.map(() => ({
        taskID: 'checkout-failure', status: 'pass', score: 1,
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
const server = createServer((request, response) => {
  if (request.url === '/events') {
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })
    response.write(`retry: 50\nevent: snapshot\ndata: ${JSON.stringify(latest)}\n\n`)
    clients.add(response)
    request.on('close', () => clients.delete(response))
  } else if (request.url === '/') {
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
    response.end(initial.replace('</body>', `<script>${liveClient}</script></body>`))
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
  const page = await browser.newPage({ viewport: { width: 700, height: 400 }, colorScheme: 'dark' })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('request', request => {
    if (!request.url().startsWith(origin + '/')) errors.push('Unexpected external request')
  })
  await page.goto(origin)
  await page.waitForFunction(() => document.querySelector('#live-status')?.textContent?.includes('Running'))
  const model = page.locator(`details[data-live-key="${investigator}"]`)
  const grading = page.locator(`section[data-live-key="grader:${grader}"]`)
  await model.locator('summary').click()
  assert.equal(await grading.locator('summary').count(), 0)
  assert.equal(await grading.locator('.grading-table-toggle').getAttribute('aria-expanded'), 'false')
  assert.ok(await grading.locator('.grading-model-total').isVisible())
  await grading.locator('.grading-table-toggle').click()

  async function checkUpdate(metric: string, attempts: number, focus: 'summary' | 'grading' | 'metric') {
    await page.locator(`[data-view="${metric}"]`).click()
    if (focus === 'summary') await model.locator('summary').focus()
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
    assert.equal(await page.locator('body').getAttribute('data-metric'), metric)
    assert.equal(await page.locator(`[data-view="${metric}"]`).getAttribute('aria-pressed'), 'true')
    assert.equal(await model.getAttribute('open'), '')
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
  assert.deepEqual(errors, [])
  console.log('Live results: metrics, disclosures, focus, page/table scroll, counts and invalid-update retention passed.')
} finally {
  await browser?.close()
  for (const client of clients) client.end()
  server.closeAllConnections()
  await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
}
