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
    logRef: 'log-1',
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

  it('renders the severity text and number beside its status dot', () => {
    renderCard({ log: makeLog({ severityText: 'ERROR', severityNumber: 17 }) })
    expect(screen.getByText('ERROR (17)')).toBeInTheDocument()
    expect(screen.queryByText('Log', { exact: true })).toBeNull()
  })

  it('shows log provenance beside the service and keeps severity separate', () => {
    const { container } = renderCard({ log: makeLog(), showLogBadge: true })
    expect(container.querySelector('.signal-row__badge')).toHaveTextContent(
      'ERROR (17)'
    )
    expect(container.querySelector('.signal-row__title')).toHaveTextContent(
      /^checkout-service · Log$/
    )
  })

  it('falls back to the severity band label when severity text is empty', () => {
    renderCard({ log: makeLog({ severityText: '', severityNumber: 5 }) })
    expect(screen.getByText('DEBUG (5)')).toBeInTheDocument()
  })

  it('places the labelled recorded timestamp after the body preview', () => {
    const log = makeLog()
    renderCard({ log })
    const timestampParts = formatTimestampParts(
      log.timestamp,
      'local',
      'milliseconds'
    )
    expect(screen.getByText(timestampParts.value)).toBeInTheDocument()
    const date = document.querySelector('.signal-date')
    expect(date).toHaveTextContent(`Recorded: ${timestampParts.value}`)
    expect(date).toHaveAttribute(
      'title',
      `Recorded: ${timestampParts.value} ${timestampParts.unit}`
    )
    expect(date).not.toHaveTextContent(timestampParts.unit)
    expect(timestampParts.value).toContain('.123')
    expect(
      document.querySelector('.signal-row__body')?.lastElementChild
    ).toContainElement(date as HTMLElement)
  })

  it('shows an associated offset as Subtle text beside the recorded timestamp', () => {
    const log = makeLog()
    renderCard({ log, offset: '+500.000 ms' })
    const offset = screen.getByText('(+500.000 ms)', { exact: true })
    expect(offset).toHaveClass('text-subtle')
    expect(offset.closest('.signal-date')).toHaveTextContent(
      `${formatTimestampParts(log.timestamp, 'local', 'milliseconds').value} (+500.000 ms)`
    )
    expect(screen.getByRole('button')).toContainElement(offset)
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
    renderCard({ log: makeLog({ logRef: 'log-42' }), onclick })
    await userEvent.click(screen.getByRole('button'))
    expect(onclick).toHaveBeenCalledWith('log-42')
  })
})
