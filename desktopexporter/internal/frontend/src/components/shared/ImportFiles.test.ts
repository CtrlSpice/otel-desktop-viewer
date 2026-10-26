// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ImportFiles from './ImportFiles.svelte'

describe('ImportFiles', () => {
  it('opens the file picker from its keyboard-accessible button', async () => {
    render(ImportFiles, { onFiles: vi.fn() })
    const input = screen.getByLabelText('Choose telemetry JSON files')
    const open = vi.spyOn(input, 'click')
    const button = screen.getByRole('button', {
      name: /^Import files\./,
    })
    button.focus()
    await userEvent.keyboard('{Enter}')
    expect(open).toHaveBeenCalledOnce()
    expect(button).toHaveFocus()
  })

  it('hands off files immediately and supports choosing the same file again', async () => {
    const onFiles = vi.fn()
    render(ImportFiles, { onFiles })
    const input = screen.getByLabelText('Choose telemetry JSON files')
    const file = new File(['{"resourceMetrics":[]}'], 'metric.json')
    await fireEvent.change(input, { target: { files: [file] } })
    expect(onFiles).toHaveBeenLastCalledWith([file])
    expect(input).toHaveValue('')
    await fireEvent.change(input, { target: { files: [file] } })
    expect(onFiles).toHaveBeenCalledTimes(2)
    await fireEvent.change(input, { target: { files: [] } })
    expect(onFiles).toHaveBeenCalledTimes(2)
  })
})
