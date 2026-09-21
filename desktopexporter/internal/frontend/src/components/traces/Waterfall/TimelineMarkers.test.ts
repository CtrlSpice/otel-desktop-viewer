// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import TimelineMarkers from './TimelineMarkers.svelte'
import { renderWithContexts } from '@/test/render-helpers'
import type { TimelineMarker } from './timeline-markers'

function mixedMarker(): TimelineMarker {
  return {
    id: 'event:span-1:0|log:log-1',
    position: { placement: 'inside', pixel: 50, percent: 50 },
    members: [
      {
        kind: 'event',
        id: 'event:span-1:0',
        timestamp: 50n,
        eventIndex: 0,
        event: {
          name: 'exception',
          timestamp: 50n,
          attributes: [],
          droppedAttributesCount: 0,
        },
      },
      {
        kind: 'log',
        id: 'log:log-1',
        timestamp: 51n,
        log: {
          id: 'log-1',
          spanID: 'span-1',
          timestamp: 51n,
          severityText: 'ERROR',
          severityNumber: 17,
          serviceName: 'checkout',
          eventName: 'payment.failed',
          bodyPreview: 'failed',
        },
      },
    ],
  }
}

describe('TimelineMarkers', () => {
  it('renders every cluster at the bar height in a 1.5rem labelled button', () => {
    renderWithContexts(TimelineMarkers, {
      markers: [mixedMarker()],
      spanColor: '#abc',
      spanStartTime: 40n,
      onSelectRecord: vi.fn(),
    })
    const button = screen.getByRole('button', {
      name: 'Open 1 event and 1 log',
    })
    expect(button).toHaveClass('timeline-marker')
    const icon = button.querySelector('svg')
    expect(icon).toHaveAttribute('width', '0.875rem')
    expect(icon?.firstElementChild).toHaveAttribute('stroke-width', '1.5')
    expect(button).toHaveTextContent('2')
  })

  it('opens an ordered interactive activity list and selects its records', async () => {
    const onSelectRecord = vi.fn()
    renderWithContexts(TimelineMarkers, {
      markers: [mixedMarker()],
      spanColor: '#abc',
      spanStartTime: 40n,
      onSelectRecord,
    })
    const button = screen.getByRole('button', {
      name: 'Open 1 event and 1 log',
    })
    await fireEvent.click(button)
    const rows = screen.getAllByRole('button', { name: /Event|Log/ })
    expect(rows[0]).toHaveTextContent(/Event\s+\+10 ns\s+exception/)
    expect(rows[1]).toHaveTextContent(/Log\s+\+11 ns\s+payment\.failed/)
    expect(screen.getByText('Event')).toHaveStyle('--activity-color: #abc')
    expect(screen.getByText('Log')).toHaveClass('badge-error')

    await fireEvent.click(rows[1]!)
    expect(onSelectRecord).toHaveBeenCalledWith(mixedMarker().members[1])
  })

  it('does not invent a name for a nameless log', async () => {
    const marker = mixedMarker()
    const logRecord = marker.members[1]!
    if (logRecord.kind !== 'log') throw new Error('expected log fixture')
    logRecord.log.eventName = ''
    marker.id = logRecord.id
    marker.members = [logRecord]
    renderWithContexts(TimelineMarkers, {
      markers: [marker],
      spanColor: '#abc',
      spanStartTime: 40n,
      onSelectRecord: vi.fn(),
    })

    await fireEvent.focus(screen.getByRole('button', { name: 'Select 1 log' }))
    const row = screen.getByRole('button', { name: /Log/ })
    expect(row).toHaveTextContent(/Log\s+\+11 ns/)
    expect(row).not.toHaveTextContent('failed')
    expect(row).not.toHaveTextContent('checkout')
    expect(row).not.toHaveTextContent('log-1')
  })

  it('previews a singleton on focus and selects it from either control', async () => {
    const marker = mixedMarker()
    marker.id = marker.members[0]!.id
    marker.members = [marker.members[0]!]
    const onSelectRecord = vi.fn()
    renderWithContexts(TimelineMarkers, {
      markers: [marker],
      spanColor: '#abc',
      spanStartTime: 1_000_000_050n,
      onSelectRecord,
    })

    const trigger = screen.getByRole('button', { name: 'Select 1 event' })
    await fireEvent.focus(trigger)
    const row = screen.getByRole('button', { name: /Event/ })
    expect(row).toHaveTextContent(/Event\s+-1\.000 s\s+exception/)
    await fireEvent.click(row)
    expect(onSelectRecord).toHaveBeenLastCalledWith(marker.members[0])

    await fireEvent.click(trigger)
    expect(onSelectRecord).toHaveBeenCalledTimes(2)
  })

  it('keeps one popover open and restores focus after Escape', async () => {
    const first = mixedMarker()
    const second = mixedMarker()
    second.id = `${second.id}-second`
    second.position = { placement: 'inside', pixel: 70, percent: 70 }
    renderWithContexts(TimelineMarkers, {
      markers: [first, second],
      spanColor: '#abc',
      spanStartTime: 40n,
      onSelectRecord: vi.fn(),
    })

    const [firstTrigger, secondTrigger] = screen.getAllByRole('button', {
      name: 'Open 1 event and 1 log',
    })
    await fireEvent.click(firstTrigger!)
    expect(firstTrigger).toHaveAttribute('aria-expanded', 'true')

    await fireEvent.click(secondTrigger!)
    expect(firstTrigger).toHaveAttribute('aria-expanded', 'false')
    expect(secondTrigger).toHaveAttribute('aria-expanded', 'true')

    await fireEvent.keyDown(document, { key: 'Escape' })
    expect(secondTrigger).toHaveAttribute('aria-expanded', 'false')
    expect(secondTrigger).toHaveFocus()
  })

  it('closes a singleton after focus leaves its trigger and row', async () => {
    const marker = mixedMarker()
    marker.id = marker.members[0]!.id
    marker.members = [marker.members[0]!]
    renderWithContexts(TimelineMarkers, {
      markers: [marker],
      spanColor: '#abc',
      spanStartTime: 40n,
      onSelectRecord: vi.fn(),
    })
    const outside = document.createElement('button')
    document.body.append(outside)
    const trigger = screen.getByRole('button', { name: 'Select 1 event' })

    trigger.focus()
    const row = screen.getByRole('button', { name: /Event/ })
    row.focus()
    await new Promise(resolve => setTimeout(resolve))
    expect(trigger).toHaveAttribute('aria-expanded', 'true')

    outside.focus()
    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
    outside.remove()
  })

  it('keeps a singleton preview open while pointer hover still owns it', async () => {
    const marker = mixedMarker()
    marker.id = marker.members[0]!.id
    marker.members = [marker.members[0]!]
    renderWithContexts(TimelineMarkers, {
      markers: [marker],
      spanColor: '#abc',
      spanStartTime: 40n,
      onSelectRecord: vi.fn(),
    })
    const outside = document.createElement('button')
    document.body.append(outside)
    const trigger = screen.getByRole('button', { name: 'Select 1 event' })
    trigger.focus()
    const row = screen.getByRole('button', { name: /Event/ })
    row.focus()
    const popover = document.querySelector('.activity-popover')!
    await fireEvent.mouseEnter(popover)

    outside.focus()
    await new Promise(resolve => setTimeout(resolve))
    expect(trigger).toHaveAttribute('aria-expanded', 'true')

    await fireEvent.mouseLeave(popover)
    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
    outside.remove()
  })

  it.each(['trigger', 'row'] as const)(
    'keeps a singleton preview open when the %s retains focus after pointer leave',
    async focusOwner => {
      const marker = mixedMarker()
      marker.id = marker.members[0]!.id
      marker.members = [marker.members[0]!]
      renderWithContexts(TimelineMarkers, {
        markers: [marker],
        spanColor: '#abc',
        spanStartTime: 40n,
        onSelectRecord: vi.fn(),
      })
      const trigger = screen.getByRole('button', { name: 'Select 1 event' })
      trigger.focus()
      const row = screen.getByRole('button', { name: /Event/ })
      const focusedElement = focusOwner === 'trigger' ? trigger : row
      const pointerTarget =
        focusOwner === 'trigger'
          ? trigger
          : document.querySelector('.activity-popover')!
      focusedElement.focus()

      await fireEvent.mouseEnter(pointerTarget)
      await fireEvent.mouseLeave(pointerTarget)
      await new Promise(resolve => setTimeout(resolve, 100))

      expect(focusedElement).toHaveFocus()
      expect(trigger).toHaveAttribute('aria-expanded', 'true')
    }
  )

  it('restores marker focus after keyboard row selection', async () => {
    const onSelectRecord = vi.fn()
    renderWithContexts(TimelineMarkers, {
      markers: [mixedMarker()],
      spanColor: '#abc',
      spanStartTime: 40n,
      onSelectRecord,
    })
    const trigger = screen.getByRole('button', {
      name: 'Open 1 event and 1 log',
    })
    await fireEvent.click(trigger)
    const row = screen.getAllByRole('button', { name: /Event|Log/ })[0]!
    row.focus()

    await userEvent.keyboard('{Enter}')

    expect(onSelectRecord).toHaveBeenCalledWith(mixedMarker().members[0])
    expect(trigger).toHaveFocus()
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  it.each(['Escape', 'keyboard row selection'] as const)(
    'does not reopen a singleton during %s focus restoration',
    async closeAction => {
      const marker = mixedMarker()
      marker.id = marker.members[0]!.id
      marker.members = [marker.members[0]!]
      const onSelectRecord = vi.fn()
      renderWithContexts(TimelineMarkers, {
        markers: [marker],
        spanColor: '#abc',
        spanStartTime: 40n,
        onSelectRecord,
      })
      const outside = document.createElement('button')
      document.body.append(outside)
      const trigger = screen.getByRole('button', { name: 'Select 1 event' })
      trigger.focus()
      const row = screen.getByRole('button', { name: /Event/ })
      row.focus()
      const popover = document.querySelector<HTMLElement>('.activity-popover')!
      const nativeHidePopover = popover.hidePopover.bind(popover)
      let hideInProgress = false
      const showPopover = vi
        .spyOn(popover, 'showPopover')
        .mockImplementation(() => {
          expect(hideInProgress).toBe(false)
          HTMLElement.prototype.showPopover.call(popover)
        })
      vi.spyOn(popover, 'hidePopover').mockImplementation(() => {
        hideInProgress = true
        nativeHidePopover()
        trigger.focus()
        hideInProgress = false
      })

      if (closeAction === 'Escape') {
        await fireEvent.keyDown(document, { key: 'Escape' })
      } else {
        await userEvent.keyboard('{Enter}')
        expect(onSelectRecord).toHaveBeenCalledWith(marker.members[0])
      }

      expect(trigger).toHaveFocus()
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
      expect(showPopover).not.toHaveBeenCalled()

      outside.focus()
      trigger.focus()
      expect(showPopover).toHaveBeenCalledOnce()
      await waitFor(() =>
        expect(trigger).toHaveAttribute('aria-expanded', 'true')
      )
      outside.remove()
    }
  )
})
