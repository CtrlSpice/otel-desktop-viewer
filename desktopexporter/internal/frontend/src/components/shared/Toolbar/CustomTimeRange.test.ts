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

  it('renders exact fields with the calendar collapsed by default', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.123Z'))
    renderComponent()
    expect(screen.queryByRole('application')).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Choose start date' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.getByRole('button', { name: 'Choose start date' })
    ).toHaveClass('btn-circle')
    expect(
      screen.getByRole('button', { name: 'Choose end date' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('button', { name: 'Choose end date' })).toHaveClass(
      'btn-circle'
    )
    expect(screen.getByLabelText('Start')).toHaveValue('')
    expect(screen.getByLabelText('Start time')).toHaveValue('00:00:00.000')
    expect(screen.getByLabelText('End')).toHaveValue('2026-01-15')
    expect(screen.getByLabelText('End')).toHaveAttribute(
      'placeholder',
      'YYYY-MM-DD'
    )
    expect(screen.getByLabelText('End time')).toHaveValue('')
    expect(screen.getByLabelText('End time')).toHaveAttribute(
      'placeholder',
      'right now'
    )
    expect(screen.getByLabelText('Start')).not.toHaveAttribute('inputmode')
    expect(screen.getByLabelText('Start time')).not.toHaveAttribute('inputmode')
    expect(screen.getByLabelText('End')).not.toHaveAttribute('inputmode')
    expect(screen.getByLabelText('End time')).not.toHaveAttribute('inputmode')
    expect(
      screen.queryByRole('button', { name: 'Now' })
    ).not.toBeInTheDocument()
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

  it('advances the implicit end at midnight in the selected timezone', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T04:59:59.000Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    setTestUrl('/traces')
    renderWithContexts(CustomTimeRange)

    expect(screen.getByLabelText('End')).toHaveValue('2026-01-14')
    await setEndpoint('Start', '2026-01-14', '23:00:00.000')

    await vi.advanceTimersByTimeAsync(1000)

    expect(screen.getByLabelText('End')).toHaveValue('2026-01-15')
    expect(screen.getByLabelText('End time')).toHaveValue('')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(JSON.parse(localStorage.getItem('time-selection')!)).toEqual({
      type: 'custom',
      start: new Date('2026-01-15T04:00:00.000Z').getTime(),
      end: new Date('2026-01-15T05:00:00.000Z').getTime(),
    })
  })

  it('updates an open calendar maximum at midnight', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T04:59:59.000Z'))
    localStorage.setItem('time-tz', 'America/New_York')
    setTestUrl('/traces')
    renderWithContexts(CustomTimeRange)

    await fireEvent.click(
      screen.getByRole('button', { name: 'Choose end date' })
    )
    const newDay = screen.getByRole('button', {
      name: 'Thursday, January 15, 2026',
    })
    expect(newDay).toHaveAttribute('aria-disabled', 'true')

    await vi.advanceTimersByTimeAsync(1000)

    expect(newDay).toHaveAttribute('aria-disabled', 'false')
  })

  it('refreshes the implicit end on apply before the clock timer runs', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T23:59:59.000Z'))
    renderComponent()
    await setEndpoint('Start', '2026-01-15', '23:00:00.000')

    vi.setSystemTime(new Date('2026-01-16T00:00:00.123Z'))
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(screen.queryByText(/Enter end time/)).not.toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem('time-selection')!)).toEqual({
      type: 'custom',
      start: new Date('2026-01-15T23:00:00.000Z').getTime(),
      end: new Date('2026-01-16T00:00:00.123Z').getTime(),
    })
  })

  it('does not advance an incomplete explicit end at midnight', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T23:59:59.000Z'))
    renderComponent()
    await setEndpoint('Start', '2026-01-15', '22:00:00.000')
    await fireEvent.input(screen.getByLabelText('End'), {
      target: { value: '2026-01-14' },
    })

    await vi.advanceTimersByTimeAsync(1000)

    expect(screen.getByLabelText('End')).toHaveValue('2026-01-14')
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(
      screen.getByText('Enter end time as HH:mm:ss.SSS')
    ).toBeInTheDocument()
  })

  it('tracks civil-date reversals without assuming dates are monotonic', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('1987-10-25T02:59:59.000Z'))
    localStorage.setItem('time-tz', 'America/Goose_Bay')
    setTestUrl('/traces')
    renderWithContexts(CustomTimeRange)

    expect(screen.getByLabelText('End')).toHaveValue('1987-10-24')
    await vi.advanceTimersByTimeAsync(1000)
    expect(screen.getByLabelText('End')).toHaveValue('1987-10-25')

    await vi.advanceTimersByTimeAsync(60_000)
    expect(screen.getByLabelText('End')).toHaveValue('1987-10-24')

    await vi.advanceTimersByTimeAsync(59 * 60_000)
    expect(screen.getByLabelText('End')).toHaveValue('1987-10-25')
  })

  it('requires a complete endpoint after the end fallback is edited', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    await setEndpoint('Start', '2026-01-15', '10:00:00.000')
    await fireEvent.input(screen.getByLabelText('End'), {
      target: { value: '2026-01-14' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(
      screen.getByText('Enter end time as HH:mm:ss.SSS')
    ).toBeInTheDocument()
    expect(localStorage.getItem('time-selection')).toBeNull()
  })

  it('selects each endpoint from its own calendar', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-20T12:00:00.000Z'))
    renderComponent()

    await fireEvent.click(
      screen.getByRole('button', { name: 'Choose start date' })
    )
    expect(
      screen.getByRole('application', {
        name: /Start date January 2026/,
      })
    ).toBeInTheDocument()
    await fireEvent.click(
      screen.getByRole('button', { name: 'Wednesday, January 14, 2026' })
    )

    expect(screen.getByLabelText('Start')).toHaveValue('2026-01-14')
    expect(
      screen.getByRole('button', { name: 'Choose start date' })
    ).toHaveAttribute('aria-expanded', 'false')

    await vi.advanceTimersByTimeAsync(150)
    await fireEvent.click(
      screen.getByRole('button', { name: 'Choose end date' })
    )
    expect(
      screen.getByRole('application', {
        name: /End date January 2026/,
      })
    ).toBeInTheDocument()
    await fireEvent.click(
      screen.getByRole('button', { name: 'Friday, January 16, 2026' })
    )

    expect(screen.getByLabelText('End')).toHaveValue('2026-01-16')
    expect(screen.getByLabelText('End time')).toHaveValue('12:00:00.000')
    expect(
      screen.getByRole('button', { name: 'Choose end date' })
    ).toHaveAttribute('aria-expanded', 'false')
  })

  it('does not couple start and end calendar selections', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-20T12:00:00.000Z'))
    renderComponent()

    await fireEvent.click(
      screen.getByRole('button', { name: 'Choose start date' })
    )
    await fireEvent.click(
      screen.getByRole('button', { name: 'Friday, January 16, 2026' })
    )

    await vi.advanceTimersByTimeAsync(150)
    await fireEvent.click(
      screen.getByRole('button', { name: 'Choose end date' })
    )
    await fireEvent.click(
      screen.getByRole('button', { name: 'Wednesday, January 14, 2026' })
    )

    expect(screen.getByLabelText('Start')).toHaveValue('2026-01-16')
    expect(screen.getByLabelText('End')).toHaveValue('2026-01-14')
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

  it("uses now again when today's explicit end time is cleared", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    await setEndpoint('Start', '2026-01-15', '10:00:00.000')
    await setEndpoint('End', '2026-01-15', '11:00:00.000')
    await fireEvent.input(screen.getByLabelText('End time'), {
      target: { value: '' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Apply range' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.end).toBe(new Date('2026-01-15T12:00:00.000Z').getTime())
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
})
