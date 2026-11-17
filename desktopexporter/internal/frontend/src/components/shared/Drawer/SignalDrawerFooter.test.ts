// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SignalDrawerFooter from './SignalDrawerFooter.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderFooter(props: {
  label: 'trace' | 'log' | 'metric'
  onDeleteAll?: () => void
}) {
  setTestUrl('/logs')
  const onDeleteAll = props.onDeleteAll ?? vi.fn()
  renderWithContexts(SignalDrawerFooter, {
    label: props.label,
    onDeleteAll,
  })
  return onDeleteAll
}

describe('SignalDrawerFooter', () => {
  it('calls onDeleteAll when the button is clicked', async () => {
    const onDeleteAll = renderFooter({ label: 'log' })
    await userEvent.click(
      screen.getByRole('button', { name: 'Delete all logs' })
    )
    expect(onDeleteAll).toHaveBeenCalledTimes(1)
  })
})
