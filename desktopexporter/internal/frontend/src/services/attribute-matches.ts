import type { DiscoveryOwner, DiscoverySignal } from './discovery-types'

export type AttributeMatchOptions = {
  signal: DiscoverySignal
  ownerType: DiscoveryOwner
  key: string
  kind: string
  valueIdentity?: string
  startTime?: string | null
  endTime?: string | null
}
export type AttributeRecordMatch = {
  itemRef: string
  spanIDs: string[]
  count: number
}

const literal = (value: string) => `'${value.replaceAll("'", "''")}'`

// Stored tagged values use Go's JSON encoding. DuckDB compares JSON text, so
// restore its HTML/line-separator escapes after the RPC JSON decoder removes them.
const storedIdentity = (identity: string) =>
  identity.replace(
    /[<>&\u2028\u2029]/g,
    character => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`
  )

/** Exact owner/value membership, including empty values, arrays, and wire double bits.
 * Signal summaries apply the submitted search. Intersect their identities with
 * these matches so child timestamps and typed values stay attached to one owner.
 */
export function attributeMatchesSQL(
  options: AttributeMatchOptions,
  offset: number
): string {
  let from: string
  let attributes: string
  let item: string
  let record: string
  let spans = "'[]'::JSON"
  let resource: string
  let scope: string
  let time: string
  let ownerTime: string
  switch (options.signal) {
    case 'traces':
      from = 'spans s'
      attributes = 's.attributes'
      item = 'trace_id_wire(s.trace_id)'
      record = 's.span_id'
      spans =
        'to_json(list(DISTINCT span_id_wire(s.span_id) ORDER BY span_id_wire(s.span_id)))'
      resource = 's.resource.attributes'
      scope = 's.scope.attributes'
      time = ownerTime = 's.start_time'
      if (options.ownerType === 'event') {
        from +=
          ' JOIN events e ON e.trace_id = s.trace_id AND e.span_id = s.span_id'
        attributes = 'e.attributes'
        ownerTime = 'e.timestamp'
      } else if (options.ownerType === 'link') {
        from +=
          ' JOIN links l ON l.trace_id = s.trace_id AND l.span_id = s.span_id'
        attributes = 'l.attributes'
      }
      break
    case 'logs':
      from = 'logs l'
      attributes = 'l.attributes'
      item = 'l.id::VARCHAR'
      record = 'l.id'
      resource = 'l.resource.attributes'
      scope = 'l.scope.attributes'
      time = ownerTime =
        'coalesce(nullif(l.timestamp, 0), l.observed_timestamp)'
      break
    case 'metrics':
      from = 'metric_datapoints d JOIN metrics m ON m.id = d.metric_id'
      attributes = 'd.attributes'
      item = 'm.id::VARCHAR'
      record = 'd.id'
      resource = 'd.resource.attributes'
      scope = 'd.scope.attributes'
      time = ownerTime = 'd.timestamp'
      if (options.ownerType === 'exemplar') {
        from += ' JOIN exemplars e ON e.metric_datapoint_id = d.id'
        attributes = 'e.attributes'
        ownerTime = 'e.timestamp'
      } else if (options.ownerType === 'metadata') attributes = 'm.metadata'
      break
  }
  if (options.ownerType === 'resource') {
    attributes = resource
  } else if (options.ownerType === 'scope') {
    attributes = scope
  }
  const key = literal(options.key)
  const value = `${attributes}[${key}]`
  const predicates = [
    `variant_exists(${attributes}, ${key})`,
    `otel_value_kind(${value}) = ${literal(options.kind)}`,
  ]
  if (options.valueIdentity !== undefined)
    predicates.push(
      `otel_value_json(${value}) = ${literal(storedIdentity(options.valueIdentity))}::JSON`
    )
  for (const timestamp of new Set([time, ownerTime])) {
    if (options.startTime != null)
      predicates.push(
        `${timestamp} >= ${literal(BigInt(options.startTime).toString())}::UBIGINT`
      )
    if (options.endTime != null)
      predicates.push(
        `${timestamp} <= ${literal(BigInt(options.endTime).toString())}::UBIGINT`
      )
  }
  return `SELECT ${item} AS item_ref, ${spans} AS span_ids, count(DISTINCT ${record}) AS record_count
FROM ${from} WHERE ${predicates.join(' AND ')} GROUP BY ${item} ORDER BY item_ref OFFSET ${offset}`
}
