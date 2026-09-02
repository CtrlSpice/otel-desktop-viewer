// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import TimeRangeFilterBody from './TimeRangeFilterBody.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(TimeRangeFilterBody)
}

async function openTimezoneOptions() {
  const heading = screen.getByText('Timezone').closest('summary')
  if (!heading) throw new Error('Timezone heading not found')
  const details = heading.closest('details')
  if (!details) throw new Error('Timezone details not found')
  details.open = true
  await fireEvent(details, new Event('toggle'))
  await tick()
}

describe('TimeRangeFilterBody', () => {
  beforeEach(() => {
    // Pin a non-UTC zone: on a UTC machine (like CI) the "local" timezone
    // option renders the same "Coordinated Universal Time" label as the UTC
    // option, making the UTC button query ambiguous.
    vi.stubEnv('TZ', 'America/New_York')
  })

  afterEach(() => {
    vi.unstubAllEnvs()
    vi.useRealTimers()
  })

  it('renders the custom range, timezone, and recents sections', () => {
    renderComponent()
    expect(screen.getByText('Custom Range')).toBeInTheDocument()
    expect(screen.getByText('Timezone')).toBeInTheDocument()
    expect(screen.getByText('Recently Used')).toBeInTheDocument()
  })

  it('switches the timezone from local to UTC', async () => {
    renderComponent()
    await openTimezoneOptions()
    // Exact name: on a UTC machine the local option's accessible name also
    // starts with "Coordinated Universal Time" (plus its "(Local)" marker).
    fireEvent.click(
      screen.getByRole('button', { name: 'Coordinated Universal Time UTC' })
    )
    expect(localStorage.getItem('time-tz')).toBe('UTC')
  })

  it('marks the machine timezone option as local', async () => {
    renderComponent()
    await openTimezoneOptions()
    expect(
      screen.getByRole('button', { name: /\(Local\)/ })
    ).toBeInTheDocument()
  })

  it('places local and UTC before a separator and named timezones', async () => {
    renderComponent()
    await openTimezoneOptions()
    const list = screen.getByLabelText('Timezone options')
    const options = list.querySelectorAll('button')
    expect(options[0]).toHaveAccessibleName(/\(Local\)/)
    expect(options[1]).toHaveAccessibleName('Coordinated Universal Time UTC')
    expect(options[2]).toHaveTextContent('/')
    expect(list.children[2]).toHaveAttribute('role', 'separator')
  })

  it('selects and persists a named IANA timezone from the list', async () => {
    renderComponent()
    await openTimezoneOptions()
    const option = screen.getByRole('button', {
      name: /^America\/Los_Angeles/,
    })
    fireEvent.click(option)
    expect(localStorage.getItem('time-tz')).toBe('America/Los_Angeles')
    expect(option).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTitle('America/Los_Angeles')).toBeInTheDocument()
  })

  it('does not repeat the machine zone among the named choices', async () => {
    renderComponent()
    await openTimezoneOptions()
    expect(
      screen.queryByRole('button', { name: /^America\/New_York/ })
    ).not.toBeInTheDocument()
  })

  it('keeps the timezone choices in one scrollable list', async () => {
    renderComponent()
    await openTimezoneOptions()
    const list = screen.getByLabelText('Timezone options')
    expect(list).toHaveClass('timezone-list')
    expect(list.querySelectorAll('button').length).toBeGreaterThan(100)
    expect(
      screen.queryByRole('button', { name: 'Use named timezone' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Named timezone' })
    ).not.toBeInTheDocument()
  })

  it('filters named timezones by city and historical abbreviation', async () => {
    renderComponent()
    await openTimezoneOptions()
    const filter = screen.getByRole('searchbox', { name: 'Filter timezones' })

    await fireEvent.input(filter, { target: { value: 'los angeles' } })
    expect(
      screen.getByRole('button', { name: /^America\/Los_Angeles/ })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /^America\/Chicago/ })
    ).not.toBeInTheDocument()

    await fireEvent.input(filter, { target: { value: 'PWT' } })
    expect(
      await screen.findByRole('button', { name: /^America\/Los_Angeles/ })
    ).toBeInTheDocument()

    await fireEvent.input(filter, { target: { value: 'PST' } })
    expect(
      screen.getByRole('button', { name: /^America\/Los_Angeles/ })
    ).toBeInTheDocument()
  })

  it('returns every matching zone for an ambiguous abbreviation', async () => {
    renderComponent()
    await openTimezoneOptions()

    await fireEvent.input(
      screen.getByRole('searchbox', { name: 'Filter timezones' }),
      { target: { value: 'CST' } }
    )
    expect(
      await screen.findByRole('button', { name: /^America\/Chicago/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /^Asia\/Shanghai/ })
    ).toBeInTheDocument()
  })

  it('shows abbreviations for the selected historical range', async () => {
    localStorage.setItem(
      'time-selection',
      JSON.stringify({
        type: 'custom',
        start: Date.parse('2020-01-14T12:00:00.000Z'),
        end: Date.parse('2020-01-15T12:00:00.000Z'),
      })
    )
    renderComponent()
    await openTimezoneOptions()

    expect(
      screen.getByRole('button', { name: 'America/Los_Angeles PST' })
    ).toBeInTheDocument()
  })

  it('scrolls the selected timezone into view when opened', async () => {
    const original = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      'scrollIntoView'
    )
    const scrollIntoView = vi.fn()
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoView,
    })

    try {
      localStorage.setItem('time-tz', 'America/Los_Angeles')
      renderComponent()
      await openTimezoneOptions()
      await vi.waitFor(() => expect(scrollIntoView).toHaveBeenCalledOnce())
      expect(scrollIntoView.mock.contexts[0]).toHaveAccessibleName(
        /^America\/Los_Angeles/
      )
    } finally {
      if (original) {
        Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', original)
      } else {
        delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView
      }
    }
  })

  it('applies a custom range through the embedded form', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    renderComponent()

    fireEvent.input(screen.getByLabelText(/Start/i), {
      target: { value: '2 hours ago' },
    })
    fireEvent.input(screen.getByLabelText(/End/i), {
      target: { value: '1 hour ago' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved.type).toBe('custom')
    expect(saved.start).toBeLessThan(saved.end)
    expect(window.location.search).toContain(`start=${saved.start}`)
    expect(window.location.search).toContain(`end=${saved.end}`)
  })
})
