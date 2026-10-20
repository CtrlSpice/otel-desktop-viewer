// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/svelte'
import WaterfallTreeGutter from '@/components/shared/TreeGutter.svelte'

describe('WaterfallTreeGutter dots', () => {
  it('renders a leaf without a caret or number', () => {
    const { container } = render(WaterfallTreeGutter, {
      depth: 0,
      tree: { childrenCount: 0, ancestorHasNextSibling: [], isLastChild: false },
      color: 'var(--color-primary)',
      subtreeCollapsed: false,
    })
    expect(container.querySelector('.gutter__dot--leaf')).toBeEmptyDOMElement()
    expect(container.querySelector('button')).toBeNull()
    expect(container.querySelector('.gutter')).toHaveAttribute('aria-hidden', 'true')
    expect(container.querySelector('svg')).toBeNull()
  })

  it('keeps parent connectors decorative and removes the stem when collapsed', async () => {
    const { container, rerender } = render(WaterfallTreeGutter, {
      depth: 2,
      tree: {
        childrenCount: 2,
        ancestorHasNextSibling: [true, true],
        isLastChild: false,
      },
      color: 'var(--color-primary)',
      subtreeCollapsed: false,
    })
    expect(container.querySelector('.gutter__dot')).toBeEmptyDOMElement()
    expect(container.querySelector('svg')).toBeNull()
    expect(container.querySelector('button')).toBeNull()
    expect(container.querySelector('.gutter__stem')).not.toBeNull()
    await rerender({ subtreeCollapsed: true })
    expect(container.querySelector('.gutter__stem')).toBeNull()
  })
})
