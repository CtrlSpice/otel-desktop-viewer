// @vitest-environment jsdom
import { beforeEach, expect, it } from 'vitest'
import { fireEvent, render } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DetailStackHarness from '@/test/DetailStackHarness.svelte'

beforeEach(() => localStorage.clear())

it('resizes the visible boundary when a middle section mounts after its siblings', async () => {
  const view = render(DetailStackHarness, {
    storageKey: 'layout',
    label: 'Details',
  })
  const telemetry = view
    .getByText('Telemetry', { exact: true })
    .closest('details')!
  await view.rerender({ assessments: true })
  const sections = [
    ...view.container.querySelectorAll<HTMLElement>(
      '.detail-bubble--resizable'
    ),
  ]
  const headers = sections.map(section => section.querySelector('summary')!)
  expect(headers[1]!.dataset.resizeBefore).toBe(sections[0]!.id)
  expect(headers[2]!.dataset.resizeBefore).toBe(sections[1]!.id)
  expect(headers[2]!.closest('details')).toBe(telemetry)
  for (const [index, section] of sections.entries()) {
    // JSDOM does not load the section's CSS minimum alongside mocked geometry.
    section.style.minHeight = '40px'
    section.getBoundingClientRect = () =>
      ({ height: index === 3 ? 40 : 240 }) as DOMRect
    headers[index]!.getBoundingClientRect = () => ({ height: 40 }) as DOMRect
  }
  await fireEvent.keyDown(headers[1]!, { key: 'ArrowDown' })
  expect(JSON.parse(localStorage.getItem('layout')!)).toEqual({
    Details: 206 / 600,
    Assessments: 194 / 600,
    Telemetry: 200 / 600,
  })
  await userEvent.click(headers[1]!)
  expect(telemetry.open).toBe(true)
  expect(JSON.parse(localStorage.getItem('layout')!).Details).toBeCloseTo(
    400 / 600
  )
  expect(JSON.parse(localStorage.getItem('layout')!).Telemetry).toBeCloseTo(
    200 / 600
  )

  await view.rerender({ assessments: false })
  await view.rerender({ assessments: true })
  expect(
    view.getByText('Assessments', { exact: true }).closest('summary')!.dataset
      .resizeBefore
  ).toBe(sections[0]!.id)
  expect(view.getByText('Telemetry', { exact: true }).closest('details')).toBe(
    telemetry
  )
})

it('updates row headings and saved layouts without replacing sections or disclosure state', async () => {
  localStorage.setItem(
    'case-layout',
    JSON.stringify({ Question: 0.75, Telemetry: 0.25 })
  )
  localStorage.setItem('tool-layout', JSON.stringify({ Tool: 1, Telemetry: 3 }))
  const view = render(DetailStackHarness, {
    storageKey: 'case-layout',
    label: 'First question',
    layoutKey: 'Question',
  })
  const stack = view.container.querySelector('.detail-stack')!
  const sections = [
    ...stack.querySelectorAll<HTMLElement>('.detail-bubble--resizable'),
  ]
  const disclosures = [...stack.querySelectorAll('details')]
  expect(parseFloat(sections[0]!.style.flex)).toBe(75)

  await view.rerender({ label: 'Second question' })
  expect(stack.querySelectorAll('details')[0]).toBe(disclosures[0])
  expect(disclosures[0]).toHaveTextContent('Second question')
  expect(parseFloat(sections[0]!.style.flex)).toBe(75)

  await view.rerender({
    storageKey: 'tool-layout',
    label: 'Tool',
    layoutKey: undefined,
  })
  expect(stack.querySelectorAll('details')[0]).toBe(disclosures[0])
  expect(disclosures.map(section => section.open)).toEqual([true, true, false])
  expect(parseFloat(sections[0]!.style.flex)).toBe(25)
  await userEvent.click(disclosures[1]!.querySelector('summary')!)
  expect(disclosures.map(section => section.open)).toEqual([true, false, false])
  expect(JSON.parse(localStorage.getItem('tool-layout')!)).toEqual({
    Tool: 1,
    Telemetry: 0.75,
  })
  expect(JSON.parse(localStorage.getItem('case-layout')!)).toEqual({
    Question: 0.75,
    Telemetry: 0.25,
  })

  await view.rerender({
    storageKey: 'case-layout',
    label: 'Third question',
    layoutKey: 'Question',
  })
  expect(disclosures.map(section => section.open)).toEqual([true, false, false])
  expect(stack.querySelectorAll('details')[0]).toBe(disclosures[0])
  await userEvent.click(disclosures[1]!.querySelector('summary')!)
  expect(parseFloat(sections[0]!.style.flex)).toBe(75)
})

