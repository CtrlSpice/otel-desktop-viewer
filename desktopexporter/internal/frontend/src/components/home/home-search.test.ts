import { describe, expect, it } from 'vitest'
import { getStaticFieldsForSearch } from '@/constants/fields'
import { parseQuery } from '@/components/shared/Search/queryParser'
import { homeFields, homeQueries, queryText } from './home-search'
import {
  serviceSignalHref,
  type ServiceSummary,
} from '@/services/discovery-types'

const fields = homeFields(
  ['traces', 'metrics', 'logs'].map(signal =>
    getStaticFieldsForSearch(signal as 'traces' | 'metrics' | 'logs')
  )
)
const predicate = (text: string) => parseQuery(text, fields)
function withoutIDs(value: unknown) {
  return JSON.parse(
    JSON.stringify(value, (key, item) => (key === 'id' ? undefined : item))
  )
}

describe('cross-signal Home queries', () => {
  it('keeps supported OR branches and excludes impossible AND branches', () => {
    const disjunction = homeQueries(
      predicate('severityNumber >= 17 OR name = "checkout"')
    )
    expect(queryText(disjunction.logs)).toBe('severityNumber >= "17"')
    expect(queryText(disjunction.traces)).toBe('name = "checkout"')
    expect(queryText(disjunction.metrics)).toBe('name = "checkout"')
    const conjunction = homeQueries(
      predicate('severityNumber >= 17 AND name = "checkout"')
    )
    expect(Object.values(conjunction)).toEqual([false, false, false])
  })

  it('limits owner-specific attributes to their actual signals', () => {
    const queries = homeQueries(
      predicate('attr(event, "tool.output", string) CONTAINS "declined"')
    )
    expect(queries.logs).toBe(false)
    expect(queries.metrics).toBe(false)
    expect(queries.traces).not.toBe(false)
  })

  it.each([
    'search("  exact spacing  ")',
    'body !~ "healthcheck"',
    'search("checkout") AND attr(resource, "service.name", string) = "api"',
    'search("a\\\"b\\\\c") OR severityNumber >= 17',
    'attr(span, "exact", int64) IN [9007199254740993, -9223372036854775808]',
    'attr(resource, "service.namespace", string) = NULL',
    'duration > 1.5s',
  ])('round-trips editable links without losing values: %s', text => {
    const node = predicate(text)
    expect(withoutIDs(predicate(queryText(node)))).toEqual(withoutIDs(node))
  })

  it('combines plain text with an exact service and time window', () => {
    const service = {
      serviceName: 'api',
      serviceNamespace: 'production',
    } as ServiceSummary
    const link = serviceSignalHref(
      service,
      'logs',
      { startTime: '1000000', endTime: '2000000' },
      queryText(predicate('checkout'))
    )
    const url = new URL(link, 'http://localhost')
    expect(url.searchParams.get('start')).toBe('1')
    expect(url.searchParams.get('end')).toBe('2')
    expect(() => predicate(url.searchParams.get('q')!)).not.toThrow()
    expect(url.searchParams.get('q')).toContain('search("checkout")')
  })
})
