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
      attributes = 's.attribute_ids'
      item = 'trace_id_wire(s.trace_id)'
      record = 's.span_id'
      spans =
        'to_json(list(DISTINCT span_id_wire(s.span_id) ORDER BY span_id_wire(s.span_id)))'
      resource = 's.resource_id'
      scope = 's.scope_id'
      time = ownerTime = 's.start_time'
      if (options.ownerType === 'event') {
        from +=
          ' JOIN events e ON e.trace_id = s.trace_id AND e.span_id = s.span_id'
        attributes = 'e.attribute_ids'
        ownerTime = 'e.timestamp'
      } else if (options.ownerType === 'link') {
        from +=
          ' JOIN links l ON l.trace_id = s.trace_id AND l.span_id = s.span_id'
        attributes = 'l.attribute_ids'
      }
      break
    case 'logs':
      from = 'logs l'
      attributes = 'l.attribute_ids'
      item = 'l.id::VARCHAR'
      record = 'l.id'
      resource = 'l.resource_id'
      scope = 'l.scope_id'
      time = ownerTime =
        'coalesce(nullif(l.timestamp, 0), l.observed_timestamp)'
      break
    case 'metrics':
      from = 'metric_datapoints d JOIN metrics m ON m.id = d.metric_id'
      attributes = 'd.attribute_ids'
      item = 'm.id::VARCHAR'
      record = 'd.id'
      resource = 'm.resource_id'
      scope = 'm.scope_id'
      time = ownerTime = 'd.timestamp'
      if (options.ownerType === 'exemplar') {
        from += ' JOIN exemplars e ON e.metric_datapoint_id = d.id'
        attributes = 'e.attribute_ids'
        ownerTime = 'e.timestamp'
      } else if (options.ownerType === 'metadata') attributes = 'm.metadata_ids'
      break
  }
  if (options.ownerType === 'resource') {
    from += ` JOIN resources r ON r.id = ${resource}`
    attributes = 'r.attribute_ids'
  } else if (options.ownerType === 'scope') {
    from += ` JOIN scopes sc ON sc.id = ${scope}`
    attributes = 'sc.attribute_ids'
  }
  const predicates = [
    `EXISTS (SELECT 1 FROM unnest(${attributes}) owned(id) JOIN attributes a ON a.id = owned.id WHERE a.key = ${literal(options.key)} AND otel_value_kind(a.value) = ${literal(options.kind)}${options.valueIdentity === undefined ? '' : ` AND otel_value_json(a.value) = ${literal(storedIdentity(options.valueIdentity))}::JSON`})`,
  ]
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
