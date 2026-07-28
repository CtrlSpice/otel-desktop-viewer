// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SignalDrawerFooter from './SignalDrawerFooter.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderFooter(props: {
  count: number
  label: 'trace' | 'log' | 'metric'
  onDeleteAll?: () => void
}) {
  setTestUrl('/logs')
  const onDeleteAll = props.onDeleteAll ?? vi.fn()
  renderWithContexts(SignalDrawerFooter, {
    count: props.count,
    label: props.label,
    onDeleteAll,
  })
  return onDeleteAll
}

describe('SignalDrawerFooter', () => {
  it('uses singular label for a single item', () => {
    renderFooter({ count: 1, label: 'log' })
    expect(screen.getByText('1 log')).toBeInTheDocument()
  })

  it('uses plural label for multiple items', () => {
    renderFooter({ count: 3, label: 'trace' })
    expect(screen.getByText('3 traces')).toBeInTheDocument()
  })

  it('uses text-base-content/50 for the count', () => {
    renderFooter({ count: 2, label: 'metric' })
    const count = screen.getByText('2 metrics')
    expect(count.className).toContain('text-base-content/50')
  })

  it('calls onDeleteAll when the button is clicked', async () => {
    const onDeleteAll = renderFooter({ count: 0, label: 'log' })
    await userEvent.click(
      screen.getByRole('button', { name: 'Delete all logs' })
    )
    expect(onDeleteAll).toHaveBeenCalledTimes(1)
  })
})
