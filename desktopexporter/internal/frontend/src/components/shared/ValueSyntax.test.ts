// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ValueSyntax from './ValueSyntax.svelte'
import { compactValueTokens, jsonContainerTokens } from './detail-value'

describe('ValueSyntax', () => {
  it('keeps compact previews non-interactive and their exact text selectable', () => {
    const text = '{message: "ready", count: 9223372036854775807, ok: true}'
    const { container } = render(ValueSyntax, {
      tokens: compactValueTokens(text),
    })
    expect(container.textContent).toBe(text)
    expect(screen.queryByRole('button')).toBeNull()
    expect(container.querySelector('.detail-code--key')).toHaveTextContent(
      'message'
    )
    expect(container.querySelector('.detail-code--number')).toHaveTextContent(
      '9223372036854775807'
    )
  })

  it('folds nested objects and arrays independently with keyboard-accessible controls', async () => {
    const user = userEvent.setup()
    const tokens = jsonContainerTokens(
      '{"items":[{"id":9223372036854775807}],"ready":true}'
    )!
    const { container } = render(ValueSyntax, {
      tokens,
      foldable: true,
      path: 'body',
    })
    const expandedText = tokens.map(token => token.text).join('')
    expect(container.textContent).toBe(expandedText)
    const array = screen.getByRole('button', {
      name: 'Collapse body.items',
    })
    array.focus()
    await user.keyboard('{Enter}')
    expect(array).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('9223372036854775807')).toBeNull()
    expect(container).toHaveTextContent('… 1 item')
    await user.keyboard(' ')
    expect(array).toHaveAttribute('aria-expanded', 'true')
    expect(container.textContent).toBe(expandedText)
    await user.click(
      screen.getByRole('button', {
        name: 'Collapse body.items[0]',
      })
    )
    expect(container).toHaveTextContent('… 1 field')
    await user.click(screen.getByRole('button', { name: 'Collapse body' }))
    expect(container).toHaveTextContent('{… 2 fields}')
    await user.click(screen.getByRole('button', { name: 'Expand body' }))
    expect(
      screen.getByRole('button', { name: 'Expand body.items[0]' })
    ).toHaveAttribute('aria-expanded', 'false')
  })

  it('does not add fold controls to empty containers', () => {
    const { container } = render(ValueSyntax, {
      tokens: jsonContainerTokens('[{},[]]')!,
      foldable: true,
    })
    expect(screen.getAllByRole('button')).toHaveLength(1)
    expect(container.textContent).toBe('[\n  {},\n  []\n]')
  })

  it('resets folding when the selected received value changes', async () => {
    const user = userEvent.setup()
    const view = render(ValueSyntax, {
      tokens: jsonContainerTokens('{"a":[1]}')!,
      foldable: true,
      path: 'body',
    })
    await user.click(screen.getByRole('button', { name: 'Collapse body' }))
    await view.rerender({ tokens: jsonContainerTokens('{"b":[2]}')! })
    expect(
      screen.getByRole('button', { name: 'Collapse body' })
    ).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('2')).toBeInTheDocument()
  })
})
