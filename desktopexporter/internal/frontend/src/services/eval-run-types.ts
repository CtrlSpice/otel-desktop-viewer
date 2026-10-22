/** Stored source facts, independent of the Evals presentation model. */
export type EvalRunOutcome = {
  key: string
  label: 'Pass' | 'Partial' | 'Fail' | 'Internal fault' | 'Unscored'
  tone: 'success' | 'warning' | 'error' | 'papaya' | 'neutral'
  count: number
}

export type EvalStoredRunSummary = {
  id: string
  adapterRunID: string
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
  repCount: number
  assessmentName: 'Recorded outcome'
  outcomes: EvalRunOutcome[]
}

export type EvalRunSearch = {
  query?: string
  outcomes?: readonly string[]
  sort?: 'started' | 'name'
  direction?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

export type EvalRunSearchPage = {
  runs: EvalStoredRunSummary[]
  total: number
  hasMore: boolean
  /** Run counts per outcome after text search, before the outcome filter. */
  facets: EvalRunOutcome[]
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

export type EvalRunSourcePage = {
  summary: EvalStoredRunSummary
  /** Exact JSON values as strings: response.json() cannot round source numbers. */
  contextJSON: string
  rows: { ordinal: number; json: string }[]
  /** Global source-row ordinal cursor, not a page number or run-relative offset. */
  nextAfter: number
  hasMore: boolean
}
