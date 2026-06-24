/** Decode a wire value (JSON string/number or bigint) to bigint. */
export function parseBigInt(value: unknown): bigint {
  if (typeof value === 'bigint') return value
  if (typeof value === 'string' || typeof value === 'number') return BigInt(value)
  throw new Error(`Invalid bigint value: ${String(value)}`)
}

/** Nullable wire bigint; null/undefined → null. */
export function parseNullableBigInt(value: unknown): bigint | null {
  if (value === null || value === undefined) return null
  return parseBigInt(value)
}
