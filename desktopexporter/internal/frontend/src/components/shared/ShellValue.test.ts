// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { EditorView } from '@codemirror/view'
import DetailProperty from './DetailProperty.svelte'
import {
  valuePresentation,
  valuePresentationKey,
} from '@/state/value-presentation.svelte'

const key = valuePresentationKey('log', 'attribute', 'command')
afterEach(() => {
  valuePresentation.setShell(key, false)
  vi.restoreAllMocks()
})

it('renders the selected shell presentation and discloses its exact document', async () => {
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  const text = 'printf "%s\\n" "$HOME"\n# keep spaces  \n'
  const result = render(DetailProperty, {
    fieldName: 'command',
    fieldType: 'string',
    fieldKind: 'attribute',
    fieldSource: 'log',
    typedValue: { kind: 'string', value: text },
  })
  expect(result.container.querySelector('.shell-value')).toBeNull()
  await act(() => valuePresentation.setShell(key, true))
  const button = screen.getByRole('button', { name: 'Expand command shell' })
  expect(button).toHaveAttribute('aria-expanded', 'false')
  const preview = result.container.querySelector('.shell-value .truncate')!
  expect(preview.textContent).toBe(text)
  expect(preview.querySelector('[class^="ͼ"]')).not.toBeNull()
  await user.click(button)
  const editor = screen.getByLabelText('Shell value of command')
  const view = EditorView.findFromDOM(editor)!
  expect(view.state.doc.toString()).toBe(text)
  expect(
    screen.getByRole('button', { name: 'Collapse command shell' })
  ).toHaveAttribute('aria-expanded', 'true')
  await user.click(screen.getByRole('button', { name: 'Copy command' }))
  expect(copy).toHaveBeenCalledExactlyOnceWith(text)
  await user.click(
    screen.getByRole('button', { name: 'Collapse command shell' })
  )
  expect(
    screen.getByRole('button', { name: 'Expand command shell' })
  ).toBeVisible()
  await act(() => valuePresentation.setShell(key, false))
  expect(result.container.querySelector('.shell-value')).toBeNull()
})

it('does not apply a shell choice to non-string received values or another source', () => {
  valuePresentation.setShell(key, true)
  const { container } = render(DetailProperty, {
    fieldName: 'command',
    fieldType: 'int64',
    fieldKind: 'attribute',
    fieldSource: 'log',
    typedValue: { kind: 'int64', value: 7n },
  })
  expect(container.querySelector('.shell-value')).toBeNull()
  expect(screen.queryByRole('combobox')).toBeNull()
  expect(
    valuePresentation.isShell(
      valuePresentationKey('resource', 'attribute', 'command')
    )
  ).toBe(false)
})
