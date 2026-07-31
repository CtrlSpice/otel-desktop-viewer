// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import LogDetailView from './LogDetailView.svelte'
import type { LogData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { SPAN_PARAM } from '@/route/query-params'

const navigateToItem = vi.hoisted(() => vi.fn())

vi.mock('@/route', async importOriginal => {
  const actual = await importOriginal<typeof import('@/route')>()
  return { ...actual, navigateToItem }
})

function makeLog(overrides: Partial<LogData> = {}): LogData {
  return {
    id: 'log-1',
    timestamp: 1_700_000_000_000_000_000n,
    observedTimestamp: 1_700_000_000_000_000_000n,
    traceID: 'trace-abc',
    spanID: 'span-xyz',
    severityText: 'INFO',
    severityNumber: 9,
    body: 'hello',
    bodyType: 'string',
    resource: {
      attributes: [{ key: 'service.name', value: 'checkout', type: 'string' }],
      droppedAttributesCount: 0,
    },
    scope: {
      name: 'checkout',
      version: '1.0',
      attributes: [],
      droppedAttributesCount: 0,
    },
    attributes: [],
    droppedAttributesCount: 0,
    flags: 0,
    eventName: '',
    ...overrides,
  }
}

function renderLog(log: LogData) {
  setTestUrl('/logs/log-1?start=0&end=1')
  return renderWithContexts(LogDetailView, { log })
}

describe('LogDetailView trace correlation', () => {
  beforeEach(() => {
    navigateToItem.mockClear()
  })

  it('links trace and span ids with span in the href', () => {
    renderLog(makeLog())
    const traceLink = screen.getByRole('link', { name: 'trace-abc' })
    const spanLink = screen.getByRole('link', { name: 'span-xyz' })
    expect(traceLink).toHaveAttribute(
      'href',
      '/traces/trace-abc?start=0&end=1&span=span-xyz'
    )
    expect(spanLink).toHaveAttribute(
      'href',
      '/traces/trace-abc?start=0&end=1&span=span-xyz'
    )
  })

  it('navigates to trace detail with span patch on click', async () => {
    renderLog(makeLog())
    await userEvent.click(screen.getByRole('link', { name: 'span-xyz' }))
    expect(navigateToItem).toHaveBeenCalledWith('traces', 'trace-abc', 'push', {
      [SPAN_PARAM]: 'span-xyz',
    })
  })

  it('renders span id as plain text when trace id is missing', () => {
    renderLog(makeLog({ traceID: null }))
    expect(screen.queryByRole('link', { name: 'span-xyz' })).toBeNull()
    expect(screen.getByText('span-xyz')).toBeInTheDocument()
  })
})
