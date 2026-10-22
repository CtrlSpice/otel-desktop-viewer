// Presentation data only. This is not the future import/storage contract.
export type EvalView = 'results' | 'time' | 'cost'

export type AssessmentDefinition = {
  id: string
  name: string
  scale: string
  maximum?: number
  aggregation: 'planned' | 'scored'
}

export type TelemetryReference = {
  traceID: string
  spanID?: string
}

export type Assessment = {
  id: string
  /** Native assertion type, retained for unique producer telemetry correlation. */
  sourceName?: string
  value: number | null
  label: string
  explanation: string
  evaluator: string
  rubric: string
  /** Recorded outcome, independent of the producer's numeric score scale. */
  pass?: boolean
  telemetry?: TelemetryReference
  cost?: number
}

export type EvalTool = {
  name: string
  state: 'OK' | 'Error'
  message?: string
  seconds?: number
  telemetry?: TelemetryReference
  cost?: number
}

export type EvalEventTool = {
  callID: string
  parent: TelemetryReference
  eventIndex?: number
  logRef?: string
  failed?: boolean
}

export function evalToolCounts(
  result: Pick<EvalResult, 'tools' | 'eventTools'>
) {
  return {
    total: result.tools.length + (result.eventTools?.length ?? 0),
    failed:
      result.tools.filter(tool => tool.state === 'Error').length +
      (result.eventTools?.filter(tool => tool.failed === true).length ?? 0),
  }
}

type EvalExecutionEvidence = {
  answer?: string
  /** Source type before formatting the output for display. */
  answerType?: 'string' | 'number' | 'boolean' | 'null' | 'array' | 'map'
  state: 'Complete' | 'Running' | 'Error' | 'Planned'
  startedAt?: string
  seconds?: number
  investigatorCost?: number
  graderCost?: number
  error?: string
  assessments: Assessment[]
  tools: EvalTool[]
  /** Received log/span-event call evidence, kept distinct from native tool spans. */
  eventTools?: EvalEventTool[]
  /** False when the source supplies no tool-call evidence. */
  toolsKnown?: boolean
  /** Target execution, distinct from an enclosing Result or retry Attempt span. */
  telemetry?: TelemetryReference
}

/** An explicit producer-recorded retry within one Result. Adapters must not invent
 * Attempts for producers, such as Promptfoo v3, that expose only Result rows. */
export type EvalAttempt = EvalExecutionEvidence & {
  id: string
  resultID: string
  attempt: number
  name: string
  /** Optional enclosing retry span; telemetry identifies target execution. */
  attemptTelemetry?: TelemetryReference
}

export type EvalResult = EvalExecutionEvidence & {
  id: string
  /** Unmodified producer result identity; id above is the local display key. */
  sourceID?: string
  caseID: string
  /** Absent when the producer did not retain a repetition identity. */
  repetition?: number
  name: string
  question: string
  /** Present only when the producer explicitly records retries within this Result. */
  attempts?: EvalAttempt[]
  /** Optional enclosing result span; telemetry identifies target execution. */
  resultTelemetry?: TelemetryReference
}

export type EvalExperiment = {
  id: string
  name: string
  evaluation: string
  promptLabel?: string
  source: string
  state: 'Complete' | 'Paused' | 'Planned' | 'Unknown'
  /** Why lifecycle state could not be established from persisted source data. */
  stateReason?: string
  pauseReason?: 'internal-error' | 'manual'
  startedAt?: string
  configuration: { name: string; value: string | number | boolean | null }[]
  assessments: AssessmentDefinition[]
  headlineAssessment: string
  results: EvalResult[]
  seconds?: number
  telemetry?: TelemetryReference
}

export function assessmentSummary(
  experiment: Pick<EvalExperiment, 'results'>,
  definition: AssessmentDefinition
) {
  let total = 0
  let scored = 0
  for (const result of experiment.results) {
    const assessment = result.assessments.find(
      item => item.id === definition.id
    )
    if (assessment?.value != null) {
      total += assessment.value
      scored++
    }
  }
  const denominator =
    definition.aggregation === 'planned' ? experiment.results.length : scored
  return {
    scored,
    denominator,
    percent:
      scored &&
      denominator &&
      definition.maximum !== undefined &&
      definition.maximum > 0
        ? (total / (denominator * definition.maximum)) * 100
        : null,
  }
}

export function percent(value: number | null): string {
  return value === null ? 'Unscored' : `${Number(value.toFixed(1))}%`
}

export function estimatedCost(
  results: EvalResult[],
  field: 'investigatorCost' | 'graderCost'
) {
  let total = 0
  let known = 0
  for (const result of results) {
    if (result[field] !== undefined) {
      total += result[field]
      known++
    }
  }
  return { total: known ? total : null, incomplete: known < results.length }
}

export function cost(value: number | null | undefined): string {
  return value == null ? 'Unknown' : `C$${value.toFixed(3)}`
}

export function localTimestamp(value: string): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
    timeZoneName: 'short',
  }).formatToParts(new Date(value))
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find(item => item.type === type)?.value ?? ''
  return `${part('year')}-${part('month')}-${part('day')} ${part('hour')}:${part('minute')}:${part('second')} ${part('timeZoneName')}`
}

export function traceHref(reference: TelemetryReference): string {
  return `/traces/${reference.traceID}${reference.spanID ? `?span=${reference.spanID}` : ''}`
}
