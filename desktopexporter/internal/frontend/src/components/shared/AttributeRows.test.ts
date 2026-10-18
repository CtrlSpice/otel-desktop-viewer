// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/svelte'
import AttributeRows from './AttributeRows.svelte'
import { renderWithContexts } from '@/test/render-helpers'
import type { Attributes } from '@/types/api-types'

function renderAttributes(attributes: Attributes, origin?: string) {
  return renderWithContexts(AttributeRows, {
    attributes,
    owner: 'span attributes',
    origin,
  })
}

describe('AttributeRows', () => {
  it('shows formatted JSON immediately and preserves duplicate attribute entries', () => {
    const { container } = renderAttributes([
      {
        id: 'one',
        key: 'payload',
        hasConflict: true,
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
        key: 'payload',
        hasConflict: true,
        value: { kind: 'string', value: 'raw' },
      },
    ])
    expect(JSON.parse(container.querySelector('pre')!.textContent!)).toEqual({
      attempts: 2,
      flags: [true],
    })
    expect(screen.getByText('raw')).toBeInTheDocument()
    expect(container.querySelectorAll('[data-field="payload"]')).toHaveLength(2)
    expect(
      screen.getAllByRole('img', {
        name: 'Conflicting typed values retained for payload in span attributes.',
      })
    ).toHaveLength(2)
    expect(container.querySelector('details')).toBeNull()
  })

  it('separates origin and type badges from the unchanged key and value', () => {
    const attributes: Attributes = [
      { key: 'service.name', value: { kind: 'string', value: 'checkout' } },
    ]
    const { container } = renderAttributes(attributes, 'resource')
    const header = container.querySelector('.detail-property__header')!
    expect(
      within(header as HTMLElement).getByText('service.name')
    ).toBeInTheDocument()
    expect(within(header as HTMLElement).getByText('resource')).toHaveClass(
      'badge-primary'
    )
    const type = within(header as HTMLElement).getByText('string')
    expect(type).toHaveClass('badge-soft', 'badge-accent')
    expect(type.style.color).toBe('')
    expect(
      screen
        .getByRole('button', { name: 'Copy service.name (resource)' })
        .closest('.detail-property__header')
    ).not.toBeNull()
    expect(container).not.toHaveTextContent('resource.service.name')
    expect(attributes[0]!.key).toBe('service.name')
  })

  it('does not warn for exact repeated canonical values', () => {
    renderAttributes([
      { id: 'one', key: 'retry', value: { kind: 'int64', value: 1n } },
      { id: 'two', key: 'retry', value: { kind: 'int64', value: 1n } },
    ])
    expect(screen.queryByRole('img')).toBeNull()
  })

  it('preserves punctuation, escaping, int64 precision and negative zero', () => {
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
    expect(container.querySelector('pre')!.textContent).toBe(
      '[\n  "line\\n\\"quoted\\"",\n  9223372036854775807,\n  -0\n]'
    )
    expect(screen.getByText('9223372036854775807')).toHaveAttribute(
      'title',
      'int64'
    )
  })

  it('uses JSON array syntax rather than injecting indices into the copied value', () => {
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
    expect(JSON.parse(container.querySelector('pre')!.textContent!)).toEqual([
      'first',
      { '[0]': 'key' },
    ])
  })

  it('warns once on a conflicted structured value while preserving every duplicate key', () => {
    const { container } = renderAttributes([
      {
        key: 'payload',
        value: {
          kind: 'map',
          conflictingKeys: ['same'],
          value: [
            { key: 'same', value: { kind: 'int64', value: 1n } },
            { key: 'same', value: { kind: 'string', value: '1' } },
            { key: 'clear', value: { kind: 'bool', value: true } },
          ],
        },
      },
    ])
    expect(screen.getAllByRole('img')).toHaveLength(1)
    expect(screen.getByRole('img')).toHaveAccessibleName(
      /Conflicting typed values retained in payload/
    )
    expect(container.querySelector('pre')!.textContent).toBe(
      '{\n  "same": 1,\n  "same": "1",\n  "clear": true\n}'
    )
  })
})
