export type ArmARunContract = {
  fixtureName: string
  traceID: string
  expectedDisplayedSpanCount: number
  expectedMaximumDisplayedDepth: number
  expectedFirstSpanID: string
}

export type ViewportGeometry = {
  x: number
  y: number
  width: number
  height: number
}

export type BigIntValueProof = {
  type: string
  decimal: string
}

export type BigIntRehydrationProof = {
  allSpanStartAndEndTimesAreBigInts: boolean
  allEventTimestampsAreBigInts: boolean
  sampleExceedsNumberSafeInteger: boolean
  firstSpanStartTime: BigIntValueProof | null
  firstSpanEndTime: BigIntValueProof | null
  firstEventTimestamp: BigIntValueProof | null
}

export type BenchmarkSpanTopology = {
  spanID: string
  parentSpanID: string | null
  depth: number
  salvaged: boolean
  cyclePoint: boolean
}

export type ArmARunResult = {
  arm: 'A'
  fixtureName: string
  traceID: string
  displayedSpanCount: number
  maximumDisplayedDepth: number
  firstDisplayedSpanID: string | null
  displayRootSpanIDs: string[]
  unplacedSpanCount: number
  viewportGeometry: ViewportGeometry
  mountedRowCount: number
  bigintRehydration: BigIntRehydrationProof
  semanticHash: {
    format: 'odv.trace-waterfall.semantic.v1+jcs'
    hash: string
  }
  topology: BenchmarkSpanTopology[]
}

export type TraceWaterfallBenchmarkAPI = {
  runArmA: (contract: ArmARunContract) => Promise<ArmARunResult>
}

declare global {
  interface Window {
    __TRACE_WATERFALL_BENCHMARK__: TraceWaterfallBenchmarkAPI
  }
}
