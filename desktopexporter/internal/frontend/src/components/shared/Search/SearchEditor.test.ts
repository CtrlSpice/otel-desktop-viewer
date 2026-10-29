// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import { EditorView } from '@codemirror/view'
import SearchEditor from './SearchEditor.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { telemetryAPI } from '@/services/telemetry-service'
import type { FieldDefinition } from '@/search/model'

afterEach(() => vi.restoreAllMocks())

it('does not discover fields on mount and loads them on search focus', async () => {
  const fields = vi
    .spyOn(telemetryAPI, 'getTraceAttributeDefinitions')
    .mockResolvedValue([])
  setTestUrl('/traces')
  renderWithContexts(SearchEditor, {
    signal: 'traces',
    sortValue: 'startTime',
    sortDirection: 'desc',
    onSearchRequest: vi.fn(),
  })
  await new Promise(resolve => setTimeout(resolve, 300))
  expect(fields).not.toHaveBeenCalled()
  await fireEvent.focus(screen.getByRole('textbox', { name: 'Search traces' }))
  await waitFor(() => expect(fields).toHaveBeenCalledTimes(1))
})

it('loads restored-query definitions immediately and does not submit a stale edited query', async () => {
  let resolve!: (fields: FieldDefinition[]) => void
  const fields = vi
    .spyOn(telemetryAPI, 'getTraceAttributeDefinitions')
    .mockImplementation(
      () =>
        new Promise(done => {
          resolve = done
        })
    )
  const submit = vi.fn()
  setTestUrl('/traces?q=' + encodeURIComponent('name = "restored"'))
  const result = renderWithContexts(SearchEditor, {
    signal: 'traces',
    sortValue: 'startTime',
    sortDirection: 'desc',
    onSearchRequest: submit,
  })
  await waitFor(() => expect(fields).toHaveBeenCalledTimes(1))
  expect(submit).not.toHaveBeenCalled()
  const editor = EditorView.findFromDOM(
    result.container.querySelector('.cm-editor')!
  )!
  editor.dispatch({
    changes: {
      from: 0,
      to: editor.state.doc.length,
      insert: 'name = "edited"',
    },
  })
  resolve([])
  await new Promise(resolve => setTimeout(resolve, 0))
  expect(submit).not.toHaveBeenCalled()
})

it('submits an untouched restored query after its definitions arrive', async () => {
  let resolve!: (fields: FieldDefinition[]) => void
  vi.spyOn(telemetryAPI, 'getTraceAttributeDefinitions').mockImplementation(
    () =>
      new Promise(done => {
        resolve = done
      })
  )
  const submit = vi.fn()
  setTestUrl(
    '/traces?q=' +
      encodeURIComponent(
        'span.attributes["attempt"]::BIGINT = 9007199254740993'
      )
  )
  renderWithContexts(SearchEditor, {
    signal: 'traces',
    sortValue: 'startTime',
    sortDirection: 'desc',
    onSearchRequest: submit,
  })
  await waitFor(() => expect(resolve).toBeDefined())
  expect(submit).not.toHaveBeenCalled()
  resolve([])
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1))
  expect(JSON.stringify(submit.mock.calls[0])).toContain('9007199254740993')
})
