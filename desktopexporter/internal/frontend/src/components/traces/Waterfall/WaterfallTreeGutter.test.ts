// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render } from '@testing-library/svelte'
import WaterfallTreeGutter from './WaterfallTreeGutter.svelte'

describe('WaterfallTreeGutter dots', () => {
  it('renders a leaf without a caret or number', () => {
    const { container } = render(WaterfallTreeGutter, {
      depth: 0,
      tree: { childrenCount: 0, ancestorHasNextSibling: [], isLastChild: false },
      color: 'var(--color-primary)',
      subtreeCollapsed: false,
      onToggleExpand: vi.fn(),
    })
    expect(container.querySelector('.gutter__btn--leaf')).toBeEmptyDOMElement()
    expect(container.querySelector('svg')).toBeNull()
  })

  it('retains parent activation and accessible child counts without a caret or dot text', async () => {
    const onToggleExpand = vi.fn()
    const { container, getByRole } = render(WaterfallTreeGutter, {
      depth: 2,
      tree: {
        childrenCount: 2,
        ancestorHasNextSibling: [true, true],
        isLastChild: false,
      },
      color: 'var(--color-primary)',
      subtreeCollapsed: false,
      onToggleExpand,
    })
    expect(container.querySelector('.gutter__btn-dot')).toBeEmptyDOMElement()
    expect(container.querySelector('svg')).toBeNull()
    await fireEvent.click(getByRole('button', { name: 'Collapse 2 child spans' }))
    expect(onToggleExpand).toHaveBeenCalledOnce()
  })
})
