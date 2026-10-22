import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { TEMPORARY } from './runtime.ts'

test('native llm-rubric retains scores, reasons and grader errors in the full export', async () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'native-grading-'))
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'state')
  const server = http.createServer(async (req, res) => {
    let body = ''
    for await (const chunk of req) body += chunk
    const request = JSON.parse(body)
    const content = JSON.stringify(request.messages).includes('MALFORMED') ? 'invalid verdict'
      : JSON.stringify({ pass: true, score: 0.5, reason: 'Recorded partial credit' })
    res.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({
      choices: [{ message: { role: 'assistant', content }, finish_reason: 'stop' }],
      usage: { prompt_tokens: 5, completion_tokens: 3, total_tokens: 8 },
    }))
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const address = server.address()
  assert.ok(address && typeof address === 'object')
  try {
    const packageName = 'promptfoo'
    const { evaluate } = await import(packageName)
    await evaluate({
      prompts: ['{{answer}}'], providers: ['echo'],
      defaultTest: {
        options: { provider: { id: 'openai:chat:fixture', config: { apiBaseUrl: `http://127.0.0.1:${address.port}`, apiKey: 'local-fixture' } } },
        assert: [{ type: 'llm-rubric', value: 'Use the recorded criterion', threshold: 1 }],
      },
      tests: ['PARTIAL', 'MALFORMED'].map(answer => ({ vars: { answer } })),
      outputPath: [path.join(root, 'promptfoo.json')], writeLatestResults: true,
    }, { cache: false, maxConcurrency: 1, share: false })
    const result = JSON.parse(fs.readFileSync(path.join(root, 'promptfoo.json'), 'utf8'))
    const [partial, malformed] = result.results.results
    assert.equal(partial.response.output, 'PARTIAL')
    assert.equal(partial.score, 0.5)
    assert.equal(partial.success, false)
    assert.equal(partial.gradingResult.componentResults[0].reason, 'Recorded partial credit')
    assert.equal(partial.gradingResult.componentResults[0].assertion.type, 'llm-rubric')
    assert.equal(malformed.gradingResult.componentResults[0].metadata.graderError, true)
    assert.equal(new Set(result.results.results.map((row: { id: string }) => row.id)).size, 2)
  } finally {
    server.closeAllConnections()
    await new Promise<void>(resolve => server.close(() => resolve()))
  }
})
