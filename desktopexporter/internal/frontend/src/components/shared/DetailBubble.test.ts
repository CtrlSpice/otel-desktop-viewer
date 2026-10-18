// @vitest-environment jsdom
import { createRawSnippet } from 'svelte'
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import { describe, expect, it } from 'vitest'
import DetailBubble from './DetailBubble.svelte'

describe('stacked detail bubbles', () => {
  it('collapses independently while retaining its contents', async () => {
    const children = createRawSnippet(() => ({
      render: () => '<input aria-label="Retained value" value="draft" />',
    }))
    const first = render(DetailBubble, {
      label: 'Fields',
      children,
    })
    const second = render(DetailBubble, {
      label: 'Activity',
      children,
    })
    const fields = first.container.querySelector('details')!
    const activity = second.container.querySelector('details')!
    expect(fields.open).toBe(true)
    expect(activity.open).toBe(true)
    const input = first.container.querySelector('input')!
    input.value = 'kept'

    await fireEvent.click(screen.getByText('Fields', { exact: true }))
    await waitFor(() => expect(fields.open).toBe(false))
    expect(activity.open).toBe(true)
    expect(input.isConnected).toBe(true)

    await fireEvent.click(screen.getByText('Fields', { exact: true }))
    await waitFor(() => expect(fields.open).toBe(true))
    expect(first.container.querySelector('input')).toBe(input)
    expect(input.value).toBe('kept')
  })
})
