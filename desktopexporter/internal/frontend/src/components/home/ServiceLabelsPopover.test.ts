// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import ServiceLabelsPopover from './ServiceLabelsPopover.svelte'
import { renderWithContexts } from '@/test/render-helpers'
import { service } from '@/test/service-map-fixtures'
import { telemetryAPI } from '@/services/telemetry-service'
import { copyText } from '@/utils/clipboard'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

afterEach(() => {
  vi.restoreAllMocks()
  vi.mocked(copyText).mockReset()
})

it('keeps the popover and controls mounted while invalidating Copy when the exact service changes', async () => {
  const old = Promise.withResolvers<{
    serviceName: string
    serviceNamespace: string
  }>()
  const labels = vi
    .spyOn(telemetryAPI, 'getServiceLabels')
    .mockReturnValueOnce(old.promise)
    .mockResolvedValue({
      serviceName: 'complete new name',
      serviceNamespace: 'complete new namespace',
    })
  const view = renderWithContexts(ServiceLabelsPopover, {
    service: service('old'),
  })
  const popover = view.container.querySelector<HTMLElement>('[popover]')!
  await fireEvent.click(
    view.getByRole('button', { name: 'Service labels', exact: true })
  )
  const button = await view.findByRole('button', {
    name: 'Copy service.namespace',
  })
  expect(labels).not.toHaveBeenCalled()
  await fireEvent.click(button)
  expect(labels.mock.calls[0][0]).toBe('old')
  await view.rerender({
    componentProps: { service: { ...service('new'), serviceName: 'old' } },
  })
  expect(labels.mock.calls[0][1]?.aborted).toBe(true)
  expect(view.getByRole('button', { name: 'Copy service.namespace' })).toBe(
    button
  )
  expect(view.container.querySelector('[popover]')).toBe(popover)
  old.resolve({
    serviceName: 'complete old name',
    serviceNamespace: 'complete old namespace',
  })
  await tick()
  expect(copyText).not.toHaveBeenCalled()
  await fireEvent.click(button)
  expect(labels.mock.calls[1][0]).toBe('new')
  await view.findByText('Copied service.namespace.')
  expect(copyText).toHaveBeenCalledExactlyOnceWith('complete new namespace')
})
