/**
 * Format a numeric metric value for compact display on chart axes,
 * tooltips, and legend cells. Uses SI prefixes at both ends of the
 * magnitude range so we get useful labels without scientific
 * notation:
 *
 *     1234567   ->  1.23M
 *        1234   ->  1.23k
 *         123   ->  123
 *           0.5 ->  500m
 *           0.0004 -> 400µ
 *           0.0000007 -> 700n
 *
 * Why not just lean on `Intl.NumberFormat({ notation: 'compact' })`?
 * Intl handles big numbers fine ('1.2M') but for sub-unit values it
 * either rounds to 0 or falls back to scientific notation. For a
 * debugging tool that shows things like "0.0004 errors/sec" we want
 * a friendly "400µ" instead.
 *
 * Negatives mirror the positive logic. Zero is rendered without a
 * prefix. NaN / Infinity stringify directly so callers don't have
 * to special-case them.
 */

// SI prefix table. Ordered so we can binary-search by magnitude in
// each direction. Each entry is { divisor, suffix }. The "1" entry
// is the no-prefix case and acts as the boundary between positive
// and negative prefixes.
const BIG_PREFIXES: ReadonlyArray<{ divisor: number; suffix: string }> = [
  { divisor: 1e12, suffix: 'T' },
  { divisor: 1e9, suffix: 'G' },
  { divisor: 1e6, suffix: 'M' },
  { divisor: 1e3, suffix: 'k' },
]

// Greek mu (U+03BC) is preferred over micro sign (U+00B5) by Unicode;
// renders the same in every monospace + sans font we ship with.
const SMALL_PREFIXES: ReadonlyArray<{ divisor: number; suffix: string }> = [
  { divisor: 1e-3, suffix: 'm' },
  { divisor: 1e-6, suffix: 'µ' },
  { divisor: 1e-9, suffix: 'n' },
  { divisor: 1e-12, suffix: 'p' },
]

export type FormatMetricValueOptions = {
  /**
   * Maximum significant digits in the mantissa. 3 keeps labels at
   * "1.23k" / "400µ" width, which fits axis tick spacing cleanly.
   * Bump to 4 for tooltips if you want a bit more precision.
   */
  maxSignificantDigits?: number
}

export type FormatMetricValuePlainOptions = FormatMetricValueOptions & {
  /** OTLP metric unit appended after the number (skipped when empty or "1"). */
  unit?: string
  /** Maximum fractional digits. Defaults to 6 for detail rows. */
  maxFractionDigits?: number
}

/**
 * Strip trailing zeros + trailing decimal point from a fixed-precision
 * string. "1.230" -> "1.23"; "1.000" -> "1"; "100" -> "100".
 */
function trimTrailingZeros(s: string): string {
  if (!s.includes('.')) return s
  return s.replace(/\.?0+$/, '')
}

function formatMantissa(value: number, sigDigits: number): string {
  // toPrecision keeps the mantissa narrow regardless of magnitude;
  // then we strip cosmetic trailing zeros. (Intl handles this too,
  // but its compact notation isn't available below ~1.)
  return trimTrailingZeros(value.toPrecision(sigDigits))
}

export function formatMetricValue(
  value: number | null | undefined,
  options: FormatMetricValueOptions = {}
): string {
  if (value === null || value === undefined) return ''
  if (!Number.isFinite(value)) return String(value)
  if (value === 0) return '0'

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

  // Smaller than 1p: still useful to see *something* rather than "0".
  // Falls back to default toPrecision (will use exponential for very
  // small numbers); rare enough that it doesn't justify another prefix
  // table entry.
  return sign + formatMantissa(abs, sigDigits)
}

/** Plain decimal + optional OTLP unit for detail rows. Avoids SI
 *  suffixes (m, k) and scientific notation in contexts where the axis
 *  does not disambiguate scale. Charts keep {@link formatMetricValue}. */
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

export function formatMetricValuePlain(
  value: number | null | undefined,
  options: FormatMetricValuePlainOptions = {}
): string {
  if (value === null || value === undefined) return ''
  if (!Number.isFinite(value)) return String(value)

  const maxFrac = options.maxFractionDigits ?? 6
  const number = trimTrailingZeros(value.toFixed(maxFrac))
  const unit = options.unit?.trim()
  if (!unit || unit === '1') return number
  return `${number} ${unit}`
}
