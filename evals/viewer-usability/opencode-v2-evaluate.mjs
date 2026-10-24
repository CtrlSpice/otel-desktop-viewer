import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { stageDataset } from './stage-dataset.ts'
import { prepare } from './prepare.ts'
import { verify } from './verify.ts'
import { stopOwnedGroup, runCancellation } from './run-cancellation.ts'
import { observerEnvironment } from './clean-environment.ts'
import { cancelOpenCodeV2 } from './opencode-v2-provider.mjs'
import { cancelPiAttempts } from './pi-provider.ts'
import { nativeTelemetryReceiver, nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'
import { runMode, runSettings, assertFreshRoot, evaluationConfig, validateSchedule } from './opencode-v2-run-plan.mjs'
import { readJSON, sha256, observerTotals, loadRunTelemetry, awaitTelemetry } from './opencode-v2-observer.mjs'

const spec = runMode(process.argv.slice(2))
process.umask(0o077)
const suite = import.meta.dirname
const parent = '/Users/moya/.local/share/otel-desktop-viewer/evals'
if (process.env.OTEL_EVAL_RUN_DIR && !path.isAbsolute(process.env.OTEL_EVAL_RUN_DIR)) throw new Error('Run root must be absolute')
if (spec.mode === 'sol-luna' && !process.env.OTEL_EVAL_RUN_DIR) throw new Error('--run-sol-luna requires the precreated OTEL_EVAL_RUN_DIR')
const root = process.env.OTEL_EVAL_RUN_DIR ? fs.realpathSync(process.env.OTEL_EVAL_RUN_DIR) : fs.mkdtempSync(path.join(parent, 'opencode-v2-native-'))
assertFreshRoot(root)
if (!process.env.OTEL_EVAL_RUN_DIR && !process.env.OTEL_EVAL_VIEWER_BINARY) {
  fs.copyFileSync(path.join(parent, 'pi-parent-events-NnaB1g/otel-desktop-viewer'), path.join(root, 'otel-desktop-viewer'), fs.constants.COPYFILE_EXCL)
  fs.chmodSync(path.join(root, 'otel-desktop-viewer'), 0o700)
}
const settings = runSettings(spec, process.env, root)
const save = (name, value) => fs.writeFileSync(path.join(root, name), JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
save('run-started.json', { spec, startedAt: new Date().toISOString(), coordinatorPID: process.pid })
save('observer-target.json', settings)
const authFile = process.env.OTEL_EVAL_OPENCODE_AUTH_FILE ?? '/Users/moya/.local/share/opencode/auth.json'
const authEntry = process.env.OTEL_EVAL_OPENCODE_AUTH_ENTRY ?? 'openai'
const piAuthFile = process.env.OTEL_EVAL_PI_AUTH_FILE ?? path.join(parent, 'pi-trial-20261021-SZzw5b/private-auth/auth.json')
const protectedFiles = [...new Set([authFile, piAuthFile, settings.binary,
  ...['tasks.json', 'rubrics.json', 'prompt.txt', 'grade.ts', 'pi-provider.ts'].map(name => path.join(suite, name)),
  ...['native-otel-six-gLWzRd', 'opencode-v2-native-lTw5Rv', 'opencode-v2-native-qeYG2j', 'opencode-v2-native-eQUlOk'].flatMap(run =>
    ['promptfoo.json', 'promptfoo-rows.jsonl'].map(name => path.join(parent, run, name))).filter(file => fs.existsSync(file)),
  ...(settings.observer ? [path.join(root, 'observer/connection.json')] : []),
])]
const fingerprint = () => Object.fromEntries(protectedFiles.map(file => [file, sha256(fs.readFileSync(file))]))
const before = fingerprint(); save('source-fingerprints.json', before)
fs.mkdirSync(path.join(root, 'sources'), { mode: 0o700 })
const sources = {}
for (const item of fs.readdirSync(suite, { withFileTypes: true })) {
  if (!item.isFile() || !/\.(mjs|ts|json|txt|md|yaml|yml)$/.test(item.name)) continue
  const bytes = fs.readFileSync(path.join(suite, item.name))
  fs.writeFileSync(path.join(root, 'sources', item.name), bytes, { flag: 'wx', mode: 0o600 })
  sources[item.name] = sha256(bytes)
}
save('source-snapshot.json', { directory: 'sources', files: sources, binarySHA256: sha256(fs.readFileSync(settings.binary)) })
process.env.OTEL_EVAL_RUN_DIR = root
process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
const cancellation = runCancellation()
let fixture, collector, failure, rows = []
const cancel = () => { void Promise.all([cancelOpenCodeV2(), cancelPiAttempts()]) }
cancellation.signal.addEventListener('abort', cancel, { once: true })
try {
  const baseline = await observerTotals(settings.observerAPI)
  if (spec.mode === 'sol-luna') assert.deepEqual(baseline, [0, 0, 0], 'Sol/Luna requires a fresh empty observer DB')
  save('observer-baseline.json', { counts: baseline, fresh: spec.mode === 'sol-luna' })
  collector = await nativeTelemetryReceiver(root, settings.observerHTTP, { directoryName: 'otlp', origin: 'promptfoo-wrapper' })
  process.env.OTEL_EVAL_OBSERVER_HTTP = collector.endpoint
  Object.assign(process.env, observerEnvironment())
  stageDataset(path.join(suite, '../../testdata/otlp/small'), path.join(root, 'fixture'))
  fixture = await prepare(root, undefined, cancellation.signal)
  await verify('verification', root)
  const connection = readJSON(path.join(root, 'connection.json'))
  assert.notEqual(connection.endpoint, settings.observerAPI, 'Fixture must be independent from observer')
  const task = readJSON(path.join(suite, 'tasks.json')).find(task => task.id === 'checkout-failure')
  const config = evaluationConfig({ suite, root, spec, task, connection, binary: settings.binary, authFile, authEntry, piAuthFile, observerHTTP: settings.observerHTTP })
  save('run-plan.json', { ...spec, task, investigators: spec.models.length * spec.repeat, maxGraders: spec.models.length * spec.repeat,
    retry: false, version: '2.0.29', grader: 'pi:openai-codex/gpt-6.1-sol', observer: settings.observerAPI, observerHTTP: settings.observerHTTP, config })
  console.log(JSON.stringify({ root, state: 'evaluating', mode: spec.mode, investigators: spec.models.length * spec.repeat }))
  const { evaluate } = await import('promptfoo')
  await evaluate(config, { ...config.evaluateOptions, share: false, abortSignal: cancellation.signal })
  const exported = readJSON(path.join(root, 'promptfoo.json'))
  save('original-export-hashes.json', Object.fromEntries(['promptfoo.json', 'promptfoo-rows.jsonl'].map(name => [name, sha256(fs.readFileSync(path.join(root, name)))])))
  assert.ok(exported.evalId && exported.config)
  rows = exported.results.results
  const identities = new Map(rows.map(row => [row.response?.metadata?.evidence, readJSON(path.join(row.response.metadata.evidence, 'identity.json'))]))
  validateSchedule(rows, spec, identities)
  // Execution errors and Fail/Partial assessments remain rows; no fail-fast,
  // replacement, extra attempt or score rewriting occurs here.
  save('result-summary.json', { evalId: exported.evalId, mode: spec.mode, results: rows.map(row => ({ id: row.id, provider: row.provider,
    repeatIndex: row.response.metadata.repeatIndex, success: row.success, score: row.score, failureReason: row.failureReason,
    error: row.error, traceId: row.traceId, gradingResult: row.gradingResult })) })
} catch (error) {
  failure = String(error); save('runner-error.json', { error: failure }); process.exitCode = 1
} finally {
  cancellation.signal.removeEventListener('abort', cancel)
  const cleanupErrors = []
  const cleanup = async action => { try { await action() } catch (error) { cleanupErrors.push(String(error)) } }
  await cleanup(() => Promise.all([cancelOpenCodeV2(), cancelPiAttempts()]))
  if (fixture) await cleanup(async () => { await stopOwnedGroup(fixture.pid, fixture.process); save('owned-shutdown.json', { pid: fixture.pid, exit: await fixture.closed }) })
  if (collector) await cleanup(async () => { await collector.close(); save('otlp-summary.json', { receipts: collector.receipts }) })
  if (cleanupErrors.length) { save('cleanup-errors.json', cleanupErrors); failure ??= 'Owned cleanup failed'; process.exitCode = 1 }
  await cleanup(async () => {
    const telemetry = loadRunTelemetry(root)
    const native = [...telemetry.attempts].map(([evidence, data]) => ({ evidence,
      ...nativeTelemetryBoundaries(data, readJSON(path.join(evidence, 'identity.json')).promptfoo.traceparent) }))
    save('native-telemetry-summary.json', { attempts: native, spans: native.reduce((sum, item) => sum + item.spans, 0), logs: native.reduce((sum, item) => sum + item.logs, 0) })
    save('observer-convergence.json', await awaitTelemetry(settings.observerAPI, telemetry, { fresh: spec.mode === 'sol-luna' }))
  })
  if (cleanupErrors.length && !failure) { failure = cleanupErrors.join('\n'); process.exitCode = 1 }
  if (cleanupErrors.length) save('finalization-errors.json', cleanupErrors)
  const after = fingerprint()
  const sourcesUnchanged = Object.entries(sources).every(([name, hash]) => sha256(fs.readFileSync(path.join(suite, name))) === hash)
  const unchanged = JSON.stringify(before) === JSON.stringify(after) && sourcesUnchanged
  save('source-preservation.json', { unchanged, sourcesUnchanged, files: after })
  if (!unchanged) { failure ??= 'Source preservation check failed'; process.exitCode = 1 }
  save('run-completed.json', { mode: spec.mode, scheduled: spec.models.length * spec.repeat, results: rows.length, failure,
    executionErrors: rows.filter(row => row.failureReason === 2).length, completedAt: new Date().toISOString() })
  fs.writeFileSync(path.join(root, 'handoff.md'), `# OpenCode V2 ${spec.mode} evaluation\n\nRoot: ${root}\n\nStatus: ${failure ?? 'Scheduling completed; inspect each native result outcome'}\n\n${spec.models.join(', ')}; ${spec.repeat} repetitions; ${rows.length} rows; ${rows.filter(row => row.failureReason === 2).length} execution errors. Separate approved Pi Sol graders after valid investigator outputs. No retries.\n\nObserver API: ${settings.observerAPI}\nOTLP destination: ${settings.observerHTTP}\nMain owns observer process/database; runner stops only its own fixture, V2 servers and relays.\n\nOriginal Promptfoo exports are at root. Wrapper raw batches: otlp/. Per-investigator native batches: runs/investigator-opencode-v2-*/native-otlp/. Full source snapshot and hashes: sources/, source-snapshot.json. Native outcomes, IDs and grader evidence: result-summary.json. Flush verification: observer-convergence.json. Sources/credentials: source-preservation.json.\n\nRun opencode-v2-verify.mjs ${root} --import once to verify and import original bytes. Never replay OTLP.\n`, { flag: 'wx', mode: 0o600 })
  cancellation.dispose()
  console.log(JSON.stringify({ root, status: failure ? 'blocked' : 'completed', results: rows.length, handoff: path.join(root, 'handoff.md') }))
}
