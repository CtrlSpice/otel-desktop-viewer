// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'svelte'
import LogCard from './LogCard.svelte'
import type { LogSummary } from '@/types/api-types'
import { formatTimestampParts } from '@/utils/time'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeLog(overrides: Partial<LogSummary> = {}): LogSummary {
  return {
    id: 'log-1',
    timestamp: 1_700_000_000_123_456_789n,
    severityText: 'ERROR',
    severityNumber: 17,
    serviceName: 'checkout-service',
    bodyPreview: 'Failed to charge card',
    ...overrides,
  }
}

function renderCard(props: ComponentProps<typeof LogCard>) {
  setTestUrl('/logs')
  return renderWithContexts(LogCard, props)
}

describe('LogCard', () => {
  it('renders the service name as the title', () => {
    renderCard({ log: makeLog({ serviceName: 'checkout-service' }) })
    expect(screen.getByText('checkout-service')).toBeInTheDocument()
  })

  it('falls back to a placeholder title when the service name is blank', () => {
    renderCard({ log: makeLog({ serviceName: '   ' }) })
    expect(screen.getByText('(unknown service)')).toBeInTheDocument()
  })

  it('renders the body preview as the description', () => {
    renderCard({ log: makeLog({ bodyPreview: 'Failed to charge card' }) })
    expect(screen.getByText('Failed to charge card')).toBeInTheDocument()
  })

  it('renders the severity text and number in the badge', () => {
    renderCard({ log: makeLog({ severityText: 'ERROR', severityNumber: 17 }) })
    expect(screen.getByText('ERROR (17)')).toBeInTheDocument()
  })

  it('falls back to the severity band label when severity text is empty', () => {
    renderCard({ log: makeLog({ severityText: '', severityNumber: 5 }) })
    expect(screen.getByText('DEBUG (5)')).toBeInTheDocument()
  })

  it('renders the formatted timestamp', () => {
    const log = makeLog()
    renderCard({ log })
    const timestampParts = formatTimestampParts(
      log.timestamp,
      'local',
      'milliseconds'
    )
    expect(screen.getByText(timestampParts.value)).toBeInTheDocument()
  })

  it('reflects the selected prop via aria-pressed', () => {
    renderCard({ log: makeLog(), selected: true })
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('is not pressed by default', () => {
    renderCard({ log: makeLog() })
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false')
  })

  it('calls onclick with the log id when clicked', async () => {
    const onclick = vi.fn()
    renderCard({ log: makeLog({ id: 'log-42' }), onclick })
    await userEvent.click(screen.getByRole('button'))
    expect(onclick).toHaveBeenCalledWith('log-42')
  })
})
