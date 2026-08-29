// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import PaneHeader, { paneTabID, type PaneTab } from './PaneHeader.svelte'

const tabs: PaneTab[] = [
  { id: 'fields', label: 'Fields' },
  { id: 'events', label: 'Events', disabled: true },
  { id: 'links', label: 'Links' },
]

function renderTabs(onSelect = vi.fn()) {
  render(PaneHeader, {
    props: {
      mode: 'tabs',
      tabs,
      activeID: 'fields',
      onSelect,
      ariaLabel: 'Detail views',
      tabPanelID: 'detail-panel',
    },
  })
  return onSelect
}

describe('PaneHeader local tabs', () => {
  it('associates every tab with its panel and exposes only the active tab in the tab order', () => {
    renderTabs()
    const fields = screen.getByRole('tab', { name: 'Fields' })
    const links = screen.getByRole('tab', { name: 'Links' })

    expect(fields).toHaveAttribute('id', paneTabID('detail-panel', 'fields'))
    expect(fields).toHaveAttribute('aria-controls', 'detail-panel')
    expect(fields).toHaveAttribute('tabindex', '0')
    expect(links).toHaveAttribute('aria-controls', 'detail-panel')
    expect(links).toHaveAttribute('tabindex', '-1')
  })

  it('moves and activates with arrow keys, wrapping and skipping disabled tabs', async () => {
    const onSelect = renderTabs()
    const user = userEvent.setup()
    const fields = screen.getByRole('tab', { name: 'Fields' })
    const links = screen.getByRole('tab', { name: 'Links' })

    fields.focus()
    await user.keyboard('{ArrowRight}')
    expect(links).toHaveFocus()
    expect(onSelect).toHaveBeenLastCalledWith('links', expect.any(MouseEvent))

    await user.keyboard('{ArrowRight}')
    expect(fields).toHaveFocus()
    expect(onSelect).toHaveBeenLastCalledWith('fields', expect.any(MouseEvent))
  })

  it('supports Home and End within the local tablist', async () => {
    const onSelect = renderTabs()
    const user = userEvent.setup()
    const fields = screen.getByRole('tab', { name: 'Fields' })
    const links = screen.getByRole('tab', { name: 'Links' })

    fields.focus()
    await user.keyboard('{End}')
    expect(links).toHaveFocus()
    await user.keyboard('{Home}')
    expect(fields).toHaveFocus()
    expect(onSelect).toHaveBeenCalledTimes(2)
  })
})
