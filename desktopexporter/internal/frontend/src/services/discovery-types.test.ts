import { describe, expect, it } from 'vitest'
import { getStaticFieldsForSearch } from '@/constants/fields'
import { parseSearchRequest } from '@/components/shared/Search/queryParser'
import { serviceSignalHref, type ServiceSummary } from './discovery-types'

describe('service signal links', () => {
  const service: ServiceSummary = {
    serviceName: 'checkout',
    serviceNamespace: '',
    spanCount: 1,
    errorSpanCount: 0,
    logCount: 0,
    errorLogCount: 0,
    metricCount: 0,
    dataPointCount: 0,
    lastSeen: null,
  }
  for (const signal of ['traces', 'logs', 'metrics'] as const) {
    it(`parses absent namespaces and quoted service identities for ${signal}`, () => {
      for (const name of ['', 'checkout', 'quote"\\service']) {
        const url = new URL(
          serviceSignalHref({ ...service, serviceName: name }, signal),
          'http://localhost'
        )
        const text = url.searchParams.get('q')!
        const request = parseSearchRequest(
          text,
          getStaticFieldsForSearch(signal),
          signal
        )
        expect(request?.predicate?.type).toBe('group')
        expect(text).toContain('"service.namespace", string) = NULL')
        expect(url.searchParams.get('time')).toBe('all')
      }
    })
  }
})
