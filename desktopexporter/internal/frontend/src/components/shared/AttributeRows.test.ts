// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { screen, within, fireEvent } from '@testing-library/svelte'
import AttributeRows from './AttributeRows.svelte'
import { renderWithContexts } from '@/test/render-helpers'
import type { Attributes } from '@/types/api-types'
import { jsonEditorView } from '@/test/json-editor-helpers'

function renderAttributes(attributes: Attributes, origin?: string) {
  return renderWithContexts(AttributeRows, {
    attributes,
    owner: 'span attributes',
    origin,
  })
}

describe('AttributeRows', () => {
  it('shows native fields and scalars immediately, formatting a full document on request', async () => {
    const { container } = renderAttributes([
      {
        id: 'one',
        key: 'payload',
        value: {
          kind: 'map',
          value: [
            { key: 'attempts', value: { kind: 'int64', value: 2n } },
            {
              key: 'flags',
              value: { kind: 'array', value: [{ kind: 'bool', value: true }] },
            },
          ],
        },
      },
      {
        id: 'two',
        key: 'message',
        value: { kind: 'string', value: 'raw' },
      },
    ])
    expect(container.querySelector('.cm-editor')).toBeNull()
    expect(
      screen.getByRole('button', { name: 'Expand flags' })
    ).toHaveAttribute('aria-expanded', 'false')
    await fireEvent.click(
      screen.getByRole('button', { name: 'Full document / Find' })
    )
    expect(JSON.parse(jsonEditorView(container).state.doc.toString())).toEqual({
      attempts: 2,
      flags: [true],
    })
    expect(screen.getByText('raw')).toBeInTheDocument()
    expect(container.querySelectorAll('[data-field="payload"]')).toHaveLength(1)
    expect(container.querySelector('details')).toBeNull()
  })

  it('keeps the unchanged key, type and provenance on the heading line', () => {
    const attributes: Attributes = [
      { key: 'service.name', value: { kind: 'string', value: 'checkout' } },
    ]
    const { container } = renderAttributes(attributes, 'resource')
    const header = container.querySelector('.detail-property__header')!
    expect(
      within(header as HTMLElement).getByText('service.name')
    ).toBeInTheDocument()
    const origin = container.querySelector<HTMLElement>(
      '.detail-property__origin'
    )!
    expect(origin).toHaveTextContent(/^resource$/)
    expect(header.querySelector('.detail-property__heading')).toContainElement(
      origin
    )
    expect(header.querySelector('.detail-property__heading')).toHaveTextContent(
      'service.name · string · resource'
    )
    expect(origin).not.toHaveClass('badge-soft')
    const type = within(header as HTMLElement).getByText('string')
    expect(header.querySelector('.detail-property__key')).toContainElement(type)
    expect(type).not.toHaveClass('badge-soft')
    expect(type.style.color).toBe('')
    expect(
      screen
        .getByRole('button', { name: 'Copy service.name (resource)' })
        .closest('.detail-property__header')
    ).not.toBeNull()
    expect(container).not.toHaveTextContent('resource.service.name')
    expect(attributes[0]!.key).toBe('service.name')
  })

  it('preserves punctuation, escaping, int64 precision and negative zero', async () => {
    const { container } = renderAttributes([
      {
        key: 'payload',
        value: {
          kind: 'array',
          value: [
            { kind: 'string', value: 'line\n"quoted"' },
            { kind: 'int64', value: 9_223_372_036_854_775_807n },
            { kind: 'double', value: -0 },
          ],
        },
      },
    ])
    await fireEvent.click(
      screen.getByRole('button', { name: 'Full document / Find' })
    )
    expect(jsonEditorView(container).state.doc.toString()).toBe(
      '[\n  "line\\n\\"quoted\\"",\n  9223372036854775807,\n  -0\n]'
    )
    expect(screen.getByLabelText('JSON value of payload')).toBeInTheDocument()
  })

  it('uses JSON array syntax rather than injecting indices into the copied value', async () => {
    const { container } = renderAttributes([
      {
        key: 'payload',
        value: {
          kind: 'array',
          value: [
            { kind: 'string', value: 'first' },
            {
              kind: 'map',
              value: [{ key: '[0]', value: { kind: 'string', value: 'key' } }],
            },
          ],
        },
      },
    ])
    await fireEvent.click(
      screen.getByRole('button', { name: 'Full document / Find' })
    )
    expect(JSON.parse(jsonEditorView(container).state.doc.toString())).toEqual([
      'first',
      { '[0]': 'key' },
    ])
  })
})
