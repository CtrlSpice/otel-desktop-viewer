import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'

export function runMode(args) {
  const modes = args.filter(arg => ['--run-one', '--run-sol-luna'].includes(arg))
  if (modes.length !== 1 || new Set(args).size !== args.length || args.some(arg => !['--run-one', '--run-sol-luna', '--native-telemetry'].includes(arg)))
    throw new Error('Usage: opencode-v2-evaluate.mjs (--run-one | --run-sol-luna) [--native-telemetry]')
  const multi = modes[0] === '--run-sol-luna'
  return { mode: multi ? 'sol-luna' : 'one', models: multi ? ['gpt-6.1-sol', 'gpt-6-luna'] : ['gpt-6.1-sol'],
    repeat: multi ? 3 : 1, nativeTelemetry: multi || args.includes('--native-telemetry'), maxConcurrency: 1, cache: false }
}

export function observerURL(value) {
  const url = new URL(value)
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || url.pathname !== '/')
    throw new Error('Observer endpoints must be HTTP(S) origins without credentials, paths or query strings')
  return url.origin
}

export function runSettings(spec, env, root) {
  if (spec.mode === 'sol-luna') for (const name of ['OTEL_EVAL_RUN_DIR', 'OTEL_EVAL_OBSERVER_HTTP', 'OTEL_EVAL_OBSERVER_API', 'OTEL_EVAL_VIEWER_BINARY'])
    if (!env[name]) throw new Error('Required for --run-sol-luna: ' + name)
  const observerHTTP = observerURL(env.OTEL_EVAL_OBSERVER_HTTP ?? 'http://127.0.0.1:56010')
  const observerAPI = observerURL(env.OTEL_EVAL_OBSERVER_API ?? 'http://127.0.0.1:56008')
  const binary = env.OTEL_EVAL_VIEWER_BINARY ?? path.join(root, 'otel-desktop-viewer')
  if (!path.isAbsolute(binary) || fs.realpathSync(binary) !== fs.realpathSync(path.join(root, 'otel-desktop-viewer')))
    throw new Error('Viewer binary must be the prebuilt root/otel-desktop-viewer required by prepare()')
  const file = path.join(root, 'observer/connection.json')
  const observer = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file)) : undefined
  if (spec.mode === 'sol-luna' && !observer) throw new Error('Missing main-owned observer/connection.json')
  if (observer) {
    assert.equal(observerURL(observer.endpoint), observerAPI, 'Observer API differs from connection.json')
    assert.equal(observerURL(observer.otlpHTTP), observerHTTP, 'Observer OTLP differs from connection.json')
    assert.ok(Number.isInteger(observer.pid) && observer.pid > 0, 'Observer PID must be recorded; runner never signals it')
    assert.ok(path.isAbsolute(observer.database) && path.resolve(observer.database).startsWith(root + path.sep), 'Observer DB must be inside this fresh evidence root')
    assert.ok(!path.resolve(observer.database).startsWith(path.join(root, 'runtime') + path.sep), 'Observer DB must be separate from fixture runtime')
  }
  return { observerHTTP, observerAPI, binary, observer }
}

export function assertFreshRoot(root) {
  for (let dir = root; ; dir = path.dirname(dir)) {
    if (fs.existsSync(path.join(dir, '.git'))) throw new Error('Evidence root must be outside repositories')
    if (dir === path.dirname(dir)) break
  }
  for (const name of ['run-plan.json', 'run-started.json', 'sources', 'fixture', 'runtime', 'runs', 'otlp', 'native-otlp', 'promptfoo.json', 'promptfoo-rows.jsonl', 'promptfoo-state'])
    if (fs.existsSync(path.join(root, name))) throw new Error('Refusing to reuse run evidence: ' + name)
}

