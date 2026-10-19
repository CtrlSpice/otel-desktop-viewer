// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DetailViewTabs from './DetailViewTabs.svelte'

describe('DetailViewTabs', () => {
  it('uses the Metrics-style joined icon and text controls without Overlays', async () => {
    render(DetailViewTabs, { id: 'log-view' })
    const fields = screen.getByRole('tab', { name: 'Properties' })
    const raw = screen.getByRole('tab', { name: 'Raw JSON' })
    expect(screen.getByRole('tablist')).toHaveClass('join')
    expect(screen.getAllByRole('tab')).toHaveLength(2)
    expect(fields).toHaveTextContent('Properties')
    expect(raw).toHaveTextContent('Raw JSON')
    expect(fields).toHaveAttribute('aria-selected', 'true')
    expect(fields).toHaveAttribute('aria-controls', 'log-view-fields')
    expect(raw).toHaveAttribute('aria-controls', 'log-view-raw')
    expect(fields).toHaveAttribute('title', 'Properties')
    expect(raw).toHaveAttribute('title', 'Raw JSON')
    for (const tab of [fields, raw]) {
      expect(tab).toHaveClass(
        'btn',
        'btn-xs',
        'join-item',
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
