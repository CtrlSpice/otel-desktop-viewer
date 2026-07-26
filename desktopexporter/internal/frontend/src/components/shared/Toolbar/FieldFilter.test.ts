// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, fireEvent } from '@testing-library/svelte'
import FieldFilter from './FieldFilter.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { TRACE_COLUMN_DEFAULTS } from '@/constants/fields'

vi.mock('@/services/telemetry-service', () => ({
  telemetryAPI: {
    getTraceAttributes: vi.fn().mockResolvedValue([]),
    getLogAttributes: vi.fn().mockResolvedValue([]),
    getMetricAttributes: vi.fn().mockResolvedValue([]),
  },
}))

function renderComponent(props = {}) {
  setTestUrl('/traces')
  return renderWithContexts(FieldFilter, {
    signal: 'traces',
    selectedFields: [],
    onToggleField: vi.fn(),
    label: 'Columns',
    columnVisibility: TRACE_COLUMN_DEFAULTS,
    ...props,
  })
}

describe('FieldFilter', () => {
  it('renders the trigger button with an accessible label', () => {
    renderComponent()
    expect(
      screen.getByRole('button', { name: 'Columns: filter columns' })
    ).toBeInTheDocument()
  })

  it('shows the active selection count when fields are selected', () => {
    renderComponent({
      selectedFields: [TRACE_COLUMN_DEFAULTS[0]],
    })
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('opens the popover and renders static field options', () => {
    renderComponent()
    fireEvent.click(
      screen.getByRole('button', { name: 'Columns: filter columns' })
    )
    expect(
      screen.getByRole('button', { name: 'Pinned column traceID' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'name' })).toBeInTheDocument()
  })

  it('calls onToggleField when a selectable field is clicked', () => {
    const onToggleField = vi.fn()
    renderComponent({ onToggleField })
    fireEvent.click(
      screen.getByRole('button', { name: 'Columns: filter columns' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'name' }))
    expect(onToggleField).toHaveBeenCalledOnce()
  })

  it('disables pinned columns so they cannot be toggled', () => {
    renderComponent()
    fireEvent.click(
      screen.getByRole('button', { name: 'Columns: filter columns' })
    )
    const pinned = screen.getByRole('button', {
      name: 'Pinned column traceID',
    })
    expect(pinned).toBeDisabled()
    expect(pinned).toContainHTML('rect')
  })
})
