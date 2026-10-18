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
    ['status.code', 'enum', '2'],
    ['status.message', 'string', 'received error message'],
  ])(
    'copies the error-toned %s value without changing its contents',
    async (fieldName, fieldType, fieldValue) => {
      const user = userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      render(DetailProperty, {
        fieldName,
        fieldType,
        fieldValue,
        valueTone: 'error',
      })
      await user.click(
        screen.getByRole('button', { name: `Copy ${fieldName}` })
      )
      expect(copy).toHaveBeenCalledWith(fieldValue)
    }
  )

  it.each([
    ['string', 'badge-accent'],
    ['int64', 'badge-warning'],
    ['bool', 'badge-primary'],
    ['map', 'badge-primary'],
    ['empty', 'badge-neutral'],
  ])(
    'groups the %s type and origin badges before the trailing copy control',
    (fieldType, tone) => {
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
      const actions = container.querySelector('.detail-property__actions')!
      const badges = container.querySelector('.detail-property__badges')!
      expect(key.nextElementSibling).toBe(actions)
      expect(actions.firstElementChild).toBe(badges)
      expect(badges.firstElementChild).toBe(badge)
      expect(badge.style.color).toBe('')
      expect(badge).toHaveClass('badge-soft', tone)
      expect(badge).not.toHaveClass('badge-outline')
      const origin = container.querySelector('.detail-property__origin')!
      expect(badge.nextElementSibling).toBe(origin)
      expect(origin).toHaveClass('badge-primary', 'badge-soft')
      expect(origin).not.toHaveClass('badge-outline')
      expect(badges.nextElementSibling).toBe(
        screen.getByRole('button', { name: 'Copy example (scope)' })
      )
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
    expect(screen.getByText('checkout')).toBeInTheDocument()
  })

  it.each([undefined, 'resource'])(
    'keeps Copy at the end when the type is hidden and origin is %s',
    origin => {
      const { container } = render(DetailProperty, {
        fieldName: 'name',
        fieldType: 'string',
        fieldValue: 'value',
        showType: false,
        origin,
      })
      const actions = container.querySelector('.detail-property__actions')!
      const copy = screen.getByRole('button', {
        name: origin ? 'Copy name (resource)' : 'Copy name',
      })
      expect(actions).toContainElement(copy)
      expect(container.querySelector('.detail-property__type')).toBeNull()
      if (origin) {
        expect(actions.firstElementChild).toHaveClass('detail-property__badges')
        expect(actions.firstElementChild!.nextElementSibling).toBe(copy)
      } else {
        expect(container.querySelector('.detail-property__badges')).toBeNull()
        expect(actions.firstElementChild).toBe(copy)
      }
    }
  )

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

  it.each([false, true])(
    'renders ordinary strings as text (typed: %s) while copying exact whitespace',
    async typed => {
      const user = userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      const text = 'heatmap-playground\n  heatmap-design-playground'
      const { container } = render(DetailProperty, {
        fieldName: 'service.name',
        fieldType: 'string',
        ...(typed
          ? { typedValue: { kind: 'string' as const, value: text } }
          : { fieldValue: text }),
      })
      expect(
        container.querySelector('.attribute-value--text')!.textContent
      ).toContain(text)
      expect(container.querySelector('pre')).toBeNull()
      await user.click(
        screen.getByRole('button', { name: 'Copy service.name' })
      )
      expect(copy).toHaveBeenCalledWith(text)
    }
  )

  it.each([false, true])(
    'formats JSON strings but copies original source (typed: %s)',
    async typed => {
      const user = userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      const text = '{"id":9223372036854775807,"items":["hello",false]}'
      const { container } = render(DetailProperty, {
        fieldName: 'body',
        fieldType: 'string',
        ...(typed
          ? { typedValue: { kind: 'string' as const, value: text } }
          : { fieldValue: text }),
      })
      expect(
        container.querySelector('.attribute-value--structured pre')
      ).toHaveTextContent('9223372036854775807')
      expect(container.querySelector('pre')!.textContent).toContain(
        '\n  "items": [\n    "hello",'
      )
      expect(container.querySelector('.detail-code--key')).toHaveTextContent(
        '"id"'
      )
      await user.click(screen.getByRole('button', { name: 'Copy body' }))
      expect(copy).toHaveBeenCalledWith(text)
    }
  )

  it.each([
    ['traceId', 'custom-id'],
    ['value', 'c7dc4131-2ce3-4d1c-a50b-cc10592aad7c'],
    ['sequence', '9223372036854775807'],
  ])('keeps technical string %s in code styling', (fieldName, fieldValue) => {
    const { container } = render(DetailProperty, {
      fieldName,
      fieldType: 'string',
      fieldValue,
    })
    expect(container.querySelector('.attribute-value--text')).toBeNull()
    expect(container.querySelector('code')).toHaveTextContent(fieldValue)
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
