// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest'
import { acceptCompletion } from '@codemirror/autocomplete'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap, type KeyBinding } from '@codemirror/view'
import { createQueryKeymap } from './keymap'

/** Test the real CodeMirror keymap contract without DOM layout. */

function bindings(onSubmit = () => {}): readonly KeyBinding[] {
  const state = EditorState.create({
    extensions: [createQueryKeymap(onSubmit)],
  })
  return state.facet(keymap).flat()
}

describe('query keymap', () => {
  it('binds Enter twice, accepting a completion before submitting', () => {
    const enter = bindings().filter(b => b.key === 'Enter')
    expect(enter).toHaveLength(2)
    expect(enter[0].run).toBe(acceptCompletion)
  })

  it('routes the second Enter binding to the submit callback', () => {
    const onSubmit = vi.fn()
    const view = new EditorView({
      extensions: [createQueryKeymap(onSubmit)],
    })
    try {
      const enter = view.state
        .facet(keymap)
        .flat()
        .filter(binding => binding.key === 'Enter')
      const submit = enter[1]?.run
      expect(submit).toBeDefined()
      if (!submit) throw new Error('Expected a submit binding')
      expect(submit(view)).toBe(true)
      expect(onSubmit).toHaveBeenCalledOnce()
    } finally {
      view.destroy()
    }
  })

  it('binds Escape once, conditionally', () => {
    expect(bindings().filter(b => b.key === 'Escape')).toHaveLength(1)
  })
})
