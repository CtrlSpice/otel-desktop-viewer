import { afterEach, expect, it, vi } from 'vitest'
import { telemetryAPI } from '@/services/telemetry-service'
import type { HomeCoveragePage } from '@/services/discovery-types'
import { loadAttributeCoverage, mergeAttributeKeys } from './attribute-coverage'

afterEach(() => vi.restoreAllMocks())

it('merges keys by exact key, kind and provenance while retaining signal ownership', () => {
  const keys = mergeAttributeKeys([
    {
      key: 'env',
      kind: 'string',
      ownerType: 'resource',
      foundOn: [{ signal: 'traces', ownerType: 'resource' }],
    },
    {
      key: 'env',
      kind: 'string',
      ownerType: 'resource',
      foundOn: [{ signal: 'logs', ownerType: 'resource' }],
    },
    {
      key: 'env',
      kind: 'int64',
      ownerType: 'resource',
      foundOn: [{ signal: 'traces', ownerType: 'resource' }],
    },
    {
      key: 'env',
      kind: 'string',
      ownerType: 'span',
      foundOn: [{ signal: 'traces', ownerType: 'span' }],
    },
  ])
  expect(keys).toHaveLength(3)
  expect(
    keys.find(key => key.kind === 'string' && key.ownerType === 'resource')
      ?.foundOn
  ).toEqual([
    { signal: 'traces', ownerType: 'resource' },
    { signal: 'logs', ownerType: 'resource' },
  ])
})

const counts = { traces: 1001, logs: 99, metrics: 0 }
const page: HomeCoveragePage = {
  values: [
    {
      identity: 'home:exact-reference',
      valueRef: 'home:exact-reference',
      kind: 'string',
      preview: 'shared',
      colorHash: 17,
      count: 600,
      counts: { traces: 501, logs: 99, metrics: 0 },
      kindCount: 1100,
      denominator: 1100,
      relativeFrequency: 600 / 1100,
      foundOn: [
        { signal: 'traces', ownerType: 'resource' },
        { signal: 'logs', ownerType: 'resource' },
      ],
    },
  ],
  kinds: [{ kind: 'string', count: 1100, counts }],
  denominators: counts,
  denominator: 1100,
  total: 501,
  truncated: true,
}

it('requests one server-sorted page without fetching other pages or reconstructing identities', async () => {
  const read = vi
    .spyOn(telemetryAPI, 'discoverHomeAttributeValues')
    .mockResolvedValue(page)
  const signal = new AbortController().signal
  const options = {
    ownerType: 'resource' as const,
    key: 'env',
    valueKind: 'string',
    offset: 25,
    limit: 25,
    sortBy: 'frequency' as const,
    sortDirection: 'desc' as const,
    queries: { traces: null, logs: null, metrics: false as const },
  }
  const coverage = await loadAttributeCoverage(options, signal)
  expect(read).toHaveBeenCalledExactlyOnceWith(options, signal)
  expect(coverage.values).toBe(page.values)
  expect(coverage.total).toBe(501)
  expect(coverage.kinds.get('string')).toEqual({ count: 1100, counts })
  expect(coverage.denominators).toEqual(counts)
})

it('does not start canceled work and rejects a response canceled while loading', async () => {
  const controller = new AbortController()
  const pending = Promise.withResolvers<HomeCoveragePage>()
  const read = vi
    .spyOn(telemetryAPI, 'discoverHomeAttributeValues')
    .mockReturnValue(pending.promise)
  const request = loadAttributeCoverage(
    { ownerType: 'resource', key: 'env' },
    controller.signal
  )
  controller.abort()
  pending.resolve(page)
  await expect(request).rejects.toThrow()
  read.mockClear()
  await expect(
    loadAttributeCoverage({ ownerType: 'resource' }, controller.signal)
  ).rejects.toThrow()
  expect(read).not.toHaveBeenCalled()
})
