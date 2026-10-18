// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { copyText } from './clipboard'

afterEach(() => vi.restoreAllMocks())

function clipboard(writeText: ReturnType<typeof vi.fn>) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
}

function copyCommand(result: boolean) {
  const command = vi.fn(() => result)
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: command,
  })
  return command
}

describe('clipboard copying', () => {
  it('uses the asynchronous clipboard API when allowed', async () => {
    const write = vi.fn().mockResolvedValue(undefined)
    clipboard(write)
    const command = copyCommand(true)
    await copyText('exact value')
    expect(write).toHaveBeenCalledWith('exact value')
    expect(command).not.toHaveBeenCalled()
  })

  it('falls back after denial and preserves focus and text selection', async () => {
    clipboard(
      vi.fn().mockRejectedValue(new DOMException('Denied', 'NotAllowedError'))
    )
    const input = document.createElement('button')
    const text = document.createElement('p')
    text.textContent = 'keep this selection'
    document.body.append(input, text)
    input.focus()
    const range = document.createRange()
    range.selectNodeContents(text)
    window.getSelection()!.removeAllRanges()
    window.getSelection()!.addRange(range)
    expect(window.getSelection()!.toString()).toBe('keep this selection')
    const command = copyCommand(true).mockImplementation(() => {
      expect(document.activeElement).toBeInstanceOf(HTMLTextAreaElement)
      expect((document.activeElement as HTMLTextAreaElement).value).toBe(
        '9223372036854775807\nsecond line'
      )
      return true
    })
    await copyText('9223372036854775807\nsecond line')
    expect(command).toHaveBeenCalledWith('copy')
    expect(document.activeElement).toBe(input)
    expect(window.getSelection()!.toString()).toBe('keep this selection')
    expect(document.querySelector('textarea')).toBeNull()
    input.remove()
    text.remove()
  })

  it('also falls back when the asynchronous API is absent', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    })
    const command = copyCommand(true)
    await copyText('value')
    expect(command).toHaveBeenCalledWith('copy')
  })

  it('reports failure if the browser also rejects the fallback and cleans up', async () => {
    clipboard(vi.fn().mockRejectedValue(new Error('denied')))
    copyCommand(false)
    await expect(copyText('value')).rejects.toThrow('Clipboard copy was denied')
    expect(document.querySelector('textarea')).toBeNull()
  })
})
