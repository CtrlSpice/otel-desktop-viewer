export const ARM_C_FLAT_FORMAT = 'odv.trace-waterfall.flat.v1' as const
export const ARM_C_FLAT_PATH =
  '/benchmark-api/trace-waterfall/flat-rows' as const

export type FixtureTopology =
  'rooted-tree' | 'multiple-roots' | 'orphan' | 'cycle'

export type ArmRunContract = {
  fixtureName: string
  fixtureSHA256: string
  fixtureBytes: number
  inputSpanCount: number
  fixtureTopology: FixtureTopology
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

export type ArmRunResult = {
  arm: 'A' | 'C'
  fixtureName: string
  fixtureSHA256: string
  fixtureBytes: number
  inputSpanCount: number
  fixtureTopology: FixtureTopology
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
  runArmA: (contract: ArmRunContract) => Promise<ArmRunResult>
  runArmC: (contract: ArmRunContract) => Promise<ArmRunResult>
}

declare global {
  interface Window {
    __TRACE_WATERFALL_BENCHMARK__: TraceWaterfallBenchmarkAPI
  }
}
