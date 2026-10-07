/** Compact SI formatting for axes, tooltips, and legends, including sub-unit values. */

const BIG_PREFIXES: ReadonlyArray<{ divisor: number; suffix: string }> = [
  { divisor: 1e12, suffix: 'T' },
  { divisor: 1e9, suffix: 'G' },
  { divisor: 1e6, suffix: 'M' },
  { divisor: 1e3, suffix: 'k' },
]

// Use Greek mu (U+03BC), Unicode's preferred micro prefix character.
const SMALL_PREFIXES: ReadonlyArray<{ divisor: number; suffix: string }> = [
  { divisor: 1e-3, suffix: 'm' },
  { divisor: 1e-6, suffix: 'µ' },
  { divisor: 1e-9, suffix: 'n' },
  { divisor: 1e-12, suffix: 'p' },
]

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
  if (!s.includes('.')) return s
  return s.replace(/\.?0+$/, '')
}

function formatMantissa(value: number, sigDigits: number): string {
  return trimTrailingZeros(value.toPrecision(sigDigits))
}

export function formatMetricValue(
  value: number | null | undefined,
  options: FormatMetricValueOptions = {}
): string {
  if (value === null || value === undefined) return ''
  if (!Number.isFinite(value)) return String(value)
  if (value === 0) return Object.is(value, -0) ? '-0' : '0'

  const sigDigits = options.maxSignificantDigits ?? 3
  const sign = value < 0 ? '-' : ''
  const abs = Math.abs(value)

  if (abs >= 1) {
    for (const { divisor, suffix } of BIG_PREFIXES) {
      if (abs >= divisor) {
        return sign + formatMantissa(abs / divisor, sigDigits) + suffix
      }
    }
    return sign + formatMantissa(abs, sigDigits)
  }

  for (const { divisor, suffix } of SMALL_PREFIXES) {
    if (abs >= divisor) {
      return sign + formatMantissa(abs / divisor, sigDigits) + suffix
    }
  }

  // Preserve values below 1p with exponential notation rather than rounding to zero.
  return sign + formatMantissa(abs, sigDigits)
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
