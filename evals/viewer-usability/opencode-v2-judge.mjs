import ClaudeProvider from './claude-provider.ts'
import PiProvider from './pi-provider.ts'
import { checkEvidenceError } from './opencode-v2-execution-stop.mjs'

export function judgeFor(investigator) {
  if (/^opencode-v2:(openai|opencode-go)\/gpt-[^/]+$/.test(investigator)) return 'claude-code:claude-opus-5-5'
  if (/^opencode-v2:(anthropic|opencode-go|kimi-code-plan-global)\/[^/]+$/.test(investigator)) return 'pi:openai-codex/gpt-6.1-sol'
  throw new Error('Unknown investigator for cross-family grading: ' + investigator)
}

export default class CrossFamilyJudge {
  constructor({ config }) { this.config = config }
  id() { return 'viewer:cross-family-judge' }
  async callApi(prompt, context) {
    const judge = judgeFor(context?.originalProvider?.id())
    const { root, authFile, cliBinary, configDir, telemetryEndpoint } = this.config
    const common = { role: 'grader', root, timeoutMs: 180000 }
    const provider = judge.startsWith('claude-code:') ?
      new ClaudeProvider({ config: { ...common, model: 'claude-opus-5-5', cliBinary, configDir, telemetryEndpoint } }) :
      new PiProvider({ config: { ...common, model: 'gpt-6.1-sol', provider: 'openai-codex', authFile } })
    try { return await provider.callApi(prompt, context) }
    catch (error) { checkEvidenceError(error, root); throw error }
  }
}
