// @vitest-environment jsdom
import { describe, expect, it, beforeEach, vi } from 'vitest'
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

  it('opens the first event by default', () => {
    const { container } = renderWithContexts(EventsPanel, {
      events: [makeEvent('first', 0n), makeEvent('second', 100n)],
      spanStartTime: 1_000n,
    })
    const details = container.querySelectorAll('details')
    expect(details).toHaveLength(2)
    expect(details[0]?.open).toBe(true)
    expect(details[1]?.open).toBe(false)
  })

  it('opens the selected event index', () => {
    const { container } = renderWithContexts(EventsPanel, {
      events: [makeEvent('first', 0n), makeEvent('second', 100n)],
      spanStartTime: 1_000n,
      selectedEventIndex: 1,
    })
    const details = container.querySelectorAll('details')
    expect(details[0]?.open).toBe(false)
    expect(details[1]?.open).toBe(true)
  })
})
