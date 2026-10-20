// Presentation data only. This is not the future import/storage contract.
export type EvalView = 'results' | 'time' | 'cost'

export type AssessmentDefinition = {
  id: string
  name: string
  scale: string
  maximum: number
  aggregation: 'planned' | 'scored'
}

export type TelemetryReference = {
  traceID: string
  spanID?: string
}

export type Assessment = {
  id: string
  value: number | null
  label: string
  explanation: string
  evaluator: string
  rubric: string
  telemetry?: TelemetryReference
  cost?: number
}

export type EvalAttempt = {
  id: string
  caseID: string
  repetition: number
  name: string
  question: string
  answer?: string
  state: 'Complete' | 'Running' | 'Error' | 'Planned'
  startedAt?: string
  seconds?: number
  investigatorCost?: number
  graderCost?: number
  error?: string
  assessments: Assessment[]
  tools: {
    name: string
    state: 'OK' | 'Error'
    message?: string
    seconds?: number
    telemetry?: TelemetryReference
    cost?: number
  }[]
  /** Optional enclosing attempt span; telemetry identifies target execution. */
  attemptTelemetry?: TelemetryReference
  telemetry?: TelemetryReference
}

export type EvalRun = {
  id: string
  name: string
  evaluation: string
  source: string
  state: 'Complete' | 'Paused' | 'Planned'
  pauseReason?: 'internal-error' | 'manual'
  startedAt?: string
  configuration: { name: string; value: string }[]
  assessments: AssessmentDefinition[]
  headlineAssessment: string
  attempts: EvalAttempt[]
  seconds?: number
  telemetry?: TelemetryReference
}

export function assessmentSummary(
  run: Pick<EvalRun, 'attempts'>,
  definition: AssessmentDefinition
) {
  let total = 0
  let scored = 0
  for (const attempt of run.attempts) {
    const assessment = attempt.assessments.find(
      item => item.id === definition.id
    )
    if (assessment?.value != null) {
      total += assessment.value
      scored++
    }
  }
  const denominator =
    definition.aggregation === 'planned' ? run.attempts.length : scored
  return {
    scored,
    denominator,
    percent:
      scored && denominator
        ? (total / (denominator * definition.maximum)) * 100
        : null,
  }
}

export function percent(value: number | null): string {
  return value === null ? 'Unscored' : `${Number(value.toFixed(1))}%`
}

export function estimatedCost(
  attempts: EvalAttempt[],
  field: 'investigatorCost' | 'graderCost'
) {
  let total = 0
  let known = 0
  for (const attempt of attempts) {
    if (attempt[field] !== undefined) {
      total += attempt[field]
      known++
    }
  }
  return { total: known ? total : null, incomplete: known < attempts.length }
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
