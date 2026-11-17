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
  it('opens the first array item once while preserving deeper folds, later items and manual collapse', async () => {
    const source = JSON.stringify(
      [
        { text: 'first output', nested: { detail: 'deep value' } },
        { text: 'later output' },
      ],
      null,
      2
    )
    const result = render(JsonEditor, { text: source, defaultOpenDepth: 1 })
    const view = jsonEditorView(result.container)
    const content = result.container.querySelector('.cm-content')!
    expect(content).toHaveTextContent('first output')
    expect(content).not.toHaveTextContent('deep value')
    expect(content).not.toHaveTextContent('later output')
    result.component.collapseAll()
    await result.rerender({
      text: source,
      defaultOpenDepth: 1,
      label: 'Updated label',
    })
    expect(content).not.toHaveTextContent('first output')
    expect(view.state.doc.toString()).toBe(source)
    await result.rerender({
      text: '[0, {"text":"later output"}]',
      defaultOpenDepth: 1,
    })
    expect(content).not.toHaveTextContent('later output')
  })

  it('opens only the first level when requested, keeping nested objects and arrays folded', () => {
    const source = JSON.stringify(
      { summary: { result: 'recorded' }, events: [1, 2, 3, 4] },
      null,
      2
    )
    const result = render(JsonEditor, { text: source, defaultOpenDepth: 1 })
    const view = jsonEditorView(result.container)
    expect(result.container.querySelector('.cm-content')).not.toHaveTextContent(
      'recorded'
    )
    const ranges: string[] = []
    foldedRanges(view.state).between(0, view.state.doc.length, (from, to) => {
      ranges.push(view.state.doc.sliceString(from, to).replace(/\s/g, ''))
    })
    expect(ranges).toEqual(['"result":"recorded"', '1,2,3,4'])
    result.component.expandAll()
    expect(foldedRanges(view.state).size).toBe(0)
    expect(view.state.doc.toString()).toBe(source)
  })

  it('decodes only value-string presentation and preserves the JSON document', () => {
    const command = `curl -d '{"method":"getTraceView"}'`
    const source = JSON.stringify(
      {
        pattern: command,
        path: String.raw`C:\new\trace.json`,
        lines: 'first\r\nsecond\tvalue',
        unicode: '🚀',
        empty: '',
        quoted: '"quoted"',
      },
      null,
      2
    ).replace('🚀', String.raw`\uD83D\uDE80`)
    const { container } = render(JsonEditor, { text: source })
    const content = container
      .querySelector('.cm-content')!
      .cloneNode(true) as HTMLElement
    content.querySelectorAll('.cm-widgetBuffer').forEach(node => node.remove())
    expect(content.textContent).toContain(`"pattern": ${command},`)
    expect(content.textContent).toContain(
      String.raw`"path": C:\new\trace.json,`
    )
    expect(content.textContent).toContain('first\r\nsecond\tvalue')
    expect(content.textContent).toContain('"unicode": 🚀')
    expect(content.textContent).toContain('"empty": ""')
    expect(content.textContent).toContain('"quoted": "quoted"')
    expect(jsonEditorView(container).state.doc.toString()).toBe(source)
  })

  it('keeps raw JSON string escapes and quotes visible', () => {
    const source = '{"pattern":"curl -d \\\"payload\\\""}'
    const { container } = render(JsonEditor, { text: source, raw: true })
    expect(container.querySelector('.cm-content')!.textContent).toBe(source)
    expect(container.querySelector('.json-decoded-escape')).toBeNull()
  })

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
    const firstView = jsonEditorView(first.container)
    first.component.collapseAll()
    expect(foldedRanges(jsonEditorView(second.container).state).size).toBe(0)
    await first.rerender({ text: '{\n  "next": 9007199254740993\n}' })
    expect(jsonEditorView(first.container)).toBe(firstView)
    expect(jsonEditorView(first.container).state.doc.toString()).toBe(
      '{\n  "next": 9007199254740993\n}'
    )
    expect(foldedRanges(jsonEditorView(first.container).state).size).toBe(0)
  })
})
