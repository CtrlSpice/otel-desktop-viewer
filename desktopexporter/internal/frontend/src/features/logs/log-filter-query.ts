export {
  LOG_SEVERITIES,
  readListFilters as readLogFilters,
  setListFilter as setLogFilter,
} from '@/components/shared/list-filter-query'

export type LogFilter = 'service' | 'severity'
