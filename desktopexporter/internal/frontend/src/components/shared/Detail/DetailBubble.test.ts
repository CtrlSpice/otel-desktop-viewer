// @vitest-environment jsdom
import { createRawSnippet } from 'svelte'
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import { describe, expect, it } from 'vitest'
import DetailBubble from './DetailBubble.svelte'

describe('stacked detail bubbles', () => {
  it('keeps zero-count headings visible without an expandable body or focus target', async () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Empty body</p>',
    }))
    const { container } = render(DetailBubble, {
      label: 'Links',
      count: 0,
      children,
    })
    expect(screen.getByRole('region', { name: 'Links' })).toBeInTheDocument()
    expect(screen.getByLabelText('0 links')).toHaveTextContent('[0]')
    expect(screen.queryByText('Empty body')).toBeNull()
    await fireEvent.click(screen.getByText('Links', { exact: true }))
    expect(
      container.querySelector('details, summary, button, [tabindex]')
    ).toBeNull()
    expect(screen.queryByText('Empty body')).toBeNull()
  })

  it('enables expansion when items arrive and removes it when the count becomes zero', async () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Populated body</p>',
    }))
    const { container, rerender } = render(DetailBubble, {
      label: 'Links',
      count: 0,
      children,
    })
    await rerender({ label: 'Links', count: 2, children })
    expect(container.querySelector('details')!.open).toBe(true)
    expect(screen.getByText('Populated body')).toBeInTheDocument()
    await rerender({ label: 'Links', count: 0, children })
    expect(container.querySelector('details')).toBeNull()
    expect(screen.queryByText('Populated body')).toBeNull()
  })

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
