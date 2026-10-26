// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import SignalDate from './SignalDate.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { formatTimestamp } from '@/utils/time'

const now = Date.parse('2026-10-24T12:00:00.000Z')
const nowNs = BigInt(now) * 1_000_000n

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
  localStorage.setItem('time-tz', 'UTC')
  setTestUrl('/traces')
})

afterEach(async () => {
  cleanup()
  await tick()
  vi.useRealTimers()
})

describe('SignalDate', () => {
  it('shows relative time while retaining every recorded digit on hover', () => {
    const timestamp = nowNs - 300_000_000_000n + 123_456_789n
    const { container } = renderWithContexts(SignalDate, {
      label: 'Start',
      timestamp,
    })
    expect(screen.getByText('4 minutes ago')).toBeInTheDocument()
    expect(container.querySelector('.signal-date')).toHaveAttribute(
      'title',
      `Start: ${formatTimestamp(timestamp, 'UTC')}`
    )
    expect(container.querySelector('.signal-date')).not.toHaveTextContent(
      '2026-'
    )
  })

  it('shares one live timer and releases it after the last date unmounts', async () => {
    const props = { label: 'Start', timestamp: nowNs - 300_000_000_000n }
    const first = renderWithContexts(SignalDate, props)
    const second = renderWithContexts(SignalDate, props)
    expect(screen.getAllByText('5 minutes ago')).toHaveLength(2)
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(60_000)
    await tick()
    expect(screen.getAllByText('6 minutes ago')).toHaveLength(2)
    first.unmount()
    await tick()
    expect(vi.getTimerCount()).toBe(1)
    second.unmount()
    await tick()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not describe future telemetry as past time', () => {
    renderWithContexts(SignalDate, {
      label: 'Recorded',
      timestamp: nowNs + 604_800_000_000_000n,
    })
    expect(screen.getByText('in 1 week')).toBeInTheDocument()
  })

  it('retains signed parent-relative offsets and suffixes without a wall clock', () => {
    renderWithContexts(SignalDate, {
      label: 'Recorded',
      timestamp: nowNs - 500_000_000n,
      relativeTo: nowNs,
      suffix: '(span event)',
    })
    expect(screen.getByText('-500.000 ms')).toBeInTheDocument()
    expect(screen.getByText('(span event)')).toHaveClass('text-subtle')
    expect(vi.getTimerCount()).toBe(0)
  })

  it('keeps missing timestamps unavailable instead of inventing a date', () => {
    const { container } = renderWithContexts(SignalDate, {
      label: 'Start',
      unavailable: 'Not recorded',
    })
    expect(screen.getByText('—')).toBeInTheDocument()
    expect(container.querySelector('.signal-date')).toHaveAttribute(
      'title',
      'Start: Not recorded'
    )
    expect(vi.getTimerCount()).toBe(0)
  })

  it('treats an epoch timestamp of zero as a received date', () => {
    renderWithContexts(SignalDate, { label: 'Recorded', timestamp: 0n })
    expect(screen.getByText('56 years ago')).toBeInTheDocument()
  })
})
