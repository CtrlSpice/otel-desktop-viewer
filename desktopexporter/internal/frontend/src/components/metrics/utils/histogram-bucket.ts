export function numberIdentity(value: number): string {
  if (Number.isNaN(value)) return 'nan'
  if (Object.is(value, -0)) return '-0'
  if (value === Infinity) return '+infinity'
  if (value === -Infinity) return '-infinity'
  return String(value)
}

export function formatHistogramBound(
  value: number,
  significantDigits = 3
): string {
  if (!Number.isFinite(value)) return value > 0 ? '+∞' : '-∞'
  return formatMetricValue(value, { maxSignificantDigits: significantDigits })
}

export function formatHistogramRange(
  lo: number,
  hi: number,
  minimumDigits = 3
): string {
  const digits = metricNumberPrecision([lo, hi], minimumDigits)
  return `(${formatHistogramBound(lo, digits)}, ${formatHistogramBound(hi, digits)}]`
}

export function exponentialZeroBucketLabel(
  zeroThreshold: number,
  significantDigits = 3
): string {
  if (!(zeroThreshold > 0)) return '0'
  const threshold = formatHistogramBound(zeroThreshold, significantDigits)
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
import { formatMetricValue, metricNumberPrecision } from './format-metric-value'
