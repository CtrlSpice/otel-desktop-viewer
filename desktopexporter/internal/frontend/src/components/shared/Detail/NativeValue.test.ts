// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/svelte'
import type { AttributeValue } from '@/types/api-types'
import DetailProperty from './DetailProperty.svelte'

it('leaves closed native descendants unread and bounds wide expansions', async () => {
  const read = vi.fn((index: number): AttributeValue => ({
    kind: 'int64',
    value: BigInt(index),
  }))
  const huge: AttributeValue = {
    kind: 'array',
    childCount: 1000000,
    childAt: read,
    get value(): AttributeValue[] {
      throw new Error('eager full-container read')
    },
  }
  render(DetailProperty, {
    fieldName: 'payload',
    fieldType: 'map',
    typedValue: { kind: 'map', value: [{ key: 'huge', value: huge }] },
  })
  expect(read).not.toHaveBeenCalled()
  await fireEvent.click(screen.getByRole('button', { name: 'Expand huge' }))
  expect(read).toHaveBeenCalledTimes(100)
  expect(
    screen.getByRole('button', { name: 'Show next 100 of 1000000' })
  ).toBeInTheDocument()
  await fireEvent.click(screen.getByRole('button', { name: 'Collapse huge' }))
  expect(read).toHaveBeenCalledTimes(100)
  await fireEvent.click(
    screen.getByRole('button', { name: 'Collapse payload' })
  )
  await fireEvent.click(screen.getByRole('button', { name: 'Expand payload' }))
  expect(screen.getByRole('button', { name: 'Expand huge' })).toHaveAttribute(
    'aria-expanded',
    'false'
  )
  await fireEvent.click(screen.getByRole('button', { name: 'Expand huge' }))
  expect(read).toHaveBeenCalledTimes(100)
})
