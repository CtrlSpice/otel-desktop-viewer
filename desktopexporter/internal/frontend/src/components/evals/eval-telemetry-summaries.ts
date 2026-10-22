import type { EvalRep, EvalRun, TelemetryReference } from './model'
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
  selectedRep?: EvalRep
): EvalTelemetrySummary[] {
  const entries = new Map<string, EvalTelemetrySummary>()
  function add(reference: TelemetryReference | undefined, label: string) {
    if (!reference) return
    const key = referenceKey(reference)
    if (!entries.has(key)) entries.set(key, { reference, label })
  }
  function repRoots(rep: EvalRep, includeExecution = false) {
    const prefix = 'Result'
    add(rep.repTelemetry, prefix)
    if (includeExecution || !rep.repTelemetry)
      add(rep.telemetry, `${prefix} · Execution`)
  }
  if (row?.kind === 'case') {
    for (const rep of run.reps) if (rep.caseID === row.caseID) repRoots(rep)
  } else if (row?.kind === 'grading') {
    for (const assessment of row.rep?.assessments ?? [])
      add(assessment.telemetry, assessment.evaluator || assessment.id)
  } else if (row && row.kind !== 'rep' && row.kind !== 'run') {
    add(row.reference, row.name)
  } else if (row?.rep || selectedRep) {
    const rep = (row?.rep ?? selectedRep)!
    repRoots(rep, true)
    for (const assessment of rep.assessments)
      add(assessment.telemetry, assessment.evaluator || assessment.id)
  } else {
    add(run.telemetry, 'Run')
    for (const rep of run.reps) repRoots(rep)
  }
  return [...entries.values()]
}
