// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte'
import { describe, expect, it, vi } from 'vitest'
import ChartKeyboardSurface from './ChartKeyboardSurface.svelte'

describe('ChartKeyboardSurface', () => {
  it('provides one named focus stop and matching polite atomic output', async () => {
    const onKeydown = vi.fn()
    const onFocusChange = vi.fn()
    render(ChartKeyboardSurface, {
      props: {
        id: 'test-chart',
        label: 'Request rate chart',
        instructions: 'Use arrow keys to inspect points.',
        readout: ' Point  2 of 4 .\n Value 10 /s ',
        shortcuts: ['ArrowLeft', 'ArrowRight', 'Enter', 'Escape'],
        onKeydown,
        onFocusChange,
        roleDescription: 'interactive time series chart',
      },
    })

    const surface = screen.getByRole('application', {
      name: 'Request rate chart',
    })
    const status = screen.getByRole('status')

    expect(surface).toHaveAttribute('tabindex', '0')
    expect(surface).toHaveAttribute(
      'aria-roledescription',
      'interactive time series chart'
    )
    expect(surface).toHaveAttribute(
      'aria-keyshortcuts',
      'ArrowLeft ArrowRight Enter Escape'
    )
    expect(surface).toHaveAccessibleDescription(
      'Use arrow keys to inspect points.'
    )
    expect(status).toHaveAttribute('aria-live', 'polite')
    expect(status).toHaveAttribute('aria-atomic', 'true')
    expect(status).toHaveTextContent('')

    await fireEvent.focus(surface)
    expect(onFocusChange).toHaveBeenLastCalledWith(true)
    expect(status).toHaveTextContent('Point 2 of 4. Value 10 /s')
    expect(document.querySelector('.chart-keyboard-readout')).toHaveTextContent(
      status.textContent ?? ''
    )

    await fireEvent.keyDown(surface, { key: 'ArrowRight' })
    expect(onKeydown).toHaveBeenCalledOnce()
    await fireEvent.blur(surface)
    expect(onFocusChange).toHaveBeenLastCalledWith(false)
  })

  it('does not advertise activation keys for an inspection-only chart', () => {
    render(ChartKeyboardSurface, {
      props: {
        id: 'snapshot-chart',
        label: 'Histogram snapshot distribution chart',
        instructions: 'Use arrow keys to inspect buckets.',
        readout: 'Bucket 1 of 3.',
        shortcuts: ['ArrowLeft', 'ArrowRight', 'Home', 'End', 'Escape'],
        onKeydown: vi.fn(),
      },
    })

    const shortcuts = screen
      .getByRole('application', {
        name: 'Histogram snapshot distribution chart',
      })
      .getAttribute('aria-keyshortcuts')
    expect(shortcuts).not.toContain('Enter')
    expect(shortcuts).not.toContain('Space')
  })
})
