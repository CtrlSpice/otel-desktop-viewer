// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import FileDropOverlay from './FileDropOverlay.svelte'

const fileTransfer = (files: File[] = []) => ({
  types: ['Files'],
  files,
  dropEffect: 'none',
})

describe('FileDropOverlay', () => {
  it('appears only for files and remains visible across nested drag targets', async () => {
    const onFiles = vi.fn()
    render(FileDropOverlay, { onFiles })
    await fireEvent.dragEnter(window, {
      dataTransfer: { types: ['text/plain'], files: [] },
    })
    expect(screen.queryByRole('status')).toBeNull()
    await fireEvent.dragEnter(window, { dataTransfer: fileTransfer() })
    expect(screen.getByRole('status')).toHaveTextContent(
      'Drop OTLP JSON files to import'
    )
    await fireEvent.dragEnter(document.body, { dataTransfer: fileTransfer() })
    await fireEvent.dragLeave(document.body, { dataTransfer: fileTransfer() })
    expect(screen.getByRole('status')).toBeInTheDocument()
    await fireEvent.dragLeave(window, { dataTransfer: fileTransfer() })
    expect(screen.queryByRole('status')).toBeNull()
    expect(onFiles).not.toHaveBeenCalled()
  })

  it('prevents browser navigation and hands off the original files immediately', async () => {
    const onFiles = vi.fn()
    render(FileDropOverlay, { onFiles })
    const files = [
      new File(['{"resourceSpans":[]}'], 'trace.json'),
      new File(['{"resourceLogs":[]}\n'], 'logs.jsonl'),
    ]
    const transfer = fileTransfer(files)
    await fireEvent.dragEnter(window, { dataTransfer: transfer })
    const over = new Event('dragover', { cancelable: true })
    Object.defineProperty(over, 'dataTransfer', { value: transfer })
    await fireEvent(window, over)
    expect(over.defaultPrevented).toBe(true)
    expect(transfer.dropEffect).toBe('copy')
    const drop = new Event('drop', { cancelable: true })
    Object.defineProperty(drop, 'dataTransfer', { value: transfer })
    await fireEvent(window, drop)
    expect(drop.defaultPrevented).toBe(true)
    expect(onFiles).toHaveBeenCalledOnce()
    expect(onFiles.mock.calls[0]![0][0]).toBe(files[0])
    expect(onFiles.mock.calls[0]![0][1]).toBe(files[1])
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('clears on window blur and removes its listeners on unmount', async () => {
    const onFiles = vi.fn()
    const view = render(FileDropOverlay, { onFiles })
    await fireEvent.dragEnter(window, { dataTransfer: fileTransfer() })
    await fireEvent.blur(window)
    expect(screen.queryByRole('status')).toBeNull()
    view.unmount()
    await fireEvent.drop(window, {
      dataTransfer: fileTransfer([new File(['{}'], 'file.json')]),
    })
    expect(onFiles).not.toHaveBeenCalled()
  })
})
