// @vitest-environment jsdom
import { cleanup, render } from '@testing-library/svelte'
import { afterEach, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import ServiceMap from './ServiceMap.svelte'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

it('restarts the service-map feedback grace period when service discovery is replaced', async () => {
  vi.useFakeTimers()
  const view = render(ServiceMap, {
    service: null,
    options: {},
    revision: 0,
    selection: null,
    onSelect: vi.fn(),
    services: [],
    loadingServices: true,
    servicesRequestKey: new AbortController().signal,
    servicesError: '',
    moreServices: false,
    onMoreServices: vi.fn(),
  })
  await tick()
  await vi.advanceTimersByTimeAsync(200)
  expect(view.getByText('Loading service map…')).toBeInTheDocument()
  await view.rerender({ servicesRequestKey: new AbortController().signal })
  expect(view.queryByText('Loading service map…')).toBeNull()
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading service map…')).toBeNull()
  await vi.advanceTimersByTimeAsync(1)
  expect(view.getByText('Loading service map…')).toBeInTheDocument()
  await view.rerender({ loadingServices: false })
  expect(view.queryByText('Loading service map…')).toBeNull()
  view.unmount()
  await vi.advanceTimersByTimeAsync(200)
  expect(view.queryByRole('status')).toBeNull()
})
