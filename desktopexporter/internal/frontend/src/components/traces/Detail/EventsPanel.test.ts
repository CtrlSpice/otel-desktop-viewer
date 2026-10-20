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
    expect(screen.queryByRole('link', { name: 'API deprecation' })).toBeNull()
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

  it('marks only exception events with a Love dot and keeps their title neutral', () => {
    const { container } = renderWithContexts(EventsPanel, {
      events: [makeEvent('exception', 0n), makeEvent('checkpoint', 1n)],
      spanStartTime: 1_000n,
    })
    const title = screen.getByText('exception')
    expect(title.parentElement).toHaveClass('text-base-content')
    expect(title.previousElementSibling).toHaveClass('bg-error', 'size-2')
    expect(title.previousElementSibling).toHaveAttribute('aria-hidden', 'true')
    expect(container.querySelectorAll('.bg-error')).toHaveLength(1)
    expect(container.querySelector('.text-error, .badge-error')).toBeNull()
    expect(screen.queryByRole('link', { name: 'API deprecation' })).toBeNull()
  })
})
