import { telemetryAPI } from '@/services/telemetry-service'
import type {
  AttributeFrequency,
  AttributeKey,
  DiscoveryOptions,
  DiscoveryOwner,
  DiscoverySignal,
} from '@/services/discovery-types'
import { attributeValueCopy } from '@/components/shared/detail-value'

export const attributeOwners: Record<DiscoverySignal, DiscoveryOwner[]> = {
  traces: ['resource', 'scope', 'span', 'event', 'link'],
  logs: ['resource', 'scope', 'log'],
  metrics: ['resource', 'scope', 'datapoint', 'exemplar', 'metadata'],
}
export const discoverySignals = ['traces', 'logs', 'metrics'] as const
export type DiscoveredAttribute = AttributeKey & { ownerType: DiscoveryOwner }
export type RecordCounts = Record<DiscoverySignal, number>
export type CoverageValue = AttributeFrequency & { counts: RecordCounts }
export type AttributeCoverage = {
  values: CoverageValue[]
  denominators: RecordCounts
  denominator: number
  overlapping: boolean
}

export function attributeIdentity(entry: DiscoveredAttribute): string {
  return JSON.stringify([entry.key, entry.kind, entry.ownerType])
}

export function mergeAttributeKeys(
  entries: DiscoveredAttribute[]
): DiscoveredAttribute[] {
  const keys = new Map<string, DiscoveredAttribute>()
  for (const entry of entries) {
    const identity = attributeIdentity(entry)
    const previous = keys.get(identity)
    keys.set(identity, {
      ...entry,
      foundOn: discoverySignals.flatMap(signal =>
        [...(previous?.foundOn ?? []), ...entry.foundOn].some(
          location => location.signal === signal
        )
          ? [{ signal, ownerType: entry.ownerType }]
          : []
      ),
    })
  }
  return [...keys.values()].sort(
    (a, b) =>
      a.key.localeCompare(b.key) ||
      a.kind.localeCompare(b.kind) ||
      a.ownerType.localeCompare(b.ownerType)
  )
}

/** Pool record counts, never percentages. Each signal has a disjoint record identity. */
export function mergeAttributeValues(
  groups: { signal: DiscoverySignal; values: AttributeFrequency[] }[]
): AttributeCoverage {
  const denominators: RecordCounts = { traces: 0, logs: 0, metrics: 0 }
  const merged = new Map<string, CoverageValue>()
  for (const group of groups) {
    denominators[group.signal] = group.values[0]?.denominator ?? 0
    for (const item of group.values) {
      const previous = merged.get(item.identity)
      const counts = previous?.counts ?? { traces: 0, logs: 0, metrics: 0 }
      counts[group.signal] = item.count
      merged.set(item.identity, {
        ...item,
        counts,
        foundOn: [...(previous?.foundOn ?? []), ...item.foundOn],
        count: Object.values(counts).reduce((sum, count) => sum + count, 0),
      })
    }
  }
  const denominator = Object.values(denominators).reduce(
    (sum, count) => sum + count,
    0
  )
  const values = [...merged.values()].map(item => ({
    ...item,
    denominator,
    relativeFrequency: denominator ? item.count / denominator : 0,
  }))
  return {
    values,
    denominators,
    denominator,
    overlapping:
      values.reduce((sum, item) => sum + item.count, 0) > denominator,
  }
}

/** Read the full distribution before combining signals, sorting, or drawing a bar. */
export async function loadAttributeCoverage(
  options: Omit<DiscoveryOptions, 'kind' | 'signal'>,
  signal: AbortSignal
): Promise<AttributeCoverage> {
  const groups = await Promise.all(
    discoverySignals
      .values()
      .filter(
        source =>
          attributeOwners[source].includes(options.ownerType!) &&
          options.queries?.[source] !== false
      )
      .map(async source => {
        const values: AttributeFrequency[] = []
        let more = true
        while (more) {
          signal.throwIfAborted()
          const page = await telemetryAPI.discoverAttributeValues(
            {
              ...options,
              signal: source,
              offset: values.length,
              limit: 500,
              sortBy: 'frequency',
              sortDirection: 'desc',
            },
            signal
          )
          values.push(...page.items)
          more = page.truncated
        }
        return { signal: source, values }
      })
  )
  signal.throwIfAborted()
  return mergeAttributeValues(groups)
}

export function sortCoverageValues(
  values: CoverageValue[],
  by: 'name' | 'frequency',
  direction: 'asc' | 'desc'
): CoverageValue[] {
  const sign = direction === 'asc' ? 1 : -1
  return [...values].sort(
    (a, b) =>
      sign *
        (by === 'frequency'
          ? a.count - b.count
          : attributeValueCopy(a.value).localeCompare(
              attributeValueCopy(b.value)
            )) || a.identity.localeCompare(b.identity)
  )
}

export function coverageCounts(
  counts: RecordCounts,
  denominators?: RecordCounts
): string {
  const units = { traces: 'spans', logs: 'logs', metrics: 'datapoints' }
  return discoverySignals
    .values()
    .filter(signal =>
      denominators ? denominators[signal] > 0 : counts[signal] > 0
    )
    .map(
      signal =>
        `${counts[signal].toLocaleString()}${denominators ? ` / ${denominators[signal].toLocaleString()}` : ''} ${units[signal]}`
    )
    .toArray()
    .join(' · ')
}
