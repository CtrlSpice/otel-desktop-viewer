import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import cases from './tasks.json' with { type: 'json' }

export function fullRunPlan() {
  const models = [
    'opencode-go/minimax-m3.1-flash', 'opencode-go/deepseek-v4.1-flash',
    'opencode-go/glm-5.3-flash',
    'kimi-code-plan-global/k3', 'kimi-code-plan-global/k3.1-pro',
    'openai/gpt-6.2-sol', 'openai/gpt-6.1-sol', 'openai/gpt-6.1-terra',
    'anthropic/claude-haiku-5-5', 'anthropic/claude-sonnet-5-5', 'anthropic/claude-opus-5-5',
  ]
  return { mode: 'full', models, taskIDs: ['checkout-failure', 'trace-logs', 'customer-count', 'request-count', 'trace-export'],
    repeat: 3, nativeTelemetry: true, maxConcurrency: 1, cache: false }
}

export function runMode(args) {
  if (args.length === 1 && args[0] === '--run-full') return fullRunPlan()
  const modes = args.filter(arg => ['--run-one', '--run-sol-luna'].includes(arg))
  if (modes.length !== 1 || new Set(args).size !== args.length || args.some(arg => !['--run-one', '--run-sol-luna', '--native-telemetry'].includes(arg)))
    throw new Error('Usage: opencode-v2-evaluate.mjs --run-full | (--run-one | --run-sol-luna) [--native-telemetry]')
  const multi = modes[0] === '--run-sol-luna'
  return { mode: multi ? 'sol-luna' : 'one', models: multi ? ['gpt-6.1-sol', 'gpt-6-luna'] : ['gpt-6.1-sol'],
    repeat: multi ? 3 : 1, nativeTelemetry: multi || args.includes('--native-telemetry'), maxConcurrency: 1, cache: false }
}

export function scheduleForAvailability(plan, availability) {
  assert.deepEqual(availability.map(m => `${m.providerID}/${m.id}`), plan.models, 'Availability must cover the planned roster')
  assert.ok(availability.every(m => typeof m.enabled === 'boolean'), 'Missing model availability')
  const models = plan.models.filter((_, i) => availability[i].enabled)
  const notStarted = plan.models.filter((_, i) => !availability[i].enabled).flatMap(model =>
    plan.taskIDs.flatMap(task => Array.from({ length: plan.repeat }, (_, repeatIndex) =>
      ({ model, task, repeatIndex, status: 'not started', reason: 'model unavailable' }))))
  return { models, notStarted }
}

export function observerURL(value) {
  const url = new URL(value)
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || url.pathname !== '/')
    throw new Error('Observer endpoints must be HTTP(S) origins without credentials, paths or query strings')
  return url.origin
}

export function runSettings(spec, env, root) {
  const batch = spec.mode === 'sol-luna' || spec.mode === 'full'
  if (batch) for (const name of ['OTEL_EVAL_RUN_DIR', 'OTEL_EVAL_OBSERVER_HTTP', 'OTEL_EVAL_OBSERVER_API', 'OTEL_EVAL_VIEWER_BINARY'])
    if (!env[name]) throw new Error('Required for --run-' + spec.mode + ': ' + name)
  const observerHTTP = observerURL(env.OTEL_EVAL_OBSERVER_HTTP ?? 'http://127.0.0.1:56010')
  const observerAPI = observerURL(env.OTEL_EVAL_OBSERVER_API ?? 'http://127.0.0.1:56008')
  const binary = env.OTEL_EVAL_VIEWER_BINARY ?? path.join(root, 'otel-desktop-viewer')
  if (!path.isAbsolute(binary) || fs.realpathSync(binary) !== fs.realpathSync(path.join(root, 'otel-desktop-viewer')))
    throw new Error('Viewer binary must be the prebuilt root/otel-desktop-viewer required by prepare()')
  const file = path.join(root, 'observer/connection.json')
  const observer = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file)) : undefined
  if (batch && !observer) throw new Error('Missing main-owned observer/connection.json')
  if (observer) {
    assert.equal(observerURL(observer.endpoint), observerAPI, 'Observer API differs from connection.json')
    assert.equal(observerURL(observer.otlpHTTP), observerHTTP, 'Observer OTLP differs from connection.json')
    assert.ok(Number.isInteger(observer.pid) && observer.pid > 0, 'Observer PID must be recorded; runner never signals it')
    assert.ok(path.isAbsolute(observer.database))
    if (spec.mode !== 'full') assert.ok(path.resolve(observer.database).startsWith(root + path.sep), 'Observer DB must be inside this fresh evidence root')
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
  const full = spec.mode === 'full'
  const tasks = full ? spec.taskIDs.map(id => cases.find(c => c.id === id)) : [task]
  return {
    description: full ? 'OpenCode 2.3 viewer usability' : spec.mode === 'sol-luna' ? 'Actual OpenCode V2 checkout-failure; Sol and Luna, three repetitions each' : 'Actual OpenCode V2 checkout-failure; one investigator and separate native rubric grader',
    prompts: ['file://' + path.join(suite, 'prompt.txt')], tracing: { enabled: true },
    providers: spec.models.map(ref => {
      const [providerID, model] = full ? ref.split('/') : ['openai', ref]
      return { id: 'file://' + path.join(suite, 'opencode-v2-provider.mjs'), label: 'OpenCode V2 ' + ref,
        config: { root, providerID, model, maxRetries: 0, ...(full ? { sharedHost: true, fixtureEndpoint: connection?.endpoint } : { authFile, authEntry }), binary,
          timeoutMs: 600000, ...(spec.nativeTelemetry ? { telemetryForward: observerHTTP } : {}) } }
    }),
    defaultTest: { vars: { ...connection, binary: './bin/otel-desktop-viewer', ...(full ? { sharedShell: true } : {}) },
      options: { provider: { id: 'file://' + path.join(suite, 'opencode-v2-judge.mjs'), config: { root, maxRetries: 0, authFile: piAuthFile } } },
      assert: [{ type: 'llm-rubric', threshold: 1, rubricPrompt: JSON.stringify([{ role: 'user', content: '{{rubric}}' }]),
        value: 'file://' + path.join(suite, 'opencode-v2-grading-evidence.mjs') }],
    },
    tests: tasks.map(task => ({ description: task.id, vars: { task_id: task.id, goal: task.goal },
      ...(task.id === 'trace-export' ? {
        assertScoringFunction: 'file://' + path.join(suite, 'score-export.mjs'),
        assert: [{ type: 'javascript', metric: 'export-file', value: 'file://' + path.join(suite, 'verify-artifact.ts') }],
      } : {}),
    })),
    evaluateOptions: { repeat: spec.repeat, maxConcurrency: 1, cache: false },
    outputPath: [path.join(root, 'promptfoo.json'), path.join(root, 'promptfoo-rows.jsonl')], writeLatestResults: true,
  }
}

