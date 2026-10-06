import { keymap } from '@codemirror/view'
import type { Command } from '@codemirror/view'
import { acceptCompletion, completionStatus } from '@codemirror/autocomplete'

export function createQueryKeymap(onSubmit: () => void) {
  const submitCommand: Command = () => {
    onSubmit()
    return true
  }

  const blurCommand: Command = view => {
    view.contentDOM.blur()
    return true
  }

  return keymap.of([
    // acceptCompletion returns false when no completion is open, allowing submit.
    { key: 'Enter', run: acceptCompletion },
    { key: 'Enter', run: submitCommand },

    // Dismissing a completion must not also blur the editor.
    {
      key: 'Escape',
      run: view =>
        completionStatus(view.state) !== null ? false : blurCommand(view),
    },
  ])
}
