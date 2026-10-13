/** UI report supplied by an importer, not an OTLP field or an API wire shape. */
export type ImportFailure = {
  fileName: string
  reason: string
  /** UI failure time: BigInt(Date.now()) * 1_000_000n, Unix ns with millisecond precision. */
  occurredAt: bigint
}
