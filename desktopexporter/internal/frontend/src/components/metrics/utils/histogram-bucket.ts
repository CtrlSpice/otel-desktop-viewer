export function numberIdentity(value: number): string {
  if (Number.isNaN(value)) return 'nan'
  if (Object.is(value, -0)) return '-0'
  if (value === Infinity) return '+infinity'
  if (value === -Infinity) return '-infinity'
  return String(value)
}

export function formatHistogramBound(value: number): string {
  if (!Number.isFinite(value)) return value > 0 ? '+∞' : '-∞'
  if (value === 0) return '0'
  if (Math.abs(value) >= 1000 || Math.abs(value) < 0.01) {
    return value.toExponential(1)
  }
  return value.toPrecision(3)
}

export function exponentialZeroBucketLabel(zeroThreshold: number): string {
  if (!(zeroThreshold > 0)) return '0'
  const threshold = formatHistogramBound(zeroThreshold)
  return `[-${threshold}, +${threshold}]`
}

export function histogramRangeIdentity(lo: number, hi: number): string {
  return `explicit:${numberIdentity(lo)}:${numberIdentity(hi)}`
}

export function indexedHistogramRangeIdentity(
  index: number,
  lo: number,
  hi: number
): string {
  return `explicit:${index}:${numberIdentity(lo)}:${numberIdentity(hi)}`
}

export function exponentialBucketIdentity(
  scale: number,
  side: 'negative' | 'positive',
  exponent: number
): string {
  return `exponential:${scale}:${side}:${exponent}`
}

export function exponentialZeroBucketIdentity(zeroThreshold: number): string {
  return `exponential:zero:${numberIdentity(zeroThreshold)}`
}
