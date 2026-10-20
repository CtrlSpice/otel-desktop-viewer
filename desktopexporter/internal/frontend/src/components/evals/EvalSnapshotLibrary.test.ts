// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalSnapshotLibrary from './EvalSnapshotLibrary.svelte'
import {
  telemetryAPI,
  type EvalSnapshotMetadata,
} from '@/services/telemetry-service'

const metadata: EvalSnapshotMetadata = {
  id: 'a'.repeat(64),
  digest: 'a'.repeat(64),
  name: 'saved.json',
  format: 'unknown-json',
  source: 'local-file',
  importedAt: '2026-10-20T00:00:00Z',
  byteSize: 2,
}
const callbacks = () => ({
  onOpen: vi.fn(),
  onDelete: vi.fn(),
  onSynthetic: vi.fn(),
})
afterEach(() => vi.restoreAllMocks())

describe('snapshot library request lifecycle', () => {
  it('rejects invalid UTF-8 instead of archiving replacement characters', async () => {
    const user = userEvent.setup()
    vi.spyOn(telemetryAPI, 'searchEvalSnapshots').mockResolvedValue({
      snapshots: [],
      hasMore: false,
    })
    const save = vi.spyOn(telemetryAPI, 'importEvalSnapshot')
    render(EvalSnapshotLibrary, callbacks())
    await user.click(screen.getByRole('button', { name: 'Snapshot library' }))
    const file = new File([], 'invalid.json', { type: 'application/json' })
    Object.defineProperty(file, 'arrayBuffer', {
      value: async () => new Uint8Array([0x22, 0xff, 0x22]).buffer,
    })
    await user.upload(screen.getByLabelText('JSON file · up to 16 MiB'), file)
    await user.selectOptions(
      screen.getByLabelText('Source format'),
      'unknown-json'
    )
    await fireEvent.submit(
      screen.getByRole('form', { name: 'Import eval snapshot' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      /encoded data|encoding/i
    )
    expect(save).not.toHaveBeenCalled()
  })

  it('cancels an import and ignores its late success', async () => {
    const user = userEvent.setup()
    const list = vi
      .spyOn(telemetryAPI, 'searchEvalSnapshots')
      .mockResolvedValue({ snapshots: [], hasMore: false })
    let finish!: (value: {
      snapshot: EvalSnapshotMetadata
      created: boolean
    }) => void
    const save = vi
      .spyOn(telemetryAPI, 'importEvalSnapshot')
      .mockImplementation(
        () =>
          new Promise(resolve => {
            finish = resolve
          })
      )
    render(EvalSnapshotLibrary, callbacks())
    await user.click(screen.getByRole('button', { name: 'Snapshot library' }))
    const file = new File(['{}'], 'saved.json', { type: 'application/json' })
    Object.defineProperty(file, 'arrayBuffer', {
      value: async () => new TextEncoder().encode('{}').buffer,
    })
    await user.upload(screen.getByLabelText('JSON file · up to 16 MiB'), file)
    await user.selectOptions(
      screen.getByLabelText('Source format'),
      'unknown-json'
    )
    await fireEvent.submit(
      screen.getByRole('form', { name: 'Import eval snapshot' })
    )
    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    await user.click(screen.getByRole('button', { name: 'Cancel request' }))
    expect(save.mock.calls[0][4]?.aborted).toBe(true)
    finish({ snapshot: metadata, created: true })
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Request canceled.')
    )
    expect(list).toHaveBeenCalledOnce()
    expect(screen.queryByText(/Archived saved.json/)).not.toBeInTheDocument()
  })

  it('does not discover or select an archive until requested, and pages metadata only', async () => {
    const user = userEvent.setup()
    const list = vi
      .spyOn(telemetryAPI, 'searchEvalSnapshots')
      .mockResolvedValue({ snapshots: [metadata], hasMore: true })
    const props = callbacks()
    render(EvalSnapshotLibrary, props)
    expect(list).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Snapshot library' }))
    await screen.findByRole('button', { name: 'Open snapshot saved.json' })
    expect(props.onOpen).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Next page' }))
    expect(list).toHaveBeenLastCalledWith(25, 25, expect.any(AbortSignal))
    await user.click(
      screen.getByRole('button', { name: 'Open snapshot saved.json' })
    )
    expect(props.onOpen).toHaveBeenCalledWith(metadata.id)
    expect(
      screen.getByRole('link', {
        name: 'Download original JSON for saved.json',
      })
    ).toHaveAttribute('href', `/api/evals/snapshots/${metadata.id}/download`)
  })

  it('aborts a hidden list and ignores a late response after reopening', async () => {
    const user = userEvent.setup()
    let finish!: (value: {
      snapshots: EvalSnapshotMetadata[]
      hasMore: boolean
    }) => void
    const list = vi
      .spyOn(telemetryAPI, 'searchEvalSnapshots')
      .mockImplementationOnce(
        () =>
          new Promise(resolve => {
            finish = resolve
          })
      )
      .mockResolvedValue({ snapshots: [], hasMore: false })
    render(EvalSnapshotLibrary, callbacks())
    const toggle = screen.getByRole('button', { name: 'Snapshot library' })
    await user.click(toggle)
    const signal = list.mock.calls[0][2]!
    await user.click(toggle)
    expect(signal.aborted).toBe(true)
    await user.click(toggle)
    await screen.findByText('No snapshots on this page.')
    finish({ snapshots: [metadata], hasMore: true })
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Open snapshot saved.json' })
      ).not.toBeInTheDocument()
    )
  })

  it('imports the file text only after explicit format selection and leaves selection unchanged', async () => {
    const user = userEvent.setup()
    vi.spyOn(telemetryAPI, 'searchEvalSnapshots').mockResolvedValue({
      snapshots: [],
      hasMore: false,
    })
    const save = vi
      .spyOn(telemetryAPI, 'importEvalSnapshot')
      .mockResolvedValue({ snapshot: metadata, created: true })
    const props = callbacks()
    render(EvalSnapshotLibrary, props)
    await user.click(screen.getByRole('button', { name: 'Snapshot library' }))
    const original = ' {"n":1.00,"n":-0}\n'
    const file = new File([original], 'saved.json', {
      type: 'application/json',
    })
    Object.defineProperty(file, 'arrayBuffer', {
      value: async () => new TextEncoder().encode(original).buffer,
    })
    await user.upload(screen.getByLabelText('JSON file · up to 16 MiB'), file)
    expect(
      screen.getByRole('button', { name: 'Import snapshot' })
    ).toBeDisabled()
    await user.selectOptions(
      screen.getByLabelText('Source format'),
      'unknown-json'
    )
    // jsdom's native required-file validation does not see user-event's FileList.
    await fireEvent.submit(
      screen.getByRole('form', { name: 'Import eval snapshot' })
    )
    await screen.findByText('Archived saved.json. Open it from the library.')
    expect(save).toHaveBeenCalledWith(
      'local-file',
      'unknown-json',
      'saved.json',
      original,
      expect.any(AbortSignal)
    )
    expect(props.onOpen).not.toHaveBeenCalled()
  })

  it('shows import errors and reports successful deletions for selection cleanup', async () => {
    const user = userEvent.setup()
    vi.spyOn(telemetryAPI, 'searchEvalSnapshots').mockResolvedValue({
      snapshots: [metadata],
      hasMore: false,
    })
    vi.spyOn(telemetryAPI, 'importEvalSnapshot').mockRejectedValue(
      new Error('Invalid JSON')
    )
    vi.spyOn(telemetryAPI, 'deleteEvalSnapshot').mockResolvedValue({
      id: metadata.id,
      deleted: true,
    })
    const props = callbacks()
    render(EvalSnapshotLibrary, props)
    await user.click(screen.getByRole('button', { name: 'Snapshot library' }))
    const file = new File(['{'], 'saved.json', { type: 'application/json' })
    Object.defineProperty(file, 'arrayBuffer', {
      value: async () => new TextEncoder().encode('{').buffer,
    })
    await user.upload(screen.getByLabelText('JSON file · up to 16 MiB'), file)
    await user.selectOptions(
      screen.getByLabelText('Source format'),
      'unknown-json'
    )
    await fireEvent.submit(
      screen.getByRole('form', { name: 'Import eval snapshot' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid JSON')
    await user.click(
      screen.getByRole('button', { name: 'Delete snapshot saved.json' })
    )
    await waitFor(() =>
      expect(props.onDelete).toHaveBeenCalledWith(metadata.id)
    )
  })
})
