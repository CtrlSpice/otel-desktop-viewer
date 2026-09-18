// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/svelte'
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
  it('renders recursive values and preserves duplicate attribute entries', () => {
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

    expect(screen.getByText('attempts')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(container.querySelectorAll('.detail-cell__key')).toHaveLength(2)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'payload in span attributes'
    )
  })

  it('does not warn for exact repeated canonical values', () => {
    renderAttributes([
      { id: 'one', key: 'retry', value: { kind: 'int64', value: 1n } },
      { id: 'two', key: 'retry', value: { kind: 'int64', value: 1n } },
    ])

    expect(screen.queryByRole('alert')).toBeNull()
  })
})
