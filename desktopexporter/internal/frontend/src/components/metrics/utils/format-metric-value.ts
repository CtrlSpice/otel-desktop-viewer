/** Display formatting only: received values and units are not converted. */
const graphNumberFormat = new Intl.NumberFormat('en-US', {
  maximumSignificantDigits: 3,
})
const hoverNumberFormat = new Intl.NumberFormat('en-US', {
  maximumSignificantDigits: 6,
})
const countFormat = new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 })
const superscripts = '⁰¹²³⁴⁵⁶⁷⁸⁹'

export type FormatMetricValueOptions = {
  /** Maximum significant digits in the mantissa; defaults to 3. */
  maxSignificantDigits?: number
}

export type FormatMetricValuePlainOptions = FormatMetricValueOptions & {
  /** OTLP metric unit appended after the number (skipped when empty or "1"). */
  unit?: string
  /** Maximum fractional digits. Defaults to 6 for detail rows. */
  maxFractionDigits?: number
}

function trimTrailingZeros(s: string): string {
  if (!s.includes('.') || s.includes('e')) return s
  return s.replace(/\.?0+$/, '')
}

function decimalFormat(significantDigits: number): Intl.NumberFormat {
  if (significantDigits === 3) return graphNumberFormat
  if (significantDigits === 6) return hoverNumberFormat
  return new Intl.NumberFormat('en-US', {
    maximumSignificantDigits: significantDigits,
  })
}

export function formatMetricValue(
  value: number | null | undefined,
  options: FormatMetricValueOptions = {}
): string {
  if (value === null || value === undefined) return ''
  if (!Number.isFinite(value)) return String(value)
  if (value === 0) return Object.is(value, -0) ? '-0' : '0'

  const sigDigits = options.maxSignificantDigits ?? 3
  const rounded = Number(value.toPrecision(sigDigits))
  const abs = Math.abs(rounded)
  if (abs >= 1e-6 && abs < 1e6) return decimalFormat(sigDigits).format(value)
  const [mantissa, exponent] = value.toExponential(sigDigits - 1).split('e')
  const power = String(Number(exponent)).replace(/[-\d]/g, digit =>
    digit === '-' ? '⁻' : superscripts[Number(digit)]!
  )
  return `${Number(mantissa)} × 10${power}`
}

export function formatMetricHoverValue(
  value: number | null | undefined
): string {
  return formatMetricValue(value, { maxSignificantDigits: 6 })
}

export function formatMetricCount(value: number | bigint): string {
  return countFormat.format(value)
}

/** Raise display precision only when distinct finite bounds would share a label. */
export function metricNumberPrecision(
  values: readonly number[],
  minimum = 3
): number {
  const distinct = [...new Set(values.filter(Number.isFinite))]
  for (let digits = minimum; digits < 17; digits++) {
    const labels = new Set(distinct.map(value => value.toPrecision(digits)))
    if (labels.size === distinct.length) return digits
  }
  // Seventeen significant decimal digits distinguish finite IEEE-754 doubles.
  return 17
}

/** Rate slope (Δrate/Δt) with optional OTLP unit suffix. */
export function formatRateSlopeValue(
  value: number | null | undefined,
  unit?: string,
  options: FormatMetricValueOptions = {}
): string {
  if (value === null || value === undefined) return ''
  const formatted = formatMetricValue(value, options)
  const trimmed = unit?.trim()
  if (!trimmed || trimmed === '1') return `${formatted}/s²`
  return `${formatted} ${trimmed}/s²`
}

/** Plain decimal with an optional OTLP unit for detail rows. */
export function formatMetricValuePlain(
  value: number | null | undefined,
  options: FormatMetricValuePlainOptions = {}
): string {
  if (value === null || value === undefined) return ''
  if (!Number.isFinite(value)) return String(value)

  const maxFrac = options.maxFractionDigits ?? 6
  const number = Object.is(value, -0)
    ? '-0'
    : trimTrailingZeros(value.toFixed(maxFrac))
  const unit = options.unit?.trim()
  if (!unit || unit === '1') return number
  return `${number} ${unit}`
}
