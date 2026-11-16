// @vitest-environment jsdom
import { render, screen } from '@testing-library/svelte'
import { describe, expect, it } from 'vitest'
import ReadonlyCodePanel from './ReadonlyCodePanel.svelte'

describe('ReadonlyCodePanel', () => {
  it('names its readonly CodeMirror textbox', async () => {
    render(ReadonlyCodePanel, {
      props: {
        code: '$ export OTEL_TRACES_EXPORTER="otlp"',
        ariaLabel: 'OTLP environment variables',
      },
    })

    expect(
      await screen.findByRole('textbox', {
        name: 'OTLP environment variables',
      })
    ).toHaveAttribute('aria-readonly', 'true')
  })
})
