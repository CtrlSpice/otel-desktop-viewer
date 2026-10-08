/** UI report supplied by an importer, not an OTLP field or an API wire shape. */
export type ImportFailure = {
  fileName: string
  reason: string
  /** Time the import failed, in Unix nanoseconds. */
  occurredAt: bigint
}
