import fs from 'node:fs'
import path from 'node:path'
import config from './legacy-config.ts'
import { readJson, record, runtimeRoot, saveJson } from './runtime.ts'
import { afterAll, cancelNativeAttempts } from './native-lifecycle.ts'
import { runCancellation } from './run-cancellation.ts'
import { observerEnvironment } from './clean-environment.ts'
import { modelExecutionErrors } from './execution-errors.ts'
import { context, propagation, trace } from '@opentelemetry/api'

const promptfooPackage = 'promptfoo'
const { evaluate } = await import(promptfooPackage)

export async function runNativeEvaluation(suite = config, repeat = 3, resultName = 'pilot.json') {
  if (path.basename(resultName) !== resultName)
    throw new Error('Result filename must stay inside the run directory')
  const cancellation = runCancellation()
  let exit = 1
  let stop: AbortController | undefined
  let cleanup: Promise<void> | undefined
  let cleanupError: unknown
  const cancel = () => {
    stop?.abort()
    cleanup ??= cancelNativeAttempts().catch(error => { cleanupError = error })
  }
  const disconnected = () => cancellation.cancel(new Error('Evaluation worker lost its parent'))
  cancellation.signal.addEventListener('abort', cancel, { once: true })
  process.once('disconnect', disconnected)
  try {
    const root = runtimeRoot()
    const observer = observerEnvironment()
    Object.assign(process.env, observer)
    if (observer.OTEL_EVAL_OBSERVER_HTTP && !process.env.PROMPTFOO_CONFIG_DIR)
      process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'promptfoo-state')
    if (!Number.isSafeInteger(repeat) || repeat < 1) throw new Error('Invalid repetition count')
    if (suite.providers.length !== 1 || suite.prompts.length !== 1) throw new Error('Native slot scheduling requires one provider and one prompt')
    const fatal = () => cancellation.signal.aborted || ['harness-blocker.json', 'execution-error.json'].some(file => fs.existsSync(path.join(root, file)))
    if (fatal() || modelExecutionErrors(root).length || fs.existsSync(path.join(root, resultName))) throw new Error('Use a fresh run directory; saved attempts must not be rerun')
    // Expand slots before evaluating so an aborted batch can resume only slots
    // that were not called. Each batch keeps its original Promptfoo summary.
    let pending = suite.tests.flatMap((test, index) => Array.from({ length: repeat }, (_, repetition) => ({ test, index, repetition })))
    const deferred: typeof pending = []
    const batches = []
    const rows = []
    const directory = path.join(root, 'evaluation-batches')
    fs.mkdirSync(directory, { mode: 0o700 })
    while (pending.length && !fatal()) {
      const paused = new Set(modelExecutionErrors(root).map(error => error.model))
      deferred.push(...pending.filter(slot => paused.has(slot.test.vars.eval_model)))
      pending = pending.filter(slot => !paused.has(slot.test.vars.eval_model))
      if (!pending.length) break
      // Promptfoo shuts down its SDK after each evaluate(), but leaves the
      // global API registrations pointing to that stopped provider. This worker
      // owns the SDK; release registrations before the next batch initializes it.
      if (batches.length && observer.OTEL_EVAL_OBSERVER_HTTP) {
        trace.disable()
        context.disable()
        propagation.disable()
      }
      stop = new AbortController()
      const batchSuite = { ...suite, tests: pending.map(slot => slot.test) }
      const result = await evaluate(observer.OTEL_EVAL_OBSERVER_HTTP
        ? { ...batchSuite, writeLatestResults: true, tracing: { enabled: true } } : batchSuite, {
        cache: false,
        writeLatestResults: false,
        share: false,
        maxConcurrency: 1,
        repeat: 1,
        abortSignal: stop.signal,
        // Promptfoo invokes this after the row and its grading evidence are saved.
        progressCallback: () => {
          if (fatal() || modelExecutionErrors(root).length > paused.size) stop!.abort()
        },
      })
      const summary = await result.toEvaluateSummary()
      const executionErrorFile = path.join(root, 'execution-error.json')
      const executionError = fs.existsSync(executionErrorFile) ? record(readJson(executionErrorFile)) : undefined
      // Hook replacement controls execution; Promptfoo still labels the inert provider.
      for (const row of summary.results) {
        const model = row.response?.metadata?.model ?? row.vars?.eval_model
        if (typeof model !== 'string') throw new Error('Missing saved attempt model')
        if (model !== row.vars?.eval_model) throw new Error('Saved attempt model mismatch')
        row.provider = { id: 'opencode:sdk:' + model, label: model }
        // Promptfoo treats thrown JavaScript assertions as failed answers. Preserve
        // our explicit grader execution failure as an error in the saved result.
        if (row === summary.results.at(-1) && executionError?.role === 'grader' && executionError.model === model && executionError.task === row.vars?.task_id) {
          row.failureReason = 2
          row.success = false
        }
      }
      const batchName: string = `batch-${String(batches.length + 1).padStart(3, '0')}.json`
      saveJson(path.join(directory, batchName), { results: summary })
      batches.push({ file: path.join('evaluation-batches', batchName), summary })
      const completed = new Set<number>()
      for (const row of summary.results) {
        const slot = pending[row.testIdx]
        if (!slot || completed.has(row.testIdx) || row.vars?.eval_model !== slot.test.vars.eval_model || row.vars?.task_id !== slot.test.vars.task_id) throw new Error('Cannot reconcile saved attempt with its planned slot')
        completed.add(row.testIdx)
        rows.push({ ...row, testIdx: slot.index, repeatIndex: slot.repetition })
      }
      pending = pending.filter((_slot, index) => !completed.has(index))
      if (!fatal() && modelExecutionErrors(root).length === paused.size && pending.length) throw new Error('Evaluation stopped without a recorded model or harness error')
    }
    const first = batches[0]?.summary
    saveJson(path.join(root, resultName), { results: batches.length === 1 ? { ...first, results: rows } : {
      version: first?.version, timestamp: first?.timestamp, results: rows,
      // Batch-local prompt metrics and token totals remain with their raw rows;
      // consumers calculate combined totals from the recorded attempts.
      batches: batches.map(batch => batch.file),
    } })
    const paused = modelExecutionErrors(root)
    saveJson(path.join(root, 'deferred-slots.json'), { models: paused.map(error => error.model), slots: [...deferred, ...pending].map(slot => ({ model: slot.test.vars.eval_model, task: slot.test.vars.task_id, repetition: slot.repetition })) })
    exit = fatal() || paused.length ? 1 : 0
  } finally {
    try {
      await afterAll()
      await cleanup
      if (cleanupError) throw cleanupError
    } finally {
      process.off('disconnect', disconnected)
      cancellation.dispose()
    }
  }
  return cancellation.signal.aborted ? 1 : exit
}

if (process.argv[1] && fs.existsSync(process.argv[1]) && fs.realpathSync(process.argv[1]) === import.meta.filename) process.exitCode = await runNativeEvaluation()
