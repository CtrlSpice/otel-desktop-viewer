import PiProvider from './pi-provider.ts'
import { checkEvidenceError } from './opencode-v2-execution-stop.mjs'

export function judgeFor(investigator) {
  if (/^opencode-v2:(openai\/gpt-|(?:anthropic|opencode-go|kimi-code-plan-global)\/)[^/]+$/.test(investigator)) return 'pi:openai-codex/gpt-6.1-sol'
  throw new Error('Unknown investigator for blind grading: ' + investigator)
}

export default class BlindJudge {
  constructor({ config }) { this.config = config }
  id() { return 'viewer:blind-sol-judge' }
  async callApi(prompt, context) {
    judgeFor(context?.originalProvider?.id())
    const { root, authFile } = this.config
    const provider = new PiProvider({ config: { role: 'grader', root, timeoutMs: 180000,
      model: 'gpt-6.1-sol', provider: 'openai-codex', authFile } })
    try { return await provider.callApi(prompt, context) }
    catch (error) { checkEvidenceError(error, root); throw error }
  }
}
