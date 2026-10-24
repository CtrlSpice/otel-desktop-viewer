// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import ValueSyntax from './ValueSyntax.svelte'
import { compactValueTokens } from './detail-value'

describe('ValueSyntax summaries', () => {
  it('keeps colourful single-line previews non-interactive and exact', () => {
    const text = '{message: "ready", count: 9223372036854775807, ok: true}'
    const { container } = render(ValueSyntax, {
      tokens: compactValueTokens(text),
    })
    expect(container.textContent).toBe(text)
    expect(container.textContent).not.toContain('\n')
    expect(screen.queryByRole('button')).toBeNull()
    expect(container.querySelector('.cm-editor')).toBeNull()
    expect(container.querySelector('.detail-code--key')).toHaveTextContent(
      'message'
    )
    expect(container.querySelector('.detail-code--number')).toHaveTextContent(
      '9223372036854775807'
    )
    expect(container.querySelector('.detail-code--literal')).toHaveTextContent(
      'true'
    )
  })

  it('can leave scalar values uncoloured without changing their text', () => {
    const { container } = render(ValueSyntax, {
      tokens: [{ text: '"9223372036854775807"', kind: 'string' }],
      highlight: false,
    })
    expect(container.querySelector('.value-syntax')).toBeNull()
    expect(container.textContent).toBe('"9223372036854775807"')
  })

  it('updates a summary when its received value changes', async () => {
    const view = render(ValueSyntax, { tokens: compactValueTokens('[1, -0]') })
    await view.rerender({
      tokens: compactValueTokens('[9007199254740993, false]'),
    })
    expect(view.container.textContent).toBe('[9007199254740993, false]')
  })
})
