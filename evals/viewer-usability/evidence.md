# Recorded Grader Evidence

`grade.ts` reads only the matching investigator's metrics, answer, stdout tool
events, commands and prompt hash. It checks task, model, answer, session IDs and
run-owned evidence/scratch paths. Missing, malformed, incomplete or mismatched
evidence raises a harness issue before a grader is launched.

The separate grader receives the private rubric, answer and evidence as JSON data.
Tool events remain original JSON strings so numeric lexemes and returned text are
not rounded or rewritten. Non-tool reasoning and investigator prompt contents are
excluded. Shell exit failures, tool errors, parsed JSON-RPC error envelopes and
structured native denials are identified separately. Native denials remain cases
needing harness review; collection does not decide whether a call was necessary.

`node collect-evidence.ts /absolute/saved/run` creates a new external packet
directory and inventory. Each saved result maps to one packet using the recorded
provider identity and answer. Attempt numbers count occurrences within each
task/model in saved row order; original row/test/prompt indices are also retained.
Source hashes are rechecked after collection. Packet and assembled prompt byte
sizes are measured, without truncation or model calls.

This delivers recorded tool results, not a new investigation or archive verifier.
Existing truncation flags remain visible in the inventory. No paths from command,
answer or tool output text are followed. Full output stored outside allowed
evidence/scratch ownership requires a separate delivery decision. Existing export
checks continue independently; archive acceptance remains unresolved.
