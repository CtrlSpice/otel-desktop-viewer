/** Compare two rows by an optional string field (nullish → ""), locale-aware. */
export function compareByStringField<T>(
  a: T,
  b: T,
  pick: (t: T) => string | undefined
): number {
  return (pick(a) ?? "").localeCompare(pick(b) ?? "")
}

/** Compare optional bigint timestamps with `<` / `>`, not subtraction, so large nanosecond values stay exact (no Number coercion). */
export function compareByTimestampField<T>(
  a: T,
  b: T,
  pick: (t: T) => bigint | undefined
): number {
  const aTime = pick(a) ?? 0n
  const bTime = pick(b) ?? 0n
  return aTime < bTime ? -1 : aTime > bTime ? 1 : 0
}