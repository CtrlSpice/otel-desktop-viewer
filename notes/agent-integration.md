# Agent support

Working plan, updated 2026-09-11.

## Goal

Let software agents investigate telemetry safely.

Agents should return concise facts and links to the relevant viewer page. A
person must be able to verify every result in the existing UI.

## Current state

Already available:

- Agents can call the same JSON-RPC API as the UI.
- Named parameters are supported.
- Nanosecond timestamps accept exact strings and JSON numbers.
- Trace, log, and metric searches support sorting and explicit limits.
- Metric series have stable links.
- Frontend route helpers preserve time and item selection.

Still missing:

- Machine-readable API discovery
- Enforced response bounds
- Concise trace descriptions
- Links in API results
- Clear handling for unavailable linked data
- Specialized GenAI visualization

## First

Add `rpc.discover` with parameter, result, error, and safety schemas.

This gives agents a reliable description of the API. It also gives later MCP
tools and GenAI queries one shared contract.

## After

Each step should be a separate, reviewable change.

1. Add a bounded trace search that reports when results were truncated.
2. Add trace and span links with fixed time bounds.
3. Add `describeTrace` for counts, services, errors, and notable spans.
4. Apply the bounded-result and link patterns to logs and metrics.
5. Test real investigations through the API before choosing MCP tools.

`findSlowest` does not need its own method. Trace search already supports
sorting by duration and limiting the result.

## Bounded queries

An unbounded trace search returned about 45,000 summaries and 9.4 MB in testing.
A single detailed trace returned about 171 KB. Agent responses need firm limits
and must say when results were truncated.

## Rules

- The UI, agents, and future MCP tools use the same backend queries.
- DuckDB computes summaries. Models should not summarize large raw payloads.
- Agent responses contain facts, not generated prose.
- Responses are bounded and say when results were truncated.
- Links include the item identity and a fixed time window.
- Destructive methods are clearly marked.
- Missing data is not called "pruned" without evidence.
- MCP is a client of the API, not a separate backend.
- GenAI telemetry remains ordinary OpenTelemetry trace data underneath.

## GenAI

API investigation comes first. GenAI visualization follows on the same
foundation.

1. Add fixtures from real OpenTelemetry GenAI instrumentation.
2. Verify that attributes, events, links, tool calls, and token counts survive
   ingestion correctly.
3. Decide how sensitive prompt and completion content should be displayed.
4. Add SQL projections for GenAI operations, models, usage, errors, and tools.
5. Build a specialized trace view with a direct path to the normal waterfall.

## Deferred

- Do not add JSON-RPC batching.
- Do not create a separate agent storage or query path.
- Do not design MCP tools before API usage shows which tools are useful.
- Do not treat GenAI telemetry as a fourth OpenTelemetry signal.
