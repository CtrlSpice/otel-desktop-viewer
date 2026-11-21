import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { randomUUID, createHash } from 'node:crypto'
import { pathToFileURL } from 'node:url'
import ClaudeProvider, { cancelClaudeAttempts } from './claude-provider.ts'
import { ownedPath } from './evidence.ts'
import { saveJson } from './runtime.ts'
import { nativeTelemetryReceiver } from './opencode-v2-telemetry.mjs'
import { importOriginal } from './opencode-v2-verify.mjs'
import { runCancellation } from './run-cancellation.ts'

export const GPT_MODELS = ['openai/gpt-6.2-astra', 'openai/gpt-6.2-sol']
export const OPUS_MODEL = 'claude-opus-5-5'
const sha = value => createHash('sha256').update(value).digest('hex')
const read = file => JSON.parse(fs.readFileSync(file, 'utf8'))

function externalRoot(directory) {
  assert.ok(path.isAbsolute(directory), 'Use an absolute external run directory')
  const root = fs.realpathSync(directory)
  for (let dir = root; ; dir = path.dirname(dir)) {
    assert.ok(!fs.existsSync(path.join(dir, '.git')), 'Evidence must remain outside repositories')
    if (dir === path.dirname(dir)) return root
  }
}

export function selectGPTResponses(source, plan) {
  assert.equal(plan.mode, 'full')
  assert.equal(plan.repeat, 3)
  assert.equal(plan.taskIDs.length, 11)
  const selected = source.results.results.filter(row => GPT_MODELS.includes(row.response?.metadata?.model))
  assert.equal(selected.length, GPT_MODELS.length * plan.taskIDs.length * plan.repeat, 'Wait for all planned GPT responses')
  const slots = new Set()
  for (const row of selected) {
    const metadata = row.response.metadata
    assert.equal(metadata.runtime, 'opencode-v2')
    assert.equal(metadata.role, 'investigator')
    assert.equal(metadata.task, row.vars.task_id)
    assert.ok(plan.taskIDs.includes(metadata.task))
    assert.ok(Number.isInteger(metadata.repeatIndex) && metadata.repeatIndex >= 0 && metadata.repeatIndex < plan.repeat)
    const slot = JSON.stringify([metadata.model, metadata.task, metadata.repeatIndex])
    assert.ok(!slots.has(slot), 'Repeated original GPT slot')
    slots.add(slot)
  }
  return selected
}

export function savedGradingInput(root, sourceID, row) {
  const metadata = row.response.metadata
  const evidence = ownedPath(root, metadata.evidence)
  assert.equal(path.dirname(evidence), path.join(root, 'runs'))
  const files = ['identity.json', 'response.json']
  if (!row.response.error) files.push('context.json', 'tools.json', 'grading-packet.json')
  const hashes = Object.fromEntries(files.map(name => {
    const file = ownedPath(root, path.join(evidence, name))
    return [file, sha(fs.readFileSync(file))]
  }))
  const identity = read(path.join(evidence, 'identity.json'))
  assert.equal(identity.promptfoo.evaluationId, sourceID)
  assert.equal(identity.promptfoo.repeatIndex, metadata.repeatIndex)
  const response = read(path.join(evidence, 'response.json'))
  assert.deepEqual(response.metadata, metadata)
  assert.equal(response.output, row.response.output)
  assert.equal(response.error, row.response.error)
  if (response.error) return { row, hashes, skipped: 'investigator-error' }
  const packet = read(path.join(evidence, 'grading-packet.json'))
  const tools = read(path.join(evidence, 'tools.json'))
  assert.equal(packet.evidence.answerSHA256, sha(response.output))
  assert.equal(packet.evidence.investigator.evidence, evidence)
  assert.equal(packet.evidence.investigator.model, metadata.model)
  assert.equal(packet.evidence.investigator.taskID, metadata.task)
  assert.deepEqual(packet.evidence.events.map(event => JSON.parse(event.raw)), tools)
  assert.ok(tools.every(tool => tool.sessionID === metadata.sessionID))
  for (const [name, hash] of Object.entries(packet.evidence.sources))
    assert.equal(hashes[path.join(evidence, name)], hash, 'Saved grading evidence changed')
  const components = row.gradingResult?.componentResults ?? []
  const rubric = components.find(component => component.assertion?.type === 'llm-rubric')
  assert.ok(rubric, 'Original rubric assessment is required before comparison grading')
  // The export hash authenticates the complete rendered text. Historical
  // renderers differ in whitespace, so do not regenerate it with current code.
  const criterion = rubric.metadata?.renderedAssertionValue
  assert.ok(typeof criterion === 'string' && criterion.endsWith('\n' + JSON.stringify(packet)),
    'Original rendered criterion must contain the saved evidence packet')
  return { row, hashes, criterion, components, identity }
}

