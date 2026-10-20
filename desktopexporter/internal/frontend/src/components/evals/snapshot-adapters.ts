import type {
  Assessment,
  EvalAttempt,
  EvalRun,
  TelemetryReference,
} from './model'

/** The caller retains payload verbatim, including for unsupported/error results.
 * These projections are disposable display data, never an archive or a rewrite. */
export type EvalSourceSnapshot = Readonly<{
  id: string
  name: string
  source: string
  format: string
  payload: string
}>

export type SnapshotRowReference = {
  attemptID: string
  /** JSON Pointer into the caller's original payload. */
  pointer: string
  resultID?: string
  evaluationID?: string
  providerID?: string
  promptID?: string
  testIndex?: number
  repeatGroupID?: string
  repeatIndex?: number
  taskID?: string
  failureReason?: number
  outcome: 'execution-error' | 'assessment-failed' | 'passed' | 'unspecified'
}

export type SnapshotProjection =
  | {
      status: 'supported'
      snapshotID: string
      adapter: 'promptfoo-v3' | 'viewer-kit'
      runs: EvalRun[]
      rows: SnapshotRowReference[]
      notes: string[]
    }
  | {
      status: 'unsupported' | 'error'
      snapshotID: string
      reason: string
      notes: string[]
    }

export const snapshotProjectionLimits = {
  payloadCharacters: 8 * 1024 * 1024,
  rows: 5000,
  nodes: 200000,
  depth: 64,
  textCharacters: 32768,
  projectedCharacters: 1024 * 1024,
  identityCharacters: 2048,
} as const

type Json = null | boolean | number | string | Json[] | { [key: string]: Json }
type JsonRecord = { [key: string]: Json }

class ProjectionError extends Error {}

// All runtime shape checks below belong to the external JSON boundary.
function object(value: Json | undefined): JsonRecord | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value
    : undefined
}

function text(value: Json | undefined): string | undefined {
  return typeof value === 'string' ? value : undefined
}

function identity(value: Json | undefined): string | undefined {
  const result = text(value) || undefined
  if (result && result.length > snapshotProjectionLimits.identityCharacters)
    throw new ProjectionError(
      'A source identity exceeds the projection identity limit.'
    )
  return result
}

function number(value: Json | undefined): number | undefined {
  return typeof value === 'number' && value >= 0 ? value : undefined
}

function index(value: Json | undefined): number | undefined {
  const result = number(value)
  return Number.isSafeInteger(result) ? result : undefined
}

function timestamp(value: Json | undefined): string | undefined {
  const result = text(value)
  return result &&
    /^\d{4}-\d\d-\d\dT/.test(result) &&
    Number.isFinite(Date.parse(result))
    ? result
    : undefined
}

function validateJson(root: Json) {
  const pending = [{ value: root, depth: 0 }]
  let nodes = 0
  while (pending.length) {
    const { value, depth } = pending.pop()!
    if (
      ++nodes > snapshotProjectionLimits.nodes ||
      depth > snapshotProjectionLimits.depth
    )
      throw new ProjectionError('JSON exceeds the projection complexity limit.')
    if (
      typeof value === 'number' &&
      (!Number.isFinite(value) ||
        (Number.isInteger(value) && !Number.isSafeInteger(value)))
    )
      throw new ProjectionError(
        'JSON contains a nonfinite or unsafe integer number. Use the original JSON for exact values.'
      )
    if (value !== null && typeof value === 'object') {
      const children = Array.isArray(value) ? value : Object.values(value)
      if (
        nodes + pending.length + children.length >
        snapshotProjectionLimits.nodes
      )
        throw new ProjectionError(
          'JSON exceeds the projection complexity limit.'
        )
      for (const child of children)
        pending.push({ value: child, depth: depth + 1 })
    }
  }
}

function reference(
  trace: Json | undefined,
  span?: Json
): TelemetryReference | undefined {
  const traceID = text(trace)
  const spanID = text(span)
  if (!traceID || !/^[a-f0-9]{32}$/.test(traceID) || /^0+$/.test(traceID))
    return undefined
  if (
    span !== undefined &&
    (!spanID || !/^[a-f0-9]{16}$/.test(spanID) || /^0+$/.test(spanID))
  )
    return undefined
  return { traceID, spanID }
}

/** Supports full Promptfoo v3 exports and the explicit {manifest,pilot} kit
 * wrapper. source is provenance, not a schema discriminator. No I/O or execution.
 * Evidence: Promptfoo ea8b3d073a63769f7be243ac3c3460f72881a930
 * src/types/index.ts (EvaluateResult, GradingResult), src/types/results.ts,
 * test/__fixtures__/sample-export.json; kit native-evaluate.ts, grade.ts,
 * eval-telemetry.ts and presentation/aggregate.ts. */
