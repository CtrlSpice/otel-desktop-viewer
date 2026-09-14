// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/svelte'
import RequiredContextConsumer from '@/test/RequiredContextConsumer.svelte'
import RequiredMetricViewContextConsumer from '@/test/RequiredMetricViewContextConsumer.svelte'
import RequiredRouteContextConsumer from '@/test/RequiredRouteContextConsumer.svelte'

describe('required app contexts', () => {
  it('throws when the time provider is absent', () => {
    expect(() => render(RequiredContextConsumer)).toThrow(/missing_context/)
  })

  it('throws when the route provider is absent', () => {
    expect(() => render(RequiredRouteContextConsumer)).toThrow(
      /missing_context/
    )
  })

  it('throws when the metric-view provider is absent', () => {
    expect(() => render(RequiredMetricViewContextConsumer)).toThrow(
      /missing_context/
    )
  })
})
