import { aggregate, type ReviewedAttempt } from './presentation/aggregate.ts'

// Source-native offline snapshots, not a viewer API or durable record schema.
export type Json = null | boolean | number | string | Json[] | JsonObject
export type JsonObject = { [key: string]: Json }

function object(value: Json | undefined, at: string): JsonObject {
  if (value === null || typeof value !== 'object' || Array.isArray(value))
    throw new Error(`${at}: expected an object`)
  return value
}

function list(value: Json | undefined, at: string): Json[] {
  if (!Array.isArray(value)) throw new Error(`${at}: expected an array`)
  return value
}

/** Reject values that cannot be represented by these finite JSON snapshots. */
function snapshot(value: unknown): JsonObject {
  const ancestors = new Set<object>()
  function copy(value: unknown): Json {
    if (value === null || typeof value === 'string' || typeof value === 'boolean') return value
    if (typeof value === 'number' && Number.isFinite(value) &&
      (!Number.isInteger(value) || Number.isSafeInteger(value))) return value
    if (typeof value !== 'object' || value === null ||
      (!Array.isArray(value) && Object.getPrototypeOf(value) !== Object.prototype && Object.getPrototypeOf(value) !== null))
      throw new Error('Expected finite JSON data with safe integer numbers')
    if (ancestors.has(value)) throw new Error('Cyclic JSON data')
    ancestors.add(value)
    const result: Json = Array.isArray(value)
      ? Array.from(value, copy)
      : Object.fromEntries(Object.entries(value).map(([key, item]) => [key, copy(item)]))
    ancestors.delete(value)
    return result
  }
  return object(copy(value), 'artifact')
}

function optionalNumber(value: Json | undefined, at: string) {
  if (value !== undefined && value !== null && typeof value !== 'number')
    throw new Error(`${at}: expected a number or null`)
}

function promptfooRows(summary: JsonObject) {
  return list(summary.results, 'results.results').map((value, index) => {
    const row = object(value, `row ${index}`)
    for (const key of ['testIdx', 'promptIdx', 'repeatIndex']) {
      const value = row[key]
      if (value !== undefined && (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0))
        throw new Error(`row ${index}.${key}: expected a nonnegative safe integer`)
    }
    if (row.success !== undefined && typeof row.success !== 'boolean')
      throw new Error(`row ${index}.success: expected a boolean`)
    if (row.failureReason !== undefined &&
      (typeof row.failureReason !== 'number' || ![0, 1, 2].includes(row.failureReason)))
      throw new Error(`row ${index}.failureReason: expected 0, 1 or 2`)
    optionalNumber(row.score, `row ${index}.score`)
    if (row.gradingResult !== undefined && row.gradingResult !== null)
      object(row.gradingResult, `row ${index}.gradingResult`)
    return row
  })
}

/** Full Promptfoo v3 export only; row streams and kit wrappers are separate. */
export function decodePromptfoo(value: unknown) {
  const source = snapshot(value)
  if (typeof source.evalId !== 'string' || !source.evalId)
    throw new Error('Promptfoo: expected evalId in a full export')
  object(source.config, 'config')
  const summary = object(source.results, 'results')
  if (summary.version !== 3) throw new Error('Promptfoo: only results.version 3 is supported')
  const rows = promptfooRows(summary)
  return {
    source,
    rows: rows.map(row => ({
      source: row,
      // error text can be an assertion explanation. Only the explicit source
      // discriminator establishes execution failure in this narrow projection.
      classification: row.failureReason === 2 ? 'execution-error' as const
        : row.failureReason === 1 ? 'assessment-failed' as const : 'unspecified' as const,
    })),
  }
}

/** Supplied kit artifact set; no directory discovery or private path reads. */
export function decodeKit(value: unknown) {
  const source = snapshot(value)
  const manifest = object(source.manifest, 'manifest')
  list(manifest.models, 'manifest.models')
  const pilot = object(source.pilot, 'pilot')
  const summary = object(pilot.results, 'pilot.results')
  const rows = promptfooRows(summary)
  // Kit multi-batch summaries may omit version; an explicit unsupported version
  // must not be mistaken for the installed kit's v3 summary.
  if (summary.version !== undefined && summary.version !== 3)
    throw new Error('Kit: unsupported pilot summary version')
  return { source, manifest, pilot, rows }
}

/** Existing allowlisted report semantics; reviews must already be validated. */
export function projectKitReport(
  decoded: ReturnType<typeof decodeKit>,
  validatedReviews: ReviewedAttempt[] = []
) {
  return aggregate(decoded.manifest, decoded.pilot, decoded.source.modelSummary,
    decoded.source.graderSummary, decoded.source.pricing, validatedReviews)
}

/** Inspect v2 JSON sample-score probe, not a ZIP/event/attachment reader. */
export function decodeInspectProbe(value: unknown) {
  const source = snapshot(value)
  if (source.version !== 2) throw new Error('Inspect probe: only log version 2 JSON is supported')
  object(source.eval, 'eval')
  return {
    source,
    samples: list(source.samples, 'samples').map((value, index) => {
      const sample = object(value, `sample ${index}`)
      if (typeof sample.id !== 'string' && (typeof sample.id !== 'number' || !Number.isSafeInteger(sample.id)))
        throw new Error(`sample ${index}.id: expected a string or safe integer`)
      const scores = sample.scores === undefined ? undefined : object(sample.scores, `sample ${index}.scores`)
      return {
        source: sample,
        scores: scores === undefined ? undefined : Object.entries(scores).map(([name, value]) => {
          const score = object(value, `sample ${index}.scores.${name}`)
          if (!Object.hasOwn(score, 'value')) throw new Error(`score ${name}: missing value`)
          return { name, source: score }
        }),
      }
    }),
  }
}
