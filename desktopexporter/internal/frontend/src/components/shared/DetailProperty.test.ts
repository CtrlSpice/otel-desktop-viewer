// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DetailProperty from './DetailProperty.svelte'

describe('DetailProperty copy controls', () => {
  it('keeps the name first and a contextual copy control on the right', () => {
    const { container } = render(DetailProperty, {
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'value',
    })
    const header = container.querySelector('.detail-property__header')!
    expect(header.firstElementChild).toHaveClass('detail-property__key')
    const copy = screen.getByRole('button', { name: 'Copy name' })
    expect(copy).toHaveClass('value-copy--contextual')
    expect(copy.closest('.detail-property__actions')).not.toBeNull()
  })

  it('copies the value when the card is clicked without aiming at the icon', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'whole card',
    })
    await user.click(container.querySelector('.detail-property__body')!)
    expect(copy).toHaveBeenCalledExactlyOnceWith('whole card')
  })

  it('puts the shared hover and selection surface on a card, not the table row', () => {
    const { container } = render(DetailProperty, {
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'example',
    })
    const row = container.querySelector('tr')!
    const card = container.querySelector('.detail-property__card')!
    expect(row).not.toHaveClass('table-row', 'item-state')
    expect(card).toHaveClass('item-state')
    expect(card.querySelector('.detail-property__header')).not.toBeNull()
    expect(card.querySelector('.detail-property__body')).not.toBeNull()
  })

  it('places readable timestamps above exact raw values, with Copy in the header', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'startTimeUnixNano',
      fieldType: 'fixed64',
      fieldValue: '1700000000000000001',
      timestampLabel: '2023-11-14, 14:13:20.000 PST',
    })
    const cell = container.querySelector('.detail-property__card')!
    expect(cell.children).toHaveLength(3)
    expect(cell.children[0]).toHaveClass('detail-property__header')
    expect(cell.children[1]).toHaveClass('detail-property__timestamp')
    expect(cell.children[2]).toHaveClass('detail-property__body')
    expect(cell.children[1]).toHaveTextContent('2023-11-14, 14:13:20.000 PST')
    expect(cell.children[2]).toHaveTextContent('1700000000000000001')
    const button = screen.getByRole('button', {
      name: 'Copy startTimeUnixNano',
    })
    expect(cell.children[0]!.contains(button)).toBe(true)
    await user.click(button)
    expect(copy).toHaveBeenCalledWith('1700000000000000001')
  })

  it('shows an enum code and parenthesized label together but copies only the code', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'kind',
      fieldType: 'enum',
      fieldValue: '2',
      valueLabel: 'Server',
    })
    const value = container.querySelector('.detail-property__value')!
    expect(value).toHaveTextContent('2 (Server)')
    expect(
      value.querySelector('.detail-property__annotation')
    ).toHaveTextContent('(Server)')
    await user.click(screen.getByRole('button', { name: 'Copy kind' }))
    expect(copy).toHaveBeenCalledWith('2')
  })

  it.each([
    ['string', 'var(--color-accent)'],
    ['int64', 'var(--color-warning)'],
    ['bool', 'var(--color-primary)'],
    ['map', 'var(--color-primary)'],
    ['empty', 'var(--color-neutral)'],
  ])(
    'places the %s type badge directly beside the key with its type color',
    (fieldType, color) => {
      const { container } = render(DetailProperty, {
        fieldName: 'example',
        fieldType,
        fieldValue: 'value',
        origin: 'scope',
      })
      const key = container.querySelector('.detail-property__key')!
      const badge = container.querySelector<HTMLElement>(
        '.detail-property__type'
      )!
      expect(key.nextElementSibling).toBe(badge)
      expect(badge.style.color).toBe(color)
      expect(badge).toHaveClass('badge-soft', 'badge-neutral')
      expect(badge).not.toHaveClass('badge-outline')
      const origin = container.querySelector('.detail-property__origin')!
      expect(badge.nextElementSibling).toBe(origin)
      expect(origin).toHaveClass('badge-neutral', 'badge-soft')
      expect(origin).not.toHaveClass('badge-outline')
    }
  )

  it('copies raw strings from the header without presentation quotes', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'service.name',
      fieldType: 'string',
      fieldValue: 'checkout',
      origin: 'resource',
    })
    const button = screen.getByRole('button', {
      name: 'Copy service.name (resource)',
    })
    expect(button.closest('.detail-property__header')).not.toBeNull()
    expect(container.querySelector('.detail-property__body button')).toBeNull()
    await user.click(button)
    expect(copy).toHaveBeenCalledWith('checkout')
    expect(screen.getByRole('status')).toHaveTextContent(
      'Copied service.name (resource).'
    )
    expect(screen.getByText('"checkout"')).toBeInTheDocument()
  })

  it('copies the entire formatted structured value, with exact int64 digits', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'body',
      fieldType: 'map',
      typedValue: {
        kind: 'map',
        value: [
          {
            key: 'sequence',
            value: { kind: 'int64', value: 9_223_372_036_854_775_807n },
          },
          { key: 'ready', value: { kind: 'bool', value: false } },
        ],
      },
    })
    await user.click(screen.getByRole('button', { name: 'Copy body' }))
    expect(copy).toHaveBeenCalledWith(
      '{\n  "sequence": 9223372036854775807,\n  "ready": false\n}'
    )
    expect(copy.mock.calls[0]![0]).toBe(
      container.querySelector('pre')!.textContent
    )
  })

  it('announces clipboard failure without falsely marking the value copied', async () => {
    const user = userEvent.setup()
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(
      new Error('denied')
    )
    render(DetailProperty, {
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'hello',
    })
    const button = screen.getByRole('button', { name: 'Copy name' })
    await user.click(button)
    expect(screen.getByRole('status')).toHaveTextContent('Could not copy name.')
    expect(button).not.toHaveClass('value-copy--copied')
  })

  it('clears success feedback when the selected value changes', async () => {
    const user = userEvent.setup()
    vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { rerender } = render(DetailProperty, {
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'first',
    })
    await user.click(screen.getByRole('button', { name: 'Copy name' }))
    await rerender({
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'second',
    })
    expect(screen.getByRole('status').textContent).toBe('')
    expect(screen.getByRole('button', { name: 'Copy name' })).not.toHaveClass(
      'value-copy--copied'
    )
  })
})
