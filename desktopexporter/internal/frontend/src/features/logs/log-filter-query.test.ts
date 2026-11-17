import { describe, expect, it } from 'vitest'
import { getStaticFieldsForSearch } from '@/constants/fields'
import { parseSearchRequest } from '@/components/shared/Search/queryParser'
import { readLogFilters, setLogFilter } from './log-filter-query'

const fields = getStaticFieldsForSearch('logs')

describe('query-backed log filters', () => {
  it('combines service alternatives with severity bands and clears each independently', () => {
    const services = setLogFilter('', fields, 'service', ['api', 'worker'])!
    const both = setLogFilter(services, fields, 'severity', ['Error', 'Fatal'])!
    expect(readLogFilters(both, fields)).toMatchObject({
      service: { values: ['api', 'worker'], custom: false },
      severity: { values: ['Error', 'Fatal'], custom: false },
    })
    const severityOnly = setLogFilter(both, fields, 'service', [])!
    expect(readLogFilters(severityOnly, fields).service.values).toEqual([])
    expect(readLogFilters(severityOnly, fields).severity.values).toEqual([
      'Error',
      'Fatal',
    ])
    expect(setLogFilter(severityOnly, fields, 'severity', [])).toBe('')
    expect(readLogFilters('', fields).severity.values).toEqual([])
  })

  it('preserves mixed OR expressions, exact literals and the final limit', () => {
    const expression = '(body CONTAINS "a\\\"b" OR severityText = "WARN")'
    const next = setLogFilter(`${expression} LIMIT 17`, fields, 'service', [
      'worker',
    ])!
    expect(next).toContain(expression)
    expect(next).toMatch(/LIMIT 17$/)
    expect(parseSearchRequest(next, fields, 'logs')?.limit).toBe(17)
    const cleared = setLogFilter(next, fields, 'service', [])!
    expect(cleared).toContain(expression)
    expect(cleared).toMatch(/LIMIT 17$/)
  })

  it('recognizes grouped top-level conjuncts without changing other owners', () => {
    const query =
      '((resource.attributes["service.name"]::VARCHAR = "api" AND log.attributes["service.name"]::VARCHAR = "payload"))'
    expect(readLogFilters(query, fields).service.values).toEqual(['api'])
    expect(setLogFilter(query, fields, 'service', [])).toBe(
      '(log.attributes["service.name"]::VARCHAR = "payload")'
    )
  })

  it.each([
    'resource.attributes["service.name"]::VARCHAR != "worker"',
    'resource.attributes["service.name"]::VARCHAR = "api" OR body CONTAINS "exception"',
    'resource.attributes["service.name"]::VARCHAR = "api" AND resource.attributes["service.name"]::VARCHAR = "worker"',
  ])('leaves custom service conditions intact: %s', query => {
    expect(readLogFilters(query, fields).service.custom).toBe(true)
    expect(setLogFilter(query, fields, 'service', ['replacement'])).toBeNull()
    expect(setLogFilter(query, fields, 'severity', ['Warn'])).toContain(query)
  })

  it.each(['severityNumber =', 'free text', 'exception'])(
    'does not rewrite drafts or global searches: %s',
    query => {
      expect(setLogFilter(query, fields, 'service', ['api'])).toBeNull()
    }
  )

  it('round-trips service names without changing whitespace or escaped characters', () => {
    const values = [
      'a"b',
      'a\\b',
      'line\nbreak',
      'tab\tname',
      'white space',
      '',
    ]
    const query = setLogFilter('', fields, 'service', values)!
    expect(readLogFilters(query, fields).service.values).toEqual(values)
  })

  it('distinguishes unspecified from Trace and preserves non-band numeric expressions', () => {
    const query = setLogFilter('', fields, 'severity', [
      'Unspecified',
      'Trace',
    ])!
    expect(query).toContain('severityNumber = 0')
    expect(query).toContain('severityNumber >= 1 AND severityNumber <= 4')
    expect(readLogFilters(query, fields).severity.values).toEqual([
      'Unspecified',
      'Trace',
    ])
    expect(readLogFilters('severityNumber >= 18', fields).severity.custom).toBe(
      true
    )
  })
})