export function projectEvalSnapshot(
  snapshot: EvalSourceSnapshot
): SnapshotProjection {
  const notes = new Set<string>()
  const unavailable = (
    status: 'unsupported' | 'error',
    reason: string
  ): SnapshotProjection => ({
    status,
    snapshotID: snapshot.id,
    reason,
    notes: [...notes],
  })
  if (
    !['json', 'application/json', 'promptfoo-v3', 'viewer-kit'].includes(
      snapshot.format.toLowerCase()
    )
  )
    return unavailable(
      'unsupported',
      'This declared format has no display adapter.'
    )
  if (snapshot.payload.length > snapshotProjectionLimits.payloadCharacters)
    return unavailable('error', 'JSON exceeds the projection text limit.')
  try {
    identity(snapshot.id)
    const parsed: Json = JSON.parse(snapshot.payload)
    validateJson(parsed)
    const root = object(parsed)
    if (!root)
      return unavailable(
        'unsupported',
        'Expected a full Promptfoo export or a viewer kit wrapper.'
      )
    const kit = 'manifest' in root || 'pilot' in root
    if (
      (snapshot.format === 'promptfoo-v3' && kit) ||
      (snapshot.format === 'viewer-kit' && !kit)
    )
      return unavailable(
        'unsupported',
        'The JSON envelope does not match the declared source format.'
      )
    const manifest = object(root.manifest)
    const config = object(root.config)
    const summary = object(kit ? object(root.pilot)?.results : root.results)
    if (kit) {
      if (
        !manifest ||
        !Array.isArray(manifest.models) ||
        !manifest.models.every(model => identity(model))
      )
        return unavailable(
          'error',
          'Viewer kit requires a manifest with a models array.'
        )
      if (!summary)
        return unavailable('error', 'Viewer kit requires pilot.results.')
    } else if (!('evalId' in root || 'results' in root || 'config' in root)) {
      return unavailable(
        'unsupported',
        'No adapter recognizes this JSON envelope.'
      )
    } else if (!identity(root.evalId) || !config || !summary) {
      return unavailable(
        'error',
        'Promptfoo requires evalId, config and results in the full export.'
      )
    }
    if (!summary) return unavailable('error', 'Missing results summary.')
    if (summary.version !== 3 && !(kit && summary.version === undefined))
      return unavailable(
        'unsupported',
        'Only results.version 3 is supported (the kit may omit version).'
      )
    if (!Array.isArray(summary.results))
      return unavailable('error', 'Expected a results.results row array.')
    if (summary.results.length > snapshotProjectionLimits.rows)
      return unavailable(
        'error',
        'Result count exceeds the projection row limit.'
      )

    notes.add(
      'This is a projection of saved rows, not proof of a completed evaluation or planned-slot coverage. Complete means a saved result without an explicit execution error; unspecified execution status cannot be represented separately in the current UI.'
    )
    notes.add(
      'Costs are omitted: the UI labels costs CAD, while Promptfoo costs are USD or provider-defined. No currency conversion or allocation of kit summary costs is performed. Usage remains in the original JSON.'
    )
    notes.add(
      'Only explicit recorded telemetry identifiers are linked; availability is not verified. Tool counts and answer prose do not create tool calls.'
    )
    notes.add(
      'Source scores, named metrics, assertion trees, reviews and unknown fields remain in the original JSON. Row pointers identify the source of each displayed attempt.'
    )
    if (kit)
      notes.add(
        'Only recorded kit completion scores 0, 0.5 and 1 are numeric. UI summaries are means of recorded scores, not the kit report planned-slot means. Reviews, retries, deferred slots and optional summaries are not reconciled.'
      )
    else
      notes.add(
        'Promptfoo scores have no guaranteed maximum (GradingResult says “typically between 0 and 1”). Only recorded pass/fail labels are projected; numeric and named scores are not normalized or reduced.'
      )

    let characters = 0
    const display = (value: string): string => {
      const available = Math.max(
        0,
        snapshotProjectionLimits.projectedCharacters - characters
      )
      const length = Math.min(
        value.length,
        snapshotProjectionLimits.textCharacters,
        available
      )
      characters += length
      if (length === value.length) return value
      notes.add(
        'Long display text is truncated; the original JSON retains the complete values.'
      )
      return value.slice(0, length) + '… [see original JSON]'
    }
    const localID = (...parts: (string | number | undefined)[]) =>
      JSON.stringify([snapshot.id, ...parts])
    const runs = new Map<string, EvalRun>()
    const rows: SnapshotRowReference[] = []
    const seenSlots = new Set<string>()
    const evaluationID = identity(root.evalId)
    const evaluation = display(text(config?.description) || snapshot.name)
    const source = display(snapshot.source)
    for (const [position, raw] of summary.results.entries()) {
      const row = object(raw)
      if (!row)
        throw new ProjectionError(`Result row ${position} must be an object.`)
      if (
        row.failureReason !== undefined &&
        row.failureReason !== 0 &&
        row.failureReason !== 1 &&
        row.failureReason !== 2
      )
        throw new ProjectionError(
          `Result row ${position} has an unsupported failureReason.`
        )
      for (const field of ['testIdx', 'promptIdx', 'repeatIndex']) {
        if (
          row[field] !== undefined &&
          (index(row[field]) === undefined ||
            row[field] === Number.MAX_SAFE_INTEGER)
        )
          throw new ProjectionError(
            `Result row ${position} has an invalid ${field}.`
          )
      }
      const provider = object(row.provider)
      const prompt = object(row.prompt)
      const response = object(row.response)
      const metadata = object(response?.metadata)
      const vars = object(row.vars)
      const test = object(row.testCase)
      const grading = object(row.gradingResult)
      const providerID = identity(provider?.id)
      const model = kit
        ? (identity(metadata?.model) ?? identity(vars?.eval_model))
        : undefined
      const providerName = identity(provider?.label) ?? model ?? providerID
      const promptID = identity(row.promptId) ?? identity(prompt?.id)
      const promptIndex = index(row.promptIdx)
      const testIndex = index(row.testIdx)
      const repeatIndex = index(row.repeatIndex)
      const repeatGroupID = identity(row.repeatGroupId)
      const taskID = kit ? identity(vars?.task_id) : undefined
      const resultID = identity(row.id)
      const hasProvider = providerID !== undefined || model !== undefined
      const hasPrompt = promptID !== undefined || promptIndex !== undefined
      const runID = localID(
        'run',
        evaluationID,
        providerID,
        providerName,
        model,
        promptID,
        promptIndex,
        hasProvider && hasPrompt ? undefined : position
      )
      let run = runs.get(runID)
      const assessmentID = localID('assessment', kit ? 'completion' : 'outcome')
      if (!run) {
        const configuration = [{ name: 'Snapshot ID', value: snapshot.id }]
        if (evaluationID)
          configuration.push({
            name: 'Source evaluation ID',
            value: evaluationID,
          })
        if (providerID)
          configuration.push({ name: 'Provider ID', value: providerID })
        if (model) configuration.push({ name: 'Model', value: model })
        if (promptID) configuration.push({ name: 'Prompt ID', value: promptID })
        if (promptIndex !== undefined)
          configuration.push({
            name: 'Prompt index',
            value: String(promptIndex),
          })
        run = {
          id: runID,
          name: display(
            [evaluation, providerName, text(prompt?.label)]
              .filter(Boolean)
              .join(' · ')
          ),
          evaluation,
          source,
          state: 'Complete',
          startedAt: timestamp(kit ? manifest?.startedAt : summary.timestamp),
          configuration,
          assessments: [
            {
              id: assessmentID,
              name: kit ? 'Recorded completion' : 'Recorded outcome',
              scale: kit
                ? 'Fail 0 · Partial 0.5 · Pass 1'
                : 'Source pass/fail only; numeric scale unavailable',
              // Required by the UI even when all values are null (outcome-only).
              maximum: 1,
              aggregation: 'scored',
            },
          ],
          headlineAssessment: assessmentID,
          attempts: [],
        }
        runs.set(runID, run)
      }
      const attemptID = localID('result', evaluationID, resultID, position)
      // A test index alone does not establish repeated execution. Keep missing
      // repetition identities separate, even when all question text is equal.
      const testKey =
        repeatGroupID !== undefined
          ? localID('repeat-group', repeatGroupID, taskID)
          : testIndex !== undefined
            ? localID('test', testIndex, taskID)
            : taskID !== undefined
              ? localID('task', taskID)
              : undefined
      const groupable =
        hasProvider &&
        hasPrompt &&
        testKey !== undefined &&
        repeatIndex !== undefined
      let caseID = groupable
        ? localID('case', runID, testKey)
        : localID('case-row', position)
      const slotID = localID(caseID, repeatIndex)
      if (seenSlots.has(slotID)) {
        caseID = localID('duplicate-slot', position)
        notes.add(
          'Duplicate repetition identities are displayed as separate cases; no retry or replacement is inferred.'
        )
      }
      seenSlots.add(slotID)
      if (!groupable)
        notes.add(
          'Rows without complete test/provider/prompt/repetition identity remain separate cases. Repetition 1 is a UI placeholder when repeatIndex is absent.'
        )
      const outcome =
        row.failureReason === 2
          ? 'execution-error'
          : row.failureReason === 1 || grading?.pass === false
            ? 'assessment-failed'
            : row.success === true || grading?.pass === true
              ? 'passed'
              : 'unspecified'
      const score =
        kit &&
        (grading?.score === 0 || grading?.score === 0.5 || grading?.score === 1)
          ? grading.score
          : null
      const assessment: Assessment = {
        id: assessmentID,
        value: score,
        label:
          kit && score !== null
            ? score === 1
              ? 'Pass'
              : score === 0.5
                ? 'Partial'
                : 'Fail'
            : outcome === 'assessment-failed'
              ? 'Fail'
              : outcome === 'passed'
                ? 'Pass'
                : 'Unscored',
        explanation: display(
          text(grading?.reason) ??
            (row.failureReason === 1 ? text(row.error) : undefined) ??
            ''
        ),
        evaluator: kit ? 'Recorded kit grader' : 'Promptfoo recorded outcome',
        rubric: '',
      }
      const telemetry = kit ? object(metadata?.telemetry) : undefined
      const execution = kit
        ? reference(telemetry?.traceID, telemetry?.spanID)
        : reference(row.traceId)
      const tools: EvalAttempt['tools'] = []
      if (kit && execution && Array.isArray(telemetry?.tools)) {
        for (const rawTool of telemetry.tools) {
          const tool = object(rawTool)
          const ref = reference(execution.traceID, tool?.spanID)
          if (
            !tool ||
            !ref?.spanID ||
            (tool.error !== true && tool.error !== false)
          ) {
            notes.add(
              'Tool entries without recorded span identity and boolean error state are omitted.'
            )
            continue
          }
          tools.push({
            name: display(text(tool.name) ?? 'Recorded tool'),
            state: tool.error ? 'Error' : 'OK',
            message: '',
            telemetry: ref,
          })
        }
      }
      if (kit && Array.isArray(grading?.componentResults)) {
        const graders = grading.componentResults.flatMap(component => {
          const report = object(object(component)?.metadata)
          return report?.role === 'grader' ? [report] : []
        })
        // A single recorded grader can be attached to the aggregate score. A
        // multi-grader assertion tree cannot be collapsed to one telemetry ID.
        if (graders.length === 1) {
          const graderMetadata = object(graders[0].metadata)
          const graderTelemetry = object(graderMetadata?.telemetry)
          assessment.telemetry = reference(
            graderTelemetry?.traceID,
            graderTelemetry?.spanID
          )
          assessment.evaluator =
            identity(graderMetadata?.model) ?? assessment.evaluator
        }
      }
      const rawOutput = response?.output
      let answer: string | undefined
      if (rawOutput !== undefined)
        answer = display(text(rawOutput) ?? JSON.stringify(rawOutput))
      const latency = number(row.latencyMs)
      run.attempts.push({
        id: attemptID,
        caseID,
        repetition: repeatIndex === undefined ? 1 : repeatIndex + 1,
        name: display(
          text(test?.description) ??
            text(row.description) ??
            taskID ??
            resultID ??
            `Result ${position + 1}`
        ),
        question: display(
          (kit ? text(vars?.goal) : undefined) ?? text(prompt?.raw) ?? ''
        ),
        answer,
        state: row.failureReason === 2 ? 'Error' : 'Complete',
        seconds:
          (kit ? number(metadata?.elapsedSeconds) : undefined) ??
          (latency === undefined ? undefined : latency / 1000),
        error:
          row.failureReason === 2
            ? display(
                text(row.error) ??
                  text(response?.error) ??
                  'Recorded execution error'
              )
            : undefined,
        assessments: [assessment],
        tools,
        telemetry: execution,
      })
      rows.push({
        attemptID,
        pointer: `${kit ? '/pilot' : ''}/results/results/${position}`,
        resultID,
        evaluationID: identity(row.evaluationId) ?? evaluationID,
        providerID,
        promptID,
        testIndex,
        repeatGroupID,
        repeatIndex,
        taskID,
        failureReason: index(row.failureReason),
        outcome,
      })
    }
    return {
      status: 'supported',
      snapshotID: snapshot.id,
      adapter: kit ? 'viewer-kit' : 'promptfoo-v3',
      runs: [...runs.values()],
      rows,
      notes: [...notes],
    }
  } catch (error) {
    return unavailable(
      'error',
      error instanceof ProjectionError
        ? error.message
        : 'JSON could not be parsed or projected.'
    )
  }
}
