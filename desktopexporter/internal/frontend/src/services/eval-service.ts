import type {
  EvalExperimentSearch,
  EvalExperimentSearchPage,
  EvalExperimentSourcePage,
} from './eval-experiment-types'

// Keep Evals transport out of the shared telemetry API so public builds can
// exclude the complete implementation, rather than merely hiding navigation.
export const evalAPI = {
  searchEvalExperiments: async (
    query: EvalExperimentSearch = {},
    signal?: AbortSignal
  ): Promise<EvalExperimentSearchPage> => {
    const params = new URLSearchParams()
    if (query.query !== undefined) params.set('q', query.query)
    for (const outcome of query.outcomes ?? [])
      params.append('outcome', outcome)
    for (const key of ['sort', 'direction', 'limit', 'offset'] as const)
      if (query[key] !== undefined) params.set(key, String(query[key]))
    const response = await fetch(`/api/evals/experiments?${params}`, { signal })
    if (!response.ok)
      throw new Error(
        (await response.text()).trim() || `HTTP ${response.status}`
      )
    return response.json()
  },
  getEvalExperimentPage: async (
    id: string,
    after = -1,
    limit = 100,
    signal?: AbortSignal
  ): Promise<EvalExperimentSourcePage> => {
    const response = await fetch(
      `/api/evals/experiments/${encodeURIComponent(id)}/rows?after=${after}&limit=${limit}`,
      { signal }
    )
    if (!response.ok)
      throw new Error(
        (await response.text()).trim() || `HTTP ${response.status}`
      )
    return response.json()
  },
  ingestEvalFile: async (
    file: File,
    signal?: AbortSignal
  ): Promise<string[]> => {
    const response = await fetch(
      `/api/evals/ingest?name=${encodeURIComponent(file.name)}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: file,
        signal,
      }
    )
    if (response.status === 404 || response.status === 403)
      return [
        'Evaluation import is unavailable. Use an Evals build and enable evals.enabled on the viewer server.',
      ]
    if (!response.ok)
      return [(await response.text()).trim() || `HTTP ${response.status}`]
    const reply: { complete?: unknown; ids?: unknown } = await response.json()
    if (
      !reply ||
      reply.complete !== true ||
      !Array.isArray(reply.ids) ||
      !reply.ids.every(
        id => typeof id === 'string' && /^[a-f0-9]{64}$/.test(id)
      )
    )
      throw new Error(
        'The server did not confirm a completed evaluation import. Retry the file to check completion.'
      )
    return []
  },
}
