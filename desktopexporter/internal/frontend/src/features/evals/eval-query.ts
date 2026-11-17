/** Accept bookmarked selections from before the Experiment/Result rename.
 * Only query keys and outline prefixes change; opaque IDs remain untouched. */
export function evalQuery(
  query: Record<string, string | undefined>
): Record<string, string | undefined> {
  const { run, rep, ...current } = query
  if (current.experiment === undefined && run !== undefined)
    current.experiment = run
  if (current.result === undefined && rep !== undefined) current.result = rep
  if (current.row?.startsWith('run/'))
    current.row = `experiment/${current.row.slice(4)}`
  else if (current.row?.startsWith('rep/'))
    current.row = `result/${current.row.slice(4)}`
  return current
}
