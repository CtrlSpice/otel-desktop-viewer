import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { createHash } from 'node:crypto'
import { stageDataset } from './stage-dataset.ts'
import { prepare } from './prepare.ts'
import { verify } from './verify.ts'
import { stopOwnedGroup, runCancellation } from './run-cancellation.ts'
import { observerEnvironment } from './clean-environment.ts'
import { cancelOpenCodeV2, V2_BINARY } from './opencode-v2-provider.mjs'
import { cancelPiAttempts } from './pi-provider.ts'
import { nativeTelemetryReceiver, readNativeTelemetry, nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'

if (!process.argv.includes('--run-one') || process.argv.slice(2).some(arg => !['--run-one', '--native-telemetry'].includes(arg))) throw new Error('Usage: node opencode-v2-evaluate.mjs --run-one [--native-telemetry] (one real investigator and separate grader)')
process.umask(0o077)
const suite = import.meta.dirname
const parent = '/Users/moya/.local/share/otel-desktop-viewer/evals'
const root = fs.mkdtempSync(path.join(parent, 'opencode-v2-native-'))
const save = (name, value) => fs.writeFileSync(path.join(root, name), JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
const authFile = process.env.OTEL_EVAL_OPENCODE_AUTH_FILE ?? '/Users/moya/.local/share/opencode/auth.json'
const authEntry = process.env.OTEL_EVAL_OPENCODE_AUTH_ENTRY ?? 'openai'
const piAuthFile = process.env.OTEL_EVAL_PI_AUTH_FILE ?? path.join(parent, 'pi-trial-20261021-SZzw5b/private-auth/auth.json')
const source = path.join(parent, 'native-otel-six-gLWzRd')
const protectedFiles = [authFile, piAuthFile, ...['tasks.json', 'rubrics.json', 'prompt.txt', 'grade.ts'].map(name => path.join(suite, name)),
  ...['promptfoo.json', 'promptfoo-rows.jsonl'].map(name => path.join(source, name)),
  ...['opencode-v2-native-lTw5Rv', 'opencode-v2-native-qeYG2j'].flatMap(run =>
    ['promptfoo.json', 'promptfoo-rows.jsonl'].map(name => path.join(parent, run, name)))]
const fingerprint = () => Object.fromEntries(protectedFiles.map(file => [file, createHash('sha256').update(fs.readFileSync(file)).digest('hex')]))
const before = fingerprint()
save('source-fingerprints.json', before)
const binary = path.join(root, 'otel-desktop-viewer')
fs.copyFileSync(path.join(parent, 'pi-parent-events-NnaB1g/otel-desktop-viewer'), binary)
fs.chmodSync(binary, 0o700)
for (const file of ['opencode-v2-provider.mjs', 'opencode-v2-evidence.mjs', 'opencode-v2-stream.mjs', 'opencode-v2-telemetry.mjs', 'opencode-v2-policy.mjs', 'opencode-v2-grading-evidence.mjs', 'opencode-v2-evaluate.mjs']) fs.copyFileSync(path.join(suite, file), path.join(root, file + '.source'))
process.env.OTEL_EVAL_RUN_DIR = root
process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
const cancellation = runCancellation()
const receipts = []
let fixture, collector, nativeTelemetry, failure
const cancel = () => { void Promise.all([cancelOpenCodeV2(), cancelPiAttempts()]) }
cancellation.signal.addEventListener('abort', cancel, { once: true })
try {
  if (process.argv.includes('--native-telemetry')) nativeTelemetry = await nativeTelemetryReceiver(root, 'http://127.0.0.1:56010')
  fs.mkdirSync(path.join(root, 'otlp'), { mode: 0o700 })
  collector = http.createServer(async (req, res) => {
    try {
      const route = new URL(req.url, 'http://127.0.0.1').pathname
      if (!['/v1/traces', '/v1/logs', '/v1/metrics'].includes(route)) { res.writeHead(404).end(); return }
      const chunks = []; for await (const chunk of req) chunks.push(chunk)
      const body = Buffer.concat(chunks)
      const name = String(receipts.length).padStart(5, '0') + '-' + route.split('/').at(-1) + '.otlp'
      const receipt = { name, route, contentType: req.headers['content-type'], contentEncoding: req.headers['content-encoding'], bytes: body.length }
      receipts.push(receipt)
      fs.writeFileSync(path.join(root, 'otlp', name), body, { flag: 'wx', mode: 0o600 })
      const response = await fetch('http://127.0.0.1:56010' + route, { method: 'POST', headers: {
        'Content-Type': receipt.contentType ?? 'application/json', ...(receipt.contentEncoding ? { 'Content-Encoding': receipt.contentEncoding } : {}),
      }, body, signal: AbortSignal.timeout(30000) })
      const reply = await response.text()
      Object.assign(receipt, { status: response.status, reply })
      fs.appendFileSync(path.join(root, 'otlp-receipts.jsonl'), JSON.stringify(receipt) + '\n', { mode: 0o600 })
      res.writeHead(response.status).end(reply)
    } catch (error) { fs.appendFileSync(path.join(root, 'otlp-errors.log'), String(error) + '\n'); res.writeHead(500).end() }
  })
  collector.listen(0, '127.0.0.1'); await once(collector, 'listening')
  process.env.OTEL_EVAL_OBSERVER_HTTP = 'http://127.0.0.1:' + collector.address().port
  Object.assign(process.env, observerEnvironment())
  stageDataset(path.join(suite, '../../testdata/otlp/small'), path.join(root, 'fixture'))
  fixture = await prepare(root, undefined, cancellation.signal)
  await verify('verification', root)
  const connection = JSON.parse(fs.readFileSync(path.join(root, 'connection.json'), 'utf8'))
  const task = JSON.parse(fs.readFileSync(path.join(suite, 'tasks.json'), 'utf8')).find(task => task.id === 'checkout-failure')
  const config = {
    description: 'Actual OpenCode V2 checkout-failure; one investigator and separate native rubric grader',
    prompts: ['file://' + path.join(suite, 'prompt.txt')], tracing: { enabled: true },
    providers: [{ id: 'file://' + path.join(suite, 'opencode-v2-provider.mjs'), label: 'OpenCode V2 gpt-6.1-sol',
      config: { root, model: 'gpt-6.1-sol', opencodeBinary: V2_BINARY, authFile, authEntry, binary, timeoutMs: 600000, telemetryEndpoint: nativeTelemetry?.endpoint } }],
    defaultTest: {
      vars: { ...connection, binary: './bin/otel-desktop-viewer' },
      options: { provider: { id: 'file://' + path.join(suite, 'pi-provider.ts'), config: { root, role: 'grader', provider: 'openai-codex', model: 'gpt-6.1-sol', authFile: piAuthFile, timeoutMs: 180000 } } },
      assert: [{ type: 'llm-rubric', threshold: 1, value: 'file://' + path.join(suite, 'opencode-v2-grading-evidence.mjs') }],
    },
    tests: [{ description: task.id, vars: { task_id: task.id, goal: task.goal } }],
    evaluateOptions: { repeat: 1, maxConcurrency: 1, cache: false },
    outputPath: [path.join(root, 'promptfoo.json'), path.join(root, 'promptfoo-rows.jsonl')], writeLatestResults: true,
  }
  save('run-plan.json', { task, investigators: 1, graders: 1, repeat: 1, retry: false, model: 'openai/gpt-6.1-sol', version: '2.0.29',
    grader: 'pi:openai-codex/gpt-6.1-sol', observer: 'http://127.0.0.1:56008', config })
  console.log(JSON.stringify({ root, state: 'evaluating-one', version: '2.0.29' }))
  const { evaluate } = await import('promptfoo')
  await evaluate(config, { ...config.evaluateOptions, share: false, abortSignal: cancellation.signal })
  const exported = JSON.parse(fs.readFileSync(path.join(root, 'promptfoo.json'), 'utf8'))
  if (!exported.evalId || !exported.config || exported.results.results.length !== 1) throw new Error('Missing native one-row full export')
  const [row] = exported.results.results
  save('result-summary.json', { evalId: exported.evalId, id: row.id, provider: row.provider, success: row.success, score: row.score,
    failureReason: row.failureReason, error: row.error, traceId: row.traceId, gradingResult: row.gradingResult })
  if (row.error || row.failureReason === 2) throw new Error(row.error ?? 'Native execution error; see full export')
} catch (error) {
  failure = String(error)
  save('runner-error.json', { error: failure })
  process.exitCode = 1
} finally {
  cancellation.signal.removeEventListener('abort', cancel)
  await Promise.all([cancelOpenCodeV2(), cancelPiAttempts()])
  if (fixture) { await stopOwnedGroup(fixture.pid, fixture.process); save('owned-shutdown.json', { pid: fixture.pid, exit: await fixture.closed }) }
  if (collector) { collector.closeAllConnections(); await new Promise(resolve => collector.close(resolve)) }
  if (nativeTelemetry) {
    try {
      await nativeTelemetry.close()
      const runs = path.join(root, 'runs')
      const investigators = fs.existsSync(runs) ? fs.readdirSync(runs).filter(name => name.startsWith('investigator-opencode-v2-')) : []
      const identityFile = investigators.length === 1 ? path.join(runs, investigators[0], 'identity.json') : undefined
      const identity = identityFile && fs.existsSync(identityFile) ? JSON.parse(fs.readFileSync(identityFile)) : undefined
      save('native-telemetry-summary.json', nativeTelemetryBoundaries(readNativeTelemetry(root, nativeTelemetry.receipts), identity?.promptfoo.traceparent))
    } catch (error) {
      save('native-telemetry-error.json', { error: String(error), rawBytesRetained: true })
      failure ??= 'Native telemetry verification failed; see native-telemetry-error.json'
      process.exitCode = 1
    }
  }
  const after = fingerprint()
  save('source-preservation.json', { unchanged: JSON.stringify(before) === JSON.stringify(after), files: after })
  save('otlp-summary.json', { receipts })
  fs.writeFileSync(path.join(root, 'handoff.md'), `# Actual OpenCode V2 one-question pass\n\nRoot: ${root}\n\nStatus: ${failure ? 'execution blocker; see runner-error.json and full received context/error bodies' : 'completed; see native promptfoo.json and result-summary.json'}\n\nOne scheduled investigator; separate Pi Sol grader only after valid investigator output. No retries. V2 2.0.29 private server, native V2 API, isolated profile and filesystem policy. Raw context, session export, SSE and tools are under runs/. OTLP receipts are under otlp/. Original sources were fingerprinted.\n`, { mode: 0o600, flag: 'wx' })
  cancellation.dispose()
  console.log(JSON.stringify({ root, status: failure ? 'blocked' : 'completed', handoff: path.join(root, 'handoff.md') }))
}
