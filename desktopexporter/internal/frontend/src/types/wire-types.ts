// JSON RPC contracts. Telemetry reads use Arrow.

export type JsonStats = {
  storage: { sizeBytes: number; maxSizeBytes: number; inMemory?: boolean }
  traces: {
    traceCount: number
    spanCount: number
    serviceCount: number
    errorCount: number
    lastReceived: string | null
  }
  logs: { logCount: number; errorCount: number; lastReceived: string | null }
  metrics: {
    metricCount: number
    dataPointCount: number
    lastReceived: string | null
  }
  rejections: {
    signal: 'traces' | 'logs' | 'metrics'
    kind: string
    occurrences: number
    samples: { traceID: string; spanID: string }[]
    firstSeen: string
    lastSeen: string
  }[]
}

export type JsonAttributeScope =
  | 'resource'
  | 'scope'
  | 'span'
  | 'event'
  | 'link'
  | 'log'
  | 'datapoint'
  | 'exemplar'
  | 'metadata'

/** count is the number of requested IDs accepted, not the number of rows removed. */
export type JsonDeleteResult = { message: string; count: number }

export type JsonQueryField = {
  owner: string
  path: readonly (string | number)[]
  type?: string
}

export type JsonQueryNode =
  | {
      id: string
      type: 'text'
      query: { fieldOperator: string; value: string }
    }
  | {
      id: string
      type: 'condition'
      query: { field: JsonQueryField; fieldOperator: string; value: string }
    }
  | {
      id: string
      type: 'group'
      group: { logicalOperator: string; children: JsonQueryNode[] }
    }
