/** Copy only in response to a user action. Embedded browsers can deny the
 * asynchronous API while still supporting the user-initiated copy command. */
export async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    return
  } catch {
    const active = document.activeElement
    const selection = window.getSelection()
    const ranges = selection
      ? Array.from({ length: selection.rangeCount }, (_, index) =>
          selection.getRangeAt(index).cloneRange()
        )
      : []
    const input = document.createElement('textarea')
    input.value = text
    input.readOnly = true
    input.tabIndex = -1
    input.setAttribute('aria-hidden', 'true')
    input.style.cssText = 'position:fixed;top:0;left:-9999px;opacity:0;'
    let copied = false
    input.addEventListener('copy', event => {
      if (!event.clipboardData) return
      // Textarea values normalize CR/CRLF. Write the received text directly.
      event.clipboardData.setData('text/plain', text)
      event.preventDefault()
      copied = true
    })
    document.body.append(input)
    try {
      input.focus({ preventScroll: true })
      input.select()
      if (!document.execCommand('copy') || !copied)
        throw new Error('Clipboard copy was denied')
    } finally {
      input.remove()
      if (active instanceof HTMLElement && active.isConnected)
        active.focus({ preventScroll: true })
      if (selection) {
        selection.removeAllRanges()
        for (const range of ranges) {
          if (range.commonAncestorContainer.isConnected)
            selection.addRange(range)
        }
      }
    }
  }
}
