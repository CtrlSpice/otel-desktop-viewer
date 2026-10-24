import type {
  Assessment,
  EvalResult,
  EvalExperiment,
  TelemetryReference,
} from './model'

/** The caller retains payload verbatim, including for unsupported/error results.
 * These projections are disposable display data, never authoritative source. */
export type EvalSourceDataset = Readonly<{
  id: string
  name: string
  source: string
  format: string
  payload: string
  /** Global source positions when payload contains only a selected experiment's rows. */
  rowOrdinals?: readonly number[]
}>

export type EvalSourceRowReference = {
  resultID: string
  /** JSON Pointer into the caller's original payload. */
  pointer: string
  /** Source-native prose only; absent entries are presentation fallbacks. */
  fields: Partial<Record<EvalSourceTextField, string>>
  sourceResultID?: string
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

export type EvalSourceTextField =
  | 'name'
  | 'question'
  | 'answer'
  | 'explanation'
  | 'error'
  | `assessment:${string}:rubric`
  | `assessment:${string}:explanation`

export type EvalExperimentTextSource = {
  evaluation?: string
  providerName?: string
  promptLabel?: string
}

export type EvalSourceProjection =
  | {
      status: 'supported'
      datasetID: string
      adapter: 'promptfoo-v3'
      experiments: EvalExperiment[]
      rows: EvalSourceRowReference[]
      experimentSources: Record<string, EvalExperimentTextSource>
      notes: string[]
    }
  | {
      status: 'unsupported' | 'error'
      datasetID: string
      reason: string
      notes: string[]
    }

export const evalProjectionLimits = {
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
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Decode an external JSON object; null and arrays are distinct source types.
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value
    : undefined
}

function text(value: Json | undefined): string | undefined {
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Accept only received strings; never coerce JSON values into prose.
  return typeof value === 'string' ? value : undefined
}

/** Display serialization for validated JSON, retaining signed zero recursively.
 * Source-backed Copy still uses original text, including numeric lexemes and duplicate keys. */
function outputText(value: Json): string {
  if (Object.is(value, -0)) return '-0'
  if (Array.isArray(value)) return '[' + value.map(outputText).join(',') + ']'
  const members = object(value)
  if (members)
    return (
      '{' +
      Object.entries(members)
        .map(([key, child]) => JSON.stringify(key) + ':' + outputText(child))
        .join(',') +
      '}'
    )
  return JSON.stringify(value)
}

function identity(value: Json | undefined): string | undefined {
  const result = text(value) || undefined
  if (result && result.length > evalProjectionLimits.identityCharacters)
    throw new ProjectionError(
      'A source identity exceeds the projection identity limit.'
    )
  return result
}

function number(value: Json | undefined): number | undefined {
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Decode nonnegative source measurements without accepting numeric-looking strings.
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

/** Test definitions and resolved variables identify the logical input, not an
 * invocation. Object key order does not change that identity. */
function canonical(value: Json): string {
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']'
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Recursive JSON identity must distinguish object members from scalar source values.
  if (value !== null && typeof value === 'object')
    return (
      '{' +
      Object.keys(value)
        .sort()
        .map(key => JSON.stringify(key) + ':' + canonical(value[key]))
        .join(',') +
      '}'
    )
  return JSON.stringify(value)
}

function hasGraderError(grading: JsonRecord | undefined): boolean {
  return (
    object(grading?.metadata)?.graderError === true ||
    (Array.isArray(grading?.componentResults) &&
      grading.componentResults.some(item => hasGraderError(object(item))))
  )
}

function validateJson(root: Json) {
  const pending = [{ value: root, depth: 0 }]
  let nodes = 0
  while (pending.length) {
    const { value, depth } = pending.pop()!
    if (
      ++nodes > evalProjectionLimits.nodes ||
      depth > evalProjectionLimits.depth
    )
      throw new ProjectionError('JSON exceeds the projection complexity limit.')
    if (
      // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Validate parsed JSON numbers before projection; source text retains values JavaScript cannot represent.
      typeof value === 'number' &&
      (!Number.isFinite(value) ||
        (Number.isInteger(value) && !Number.isSafeInteger(value)))
    )
      throw new ProjectionError(
        'JSON contains a nonfinite or unsafe integer number. Use the original JSON for exact values.'
      )
    // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Traverse only JSON containers when enforcing the external-payload complexity budget.
    if (value !== null && typeof value === 'object') {
      const children = Array.isArray(value) ? value : Object.values(value)
      if (nodes + pending.length + children.length > evalProjectionLimits.nodes)
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

/** Supports full Promptfoo v3 exports. Source is provenance, not a schema
 * discriminator. No I/O or execution.
 * Evidence: Promptfoo ea8b3d073a63769f7be243ac3c3460f72881a930
 * src/types/index.ts (EvaluateResult, GradingResult), src/types/results.ts,
 * and test/__fixtures__/sample-export.json. */
export function projectPromptfooDataset(
  dataset: EvalSourceDataset
): EvalSourceProjection {
  const notes = new Set<string>()
  const unavailable = (
    status: 'unsupported' | 'error',
    reason: string
  ): EvalSourceProjection => ({
    status,
    datasetID: dataset.id,
    reason,
    notes: [...notes],
  })
  if (
    !['json', 'application/json', 'promptfoo-v3'].includes(
      dataset.format.toLowerCase()
    )
  )
    return unavailable(
      'unsupported',
      'This declared format has no display adapter.'
    )
  if (dataset.payload.length > evalProjectionLimits.payloadCharacters)
    return unavailable('error', 'JSON exceeds the projection text limit.')
  try {
    identity(dataset.id)
    const parsed: Json = JSON.parse(dataset.payload)
    validateJson(parsed)
    const root = object(parsed)
    if (!root)
      return unavailable('unsupported', 'Expected a full Promptfoo export.')
    const config = object(root.config)
    const summary = object(root.results)
    if (!('evalId' in root || 'results' in root || 'config' in root)) {
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
    if (summary.version !== 3)
      return unavailable('unsupported', 'Only results.version 3 is supported.')
    if (!Array.isArray(summary.results))
      return unavailable('error', 'Expected a results.results row array.')
    if (summary.results.length > evalProjectionLimits.rows)
      return unavailable(
        'error',
        'Result count exceeds the projection row limit.'
      )

    notes.add(
      'This is a projection of saved rows, not proof of a completed evaluation or planned-slot coverage. Complete means a saved result without an explicit execution error; unspecified execution status cannot be represented separately in the current UI.'
    )
    notes.add(
      'Costs are omitted: the UI labels costs CAD, while Promptfoo costs are USD or provider-defined. No currency conversion is performed. Usage remains in the original JSON.'
    )
    notes.add(
      'Only explicit recorded telemetry identifiers are linked; availability is not verified. Tool counts and answer prose do not create tool calls.'
    )
    notes.add(
      'Source scores and assertion results are displayed without rescaling. Complete assertion trees, reviews and unknown fields remain in the original JSON.'
    )
    notes.add(
      'Promptfoo scores have no guaranteed maximum. Scores are displayed as recorded; no percentage or maximum is inferred.'
    )

    let characters = 0
    const display = (value: string): string => {
      const available = Math.max(
        0,
        evalProjectionLimits.projectedCharacters - characters
      )
      const length = Math.min(
        value.length,
        evalProjectionLimits.textCharacters,
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
      JSON.stringify([dataset.id, ...parts])
    const experiments = new Map<string, EvalExperiment>()
    const rows: EvalSourceRowReference[] = []
    const experimentSources: Record<string, EvalExperimentTextSource> = {}
    const testDefinitions = new Map<string, number>()
    const evaluationID = identity(root.evalId)
    const evaluation = display(text(config?.description) || dataset.name)
    const source = display(dataset.source)
    if (
      dataset.rowOrdinals &&
      (dataset.rowOrdinals.length !== summary.results.length ||
        dataset.rowOrdinals.some(
          (ordinal, i, ordinals) =>
            !Number.isSafeInteger(ordinal) ||
            ordinal < 0 ||
            (i > 0 && ordinal <= ordinals[i - 1])
        ))
    )
      throw new ProjectionError(
        'Source row ordinals must be increasing nonnegative safe integers, one per result.'
      )
    for (const [localPosition, raw] of summary.results.entries()) {
      const position = dataset.rowOrdinals?.[localPosition] ?? localPosition
      const row = object(raw)
      if (!row)
        throw new ProjectionError(`Result row ${position} must be an object.`)
      const pointer = `/results/results/${localPosition}`
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
      const test = object(row.testCase)
      const grading = object(row.gradingResult)
      const providerID = identity(provider?.id)
      const providerName = identity(provider?.label) ?? providerID
      const promptID = identity(row.promptId) ?? identity(prompt?.id)
      const promptIndex = index(row.promptIdx)
      const testIndex = index(row.testIdx)
      const repeatIndex = index(row.repeatIndex)
      const repeatGroupID = identity(row.repeatGroupId)
      const sourceResultID = identity(row.id)
      const hasProvider = providerID !== undefined
      const hasPrompt = promptID !== undefined || promptIndex !== undefined
      const experimentID = localID(
        'run',
        evaluationID,
        providerID,
        providerName,
        undefined,
        promptID,
        promptIndex,
        hasProvider && hasPrompt ? undefined : position
      )
      let experiment = experiments.get(experimentID)
      const assessmentID = localID('assessment', 'outcome')
      if (!experiment) {
        experimentSources[experimentID] = {
          evaluation: text(config?.description)
            ? '/config/description'
            : undefined,
          providerName: identity(provider?.label)
            ? pointer + '/provider/label'
            : providerID
              ? pointer + '/provider/id'
              : undefined,
          promptLabel: text(prompt?.label)
            ? pointer + '/prompt/label'
            : undefined,
        }
        const configuration: EvalExperiment['configuration'] = [
          { name: 'Dataset ID', value: dataset.id },
        ]
        if (evaluationID)
          configuration.push({
            name: 'Source evaluation ID',
            value: evaluationID,
          })
        if (providerID)
          configuration.push({ name: 'Provider ID', value: providerID })
        if (promptID) configuration.push({ name: 'Prompt ID', value: promptID })
        if (promptIndex !== undefined)
          configuration.push({
            name: 'Prompt index',
            value: promptIndex,
          })
        experiment = {
          id: experimentID,
          name: display(providerName ?? evaluation),
          evaluation,
          promptLabel:
            text(prompt?.label) === undefined
              ? undefined
              : display(text(prompt?.label)!),
          source,
          state: 'Unknown',
          stateReason:
            'Received result rows do not establish experiment lifecycle or planned-slot coverage.',
          startedAt: timestamp(summary.timestamp),
          configuration,
          assessments: [
            {
              id: assessmentID,
              name: 'Overall',
              scale: 'Source score and pass/fail; numeric scale unavailable',
              aggregation: 'scored',
            },
          ],
          headlineAssessment: assessmentID,
          results: [],
        }
        experiments.set(experimentID, experiment)
      }
      const resultID = localID('result', evaluationID, sourceResultID, position)
      // Promptfoo expands testIdx across repetitions. Group the complete native
      // test definition plus resolved variables, never question text alone.
      const definition = test
        ? experimentID +
          '\n' +
          canonical(test) +
          '\n' +
          canonical(object(row.vars) ?? object(test.vars) ?? {})
        : undefined
      if (definition !== undefined && !testDefinitions.has(definition))
        testDefinitions.set(definition, position)
      const testKey =
        repeatGroupID !== undefined
          ? localID('repeat-group', repeatGroupID)
          : definition !== undefined
            ? localID('test-definition', testDefinitions.get(definition))
            : testIndex !== undefined
              ? localID('test', testIndex)
              : undefined
      const groupable = hasProvider && hasPrompt && testKey !== undefined
      const caseID = groupable
        ? localID('case', experimentID, testKey)
        : localID('case-row', position)
      if (!groupable)
        notes.add(
          'Rows without test/provider/prompt identity remain separate test cases. Missing repetition identities are not numbered.'
        )
      const machineryError = row.failureReason === 2 || hasGraderError(grading)
      const outcome = machineryError
        ? 'execution-error'
        : row.failureReason === 1 || grading?.pass === false
          ? 'assessment-failed'
          : row.success === true || grading?.pass === true
            ? 'passed'
            : 'unspecified'
      const assessment: Assessment = {
        id: assessmentID,
        value:
          // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Recorded scores accept negative/fractional numbers, but not numeric strings.
          typeof row.score === 'number'
            ? row.score
            : // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Apply the same native-number contract to the fallback grading score.
              typeof grading?.score === 'number'
              ? grading.score
              : null,
        pass:
          outcome === 'passed'
            ? true
            : outcome === 'assessment-failed'
              ? false
              : undefined,
        label:
          outcome === 'assessment-failed'
            ? 'Fail'
            : outcome === 'passed'
              ? 'Pass'
              : 'Unscored',
        explanation: display(
          text(grading?.reason) ??
            (row.failureReason === 1 ? text(row.error) : undefined) ??
            ''
        ),
        evaluator: 'Promptfoo recorded outcome',
        rubric: '',
      }
      const execution = reference(row.traceId)
      const assessments = [assessment]
      const assessmentFields: Partial<Record<EvalSourceTextField, string>> = {}
      const components = Array.isArray(grading?.componentResults)
        ? grading.componentResults
        : []
      for (const [componentIndex, rawComponent] of components.entries()) {
        const component = object(rawComponent)
        if (!component) continue
        const assertion = object(component.assertion)
        const id = localID(
          'assertion',
          componentIndex,
          text(assertion?.type),
          text(assertion?.metric)
        )
        if (text(assertion?.value) !== undefined)
          assessmentFields[`assessment:${id}:rubric`] =
            `${pointer}/gradingResult/componentResults/${componentIndex}/assertion/value`
        if (text(component.reason) !== undefined)
          assessmentFields[`assessment:${id}:explanation`] =
            `${pointer}/gradingResult/componentResults/${componentIndex}/reason`
        if (!experiment.assessments.some(item => item.id === id))
          experiment.assessments.push({
            id,
            name: display(
              text(assertion?.metric) ??
                text(assertion?.type) ??
                `Assessment ${componentIndex + 1}`
            ),
            scale: 'Recorded score; numeric scale unavailable',
            aggregation: 'scored',
          })
        assessments.push({
          id,
          sourceName: text(assertion?.type),
          // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Decode each native assertion score without coercion or nonnegative-range policy.
          value: typeof component.score === 'number' ? component.score : null,
          pass:
            // oxlint-disable-next-line anti-slop/no-runtime-typeof -- An assertion outcome is recorded only for an actual JSON boolean.
            typeof component.pass === 'boolean' ? component.pass : undefined,
          label:
            component.pass === true
              ? 'Pass'
              : component.pass === false
                ? 'Fail'
                : 'Unscored',
          explanation: display(text(component.reason) ?? ''),
          evaluator: display(text(assertion?.type) ?? ''),
          rubric: display(text(assertion?.value) ?? ''),
        })
      }
      const tools: EvalResult['tools'] = []
      const rawOutput = response?.output
      let answer: string | undefined
      if (rawOutput !== undefined)
        answer = display(text(rawOutput) ?? outputText(rawOutput))
      const fields: EvalSourceRowReference['fields'] = assessmentFields
      const sourceField = (
        field: EvalSourceTextField,
        candidates: [Json | undefined, string][]
      ) => {
        const selected = candidates.find(([value]) => text(value) !== undefined)
        if (selected) fields[field] = pointer + selected[1]
      }
      sourceField('name', [
        [test?.description, '/testCase/description'],
        [row.description, '/description'],
        [sourceResultID, '/id'],
      ])
      sourceField('question', [[prompt?.raw, '/prompt/raw']])
      sourceField('explanation', [
        [grading?.reason, '/gradingResult/reason'],
        [row.failureReason === 1 ? row.error : undefined, '/error'],
      ])
      if (machineryError)
        sourceField('error', [
          [row.error, '/error'],
          [response?.error, '/response/error'],
          [grading?.reason, '/gradingResult/reason'],
        ])
      if (rawOutput !== undefined) fields.answer = pointer + '/response/output'
      // The built-in echo provider replays a saved answer. Its latency is not
      // the original model execution duration; the export retains that value.
      const latency = providerID === 'echo' ? undefined : number(row.latencyMs)
      experiment.results.push({
        id: resultID,
        sourceID: sourceResultID,
        caseID,
        repetition: repeatIndex === undefined ? undefined : repeatIndex + 1,
        name: display(
          text(test?.description) ??
            text(row.description) ??
            sourceResultID ??
            `Result ${position + 1}`
        ),
        question: display(text(prompt?.raw) ?? ''),
        answer,
        answerType:
          rawOutput === undefined
            ? undefined
            : rawOutput === null
              ? 'null'
              : Array.isArray(rawOutput)
                ? 'array'
                : // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Label the received output type after excluding null and arrays; display formatting does not determine type.
                  typeof rawOutput === 'object'
                  ? 'map'
                  : // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Preserve native numeric output classification, including signed zero.
                    typeof rawOutput === 'number'
                    ? 'number'
                    : // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Preserve native boolean output classification without interpreting strings.
                      typeof rawOutput === 'boolean'
                      ? 'boolean'
                      : 'string',
        state: machineryError ? 'Error' : 'Complete',
        seconds: latency === undefined ? undefined : latency / 1000,
        error: machineryError
          ? display(
              text(row.error) ??
                text(response?.error) ??
                text(grading?.reason) ??
                'Recorded execution error'
            )
          : undefined,
        assessments,
        tools,
        toolsKnown: false,
        telemetry: execution,
      })
      rows.push({
        resultID,
        pointer,
        fields,
        sourceResultID,
        evaluationID: identity(row.evaluationId) ?? evaluationID,
        providerID,
        promptID,
        testIndex,
        repeatGroupID,
        repeatIndex,
        failureReason: index(row.failureReason),
        outcome,
      })
    }
    return {
      status: 'supported',
      datasetID: dataset.id,
      adapter: 'promptfoo-v3',
      experiments: [...experiments.values()],
      rows,
      experimentSources,
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
