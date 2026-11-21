/** Keep both component results and their usage. The export task itself is pass/fail. */
export default function scoreExport(_metrics, { componentResults }) {
  const pass = componentResults.some(c => c.assertion?.type === 'llm-rubric') &&
    componentResults.some(c => c.assertion?.type === 'javascript' && c.assertion.metric === 'export-file') &&
    componentResults.every(c => c.pass)
  return { pass, score: pass ? 1 : 0, reason: pass ? 'The written handoff and file replay both passed.'
    : componentResults.filter(c => !c.pass).map(c => c.reason).join('; ') || 'Missing export check.' }
}
