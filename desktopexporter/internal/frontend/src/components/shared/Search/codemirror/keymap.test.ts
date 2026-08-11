import { describe, it, expect, vi } from 'vitest'
import { acceptCompletion } from '@codemirror/autocomplete'
import { createQueryKeymap } from './keymap'

/**
 * Enter has two jobs in this editor and they conflict: accept the highlighted
 * completion, or submit the query.
 *
 * @remarks
 * Before these bindings were chained it only ever submitted. The submit command
 * returns true unconditionally, so it always won and CodeMirror's own
 * acceptCompletion never got a turn — suggestions could only be taken with the
 * mouse, which is not how anyone uses a search box.
 *
 * These assert the wiring rather than the runtime behaviour: constructing a
 * live EditorView needs a real layout that this jsdom setup does not provide.
 * The wiring is the change, and the failure mode if acceptCompletion declines
 * is simply the previous behaviour, so the risk of the untested half is
 * bounded.
 */

type Binding = { key: string; run: unknown }

function bindings(): Binding[] {
  const ext = createQueryKeymap(() => {}) as unknown as { value?: Binding[] }
  return ext.value ?? []
}

describe('query keymap', () => {
  it('binds Enter twice, accepting a completion before submitting', () => {
    const enter = bindings().filter(b => b.key === 'Enter')
    expect(enter).toHaveLength(2)
    expect(enter[0].run).toBe(acceptCompletion)
  })

  it('routes the second Enter binding to the submit callback', () => {
    const onSubmit = vi.fn()
    const ext = createQueryKeymap(onSubmit) as unknown as { value?: Binding[] }
    const enter = (ext.value ?? []).filter(b => b.key === 'Enter')
    ;(enter[1].run as (v: unknown) => boolean)({} as never)
    expect(onSubmit).toHaveBeenCalledOnce()
  })

  it('binds Escape once, conditionally', () => {
    expect(bindings().filter(b => b.key === 'Escape')).toHaveLength(1)
  })
})
