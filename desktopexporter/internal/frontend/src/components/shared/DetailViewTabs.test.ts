// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DetailViewTabs from './DetailViewTabs.svelte'

describe('DetailViewTabs', () => {
  it('supports wrapping arrow navigation and Home/End with one tab stop', async () => {
    const user = userEvent.setup()
    render(DetailViewTabs, { id: 'keyboard-view' })
    const fields = screen.getByRole('tab', { name: 'Properties' })
    const raw = screen.getByRole('tab', { name: 'JSON' })
    await user.tab()
    expect(fields).toHaveFocus()
    expect(fields).toHaveAttribute('tabindex', '0')
    expect(raw).toHaveAttribute('tabindex', '-1')
    for (const [key, selected, other] of [
      ['{ArrowRight}', raw, fields],
      ['{ArrowRight}', fields, raw],
      ['{ArrowLeft}', raw, fields],
      ['{Home}', fields, raw],
      ['{End}', raw, fields],
    ] as const) {
      await user.keyboard(key)
      expect(selected).toHaveFocus()
      expect(selected).toHaveAttribute('aria-selected', 'true')
      expect(selected).toHaveAttribute('tabindex', '0')
      expect(other).toHaveAttribute('aria-selected', 'false')
      expect(other).toHaveAttribute('tabindex', '-1')
    }
    await user.tab()
    expect(raw).not.toHaveFocus()
    expect(fields).not.toHaveFocus()
  })

  it('uses compact joined icon and text tabs with left-facing tooltips', async () => {
    render(DetailViewTabs, { id: 'log-view' })
    const fields = screen.getByRole('tab', { name: 'Properties' })
    const raw = screen.getByRole('tab', { name: 'JSON' })
    expect(screen.getByRole('tablist')).toHaveClass('join')
    expect(screen.getAllByRole('tab')).toHaveLength(2)
    expect(fields).toHaveTextContent('Properties')
    expect(raw).toHaveTextContent('JSON')
    expect(fields).toHaveAttribute('aria-selected', 'true')
    expect(fields).toHaveAttribute('aria-controls', 'log-view-fields')
    expect(raw).toHaveAttribute('aria-controls', 'log-view-raw')
    expect(fields).toHaveAttribute('data-tip', 'Properties')
    expect(raw).toHaveAttribute('data-tip', 'JSON')
    for (const tab of [fields, raw]) {
      expect(tab).toHaveClass(
        'btn',
        'btn-xs',
        'join-item',
        'tooltip-left',
        'chart-header-control'
      )
      expect(tab.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
      expect(tab.querySelector('svg')).toHaveAttribute('width', '16')
    }
    await userEvent.click(raw)
    expect(raw).toHaveAttribute('aria-selected', 'true')
    expect(raw).toHaveClass('chart-header-control--active')
    expect(fields).not.toHaveClass('chart-header-control--active')
    expect(fields).toHaveAttribute('aria-selected', 'false')
    await userEvent.click(fields)
    expect(fields).toHaveAttribute('aria-selected', 'true')
  })
})
