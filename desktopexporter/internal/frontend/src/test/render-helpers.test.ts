// @vitest-environment jsdom
import { expect, it } from 'vitest'
import { screen } from '@testing-library/svelte'
import RenderPropsProbe from './RenderPropsProbe.svelte'
import { renderWithContexts } from './render-helpers'

it('rerenders with the concrete child prop contract', async () => {
  const view = renderWithContexts(RenderPropsProbe, {
    label: 'first',
    count: 1,
  })
  type RerenderUpdate = Parameters<typeof view.rerender>[0]
  const invalidUpdate: RerenderUpdate = {
    componentProps: {
      // @ts-expect-error RenderPropsProbe requires a string label.
      label: 2,
    },
  }
  void invalidUpdate

  await view.rerender({
    componentProps: { label: 'second', count: 2 },
  })

  expect(screen.getByText('second: 2')).toBeInTheDocument()
})
