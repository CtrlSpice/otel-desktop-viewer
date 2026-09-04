// @vitest-environment jsdom
import { describe, expect, it, vi, afterEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import RecentTimeRanges from './RecentTimeRanges.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { type RecentTimeRange } from '@/utils/time'

const RECENT_STORAGE_KEY = 'datetime-filter-recent'

function renderComponent() {
  setTestUrl('/traces')
  return renderWithContexts(RecentTimeRanges)
}

describe('RecentTimeRanges', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows an empty state when no recent ranges exist', () => {
    renderComponent()
    expect(screen.getByText('No recent time ranges')).toBeInTheDocument()
  })

  it('renders seeded recent ranges and applies one when clicked', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))

    const recents: RecentTimeRange[] = [
      {
        start: Date.now() - 600_000,
        end: Date.now(),
        usedAt: Date.now(),
      },
      {
        start: Date.now() - 1_200_000,
        end: Date.now() - 600_000,
        usedAt: Date.now() - 1000,
      },
    ]
    localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(recents))

    renderComponent()

    const list = screen.getByRole('list', {
      name: 'Recent time ranges',
    })
    expect(list).toHaveClass('recent-range-list')
    const buttons = list.querySelectorAll<HTMLButtonElement>(
      '.recent-range-button'
    )
    expect(buttons).toHaveLength(2)
    expect(buttons[0]).toHaveAttribute('aria-pressed', 'false')
    expect(screen.queryByText('No recent time ranges')).not.toBeInTheDocument()

    await fireEvent.click(buttons[0])

    const saved = JSON.parse(localStorage.getItem('time-selection')!)
    expect(saved).toMatchObject({
      type: 'recent',
      start: recents[0].start,
      end: recents[0].end,
    })
    expect(buttons[0]).toHaveAttribute('aria-pressed', 'true')
    expect(window.location.search).toContain(`start=${recents[0].start}`)
    expect(window.location.search).toContain(`end=${recents[0].end}`)
  })

  it('removes one persisted recent range without selecting it', async () => {
    const recents: RecentTimeRange[] = [
      { start: 1, end: 2, usedAt: 200 },
      { start: 3, end: 4, usedAt: 100 },
    ]
    localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(recents))

    renderComponent()
    const removeButtons = screen.getAllByRole('button', {
      name: /Remove recent range from/,
    })

    removeButtons[0].focus()
    await fireEvent.click(removeButtons[0])

    expect(JSON.parse(localStorage.getItem(RECENT_STORAGE_KEY)!)).toEqual([
      recents[1],
    ])
    expect(
      screen
        .getByRole('list', { name: 'Recent time ranges' })
        .querySelectorAll('.recent-range-button')
    ).toHaveLength(1)
    expect(localStorage.getItem('time-selection')).toBeNull()

    const remainingRemove = screen.getByRole('button', {
      name: /Remove recent range from/,
    })
    expect(remainingRemove).toHaveFocus()
    await fireEvent.click(remainingRemove)

    expect(localStorage.getItem(RECENT_STORAGE_KEY)).toBeNull()
    expect(screen.getByText('No recent time ranges')).toBeInTheDocument()
    expect(
      screen.getByText('Recent', { exact: true }).closest('summary')
    ).toHaveFocus()
  })
})
