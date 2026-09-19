// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import AttributeRows from './AttributeRows.svelte'
import { renderWithContexts } from '@/test/render-helpers'
import type { Attributes } from '@/types/api-types'

function renderAttributes(attributes: Attributes) {
  return renderWithContexts(AttributeRows, {
    attributes,
    owner: 'span attributes',
  })
}

describe('AttributeRows', () => {
  it('renders recursive values and preserves duplicate attribute entries', async () => {
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

    expect(screen.queryByText('attempts')).toBeNull()
    expect(screen.getByText('{…}')).toBeInTheDocument()
    expect(screen.getByLabelText('2 entries')).toBeInTheDocument()
    const markers = screen.getAllByRole('img', {
      name: 'Conflicting typed values retained for payload in span attributes.',
    })
    expect(markers).toHaveLength(2)
    for (const marker of markers) {
      expect(
        marker.nextElementSibling?.classList.contains('detail-cell__key')
      ).toBe(true)
    }

    await userEvent.click(screen.getByText('{…}'))

    expect(screen.getByText('"attempts":')).toBeInTheDocument()
    expect(screen.getByText('2 entries')).toBeInTheDocument()
    expect(screen.getByText('[…]')).toBeInTheDocument()
    expect(screen.getByText(',')).toBeInTheDocument()
    expect(
      container.querySelector(
        '.attribute-value__children > .attribute-value__child > .attribute-value__container'
      )
    ).toBeInTheDocument()
    expect(container.querySelectorAll('.detail-cell__key')).toHaveLength(2)
  })

  it('does not warn for exact repeated canonical values', () => {
    renderAttributes([
      { id: 'one', key: 'retry', value: { kind: 'int64', value: 1n } },
      { id: 'two', key: 'retry', value: { kind: 'int64', value: 1n } },
    ])

    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('formats structured values without losing punctuation or scalar fidelity', async () => {
    renderAttributes([
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

    await userEvent.click(screen.getByText('[…]'))

    expect(screen.getByText('[')).toBeInTheDocument()
    expect(screen.getByText(']')).toBeInTheDocument()
    expect(screen.getAllByText(',')).toHaveLength(2)
    expect(screen.getByText('"line\\n\\"quoted\\""')).toBeInTheDocument()
    expect(screen.getByText('9223372036854775807')).toBeInTheDocument()
    expect(screen.getByText('-0')).toBeInTheDocument()
  })

  it('renders array indices structurally while retaining quoted map keys', async () => {
    renderAttributes([
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

    await userEvent.click(screen.getByText('[…]'))
    expect(screen.getByText('[0]:')).toBeInTheDocument()
    expect(screen.queryByText('"[0]":')).toBeNull()

    await userEvent.click(screen.getByText('{…}'))
    expect(screen.getByText('"[0]":')).toBeInTheDocument()
  })

  it('marks affected top-level rows without rendering nested markers', async () => {
    renderAttributes([
      {
        id: 'one',
        key: 'demo.conflict',
        hasConflict: true,
        value: { kind: 'int64', value: 1n },
      },
      {
        id: 'two',
        key: 'demo.conflict',
        hasConflict: true,
        value: { kind: 'string', value: '1' },
      },
      {
        key: 'demo.nested_conflict',
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

    expect(
      screen.getAllByRole('img', {
        name: 'Conflicting typed values retained for demo.conflict in span attributes.',
      })
    ).toHaveLength(2)
    expect(screen.getAllByRole('img')).toHaveLength(2)

    await userEvent.click(screen.getByText('{…}'))

    expect(screen.getAllByRole('img')).toHaveLength(2)
    expect(screen.queryByRole('img', { name: /clear/ })).toBeNull()
  })

  it('keeps warning badges and gold guides on affected containers without nested markers', async () => {
    const { container } = renderAttributes([
      {
        key: 'payload',
        value: {
          kind: 'map',
          conflictingKeys: ['same'],
          value: [
            { key: 'same', value: { kind: 'int64', value: 1n } },
            { key: 'clear', value: { kind: 'bool', value: true } },
          ],
        },
      },
    ])

    const badge = screen.getByLabelText(
      '2 entries; contains conflicting typed values'
    )
    expect(badge).toHaveClass('badge-warning')

    await userEvent.click(screen.getByText('{…}'))

    expect(screen.queryByRole('img')).toBeNull()
    expect(
      container.querySelectorAll('.attribute-value__child--conflicted')
    ).toHaveLength(1)
  })
})
