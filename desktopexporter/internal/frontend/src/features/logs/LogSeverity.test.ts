// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import LogSeverity from './LogSeverity.svelte'
import { severityBand } from './log-severity'

describe('LogSeverity', () => {
  it('distinguishes omitted severity from TRACE and preserves supplied text', async () => {
    const view = render(LogSeverity, { number: 0, text: '' })
    expect(screen.getByText('Unspecified (0)')).toBeInTheDocument()
    expect(severityBand(0)).toBe('unspecified')
    await view.rerender({ text: 'CUSTOM' })
    expect(screen.getByText('CUSTOM (0)')).toBeInTheDocument()
    await view.rerender({ number: 1, text: '' })
    expect(screen.getByText('TRACE (1)')).toBeInTheDocument()
    expect(severityBand(1)).toBe('trace')
  })

  it.each([
    [1, 'TRACE', 'neutral'],
    [5, 'DEBUG', 'success'],
    [9, 'INFO', 'info'],
    [13, 'WARN', 'warning'],
    [17, 'ERROR', 'error'],
    [24, 'FATAL', 'error'],
    [0, 'UNSPECIFIED', 'neutral'],
  ])(
    'shows severity %s as a coloured dot followed by its label and number',
    (number, text, colour) => {
      const { container } = render(LogSeverity, { number, text })
      const dot = container.querySelector<HTMLElement>('.log-severity__dot')!
      expect(dot).toBeEmptyDOMElement()
      expect(dot).toHaveAttribute('aria-hidden', 'true')
      expect(dot.style.backgroundColor).toBe(`var(--color-${colour})`)
      expect(container.querySelector('.log-severity__label')).toHaveTextContent(
        `${text} (${number})`
      )
      expect(screen.queryByRole('button')).toBeNull()
      expect(container.querySelector('.badge')).toBeNull()
    }
  )

  it('retains received labels and derives a label only when none was received', async () => {
    const view = render(LogSeverity, { number: 18, text: 'CUSTOM ERROR' })
    expect(screen.getByText('CUSTOM ERROR (18)')).toBeInTheDocument()
    await view.rerender({ text: '' })
    expect(screen.getByText('ERROR (18)')).toBeInTheDocument()
  })
})
