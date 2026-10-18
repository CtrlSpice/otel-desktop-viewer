// @vitest-environment jsdom
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import userEvent from '@testing-library/user-event'
import EventsPanel from './EventsPanel.svelte'
import type { EventData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeEvent(name: string, offsetNs: bigint): EventData {
  return {
    name,
    timestamp: 1_000n + offsetNs,
    attributes: [],
    droppedAttributesCount: 0,
  }
}

describe('EventsPanel selected event', () => {
  beforeEach(() => {
    setTestUrl('/traces/trace-1')
    Element.prototype.scrollIntoView = vi.fn()
  })

  it('starts collapsed and expands each event independently with click or keyboard', async () => {
    const { container } = renderWithContexts(EventsPanel, {
      events: [makeEvent('first', 0n), makeEvent('second', 100n)],
      spanStartTime: 1_000n,
    })
    expect(container.querySelectorAll('details, summary')).toHaveLength(0)
    const cards = screen.getAllByRole('button', { expanded: false })
    expect(cards).toHaveLength(2)
    expect(screen.queryByRole('table')).toBeNull()
    await userEvent.click(cards[0]!)
    expect(screen.getByRole('table', { name: 'Event first' })).toBeVisible()
    expect(cards[1]).toHaveAttribute('aria-expanded', 'false')
    cards[1]!.focus()
    await userEvent.keyboard('{Enter}')
    expect(screen.getByRole('table', { name: 'Event second' })).toBeVisible()
    expect(cards[0]).toHaveAttribute('aria-expanded', 'true')
    await userEvent.keyboard(' ')
    expect(cards[1]).toHaveAttribute('aria-expanded', 'false')
    expect(cards[0]!.querySelector('a, button')).toBeNull()
    expect(screen.getAllByText('Span event', { exact: true })).toHaveLength(2)
    const warning = screen.getAllByRole('link', { name: 'API deprecation' })[0]!
    expect(warning).toHaveAttribute(
      'href',
      'https://opentelemetry.io/blog/2026/deprecating-span-events/'
    )
    expect(warning).toHaveAttribute(
      'title',
      expect.stringContaining('Existing span-event data remains supported.')
    )
  })

  it('reveals and scrolls to the selected event while other cards remain available', async () => {
    const { container } = renderWithContexts(EventsPanel, {
      events: [makeEvent('first', 0n), makeEvent('second', 100n)],
      spanStartTime: 1_000n,
      selectedEventIndex: 1,
    })
    await tick()
    expect(container.querySelectorAll('details, summary')).toHaveLength(0)
    expect(screen.queryByRole('table', { name: 'Event first' })).toBeNull()
    expect(screen.getByRole('table', { name: 'Event second' })).toBeVisible()
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({
      block: 'nearest',
    })
  })
})
