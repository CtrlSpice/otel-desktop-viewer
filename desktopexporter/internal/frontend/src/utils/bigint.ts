export type BigIntSource = string | number | bigint

type BigIntCandidate = BigIntSource | boolean | null | undefined

/** Identify the runtime representations accepted by the bigint converter. */
export function isBigIntSource<T>(value: T): value is T & BigIntSource {
  switch (typeof value) {
    case 'bigint':
    case 'string':
    case 'number':
      return true
    default:
      return false
  }
}

/** Decode a wire value (JSON string/number or bigint) to bigint. */
export function parseBigInt(value: BigIntCandidate): bigint {
  if (isBigIntSource(value)) return BigInt(value)
  throw new Error(`Invalid bigint value: ${String(value)}`)
}

/** Nullable wire bigint; null/undefined → null. */
export function parseNullableBigInt(value: BigIntCandidate): bigint | null {
  if (value === null || value === undefined) return null
  return parseBigInt(value)
}