export function comparisonExport(source, selected, judgments, id, timestamp) {
  const results = selected.map(row => {
    const grade = judgments.get(row.id)
    const comparison = { evaluationId: source.evalId, resultID: row.id, grader: `claude-code:${OPUS_MODEL}` }
    if (!grade) {
      assert.ok(row.response.error, 'A successful investigation must have its requested Opus judgment')
      return { ...row, metadata: { ...row.metadata, comparison: { ...comparison, skipped: 'investigator-error' } } }
    }
    const { error: originalError, ...original } = row
    const graderError = grade.componentResults.some(component => component.metadata?.graderError)
    return { ...original, gradingResult: grade, success: grade.pass, score: grade.score,
      failureReason: graderError ? 2 : grade.pass ? 0 : 1,
      ...(graderError ? { error: grade.reason } : {}),
      metadata: { ...row.metadata, comparison } }
  })
  // This is a derived comparison report, not an untouched Promptfoo export.
  // Keep received investigator responses and IDs; do not copy stale aggregate metrics.
  return { version: 3, evalId: id,
    config: { description: `GPT answers from ${source.evalId}, graded by Claude Opus 5.5`,
      metadata: { comparisonOf: source.evalId, grader: `claude-code:${OPUS_MODEL}`, derivedComparison: true } },
    results: { version: 3, timestamp, results } }
}

export function savedCriterion(_output, context) {
  const criterion = context.vars.__opusComparisonCriterion
  assert.equal(typeof criterion, 'string', 'Saved criterion must remain literal text')
  return criterion
}

export async function gradeSavedInput(input, grader, runAssertions) {
  const assertions = input.components.map(component => {
    if (component.assertion.type === 'llm-rubric') {
      const { provider, ...assertion } = component.assertion
      // Use the same file-function boundary as the original grading pass.
      // A plain string would make Nunjucks interpret recorded {{...}} and {%...%}.
      return { ...assertion, value: 'file://' + import.meta.filename + ':savedCriterion' }
    }
    // Artifact checks describe the saved investigation, not another execution.
    // Let Promptfoo combine their recorded results with the new model judgment.
    return { type: 'javascript', weight: component.assertion.weight,
      value: () => ({ ...component, metadata: { ...component.metadata, reusedOriginalAssertion: true } }) }
  })
  const result = await runAssertions({ providerResponse: input.row.response, prompt: input.row.prompt?.raw,
    test: { vars: { ...input.row.vars, __opusComparisonCriterion: input.criterion },
      assert: assertions, options: { provider: grader }, threshold: 1 } })
  for (const [index, component] of result.componentResults.entries())
    if (component.metadata?.reusedOriginalAssertion) component.assertion = input.components[index].assertion
  return result
}

export function loadCompletedSource(directory) {
  const root = externalRoot(directory)
  assert.ok(fs.existsSync(path.join(root, 'run-completed.json')), 'The original run is still running; do not change it')
  assert.equal(read(path.join(root, 'source-preservation.json')).unchanged, true)
  const plan = read(path.join(root, 'run-plan.json'))
  const source = read(path.join(root, 'promptfoo.json'))
  const hashes = {}
  const check = file => { hashes[file] = sha(fs.readFileSync(file)); return hashes[file] }
  for (const name of ['run-completed.json', 'source-preservation.json', 'run-plan.json', 'source-snapshot.json', 'original-export-hashes.json'])
    check(path.join(root, name))
  const exportHashes = read(path.join(root, 'original-export-hashes.json'))
  assert.ok(exportHashes['promptfoo.json'], 'Original export fingerprint required')
  for (const [name, hash] of Object.entries(exportHashes))
    assert.equal(check(ownedPath(root, path.join(root, name))), hash, 'Original native export changed')
  const snapshot = read(path.join(root, 'source-snapshot.json'))
  for (const [name, hash] of Object.entries(snapshot.files))
    assert.equal(check(ownedPath(root, path.join(root, 'sources', name))), hash, 'Original source snapshot changed')
  for (const name of ['grade.ts', 'tasks.json', 'rubrics.json'])
    assert.ok(Object.hasOwn(snapshot.files, name), 'Missing original grading source: ' + name)
  const selected = selectGPTResponses(source, plan)
  const inputs = selected.map(row => savedGradingInput(root, source.evalId, row))
  for (const input of inputs) Object.assign(hashes, input.hashes)
  return { root, source, selected, inputs, hashes, plan }
}

