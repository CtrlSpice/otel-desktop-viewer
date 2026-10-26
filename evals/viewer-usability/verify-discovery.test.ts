import test from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'
import { once } from 'node:events'
import { verifyDiscovery } from './verify-discovery.ts'

const start = 1791359675000000000n
const end = 1791360275000000000n

test('root discovery follows advertised paths and obtains the actual counter result', async () => {
  const calls: string[] = []
  let htmlFallback = false
  let value = '104'
  const server = http.createServer(async (req, res) => {
    calls.push(req.method + ' ' + req.url)
    if (req.url === '/') {
      res.setHeader('Link', '</description.json>; rel="service-desc"')
      res.end('<html>Viewer</html>')
    } else if (req.url === '/description.json') {
      res.setHeader('Content-Type', htmlFallback ? 'text/html' : 'application/json')
      res.end(htmlFallback ? '<html>Viewer</html>' : JSON.stringify({
        openrpc: '1.3.2', servers: [{ url: '/discovered-rpc' }],
        methods: [{ name: 'query', params: [{ name: 'sql' }, { name: 'limit' }] }],
      }))
    } else if (req.url === '/discovered-rpc') {
      let body = ''
      for await (const chunk of req) body += chunk
      const request = JSON.parse(body)
      assert.equal(req.method, 'POST')
      assert.equal(request.method, 'query')
      assert.equal(request.params.limit, 1)
      assert.match(request.params.sql, /http\.server\.request\.count/)
      assert.ok(request.params.sql.includes(start + '::UBIGINT'))
      assert.ok(request.params.sql.includes(end + '::UBIGINT'))
      res.setHeader('Content-Type', 'application/json')
      res.end(JSON.stringify({
        jsonrpc: '2.0', id: request.id,
        result: { columns: [], rows: [[
          'http.server.request.count', 2, true, String(start), String(end), value,
        ]], truncated: false },
      }))
    } else {
      res.writeHead(404).end()
    }
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const address = server.address()
  assert.ok(address && typeof address !== 'string')
  const endpoint = `http://127.0.0.1:${address.port}`
  try {
    const proof = await verifyDiscovery(endpoint, start, end)
    assert.equal(proof.rpcURL, endpoint + '/discovered-rpc')
    assert.deepEqual(calls, [
      'GET /', 'GET /description.json', 'POST /discovered-rpc',
    ])
    value = '101'
    await assert.rejects(verifyDiscovery(endpoint, start, end))
    htmlFallback = true
    const before = calls.length
    await assert.rejects(
      verifyDiscovery(endpoint, start, end), /something other than JSON/
    )
    assert.deepEqual(calls.slice(before), ['GET /', 'GET /description.json'])
  } finally {
    server.closeAllConnections()
    await new Promise<void>(resolve => server.close(() => resolve()))
  }
})
