import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { once } from 'node:events'
import http from 'node:http'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { checkCurlCommand, viewerRequest } from './opencode-v2-curl.mjs'

test('curl -m uses the existing viewer request timeout policy', () => {
  const endpoint = 'http://127.0.0.1:8000'
  const url = endpoint + '/llms.txt'
  for (const seconds of ['2', '0.5']) {
    checkCurlCommand(`curl -s -m ${seconds} ${url}`, endpoint)
    const args = viewerRequest(['-s', '-m', seconds, url], endpoint)
    assert.deepEqual(args, viewerRequest(['-s', '--max-time', seconds, url], endpoint))
    assert.equal(args[args.indexOf('--max-time') + 1], seconds)
  }
  for (const args of [['-m'], ['-m', '-1', url], ['-m', '2s', url], ['-m', '2', endpoint + '/unknown']]) {
    assert.throws(() => checkCurlCommand(`curl ${args.join(' ')}`, endpoint))
    assert.throws(() => viewerRequest(args, endpoint))
  }
})

test('the curl wrapper enforces a fractional -m timeout on the assigned viewer', async t => {
  let received = false
  const server = http.createServer(() => { received = true })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  t.after(() => { server.closeAllConnections(); return new Promise(resolve => server.close(resolve)) })
  const endpoint = `http://127.0.0.1:${server.address().port}`
  const wrapper = fileURLToPath(new URL('./opencode-v2-curl.mjs', import.meta.url))
  await assert.rejects(promisify(execFile)(process.execPath, [wrapper, '-s', '-m', '0.5', endpoint + '/llms.txt'], {
    env: { OTEL_EVAL_CURL_ENDPOINT: endpoint }, timeout: 10000,
  }), { code: 28 }) // curl's transfer timeout, not the test process deadline
  assert.equal(received, true)
})