export function validateSchedule(rows, spec, identities) {
  const tasks = spec.taskIDs ?? ['checkout-failure']
  assert.equal(rows.length, spec.models.length * tasks.length * spec.repeat, 'Scheduled result count')
  for (const key of ['id', 'traceId']) assert.equal(new Set(rows.map(row => row[key])).size, rows.length, 'Distinct result ' + key)
  const slots = new Set(), evidence = new Set(), sessions = new Set()
  for (const row of rows) {
    assert.ok(row.id); assert.match(row.traceId, /^[a-f0-9]{32}$/)
    const metadata = row.response?.metadata
    assert.ok(metadata, 'Every provider response, including errors, must retain evidence metadata')
    const model = spec.mode === 'full' ? metadata.model : metadata.model?.replace(/^openai\//, '')
    assert.ok(spec.models.includes(model), 'Unexpected investigator model')
    assert.equal(row.provider.id, 'opencode-v2:' + (spec.mode === 'full' ? model : 'openai/' + model))
    assert.equal(row.provider.label, 'OpenCode V2 ' + model)
    assert.ok(tasks.includes(row.vars.task_id), 'Unexpected question')
    const identity = identities.get(metadata.evidence)
    assert.ok(identity, 'Missing per-attempt identity')
    if (spec.mode === 'full') {
      assert.equal(identity.task, row.vars.task_id)
      assert.equal(metadata.task, row.vars.task_id)
    }
    const repeat = identity.promptfoo.repeatIndex
    assert.ok(Number.isInteger(repeat) && repeat >= 0 && repeat < spec.repeat, 'Native repeat index required')
    if (metadata.repeatIndex !== undefined) assert.equal(metadata.repeatIndex, repeat)
    assert.equal(identity.provider, row.provider.id)
    assert.equal(identity.promptfoo.traceparent?.split('-')[1], row.traceId)
    const slot = spec.mode === 'full' ? JSON.stringify([model, row.vars.task_id, repeat]) : model + '/' + repeat
    assert.ok(!slots.has(slot), 'Duplicate model/repetition slot'); slots.add(slot)
    assert.ok(!evidence.has(metadata.evidence), 'Evidence reused'); evidence.add(metadata.evidence)
    if (metadata.sessionID) { assert.ok(!sessions.has(metadata.sessionID), 'Session reused'); sessions.add(metadata.sessionID) }
    assert.ok([0, 1, 2].includes(row.failureReason), 'Keep native execution/assessment outcomes')
  }
  return [...slots].sort()
}

function hasGraderError(result) {
  return result?.metadata?.graderError === true || result?.componentResults?.some(hasGraderError) === true
}

export function isExecutionError(row) {
  return row.failureReason === 2 || hasGraderError(row.gradingResult)
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
