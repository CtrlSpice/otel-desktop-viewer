// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import { EditorView } from '@codemirror/view'
import { foldedRanges } from '@codemirror/language'
import { jsonEditorView } from '@/test/json-editor-helpers'
import JsonEditor from './JsonEditor.svelte'

const text =
  '{\n  "same": 9223372036854775807,\n  "same": -0,\n  "items": [\n    "line\\n  content",\n    false\n  ]\n}'

describe('JsonEditor', () => {
  it('highlights matching brackets at the cursor without changing the document', async () => {
    const { container } = render(JsonEditor, { text: '{"items": [1, 2]}' })
    const view = jsonEditorView(container)
    view.focus()
    view.dispatch({ selection: { anchor: 1 } })
    await waitFor(() =>
      expect(
        [...container.querySelectorAll('.cm-matchingBracket')].map(
          node => node.textContent
        )
      ).toEqual(['{', '}'])
    )
    view.dispatch({ selection: { anchor: 11 } })
    await waitFor(() =>
      expect(
        [...container.querySelectorAll('.cm-matchingBracket')].map(
          node => node.textContent
        )
      ).toEqual(['[', ']'])
    )
    expect(view.state.doc.toString()).toBe('{"items": [1, 2]}')
  })

  it('uses a labelled, read-only CodeMirror document without changing any lexemes', () => {
    const { container } = render(JsonEditor, {
      text,
      label: 'JSON value of body',
    })
    const view = jsonEditorView(container)
    expect(view.state.doc.toString()).toBe(text)
    expect(view.state.readOnly).toBe(true)
    expect(view.state.facet(EditorView.editable)).toBe(false)
    expect(screen.getByLabelText('JSON value of body')).toHaveAttribute(
      'contenteditable',
      'false'
    )
    expect(container.querySelector('.cm-lineNumbers')).toBeNull()
    expect(container.querySelector('.json-indent-bands')).toBeNull()
    expect(container.querySelector('.cm-foldGutter')).not.toBeNull()
  })

  it('uses CodeMirror’s ellipsis placeholders and leaves the full document intact', () => {
    const result = render(JsonEditor, { text })
    const view = jsonEditorView(result.container)
    result.component.collapseAll()
    expect(foldedRanges(view.state).size).toBeGreaterThan(0)
    expect(
      result.container.querySelector('.cm-foldPlaceholder')
    ).toHaveTextContent('…')
    expect(view.state.doc.toString()).toBe(text)
    result.component.expandAll()
    expect(foldedRanges(view.state).size).toBe(0)
    expect(result.container.querySelector('.cm-foldPlaceholder')).toBeNull()
    expect(view.state.doc.toString()).toBe(text)
  })

  it('keeps independently mounted editors independent and resets for a new received value', async () => {
    const first = render(JsonEditor, { text, label: 'First JSON' })
    const second = render(JsonEditor, {
      text: '[\n  1\n]',
      label: 'Second JSON',
    })
    first.component.collapseAll()
    expect(foldedRanges(jsonEditorView(second.container).state).size).toBe(0)
    await first.rerender({ text: '{\n  "next": 9007199254740993\n}' })
    expect(jsonEditorView(first.container).state.doc.toString()).toBe(
      '{\n  "next": 9007199254740993\n}'
    )
    expect(foldedRanges(jsonEditorView(first.container).state).size).toBe(0)
  })
})
