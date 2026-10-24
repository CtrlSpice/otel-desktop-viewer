// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, expect, it, vi } from 'vitest'
import TableColumnsPicker from './TableColumnsPicker.svelte'

afterEach(cleanup)

it('offers independent pins only when requested and keeps required visibility intact', async () => {
  const onPin = vi.fn()
  const onchange = vi.fn()
  render(TableColumnsPicker, {
    options: [
      { id: 'name', label: 'Name', group: 'Attribute', disabled: true },
      { id: 'count', label: 'Records', group: 'Records' },
      { id: 'source', label: 'Source', group: 'Attribute' },
      { id: 'copy', label: 'Copy', group: 'Actions', pinnable: false },
    ],
    selected: ['name', 'count', 'copy'],
    pinned: ['name'],
    onPin,
    onchange,
  })
  await fireEvent.click(screen.getByRole('button', { name: 'Columns' }))
  expect(screen.getByRole('checkbox', { name: 'Name' })).toBeDisabled()
  const pin = screen.getByRole('button', { name: 'Pin Records column' })
  expect(pin).toHaveAttribute('aria-pressed', 'false')
  await fireEvent.click(pin)
  expect(onPin).toHaveBeenCalledExactlyOnceWith('count')
  expect(onchange).not.toHaveBeenCalled()
  expect(
    screen.getByRole('button', { name: 'Unpin Name column' })
  ).toHaveAttribute('aria-pressed', 'true')
  expect(
    screen.getByRole('button', { name: 'Pin Source column' })
  ).toBeDisabled()
  expect(screen.queryByRole('button', { name: 'Pin Copy column' })).toBeNull()
})

it('leaves existing non-pinnable column pickers without extra controls', async () => {
  render(TableColumnsPicker, {
    options: [{ id: 'name', label: 'Name', group: 'Fields' }],
    selected: ['name'],
    onchange: vi.fn(),
  })
  await fireEvent.click(screen.getByRole('button', { name: 'Columns' }))
  expect(screen.queryByRole('button', { name: /pin .* column/i })).toBeNull()
})
