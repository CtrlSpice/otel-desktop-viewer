/** A retention/refresh race must not turn a missing row into a sort value. */
export function validateSeriesOrder(
  order: readonly string[],
  cohort: readonly string[]
): void {
  const expected = new Set(cohort)
  for (const id of order) {
    if (!expected.delete(id))
      throw new Error('Series changed. Refresh the metric.')
  }
  if (expected.size) throw new Error('Series changed. Refresh the metric.')
}
