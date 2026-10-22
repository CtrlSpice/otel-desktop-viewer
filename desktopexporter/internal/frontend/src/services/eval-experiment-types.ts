/** Stored source facts, independent of the Evals presentation model. */
export type EvalExperimentOutcome = {
  key: string
  label: 'Pass' | 'Partial' | 'Fail' | 'Internal fault' | 'Unscored'
  tone: 'success' | 'warning' | 'error' | 'papaya' | 'neutral'
  count: number
}

export type EvalStoredExperimentSummary = {
  id: string
  adapterExperimentID: string
  datasetID: string
  name: string
  evaluation: string
  source: string
  format: 'promptfoo-v3'
  state: 'Unknown'
  stateReason: string
  pauseReason: string | null
  startedAt: string | null
  plannedCount: number | null
  resultCount: number
  assessmentName: 'Recorded outcome'
  outcomes: EvalExperimentOutcome[]
}

export type EvalExperimentSearch = {
  query?: string
  outcomes?: readonly string[]
  sort?: 'started' | 'name'
  direction?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

export type EvalExperimentSearchPage = {
  experiments: EvalStoredExperimentSummary[]
  total: number
  hasMore: boolean
  /** Experiment counts per outcome after text search, before the outcome filter. */
  facets: EvalExperimentOutcome[]
  indexing: {
    remaining: number
    unsupported: number
    empty: number
    issues: {
      datasetID: string
      kind: 'dataset'
      status: 'unsupported' | 'empty'
      reason: string
    }[]
    hasMoreIssues: boolean
  }
}

export type EvalExperimentSourcePage = {
  summary: EvalStoredExperimentSummary
  /** Exact JSON values as strings: response.json() cannot round source numbers. */
  contextJSON: string
  rows: { ordinal: number; json: string }[]
  /** Global source-row ordinal cursor, not a page number or experiment-relative offset. */
  nextAfter: number
  hasMore: boolean
}
