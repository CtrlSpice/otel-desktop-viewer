// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import WaterfallEventDots from './WaterfallEventDots.svelte'

describe('WaterfallEventDots', () => {
  it('calls onSelectEvent with the marker index', async () => {
    const onSelectEvent = vi.fn()

    render(WaterfallEventDots, {
      props: {
        markers: [{ percent: 50, name: 'exception', eventIndex: 2 }],
        color: '#f00',
        layer: 'tooltips',
        onSelectEvent,
      },
    })

    await userEvent.click(
      screen.getByRole('button', { name: 'Event: exception' })
    )

    expect(onSelectEvent).toHaveBeenCalledWith(2)
  })
})
