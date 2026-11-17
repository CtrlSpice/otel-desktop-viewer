// Contract: the captured 2.0.29 OpenAPI Session.Message.ToolState.* schemas
// require input in every state; Assistant.tokens itself is optional.
const present = (value, key) => Object.hasOwn(value, key) && value[key] !== undefined

export function toolEvidence(tool) {
  const state = tool.state
  const errors = []
  if (!present(state, 'input')) errors.push({ toolID: tool.id, field: 'state.input', reason: 'Missing required V2 tool input' })
  const attributes = {
    'gen_ai.tool.name': tool.name, 'gen_ai.tool.call.id': tool.id,
    'opencode.session.id': tool.sessionID, 'opencode.message.id': tool.messageID,
    'opencode.runtime.version': '2.0.29', 'opencode.tool.raw': JSON.stringify(tool),
    'tool.status': state.status,
    ...(present(state, 'input') ? { 'tool.input': JSON.stringify(state.input) } : {}),
    ...(present(state, 'content') ? { 'tool.output': JSON.stringify(state.content) }
      : present(state, 'error') ? { 'tool.output': JSON.stringify(state.error) } : {}),
    ...(['completed', 'error'].includes(state.status) ? { 'tool.is_error': state.status === 'error' } : {}),
    ...(tool.time.completed !== undefined && tool.time.ran !== undefined ? { 'tool.duration_ms': tool.time.completed - tool.time.ran } : {}),
    'opencode.event.timestamp.source': tool.time.completed === undefined ? 'tool.time.created' : 'tool.time.completed',
  }
  return { attributes, timestamp: tool.time.completed ?? tool.time.created, errors }
}

export function aggregateUsage(assistants) {
  const usage = {}, recorded = {}
  for (const field of ['input', 'output', 'reasoning', 'cache.read', 'cache.write']) {
    const values = assistants.map(message => field.split('.').reduce((value, key) => value?.[key], message.tokens))
      .filter(value => typeof value === 'number' && Number.isFinite(value) && value >= 0)
    recorded[field] = values.length
    // An aggregate is available only if every assistant recorded this field.
    // Do not turn a missing message/field into zero or publish a partial sum.
    if (!assistants.length || values.length !== assistants.length) continue
    const sum = values.reduce((total, value) => total + value, 0)
    if (field.startsWith('cache.')) (usage.cache ??= {})[field.split('.')[1]] = sum
    else usage[field] = sum
  }
  const counts = Object.values(recorded)
  const usageAvailability = {
    status: counts.every(count => count === 0) ? 'unavailable'
      : counts.every(count => count === assistants.length) ? 'complete' : 'partial',
    assistants: assistants.length, recorded,
    // TokenUsage.Info defines separate numeric cache.read/write fields, but
    // does not specify overlap with input/output/reasoning or define a total.
    total: 'unavailable: V2 2.0.29 schema does not define additive token accounting',
  }
  const tokenUsage = {}, completionDetails = {}
  if (usage.input !== undefined) tokenUsage.prompt = usage.input
  if (usage.output !== undefined) tokenUsage.completion = usage.output
  if (usage.cache?.read !== undefined) tokenUsage.cached = completionDetails.cacheReadInputTokens = usage.cache.read
  if (usage.cache?.write !== undefined) completionDetails.cacheCreationInputTokens = usage.cache.write
  if (usage.reasoning !== undefined) completionDetails.reasoning = usage.reasoning
  if (Object.keys(completionDetails).length) tokenUsage.completionDetails = completionDetails
  return { usage, usageAvailability, ...(Object.keys(tokenUsage).length ? { tokenUsage } : {}) }
}