export function evaluationConfig({ suite, root, spec, task, connection, binary, authFile, authEntry, piAuthFile, observerHTTP }) {
  return {
    description: spec.mode === 'sol-luna' ? 'Actual OpenCode V2 checkout-failure; Sol and Luna, three repetitions each' : 'Actual OpenCode V2 checkout-failure; one investigator and separate native rubric grader',
    prompts: ['file://' + path.join(suite, 'prompt.txt')], tracing: { enabled: true },
    providers: spec.models.map(model => ({ id: 'file://' + path.join(suite, 'opencode-v2-provider.mjs'), label: 'OpenCode V2 ' + model,
      config: { root, model, authFile, authEntry, binary, timeoutMs: 600000, ...(spec.nativeTelemetry ? { telemetryForward: observerHTTP } : {}) } })),
    defaultTest: { vars: { ...connection, binary: './bin/otel-desktop-viewer' },
      options: { provider: { id: 'file://' + path.join(suite, 'pi-provider.ts'), config: { root, role: 'grader', provider: 'openai-codex', model: 'gpt-6.1-sol', authFile: piAuthFile, timeoutMs: 180000 } } },
      assert: [{ type: 'llm-rubric', threshold: 1, value: 'file://' + path.join(suite, 'opencode-v2-grading-evidence.mjs') }],
    },
    tests: [{ description: task.id, vars: { task_id: task.id, goal: task.goal } }],
    evaluateOptions: { repeat: spec.repeat, maxConcurrency: 1, cache: false },
    outputPath: [path.join(root, 'promptfoo.json'), path.join(root, 'promptfoo-rows.jsonl')], writeLatestResults: true,
  }
}

export function validateSchedule(rows, spec, identities) {
  assert.equal(rows.length, spec.models.length * spec.repeat, 'Scheduled result count')
  for (const key of ['id', 'traceId']) assert.equal(new Set(rows.map(row => row[key])).size, rows.length, 'Distinct result ' + key)
  const slots = new Set(), evidence = new Set(), sessions = new Set()
  for (const row of rows) {
    assert.ok(row.id); assert.match(row.traceId, /^[a-f0-9]{32}$/)
    const metadata = row.response?.metadata
    assert.ok(metadata, 'Every provider response, including errors, must retain evidence metadata')
    const model = metadata.model?.replace(/^openai\//, '')
    assert.ok(spec.models.includes(model), 'Unexpected investigator model')
    assert.equal(row.provider.id, 'opencode-v2:openai/' + model)
    assert.equal(row.provider.label, 'OpenCode V2 ' + model)
    assert.equal(row.vars.task_id, 'checkout-failure')
    const identity = identities.get(metadata.evidence)
    assert.ok(identity, 'Missing per-attempt identity')
    const repeat = identity.promptfoo.repeatIndex
    assert.ok(Number.isInteger(repeat) && repeat >= 0 && repeat < spec.repeat, 'Native repeat index required')
    if (metadata.repeatIndex !== undefined) assert.equal(metadata.repeatIndex, repeat)
    assert.equal(identity.provider, row.provider.id)
    assert.equal(identity.promptfoo.traceparent?.split('-')[1], row.traceId)
    const slot = model + '/' + repeat
    assert.ok(!slots.has(slot), 'Duplicate model/repetition slot'); slots.add(slot)
    assert.ok(!evidence.has(metadata.evidence), 'Evidence reused'); evidence.add(metadata.evidence)
    if (metadata.sessionID) { assert.ok(!sessions.has(metadata.sessionID), 'Session reused'); sessions.add(metadata.sessionID) }
    assert.ok([0, 1, 2].includes(row.failureReason), 'Keep native execution/assessment outcomes')
  }
  return [...slots].sort()
}

// Only observations are repeated. This function never submits or retries uploads.
export async function converge(read, ready, { timeoutMs = 60000, intervalMs = 250, now = () => performance.now(), sleep = delay } = {}) {
  const deadline = now() + timeoutMs
  let last, lastError
  do {
    try { last = await read(); if (ready(last)) return last; lastError = undefined }
    catch (error) { lastError = String(error) }
    if (now() >= deadline) break
    await sleep(Math.min(intervalMs, deadline - now()))
  } while (now() < deadline)
  throw new Error('Observer did not converge within deadline: ' + JSON.stringify({ last, lastError }))
}
