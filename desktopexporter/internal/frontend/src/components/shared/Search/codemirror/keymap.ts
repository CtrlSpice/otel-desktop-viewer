import { keymap } from '@codemirror/view'
import type { Command } from '@codemirror/view'
import { acceptCompletion, completionStatus } from '@codemirror/autocomplete'

export function createQueryKeymap(onSubmit: () => void) {
  const submitCommand: Command = view => {
    onSubmit()
    return true
  }

  const blurCommand: Command = view => {
    view.contentDOM.blur()
    return true
  }

  return keymap.of([
    // Enter accepts an open completion, and only submits when there is none.
    //
    // Without the first binding, Enter always submitted: it returns true
    // unconditionally, so it won every time and CodeMirror's own
    // acceptCompletion never ran. Completions could only be taken with the
    // mouse, which is not how anyone uses a search box -- you type, you see the
    // suggestion, you press Enter. acceptCompletion returns false when no
    // completion is open, and CodeMirror then falls through to the next binding
    // for the same key, so submitting is unaffected.
    { key: 'Enter', run: acceptCompletion },
    { key: 'Enter', run: submitCommand },

    // Escape closes the completion list if one is open, and only blurs
    // otherwise -- same reasoning: dismissing a suggestion should not also
    // throw away focus.
    {
      key: 'Escape',
      run: view => (completionStatus(view.state) !== null ? false : blurCommand(view)),
    },
  ])
}
