import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'

export type HeatmapDatum = {
  columnKey: string
  timestampNs: bigint
  timestampMs: number
  bucketKey: string
  bucketLabel: string
  count: number
}

export type HeatmapColumn = {
  key: string
  timestampNs: bigint
  timestampMs: number
}

export type HeatmapBucketRow = {
  key: string
  label: string
  order: number
}

export type HistogramHeatmapData = {
  columns: HeatmapColumn[]
  rows: HeatmapBucketRow[]
  cells: HeatmapDatum[]
  countByColumn: ReadonlyMap<string, ReadonlyMap<string, number>>
  maxCount: number
  distinctNonZeroCount: number
}

export type HistogramHeatmapBuildStats = {
  explicitSchemaCount: number
  exponentialSchemaCount: number
  descriptorConstructionCount: number
}

type ExplicitPoint = Extract<HistogramSlicePoint, { kind: 'histogram' }>
type ExponentialPoint = Extract<HistogramSlicePoint, { kind: 'expHistogram' }>
type ExplicitSchema = {
  bounds: readonly number[]
  countLength: number
  descriptors: HeatmapBucketRow[]
}
type ExponentialSchema = {
  scale: number
  zeroThreshold: number
  negativeOffset: number
  negativeCount: number
  positiveOffset: number
  positiveCount: number
  descriptors: HeatmapBucketRow[]
}

function formatBound(value: number): string {
  if (value === 0) return '0'
  if (Math.abs(value) >= 1000) return value.toExponential(1)
  if (Math.abs(value) < 0.01) return value.toExponential(1)
  return value.toPrecision(3)
}

function numberIdentity(value: number): string {
  if (Number.isNaN(value)) return 'nan'
  if (Object.is(value, -0)) return '-0'
  if (value === Infinity) return '+infinity'
  if (value === -Infinity) return '-infinity'
  return String(value)
}

function exponentialZeroLabel(zeroThreshold: number): string {
  if (!(zeroThreshold > 0)) return '0'
  const threshold = formatBound(zeroThreshold)
  return `[-${threshold}, +${threshold}]`
}

