import type { EvalResult, EvalExperiment, TelemetryReference } from './model'
import type { EvalOutlineRow } from './eval-outline'
import { referenceKey } from './eval-traces'

export type EvalTelemetrySummary = {
  reference: TelemetryReference
  label: string
}

/** Logical ownership labels references; it never invents an enclosing span. */
export function evalTelemetrySummaries(
  experiment: EvalExperiment,
  row?: EvalOutlineRow,
  selectedResult?: EvalResult
): EvalTelemetrySummary[] {
  const entries = new Map<string, EvalTelemetrySummary>()
  function add(reference: TelemetryReference | undefined, label: string) {
    if (!reference) return
    const key = referenceKey(reference)
    if (!entries.has(key)) entries.set(key, { reference, label })
  }
  function resultRoots(result: EvalResult, includeExecution = false) {
    const prefix = 'Result'
    add(result.resultTelemetry, prefix)
    if (includeExecution || !result.resultTelemetry)
      add(result.telemetry, `${prefix} · Execution`)
  }
  if (row?.kind === 'case') {
    for (const result of experiment.results)
      if (result.caseID === row.caseID) resultRoots(result)
  } else if (row?.kind === 'grading') {
    for (const assessment of row.result?.assessments ?? [])
      add(assessment.telemetry, assessment.evaluator || assessment.id)
  } else if (row && row.kind !== 'result' && row.kind !== 'experiment') {
    add(row.reference, row.name)
  } else if (row?.result || selectedResult) {
    const result = (row?.result ?? selectedResult)!
    resultRoots(result, true)
    for (const assessment of result.assessments)
      add(assessment.telemetry, assessment.evaluator || assessment.id)
  } else {
    add(experiment.telemetry, 'Experiment')
    for (const result of experiment.results) resultRoots(result)
  }
  return [...entries.values()]
}
