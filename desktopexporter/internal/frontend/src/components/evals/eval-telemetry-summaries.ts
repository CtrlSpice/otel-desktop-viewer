import type { EvalAttempt, EvalRun, TelemetryReference } from './model'
import type { EvalOutlineRow } from './eval-outline'
import { referenceKey } from './eval-traces'

export type EvalTelemetrySummary = {
  reference: TelemetryReference
  label: string
}

/** Logical ownership labels references; it never invents an enclosing span. */
export function evalTelemetrySummaries(
  run: EvalRun,
  row?: EvalOutlineRow,
  selectedAttempt?: EvalAttempt
): EvalTelemetrySummary[] {
  const entries = new Map<string, EvalTelemetrySummary>()
  function add(reference: TelemetryReference | undefined, label: string) {
    if (!reference) return
    const key = referenceKey(reference)
    if (!entries.has(key)) entries.set(key, { reference, label })
  }
  function attemptRoots(attempt: EvalAttempt, includeExecution = false) {
    const prefix = `Rep ${attempt.repetition}`
    add(attempt.attemptTelemetry, `${prefix} · Attempt`)
    if (includeExecution || !attempt.attemptTelemetry)
      add(attempt.telemetry, `${prefix} · Execution`)
  }
  if (row?.kind === 'case') {
    for (const attempt of run.attempts)
      if (attempt.caseID === row.caseID) attemptRoots(attempt)
  } else if (row?.kind === 'grading') {
    for (const assessment of row.attempt?.assessments ?? [])
      add(assessment.telemetry, assessment.evaluator || assessment.id)
  } else if (row && row.kind !== 'attempt') {
    add(row.reference, row.name)
  } else if (row?.attempt || selectedAttempt) {
    const attempt = (row?.attempt ?? selectedAttempt)!
    attemptRoots(attempt, true)
    for (const assessment of attempt.assessments)
      add(assessment.telemetry, assessment.evaluator || assessment.id)
  } else {
    add(run.telemetry, 'Run')
    for (const attempt of run.attempts) attemptRoots(attempt)
  }
  return [...entries.values()]
}
