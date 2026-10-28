import { describe, expect, it } from 'vitest'
import { getStaticFieldsForSearch } from '@/constants/fields'
import { parseSearchRequest } from '@/components/shared/Search/queryParser'
import { readListFilters, setListFilter } from './list-filter-query'

describe('query-backed telemetry list filters', () => {
  it('uses the metric search type field, not the summary property name', () => {
    const fields = getStaticFieldsForSearch('metrics')
    const query = setListFilter(
      'name CONTAINS "request" LIMIT 17',
      fields,
      'metricType',
      ['Sum', 'Histogram'],
      'metrics'
    )!
    expect(query).toContain("type IN ('Sum', 'Histogram')")
    expect(query).toContain('name CONTAINS "request"')
    expect(parseSearchRequest(query, fields, 'metrics')?.limit).toBe(17)
    expect(readListFilters(query, fields, 'metrics').metricType).toEqual({
      values: ['Sum', 'Histogram'],
      custom: false,
    })
    expect(
      setListFilter(query, fields, 'metricType', [], 'metrics')
    ).not.toContain('type IN')
  })

  it('composes typed resource service and numeric span error in one predicate', () => {
    const fields = getStaticFieldsForSearch('traces')
    const service = setListFilter('', fields, 'service', ['worker'], 'traces')!
    const query = setListFilter(
      service,
      fields,
      'errors',
      ['Has errors'],
      'traces'
    )!
    expect(query).toContain(
      "resource.attributes['service.name']::VARCHAR IN ('worker')"
    )
    expect(query).toContain('statusCodeValue = 2')
    expect(readListFilters(query, fields, 'traces')).toMatchObject({
      service: { values: ['worker'], custom: false },
      errors: { values: ['Has errors'], custom: false },
    })
  })

  it('does not rewrite mixed OR, negated facets or incomplete drafts', () => {
    const fields = getStaticFieldsForSearch('metrics')
    for (const query of [
      'type != "Sum"',
      'type = "Sum" OR name = "other"',
      'type =',
    ])
      expect(
        setListFilter(query, fields, 'metricType', ['Gauge'], 'metrics')
      ).toBeNull()
  })

  it('preserves an unrelated complex expression and differently typed service attribute', () => {
    const fields = getStaticFieldsForSearch('traces')
    const original =
      '(name = "a" OR name = "b") AND resource.attributes["service.name"]::BIGINT = 0'
    const query = setListFilter(original, fields, 'service', ['api'], 'traces')!
    expect(query).toContain(original)
    const cleared = setListFilter(query, fields, 'service', [], 'traces')
    expect(cleared).toContain('name = "a" OR name = "b"')
    expect(cleared).toContain('resource.attributes["service.name"]::BIGINT = 0')
  })
})