function projectHistogramHeatmapData(
  points: readonly HistogramSlicePoint[],
  stats?: HistogramHeatmapBuildStats
): HistogramHeatmapData {
  const occurrences = new Map<bigint, number>()
  const columns = points.map(point => {
    const occurrence = occurrences.get(point.timestamp) ?? 0
    occurrences.set(point.timestamp, occurrence + 1)
    const timestampKey = `timestamp:${point.timestamp}`
    return {
      key:
        occurrence === 0
          ? timestampKey
          : `${timestampKey}:occurrence:${occurrence}`,
      timestampNs: point.timestamp,
      timestampMs: Number(point.timestamp / 1_000_000n),
    }
  })

  const rowByKey = new Map<string, HeatmapBucketRow>()
  const cells: HeatmapDatum[] = []
  const countByColumn = new Map<string, Map<string, number>>()
  const distinctNonZeroCounts = new Set<number>()
  // Shared-array references are an O(1) fast path. Hash buckets reuse schemas
  // across separately decoded arrays; raw comparisons make collisions benign.
  const explicitSchemasByReference = new WeakMap<
    readonly number[],
    Map<number, HeatmapBucketRow[]>
  >()
  const explicitSchemasByHash = new Map<number, ExplicitSchema[]>()
  const exponentialSchemasByHash = new Map<number, ExponentialSchema[]>()
  const numberBits = new DataView(new ArrayBuffer(8))
  let maxCount = 0

  function mixNumber(hash: number, value: number): number {
    if (Number.isNaN(value)) {
      numberBits.setUint32(0, 0x7ff80000)
      numberBits.setUint32(4, 0)
    } else {
      numberBits.setFloat64(0, value)
    }
    hash = Math.imul(hash ^ numberBits.getUint32(0), 16777619)
    hash = Math.imul(hash ^ numberBits.getUint32(4), 16777619)
    return hash >>> 0
  }

  function explicitSchemaHash(
    bounds: readonly number[],
    countLength: number
  ): number {
    let hash = mixNumber(2166136261, countLength)
    hash = mixNumber(hash, bounds.length)
    for (const bound of bounds) hash = mixNumber(hash, bound)
    return hash
  }

  function exponentialSchemaHash(point: ExponentialPoint): number {
    let hash = mixNumber(2166136261, point.scale)
    hash = mixNumber(hash, point.zeroThreshold)
    hash = mixNumber(hash, point.negativeOffset)
    hash = mixNumber(hash, point.negativeCounts.length)
    hash = mixNumber(hash, point.positiveOffset)
    return mixNumber(hash, point.positiveCounts.length)
  }

  function sameBounds(
    left: readonly number[],
    right: readonly number[]
  ): boolean {
    if (left.length !== right.length) return false
    for (let index = 0; index < left.length; index++) {
      if (!Object.is(left[index], right[index])) return false
    }
    return true
  }

  function buildExplicitDescriptors(
    bounds: readonly number[],
    countLength: number
  ): HeatmapBucketRow[] {
    const descriptors: HeatmapBucketRow[] = []
    for (let bucketIndex = 0; bucketIndex < countLength; bucketIndex++) {
      let label: string
      let order: number
      let lo: number
      let hi: number
      if (bounds.length === 0) {
        label = 'all values'
        order = 0
        lo = -Infinity
        hi = Infinity
      } else if (bucketIndex === 0) {
        label = `≤${formatBound(bounds[0]!)}`
        order = -Infinity
        lo = -Infinity
        hi = bounds[0]!
      } else if (bucketIndex < bounds.length) {
        lo = bounds[bucketIndex - 1]!
        hi = bounds[bucketIndex]!
        order = (lo + hi) / 2
        label = `(${formatBound(lo)}, ${formatBound(hi)}]`
      } else {
        lo = bounds[bounds.length - 1]!
        hi = Infinity
        order = Infinity
        label = `>${formatBound(lo)}`
      }
      descriptors.push({
        key: `explicit:${numberIdentity(lo)}:${numberIdentity(hi)}`,
        label,
        order,
      })
    }
    return descriptors
  }

  function rememberExplicitReference(
    bounds: readonly number[],
    countLength: number,
    descriptors: HeatmapBucketRow[]
  ) {
    let byCountLength = explicitSchemasByReference.get(bounds)
    if (!byCountLength) {
      byCountLength = new Map()
      explicitSchemasByReference.set(bounds, byCountLength)
    }
    byCountLength.set(countLength, descriptors)
  }

  function explicitDescriptors(point: ExplicitPoint): HeatmapBucketRow[] {
    const referenced = explicitSchemasByReference
      .get(point.bounds)
      ?.get(point.counts.length)
    if (referenced) return referenced

    const hash = explicitSchemaHash(point.bounds, point.counts.length)
    const candidates = explicitSchemasByHash.get(hash)
    const matched = candidates?.find(
      candidate =>
        candidate.countLength === point.counts.length &&
        sameBounds(candidate.bounds, point.bounds)
    )
    if (matched) {
      rememberExplicitReference(
        point.bounds,
        point.counts.length,
        matched.descriptors
      )
      return matched.descriptors
    }

    const descriptors = buildExplicitDescriptors(
      point.bounds,
      point.counts.length
    )
    const schema = {
      bounds: point.bounds,
      countLength: point.counts.length,
      descriptors,
    }
    if (candidates) candidates.push(schema)
    else explicitSchemasByHash.set(hash, [schema])
    rememberExplicitReference(point.bounds, point.counts.length, descriptors)
    if (stats) {
      stats.explicitSchemaCount++
      stats.descriptorConstructionCount += descriptors.length
    }
    return descriptors
  }

  function buildExponentialDescriptors(
    point: ExponentialPoint
  ): HeatmapBucketRow[] {
    const descriptors: HeatmapBucketRow[] = []
    const base = Math.pow(2, Math.pow(2, -point.scale))
    for (
      let countIndex = point.negativeCounts.length - 1;
      countIndex >= 0;
      countIndex--
    ) {
      const exponent = point.negativeOffset + countIndex
      const lo = -Math.pow(base, exponent + 1)
      const hi = -Math.pow(base, exponent)
      const order = (lo + hi) / 2
      descriptors.push({
        key: `exponential:${point.scale}:negative:${exponent}`,
        label: formatBound(order),
        order,
      })
    }
    descriptors.push({
      key: `exponential:zero:${numberIdentity(point.zeroThreshold)}`,
      label: exponentialZeroLabel(point.zeroThreshold),
      order: 0,
    })
    for (
      let countIndex = 0;
      countIndex < point.positiveCounts.length;
      countIndex++
    ) {
      const exponent = point.positiveOffset + countIndex
      const lo = Math.pow(base, exponent)
      const hi = Math.pow(base, exponent + 1)
      const order = (lo + hi) / 2
      descriptors.push({
        key: `exponential:${point.scale}:positive:${exponent}`,
        label: formatBound(order),
        order,
      })
    }
    return descriptors
  }

  function exponentialDescriptors(point: ExponentialPoint): HeatmapBucketRow[] {
    const hash = exponentialSchemaHash(point)
    const candidates = exponentialSchemasByHash.get(hash)
    const matched = candidates?.find(
      candidate =>
        Object.is(candidate.scale, point.scale) &&
        Object.is(candidate.zeroThreshold, point.zeroThreshold) &&
        Object.is(candidate.negativeOffset, point.negativeOffset) &&
        candidate.negativeCount === point.negativeCounts.length &&
        Object.is(candidate.positiveOffset, point.positiveOffset) &&
        candidate.positiveCount === point.positiveCounts.length
    )
    if (matched) return matched.descriptors

    const descriptors = buildExponentialDescriptors(point)
    const schema = {
      scale: point.scale,
      zeroThreshold: point.zeroThreshold,
      negativeOffset: point.negativeOffset,
      negativeCount: point.negativeCounts.length,
      positiveOffset: point.positiveOffset,
      positiveCount: point.positiveCounts.length,
      descriptors,
    }
    if (candidates) candidates.push(schema)
    else exponentialSchemasByHash.set(hash, [schema])
    if (stats) {
      stats.exponentialSchemaCount++
      stats.descriptorConstructionCount += descriptors.length
    }
    return descriptors
  }

  function addCount(
    column: HeatmapColumn,
    descriptor: HeatmapBucketRow,
    count: number
  ) {
    if (!rowByKey.has(descriptor.key)) rowByKey.set(descriptor.key, descriptor)
    if (count === 0) return

    let columnCounts = countByColumn.get(column.key)
    if (!columnCounts) {
      columnCounts = new Map()
      countByColumn.set(column.key, columnCounts)
    }
    columnCounts.set(
      descriptor.key,
      (columnCounts.get(descriptor.key) ?? 0) + count
    )
  }

  for (let pointIndex = 0; pointIndex < points.length; pointIndex++) {
    const point = points[pointIndex]!
    const column = columns[pointIndex]!
    if (point.kind === 'histogram') {
      const descriptors = explicitDescriptors(point)
      for (let index = 0; index < point.counts.length; index++) {
        addCount(column, descriptors[index]!, point.counts[index]!)
      }
      continue
    }

    const descriptors = exponentialDescriptors(point)
    let descriptorIndex = 0
    for (
      let countIndex = point.negativeCounts.length - 1;
      countIndex >= 0;
      countIndex--
    ) {
      addCount(
        column,
        descriptors[descriptorIndex++]!,
        point.negativeCounts[countIndex]!
      )
    }

    addCount(column, descriptors[descriptorIndex++]!, point.zeroCount)

    for (const count of point.positiveCounts) {
      addCount(column, descriptors[descriptorIndex++]!, count)
    }
  }

  for (const column of columns) {
    const columnCounts = countByColumn.get(column.key)
    if (!columnCounts) continue
    for (const [bucketKey, count] of columnCounts) {
      const descriptor = rowByKey.get(bucketKey)!
      cells.push({
        columnKey: column.key,
        timestampNs: column.timestampNs,
        timestampMs: column.timestampMs,
        bucketKey,
        bucketLabel: descriptor.label,
        count,
      })
      if (count > maxCount) maxCount = count
      if (count > 0) distinctNonZeroCounts.add(count)
    }
  }

  const rows = [...rowByKey.values()]
    .sort((a, b) => a.order - b.order)
    .reverse()

  return {
    columns,
    rows,
    cells,
    countByColumn,
    maxCount,
    distinctNonZeroCount: distinctNonZeroCounts.size,
  }
}

/**
 * Builds a sparse rendering and lookup model. Columns and rows describe the
 * logical keyboard grid; only nonzero intersections allocate cells/counts.
 */
export function buildHistogramHeatmapData(
  points: readonly HistogramSlicePoint[]
): HistogramHeatmapData {
  return projectHistogramHeatmapData(points)
}

/** Pure instrumentation entry point for structural projection tests. */
export function buildHistogramHeatmapDataWithStats(
  points: readonly HistogramSlicePoint[]
): { data: HistogramHeatmapData; stats: HistogramHeatmapBuildStats } {
  const stats: HistogramHeatmapBuildStats = {
    explicitSchemaCount: 0,
    exponentialSchemaCount: 0,
    descriptorConstructionCount: 0,
  }
  return {
    data: projectHistogramHeatmapData(points, stats),
    stats,
  }
}
