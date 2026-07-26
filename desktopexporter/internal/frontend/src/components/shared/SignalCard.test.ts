// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { createRawSnippet } from 'svelte'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SignalCard from './SignalCard.svelte'

const leadContent = createRawSnippet(() => ({
  render: () => '<p data-testid="lead-content">custom lead</p>',
}))

describe('SignalCard', () => {
  it('renders the idLine footer text', () => {
    render(SignalCard, {
      props: { id: 'row-1', title: 'Row title', idLine: 'row-1-id-line' },
    })
    expect(screen.getByText('row-1-id-line')).toBeInTheDocument()
  })

  it('renders the lead snippet when no description is provided', () => {
    render(SignalCard, {
      props: { id: 'row-1', title: 'Row title', lead: leadContent },
    })
    expect(screen.getByTestId('lead-content')).toHaveTextContent('custom lead')
  })

  it('prefers the description over the lead snippet when both are given', () => {
    render(SignalCard, {
      props: {
        id: 'row-1',
        title: 'Row title',
        description: 'the description',
        lead: leadContent,
      },
    })
    expect(screen.getByText('the description')).toBeInTheDocument()
    expect(screen.queryByTestId('lead-content')).not.toBeInTheDocument()
  })

  it('renders timestamp and duration without labels in the plain time layout', () => {
    render(SignalCard, {
      props: {
        id: 'row-1',
        title: 'Row title',
        timeLayout: 'plain',
        timestamp: '12:00:00',
        duration: '5 ms',
      },
    })
    expect(screen.getByText('12:00:00')).toBeInTheDocument()
    expect(screen.getByText('5 ms')).toBeInTheDocument()
    expect(screen.queryByText('Start:')).not.toBeInTheDocument()
    expect(screen.queryByText('Duration:')).not.toBeInTheDocument()
  })

  it('reflects the selected prop via aria-pressed', () => {
    render(SignalCard, {
      props: { id: 'row-1', title: 'Row title', selected: true },
    })
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('calls onclick with the id when clicked', async () => {
    const onclick = vi.fn()
    render(SignalCard, {
      props: { id: 'row-99', title: 'Row title', onclick },
    })
    await userEvent.click(screen.getByRole('button'))
    expect(onclick).toHaveBeenCalledWith('row-99')
  })
})
