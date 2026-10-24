// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import TableValueCell from './TableValueCell.svelte'

it('preserves dragged text without inspection and supports keyboard inspection', async () => {
  const onInspect = vi.fn()
  render(TableValueCell, {
    label: 'resource: service.name',
    text: 'checkout',
    onInspect,
  })
  const button = screen.getByRole('button', {
    name: 'Show resource: service.name in details',
  })
  const selection = window.getSelection()!
  const range = document.createRange()
  range.selectNodeContents(button)
  selection.removeAllRanges()
  selection.addRange(range)
  await fireEvent.click(button, { detail: 1 })
  expect(selection.toString()).toBe('checkout')
  expect(onInspect).not.toHaveBeenCalled()
  button.focus()
  await userEvent.keyboard('{Enter}')
  expect(onInspect).toHaveBeenCalledOnce()
  selection.removeAllRanges()
})
