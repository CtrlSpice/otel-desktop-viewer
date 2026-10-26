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
    expect(fields.closest('[role="tablist"]')).not.toHaveClass('tabs-lift')
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

describe('PaneHeader main pill', () => {
  it('keeps counts in the title row beside the title and service', () => {
    const { container } = render(PaneHeader, {
      mode: 'title',
      title: 'Main header',
      subtitle: 'service',
      badges: [{ label: 'Gauge' }],
    })
    const title = container.querySelector('.pane-header__title-row')!
    const badges = container.querySelector('.pane-header__badges')!
    expect(title).toContainElement(badges as HTMLElement)
    expect(badges.parentElement).toBe(title)
  })

  it('keeps graph tabs and trailing badges in the compact header without changing tab semantics', async () => {
    const onSelect = vi.fn()
    const { container } = render(PaneHeader, {
      mode: 'title-tabs',
      title: 'Requests',
      tabs,
      activeID: 'fields',
      badges: [{ label: 'Sum' }],
      onSelect,
      tabPanelID: 'graph',
    })
    const top = container.querySelector('.pane-header__top')!
    expect(
      top.querySelector('.pane-header__title-row .pane-header__badges')
    ).toHaveTextContent('Sum')
    const controls = top.nextElementSibling!
    expect(controls).toHaveClass('pane-header__tab-group')
    expect(controls).toContainElement(screen.getByRole('tablist'))
    const active = screen.getByRole('tab', { name: 'Fields' })
    expect(active).toHaveAttribute('aria-controls', 'graph')
    active.focus()
    await userEvent.setup().keyboard('{ArrowRight}')
    expect(screen.getByRole('tab', { name: 'Links' })).toHaveFocus()
    expect(onSelect).toHaveBeenCalledWith('links', expect.any(MouseEvent))
  })

  it('opts into full rounding without changing other header defaults', async () => {
    const view = render(PaneHeader, {
      mode: 'title',
      title: 'Main header',
      pill: true,
    })
    expect(screen.getByRole('region', { name: 'Main header' })).toHaveClass(
      'pane-header--pill'
    )
    expect(screen.getByRole('region', { name: 'Main header' })).not.toHaveClass(
      'pane-header--rounded'
    )
    await view.rerender({ mode: 'title', title: 'Main header', pill: false })
    expect(screen.getByRole('region', { name: 'Main header' })).toHaveClass(
      'pane-header--rounded'
    )
    expect(screen.getByRole('region', { name: 'Main header' })).not.toHaveClass(
      'pane-header--pill'
    )
  })
})
