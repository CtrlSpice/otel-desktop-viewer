// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DetailProperty from './DetailProperty.svelte'
import { foldAll, foldedRanges } from '@codemirror/language'
import { jsonEditorView } from '@/test/json-editor-helpers'

describe('DetailProperty copy controls', () => {
  it.each(['body', 'output', 'attributes'])(
    'formats nested runtime code in shared %s Details without changing Copy or type',
    async fieldName => {
      const user = userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      const inner =
        '{"event":{"location":{"workspaceID":undefined},"created":9223372036854775807}}'
      const result = render(DetailProperty, {
        fieldName,
        fieldType: 'array',
        typedValue: {
          kind: 'array',
          value: [
            { kind: 'string', value: 'event' },
            { kind: 'string', value: inner },
          ],
        },
      })
      const view = jsonEditorView(result.container)
      expect(view.state.doc.toString()).toContain('"workspaceID": undefined')
      await user.click(
        screen.getByRole('button', { name: `Expand all ${fieldName} JSON` })
      )
      expect(
        result.container.querySelector('.json-runtime-literal')
      ).toHaveTextContent('undefined')
      expect(
        result.container.querySelector('.detail-property__type')
      ).toHaveTextContent('array')
      await user.click(
        screen.getByRole('button', { name: `Copy ${fieldName}` })
      )
      expect(copy).toHaveBeenCalledExactlyOnceWith(
        JSON.stringify(['event', inner], null, 2)
      )
      const document = view.state.doc.toString()
      await user.click(
        screen.getByRole('button', { name: `Collapse all ${fieldName} JSON` })
      )
      await result.rerender({ fieldName })
      expect(view.state.doc.toString()).toBe(document)
      expect(foldedRanges(view.state).size).toBeGreaterThan(0)
    }
  )

  it('opens the first JSON level and copies the entire source through header controls', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const source =
      '{"outer":{"deep":{"id":9223372036854775807}},"items":[{"keep":-0}]}'
    const result = render(DetailProperty, {
      fieldName: 'Output',
      fieldType: 'string',
      fieldValue: source,
    })
    const view = jsonEditorView(result.container)
    const ranges: string[] = []
    foldedRanges(view.state).between(0, view.state.doc.length, (from, to) => {
      ranges.push(view.state.doc.sliceString(from, to).replace(/\s/g, ''))
    })
    expect(ranges).toEqual(['"deep":{"id":9223372036854775807}', '{"keep":-0}'])
    expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull()
    const header = result.container.querySelector('.detail-property__header')!
    const expand = screen.getByRole('button', {
      name: 'Expand all Output JSON',
    })
    const collapse = screen.getByRole('button', {
      name: 'Collapse all Output JSON',
    })
    expect(header).toContainElement(expand)
    expect(header).toContainElement(collapse)
    await user.click(expand)
    expect(foldedRanges(view.state).size).toBe(0)
    await user.click(collapse)
    expect(foldedRanges(view.state).size).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: 'Copy Output' }))
    expect(copy).toHaveBeenCalledExactlyOnceWith(source)
    const editorNode = result.container.querySelector('.cm-editor')
    await result.rerender({ fieldValue: '{"next":{"child":{"value":true}}}' })
    expect(jsonEditorView(result.container)).toBe(view)
    expect(result.container.querySelector('.cm-editor')).toBe(editorNode)
    expect(foldedRanges(view.state).size).toBe(1)
    expect(view.state.doc.toString()).toContain('"value": true')
  })

  it('formats a stringified object inside a log body but copies the received array', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const inner = '{"path":"/recorded/config.jsonc"}'
    const { container } = render(DetailProperty, {
      fieldName: 'body',
      fieldType: 'array',
      typedValue: {
        kind: 'array',
        value: [
          { kind: 'string', value: 'loading' },
          { kind: 'string', value: inner },
        ],
      },
    })
    expect(jsonEditorView(container).state.doc.toString()).toBe(
      '[\n  "loading",\n  {\n    "path": "/recorded/config.jsonc"\n  }\n]'
    )
    await user.click(screen.getByRole('button', { name: 'Copy body' }))
    expect(copy).toHaveBeenCalledExactlyOnceWith(
      JSON.stringify(['loading', inner], null, 2)
    )
  })

  it('colours the value without changing its copy text or colouring the field header', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'severityText',
      fieldType: 'string',
      fieldValue: 'CUSTOM ERROR',
      valueColor: 'var(--color-error)',
    })
    expect(
      container
        .querySelector<HTMLElement>('[data-field="severityText"]')!
        .style.getPropertyValue('--detail-property-value-color')
    ).toBe('var(--color-error)')
    expect(
      container.querySelector<HTMLElement>('.detail-property__header')!.style
        .color
    ).toBe('')
    await user.click(screen.getByRole('button', { name: 'Copy severityText' }))
    expect(copy).toHaveBeenCalledWith('CUSTOM ERROR')
  })

  it('keeps key and type together, followed by contextual Copy opening left', () => {
    const { container } = render(DetailProperty, {
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'value',
    })
    const header = container.querySelector('.detail-property__header')!
    expect(header.firstElementChild).toHaveClass('detail-property__heading')
    expect(header.firstElementChild).toHaveTextContent('name · string')
    const copy = screen.getByRole('button', { name: 'Copy name' })
    expect(copy).toHaveClass('value-copy--contextual')
    expect(copy).toHaveClass('tooltip-left')
    expect(copy).not.toHaveClass('tooltip-right')
    expect(header.querySelector(':scope > .value-copy')).toBe(copy)
    expect(header).toContainElement(copy)
    expect(
      container.querySelector('.detail-property__body')
    ).not.toContainElement(copy)
  })

  it('copies only through the explicit copy button, not the card or value', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'name',
      fieldType: 'string',
      fieldValue: 'whole card',
    })
    await user.click(container.querySelector('.detail-property__header')!)
    await user.click(container.querySelector('.detail-property__body')!)
    await user.click(screen.getByText('whole card'))
    expect(copy).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Copy name' }))
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

  it('places readable timestamps above exact raw values, with Copy beside their key', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const { container } = render(DetailProperty, {
      fieldName: 'startTimeUnixNano',
      fieldType: 'fixed64',
      fieldValue: '1700000000000000001',
      timestampLabel: '2023-11-14, 14:13:20.000 PST',
    })
    const cell = container.querySelector('.detail-property__card')!
    expect(cell.children).toHaveLength(2)
    expect(cell.children[0]).toHaveClass('detail-property__header')
    expect(cell.children[1]).toHaveClass('detail-property__body')
    const content = cell.querySelector('.detail-property__body')!
    expect(content.querySelector('.detail-property__value')).toHaveTextContent(
      '2023-11-14, 14:13:20.000 PST'
    )
    expect(
      content.querySelector('.detail-property__raw-timestamp')
    ).toHaveTextContent('1700000000000000001')
    const button = screen.getByRole('button', {
      name: 'Copy startTimeUnixNano',
    })
    expect(
      cell.querySelector('.detail-property__heading')!.nextElementSibling
    ).toBe(button)
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

  it.each(['string', 'int64', 'bool', 'map', 'empty'])(
    'keeps the %s type and provenance on the heading line',
    fieldType => {
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
      expect(
        container.querySelector('.detail-property__header > .value-copy')
      ).toBe(screen.getByRole('button', { name: 'Copy example (scope)' }))
      expect(key).toHaveTextContent(`example · ${fieldType}`)
      expect(key).toContainElement(badge)
      expect(badge).toHaveTextContent(fieldType)
      expect(badge.style.color).toBe('')
      expect(badge).not.toHaveClass('badge-soft')
      expect(badge).not.toHaveClass('badge-outline')
      const origin = container.querySelector<HTMLElement>(
        '.detail-property__origin'
      )!
      expect(
        container.querySelector('.detail-property__heading')
      ).toContainElement(origin)
      expect(
        container.querySelector('.detail-property__heading')
      ).toHaveTextContent(`example · ${fieldType} · scope`)
      expect(origin).toHaveTextContent(/^scope$/)
      expect(origin).not.toHaveClass('badge-soft')
      expect(origin).not.toHaveClass('badge-outline')
      expect(
        container.querySelector('.detail-property__header')
      ).toContainElement(
        screen.getByRole('button', { name: 'Copy example (scope)' })
      )
    }
  )

  it('copies raw strings beside the key without presentation quotes', async () => {
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
    'keeps Copy beside the key when the type is hidden and origin is %s',
    origin => {
      const { container } = render(DetailProperty, {
        fieldName: 'name',
        fieldType: 'string',
        fieldValue: 'value',
        showType: false,
        origin,
      })
      const header = container.querySelector('.detail-property__header')!
      const copy = screen.getByRole('button', {
        name: origin ? 'Copy name (resource)' : 'Copy name',
      })
      expect(header).toContainElement(copy)
      expect(header.querySelector(':scope > .value-copy')).toBe(copy)
      expect(container.querySelector('.detail-property__type')).toBeNull()
      if (origin) {
        expect(
          container.querySelector('.detail-property__origin')
        ).toHaveTextContent(origin)
        expect(
          header.querySelector('.detail-property__heading')
        ).toContainElement(container.querySelector('.detail-property__origin'))
      } else {
        expect(container.querySelector('.detail-property__origin')).toBeNull()
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
      jsonEditorView(container).state.doc.toString()
    )
  })

  it.each(['body', 'gen_ai.input.messages'])(
    'copies the complete %s even while its structure is collapsed',
    async fieldName => {
      const user = userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      const { container } = render(DetailProperty, {
        fieldName,
        fieldType: 'map',
        typedValue: {
          kind: 'map',
          value: [
            {
              key: 'same',
              value: { kind: 'int64', value: 9_223_372_036_854_775_807n },
            },
            { key: 'same', value: { kind: 'double', value: -0 } },
          ],
        },
      })
      foldAll(jsonEditorView(container))
      expect(container.querySelector('.cm-foldPlaceholder')).toHaveTextContent(
        '…'
      )
      expect(screen.queryByText('9223372036854775807')).toBeNull()
      expect(copy).not.toHaveBeenCalled()
      await user.click(
        screen.getByRole('button', { name: `Copy ${fieldName}` })
      )
      expect(copy).toHaveBeenCalledWith(
        '{\n  "same": 9223372036854775807,\n  "same": -0\n}'
      )
    }
  )

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
      expect(container.querySelector('.cm-content')).toHaveTextContent(
        '9223372036854775807'
      )
      expect(jsonEditorView(container).state.doc.toString()).toContain(
        '\n  "items": [\n    "hello",'
      )
      foldAll(jsonEditorView(container))
      expect(container.querySelector('.cm-foldPlaceholder')).toHaveTextContent(
        '…'
      )
      expect(container.querySelector('.cm-content')).not.toHaveTextContent(
        '9223372036854775807'
      )
      await user.click(screen.getByRole('button', { name: 'Copy body' }))
      expect(copy).toHaveBeenCalledWith(text)
    }
  )

  it.each([
    ['traceId', 'custom-id'],
    ['value', 'c7dc4131-2ce3-4d1c-a50b-cc10592aad7c'],
    ['sequence', '9223372036854775807'],
  ])(
    'displays technical string %s without JSON wrapper quotes',
    (fieldName, fieldValue) => {
      const { container } = render(DetailProperty, {
        fieldName,
        fieldType: 'string',
        fieldValue,
      })
      expect(
        container.querySelector('.attribute-value--text')?.textContent?.trim()
      ).toBe(fieldValue)
    }
  )

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
