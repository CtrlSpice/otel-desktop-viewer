import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { readLiveData } from './live-data.ts'
import type { LiveSelection, LiveState } from './live-data.ts'
import { render, escapeHtml } from './render.ts'
import { presentationAssets } from './assets.ts'
import { liveClient } from './live-client.ts'

type Snapshot = { html: string; state: LiveState; updatedAt: string }
const savedFiles = new Set(['run-manifest.json', 'pricing-snapshot.json', 'pilot.json', 'grading.json', 'metrics.json', 'failure.json',
  'execution-error.json', 'harness-blocker.json', 'launch.json', 'run-completion.json', 'run-failure.json',
  'pilot-completion.json', 'overnight-completion.json', 'overnight-failure.json', 'completed-runs.json'])

export async function serveResults(selection: LiveSelection) {
  const roots = [...new Set(selection.runs.map(root => fs.realpathSync(root)))]
  if (roots.length === 0) throw new Error('Select at least one saved run directory')
  const manifest = fs.realpathSync(selection.manifest)
  const assets = presentationAssets()
  const clients = new Set<http.ServerResponse>()
  let snapshot: Snapshot
  let document = ''
  const update = () => {
    try {
      const { data, recoveries, state } = readLiveData({ manifest, runs: roots })
      const notes = recoveries.flatMap(recovery => recovery.models.map(model =>
        `<p class="footnote">Recovery · ${escapeHtml(model.model)} · ${model.result.displayPass} complete, ${model.result.displayPartial} partial, ${model.result.displayFail} incorrect, ${model.result.error ? `<span class="badge badge-soft result-badge result-execution-error">${model.result.error} execution errors</span>` : '0 execution errors'}. Original attempts remain in the table.</p>`)).join('')
      const rendered = render(data).replace('</header>', '</header><p class="footnote" id="live-status" role="status"></p>' + notes)
      const start = rendered.indexOf('<main>') + '<main>'.length
      const end = rendered.lastIndexOf('</main>')
      if (start < '<main>'.length || end < start) throw new Error('Missing results content')
      const html = rendered.slice(start, end)
      if (snapshot?.html === html && snapshot.state === state) return
      snapshot = { html, state, updatedAt: new Date().toISOString() }
      document = rendered.replace('</body>', `<script>${liveClient}</script></body>`)
    } catch {
      if (!snapshot) throw new Error('Cannot read the selected saved evaluation results')
      // A partially written report must not replace the last complete display.
      if (snapshot.state === 'unavailable') return
      snapshot = { ...snapshot, state: 'unavailable' }
    }
    for (const client of clients) client.write('event: snapshot\ndata: ' + JSON.stringify(snapshot) + '\n\n')
  }
  update()
  const server = http.createServer((request, response) => {
    const address = server.address()
    const host = address && typeof address !== 'string' ? `127.0.0.1:${address.port}` : ''
    if (request.headers.host !== host || (request.headers.origin && request.headers.origin !== `http://${host}`)) {
      response.writeHead(403).end(); return
    }
    if (request.method !== 'GET') { response.writeHead(405, { Allow: 'GET' }).end(); return }
    response.setHeader('Cache-Control', 'no-store')
    response.setHeader('X-Content-Type-Options', 'nosniff')
    const asset = assets.get(request.url?.replace(/^\/assets\//, '') ?? '')
    if (request.url?.startsWith('/assets/') && asset) {
      response.writeHead(200, { 'Content-Type': asset.type }).end(asset.body)
    } else if (request.url === '/') {
      response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' }).end(document)
    } else if (request.url === '/events') {
      response.writeHead(200, { 'Content-Type': 'text/event-stream; charset=utf-8', Connection: 'keep-alive' })
      response.write('event: snapshot\ndata: ' + JSON.stringify(snapshot) + '\n\n')
      clients.add(response)
      response.on('close', () => clients.delete(response))
    } else response.writeHead(404).end()
  })
  let queued: NodeJS.Immediate | undefined
  const changed = (_event: string, filename: string | Buffer | null) => {
    if (filename !== null && !savedFiles.has(path.basename(String(filename)))) return
    if (!queued) queued = setImmediate(() => { queued = undefined; update() })
  }
  const watchers: fs.FSWatcher[] = []
  try {
    for (const root of roots) watchers.push(fs.watch(root, { recursive: true }, changed))
    watchers.push(fs.watch(manifest, () => changed('change', null)))
    for (const watcher of watchers) watcher.on('error', () => {
      snapshot = { ...snapshot, state: 'unavailable' }
      for (const client of clients) client.write('event: snapshot\ndata: ' + JSON.stringify(snapshot) + '\n\n')
    })
    server.listen(0, '127.0.0.1')
    await once(server, 'listening')
    update() // Cover files written while watchers were being attached.
  } catch (error) {
    for (const watcher of watchers) watcher.close()
    server.close()
    throw error
  }
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('Missing local server address')
  return {
    url: `http://127.0.0.1:${address.port}`,
    close: async () => {
      if (queued) clearImmediate(queued)
      for (const watcher of watchers) watcher.close()
      for (const client of clients) client.end()
      server.closeAllConnections()
      await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
    },
  }
}

if (process.argv[1] && fs.realpathSync(process.argv[1]) === import.meta.filename) {
  const [manifest, ...runs] = process.argv.slice(2)
  if (!manifest || !runs.length) throw new Error('Usage: node presentation/serve.ts MANIFEST.json RUN_DIRECTORY [RUN_DIRECTORY ...]')
  const server = await serveResults({ manifest, runs })
  console.log(server.url)
  const close = () => { void server.close().then(() => { process.exitCode = 0 }) }
  process.once('SIGINT', close)
  process.once('SIGTERM', close)
}