it('saves keyboard resizing under the current section layout key', async () => {
  const view = render(DetailStackHarness, {
    storageKey: 'layout',
    label: 'Old heading',
  })
  await view.rerender({ label: 'New heading', layoutKey: 'Stable heading' })
  const disclosures = [...view.container.querySelectorAll('details')]
  for (const section of view.container.querySelectorAll<HTMLElement>(
    '.detail-bubble--resizable'
  )) {
    section.style.minHeight = '40px'
    section.getBoundingClientRect = () =>
      ({ height: section.querySelector('details')!.open ? 240 : 40 }) as DOMRect
    section.querySelector('summary')!.getBoundingClientRect = () =>
      ({ height: 40 }) as DOMRect
  }
  await fireEvent.keyDown(disclosures[1]!.querySelector('summary')!, {
    key: 'ArrowDown',
  })
  expect(JSON.parse(localStorage.getItem('layout')!)).toEqual({
    'Stable heading': 0.51,
    Telemetry: 0.49,
  })
  expect(disclosures[0]).toHaveTextContent('New heading')
})

it('resizes only open neighbours and never reopens a closed section during resizing', async () => {
  const view = render(DetailStackHarness, {
    storageKey: 'layout',
    label: 'Details',
    assessments: true,
  })
  const sections = [
    ...view.container.querySelectorAll<HTMLElement>(
      '.detail-bubble--resizable'
    ),
  ]
  const headers = sections.map(section => section.querySelector('summary')!)
  await userEvent.click(headers[1]!)
  expect(headers[1]!.closest('details')!.open).toBe(false)
  // A closed card must not become a resize donor.
  for (const [index, section] of sections.entries()) {
    section.style.minHeight = '40px'
    section.getBoundingClientRect = () =>
      ({ height: [300, 180, 240, 40][index] }) as DOMRect
    headers[index]!.getBoundingClientRect = () => ({ height: 40 }) as DOMRect
  }
  await fireEvent.keyDown(headers[2]!, { key: 'ArrowDown' })
  expect(headers[1]!.closest('details')!.open).toBe(false)
  expect(headers[1]!.dataset.resizeBefore).toBeUndefined()
  expect(headers[2]!.dataset.resizeBefore).toBe(sections[0]!.id)
  expect(JSON.parse(localStorage.getItem('layout')!).Details).toBeCloseTo(
    264.6 / 460
  )
  expect(JSON.parse(localStorage.getItem('layout')!).Telemetry).toBeCloseTo(
    195.4 / 460
  )
})

it('keeps fixed section order across disclosure changes, retaining nodes and one open section', async () => {
  const view = render(DetailStackHarness, {
    storageKey: 'layout',
    label: 'Details',
    assessments: true,
    unavailable: true,
  })
  const stack = view.container.querySelector('.detail-stack')!
  const keys = () =>
    [...stack.querySelectorAll<HTMLElement>(':scope > .detail-bubble')].map(
      element => element.dataset.detailSection
    )
  const details = view.getByText('Details', { exact: true }).closest('details')!
  const assessments = view
    .getByText('Assessments', { exact: true })
    .closest('details')!
  const telemetry = view
    .getByText('Telemetry', { exact: true })
    .closest('details')!
  const configuration = view
    .getByText('Configuration', { exact: true })
    .closest('details')!
  expect(keys()).toEqual([
    'details',
    'assessments',
    'unavailable',
    'telemetry',
    'configuration',
  ])
  const header = assessments.querySelector('summary')!
  header.focus()
  await userEvent.click(header)
  expect(keys()).toEqual([
    'details',
    'assessments',
    'unavailable',
    'telemetry',
    'configuration',
  ])
  expect(
    view.getByText('Assessments', { exact: true }).closest('details')
  ).toBe(assessments)
  expect(header).toHaveFocus()
  expect(header).not.toHaveClass('detail-bubble__heading--resizable')
  expect(configuration.querySelector('summary')).not.toHaveClass(
    'detail-bubble__heading--resizable'
  )
  await userEvent.click(telemetry.querySelector('summary')!)
  expect(details.open).toBe(true)
  expect(details.querySelector('summary')).toHaveAttribute(
    'aria-disabled',
    'true'
  )
  await userEvent.click(details.querySelector('summary')!)
  expect(details.open).toBe(true)
  expect(keys()).toEqual([
    'details',
    'assessments',
    'unavailable',
    'telemetry',
    'configuration',
  ])
  await userEvent.click(header)
  expect(assessments.open).toBe(true)
  expect(header).toHaveClass('detail-bubble__heading--resizable')
  expect(stack.querySelector('[data-detail-section="details"] details')).toBe(
    details
  )
})