export async function runRegrading(sourceDirectory, outputDirectory, env = process.env) {
  const saved = loadCompletedSource(sourceDirectory)
  const root = externalRoot(outputDirectory)
  assert.notEqual(root, saved.root)
  assert.equal(fs.readdirSync(root).length, 0, 'Use a fresh comparison directory; never repeat a grading attempt')
  const binary = env.OTEL_EVAL_CLAUDE_BINARY
  const configDir = env.OTEL_EVAL_CLAUDE_CONFIG_DIR
  assert.ok(binary && path.isAbsolute(binary), 'Set OTEL_EVAL_CLAUDE_BINARY to the installed evaluation CLI')
  assert.ok(configDir && path.isAbsolute(configDir), 'Set OTEL_EVAL_CLAUDE_CONFIG_DIR to the existing subscription profile')
  const comparisonID = `eval-opus-${randomUUID()}`
  saveJson(path.join(root, 'run-started.json'), { source: saved.root, evaluationId: saved.source.evalId,
    comparisonID, grader: OPUS_MODEL, selected: saved.selected.map(row => row.id), originalHashes: saved.hashes,
    cli: binary, cliSHA256: sha(fs.readFileSync(binary)), configurationDirectory: configDir, startedAt: new Date().toISOString() })
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
  const { assertions, cache } = await import('promptfoo')
  cache.disableCache()
  const receiver = await nativeTelemetryReceiver(root, saved.plan.observerHTTP, { directoryName: 'otlp-opus', origin: 'claude-grader' })
  const cancellation = runCancellation()
  const cancel = () => { void cancelClaudeAttempts() }
  cancellation.signal.addEventListener('abort', cancel, { once: true })
  const judgments = new Map()
  let failure
  try {
    for (const input of saved.inputs) {
      cancellation.signal.throwIfAborted()
      if (input.skipped) continue
      const row = input.row
      const source = { evaluationId: saved.source.evalId, resultID: row.id, repeatIndex: row.response.metadata.repeatIndex,
        traceparent: input.identity.promptfoo.traceparent }
      const provider = new ClaudeProvider({ config: { root, role: 'grader', model: OPUS_MODEL, cliBinary: binary,
        configDir, source, telemetryEndpoint: receiver.endpoint, timeoutMs: 180000 } })
      const grade = await gradeSavedInput(input, provider, assertions.runAssertions)
      judgments.set(row.id, grade)
      fs.appendFileSync(path.join(root, 'grades.jsonl'), JSON.stringify({ source, gradingResult: grade }) + '\n', { mode: 0o600 })
      console.log(JSON.stringify({ resultID: row.id, investigator: row.response.metadata.model, task: row.vars.task_id,
        repeatIndex: source.repeatIndex, score: grade.score }))
      if (grade.componentResults.some(component => component.metadata?.graderError)) throw new Error(`Opus grading execution failed for ${row.id}; no retry`)
    }
    for (const [file, hash] of Object.entries(saved.hashes)) assert.equal(sha(fs.readFileSync(file)), hash, 'Original evidence changed during comparison')
    const result = comparisonExport(saved.source, saved.selected, judgments, comparisonID, new Date().toISOString())
    const bytes = Buffer.from(JSON.stringify(result, null, 2) + '\n')
    fs.writeFileSync(path.join(root, 'promptfoo.json'), bytes, { flag: 'wx', mode: 0o600 })
    saveJson(path.join(root, 'comparison-import.json'), await importOriginal(root, saved.plan.observer, bytes, result.results.results))
  } catch (error) {
    failure = String(error)
    saveJson(path.join(root, 'runner-error.json'), { error: failure })
    throw error
  } finally {
    const cleanupErrors = []
    try { await cancelClaudeAttempts() } catch (error) { cleanupErrors.push(String(error)) }
    try { await receiver.close() } catch (error) { cleanupErrors.push(String(error)) }
    cancellation.signal.removeEventListener('abort', cancel)
    cancellation.dispose()
    const changed = Object.entries(saved.hashes).filter(([file, hash]) => sha(fs.readFileSync(file)) !== hash).map(([file]) => file)
    saveJson(path.join(root, 'source-preservation.json'), { unchanged: changed.length === 0, changed })
    saveJson(path.join(root, 'run-completed.json'), { comparisonID, source: saved.source.evalId, graded: judgments.size,
      skipped: saved.inputs.filter(input => input.skipped).length, failure, cleanupErrors, completedAt: new Date().toISOString() })
    assert.equal(changed.length, 0, 'Original evidence changed')
    assert.deepEqual(cleanupErrors, [], 'Owned grader cleanup failed')
  }
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  process.umask(0o077)
  const [mode, source, destination, ...rest] = process.argv.slice(2)
  assert.ok(['--plan', '--run'].includes(mode) && source && !rest.length && (mode === '--plan' ? !destination : destination),
    'Usage: opencode-v2-regrade.mjs --plan <original-root> | --run <original-root> <fresh-comparison-root>')
  if (mode === '--plan') {
    const root = externalRoot(source)
    if (!fs.existsSync(path.join(root, 'run-completed.json')))
      console.log(JSON.stringify({ source: root, state: 'waiting-for-original-run', models: GPT_MODELS, grader: OPUS_MODEL }))
    else {
      const saved = loadCompletedSource(root)
      console.log(JSON.stringify({ source: root, evaluationId: saved.source.evalId, models: GPT_MODELS, grader: OPUS_MODEL,
        selected: saved.selected.length, gradable: saved.inputs.filter(input => !input.skipped).length }))
    }
  } else await runRegrading(source, destination)
}
