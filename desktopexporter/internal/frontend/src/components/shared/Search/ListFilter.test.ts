// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, expect, it, vi } from 'vitest'
import ListFilterHarness from '@/test/ListFilterHarness.svelte'

afterEach(cleanup)

it('opens from a labelled icon, focuses the field and retains the query on Escape', async () => {
  const onkeydown = vi.fn()
  render(ListFilterHarness, { onkeydown })
  const trigger = screen.getByRole('button', { name: 'Filter items' })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('searchbox')).toBeNull()
  expect(
    document.getElementById(trigger.getAttribute('aria-controls')!)
  ).toHaveAttribute('hidden')

  await fireEvent.click(trigger)
  const input = screen.getByRole('searchbox', { name: 'Filter items' })
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(input).toHaveFocus()
  await fireEvent.input(input, { target: { value: 'a received value' } })
  expect(trigger).toHaveAccessibleName('Filter items (active)')
  expect(trigger).toHaveClass('text-primary')

  const escape = new KeyboardEvent('keydown', {
    key: 'Escape',
    bubbles: true,
    cancelable: true,
  })
  await fireEvent(input, escape)
  expect(trigger).toHaveFocus()
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('searchbox')).toBeNull()
  expect(input).toHaveValue('a received value')
  expect(escape.defaultPrevented).toBe(true)
  expect(onkeydown).not.toHaveBeenCalled()

  await fireEvent.click(trigger)
  expect(input).toHaveFocus()
  expect(input).toHaveValue('a received value')
  await fireEvent.input(input, { target: { value: '' } })
  expect(trigger).toHaveAccessibleName('Filter items')
  expect(trigger).not.toHaveClass('text-primary')
  await fireEvent.keyDown(input, { key: 'ArrowDown' })
  expect(onkeydown).toHaveBeenCalledOnce()
  await fireEvent.click(trigger)
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
})

it('uses unique controls for independent filters and marks an initially active query', async () => {
  render(ListFilterHarness, { label: 'First filter', value: 'saved' })
  render(ListFilterHarness, { label: 'Second filter', value: '  ' })
  const first = screen.getByRole('button', { name: 'First filter (active)' })
  const second = screen.getByRole('button', { name: 'Second filter' })
  expect(first.getAttribute('aria-controls')).not.toBe(
    second.getAttribute('aria-controls')
  )
  await fireEvent.click(first)
  expect(screen.getByRole('searchbox', { name: 'First filter' })).toHaveValue(
    'saved'
  )
  expect(second).toHaveAttribute('aria-expanded', 'false')
})
