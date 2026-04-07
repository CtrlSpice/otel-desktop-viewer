import { keymap } from '@codemirror/view';
import type { Command } from '@codemirror/view';

export function createQueryKeymap(onSubmit: () => void) {
  const submitCommand: Command = (view) => {
    onSubmit();
    return true;
  };

  const blurCommand: Command = (view) => {
    view.contentDOM.blur();
    return true;
  };

  return keymap.of([
    { key: 'Mod-Enter', run: submitCommand },
    { key: 'Escape', run: blurCommand },
  ]);
}
