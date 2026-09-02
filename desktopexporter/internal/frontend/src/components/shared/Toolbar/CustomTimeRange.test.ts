// @vitest-environment jsdom
import { describe, expect, it, afterEach, vi } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import CustomTimeRange from './CustomTimeRange.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderComponent() {
  localStorage.setItem('time-tz', 'UTC')
  setTestUrl('/traces')
  return renderWithContexts(CustomTimeRange)
}

async function setEndpoint(
  endpoint: 'Start' | 'End',
  date: string,
  time: string
) {
  await fireEvent.input(screen.getByLabelText(endpoint), {
    target: { value: date },
  })
  await fireEvent.input(screen.getByLabelText(`${endpoint} time`), {
    target: { value: time },
  })
}

describe('CustomTimeRange', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders a themed calendar and a resolved now boundary by default', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.123Z'))
    renderComponent()
    expect(
      screen.getByRole('application', {
        name: /Custom time range January 2026/,
      })
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Start')).toHaveValue('')
    expect(screen.getByLabelText('Start time')).toHaveValue('00:00:00.000')
    expect(screen.getByLabelText('End')).toHaveValue('2026-01-15')
    expect(screen.getByLabelText('End time')).toHaveValue('12:00:00.123')
    expect(screen.getByLabelText('Start')).not.toHaveAttribute('inputmode')
    expect(screen.getByLabelText('Start time')).not.toHaveAttribute('inputmode')
    expect(screen.getByLabelText('End')).not.toHaveAttribute('inputmode')
    expect(screen.getByLabelText('End time')).not.toHaveAttribute('inputmode')
    expect(screen.getByRole('button', { name: 'Now' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByText(/Now · .*12:00:00\.123 UTC/)).toBeInTheDocument()
  })

  it('applies a millisecond-precise range ending now', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.789Z'))
    renderComponent()

    await setEndpoint('Start', '2026-01-15', '10:11:12.345')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved).toEqual({
      type: 'custom',
      start: new Date('2026-01-15T10:11:12.345Z').getTime(),
      end: new Date('2026-01-15T12:00:00.789Z').getTime(),
    })
    expect(window.location.search).toContain(`start=${saved.start}`)
    expect(window.location.search).toContain(`end=${saved.end}`)
  })

  it('updates date drafts from calendar range selection', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-20T12:00:00.000Z'))
    renderComponent()

    await fireEvent.click(
      screen.getByRole('button', { name: 'Wednesday, January 14, 2026' })
    )
    await fireEvent.click(
      screen.getByRole('button', { name: 'Friday, January 16, 2026' })
    )

    expect(screen.getByLabelText('Start')).toHaveValue('2026-01-14')
    expect(screen.getByLabelText('End')).toHaveValue('2026-01-16')
    expect(screen.getByRole('button', { name: 'Now' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
  })

  it('validates the exact time format and endpoint order', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    await setEndpoint('Start', '2026-01-15', '10:00')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(
      screen.getByText('Enter start time as HH:mm:ss.SSS')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Start')).toHaveAttribute(
      'aria-describedby',
      'custom-time-range-error'
    )
    expect(screen.getByLabelText('End')).not.toHaveAttribute('aria-describedby')
    expect(screen.getByLabelText('End time')).not.toHaveAttribute(
      'aria-describedby'
    )

    await setEndpoint('Start', '2026-01-15', '11:00:00.000')
    await setEndpoint('End', '2026-01-15', '10:00:00.000')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(
      screen.getByText('Start time must be before end time')
    ).toBeInTheDocument()
    expect(localStorage.getItem('time-selection')).toBeNull()
  })

  it('shows an error when the fixed end time is in the future', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    await setEndpoint('Start', '2026-01-15', '10:00:00.000')
    await setEndpoint('End', '2026-01-15', '13:00:00.000')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(
      screen.getByText('End time cannot be in the future')
    ).toBeInTheDocument()
    expect(localStorage.getItem('time-selection')).toBeNull()
    expect(window.location.search).toBe('')
  })

  it('keeps now active while focus moves through the end fields', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    await fireEvent.focus(screen.getByLabelText('End'))
    await fireEvent.focus(screen.getByLabelText('End time'))
    expect(screen.getByRole('button', { name: 'Now' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )

    await fireEvent.input(screen.getByLabelText('End time'), {
      target: { value: '11:00:00.000' },
    })
    expect(screen.getByRole('button', { name: 'Now' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
  })

  it('interprets structured wall-clock fields in a named timezone', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-16T00:00:00Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    setTestUrl('/traces')
    renderWithContexts(CustomTimeRange)

    await setEndpoint('Start', '2026-01-15', '07:00:00.123')
    await setEndpoint('End', '2026-01-15', '08:00:00.456')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.start).toBe(new Date('2026-01-15T12:00:00.123Z').getTime())
    expect(saved.end).toBe(new Date('2026-01-15T13:00:00.456Z').getTime())
  })

  it('rejects a DST gap in the selected timezone', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-09T00:00:00Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    renderWithContexts(CustomTimeRange)

    await setEndpoint('Start', '2026-03-08', '02:30:00.000')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(
      screen.getByText(
        'This time does not exist in America/New_York because the clocks changed'
      )
    ).toBeInTheDocument()
  })

  it('offers both occurrences of an ambiguous DST time', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-02T00:00:00Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    setTestUrl('/traces')
    renderWithContexts(CustomTimeRange)

    await setEndpoint('Start', '2026-11-01', '01:30:00.123')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(
      screen.getByText('Choose which start time you mean')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Earlier · EDT · -04:00' })
    ).toBeInTheDocument()
    await fireEvent.click(
      screen.getByRole('button', { name: 'Later · EST · -05:00' })
    )
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.start).toBe(new Date('2026-11-01T06:30:00.123Z').getTime())
  })

  it('round-trips an existing range during a DST overlap', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-02T00:00:00Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    localStorage.setItem(
      'time-selection',
      JSON.stringify({
        type: 'custom',
        start: new Date('2026-11-01T05:30:00Z').getTime(),
        end: new Date('2026-11-01T07:30:00Z').getTime(),
      })
    )
    setTestUrl('/traces')
    renderWithContexts(CustomTimeRange)

    expect(screen.getByLabelText('Start')).toHaveValue('2026-11-01')
    expect(screen.getByLabelText('Start time')).toHaveValue('01:30:00.000')
    expect(
      screen.getByRole('button', { name: 'Earlier · EDT · -04:00' })
    ).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByLabelText('End')).toHaveValue('2026-11-01')
    expect(screen.getByLabelText('End time')).toHaveValue('02:30:00.000')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.start).toBe(new Date('2026-11-01T05:30:00Z').getTime())
    expect(saved.end).toBe(new Date('2026-11-01T07:30:00Z').getTime())
  })

  it('does not reset edited drafts when the now preview ticks', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00Z'))
    localStorage.setItem(
      'time-selection',
      JSON.stringify({
        type: 'custom',
        start: new Date('2026-01-15T10:00:00Z').getTime(),
        end: new Date('2026-01-15T11:00:00Z').getTime(),
      })
    )
    renderComponent()

    await fireEvent.input(screen.getByLabelText('Start time'), {
      target: { value: '10:15:00.123' },
    })
    await vi.advanceTimersByTimeAsync(1000)

    expect(screen.getByLabelText('Start time')).toHaveValue('10:15:00.123')
  })
})
