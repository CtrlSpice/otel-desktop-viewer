import { telemetryAPI } from '@/services/telemetry-service'
import {
  SIGNAL_ORDER as discoverySignals,
  type HomeCoverageValue,
  type AttributeKey,
  type DiscoveryOptions,
  type DiscoveryOwner,
  type DiscoverySignal,
} from '@/services/discovery-types'

export const attributeOwners: Record<DiscoverySignal, DiscoveryOwner[]> = {
  traces: ['resource', 'scope', 'span', 'event', 'link'],
  logs: ['resource', 'scope', 'log'],
  metrics: ['resource', 'scope', 'datapoint', 'exemplar', 'metadata'],
}
export { discoverySignals }
export type DiscoveredAttribute = AttributeKey & { ownerType: DiscoveryOwner }
export type RecordCounts = Record<DiscoverySignal, number>
export type CoverageValue = HomeCoverageValue
export type AttributeValueSort = 'name' | 'frequency'
export type AttributeCoverage = {
  values: CoverageValue[]
  kinds: Map<string, { counts: RecordCounts; count: number }>
  denominators: RecordCounts
  denominator: number
  total: number
  truncated: boolean
}

export function attributeIdentity(entry: DiscoveredAttribute): string {
  return JSON.stringify([entry.key, entry.kind, entry.ownerType])
}

export function attributeScope(
  options: Omit<DiscoveryOptions, 'kind'>
): string {
  return JSON.stringify([
    options.serviceIdentity ?? [
      options.namespace ?? '',
      options.serviceName ?? '',
    ],
    options.startTime ?? null,
    options.endTime ?? null,
    ...discoverySignals.map(signal => options.queries?.[signal] ?? null),
  ])
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

/** One globally aggregated, sorted server page. Full values remain server-side. */
export async function loadAttributeCoverage(
  options: Omit<DiscoveryOptions, 'kind' | 'signal'>,
  signal: AbortSignal
): Promise<AttributeCoverage> {
  signal.throwIfAborted()
  const page = await telemetryAPI.discoverHomeAttributeValues(options, signal)
  signal.throwIfAborted()
  return {
    ...page,
    kinds: new Map(page.kinds.map(({ kind, ...counts }) => [kind, counts])),
  }
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
