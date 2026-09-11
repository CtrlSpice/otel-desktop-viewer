# Agent integration

Working notes, not a plan of record. Written while the query layer was being
optimised, so several numbers below come from that work rather than from
anything built for agents.

Sections get added as they are settled. Open questions are marked as such
rather than resolved silently.

---

## 1. What the RPC surface already gives you

`POST /rpc`, JSON-RPC 2.0, ~20 methods across traces, metrics, logs, stats and
deletes. **An agent with HTTP access can drive the whole product today with no
new code in the viewer.** The question is not whether it is possible but
whether it is pleasant enough to be reliable.

What is genuinely right about it:

- One endpoint, one transport, one envelope.
- Filtering happens server-side in DuckDB, so an agent narrows by asking rather
  than by fetching rows and discarding them.
- **It is the same API the UI uses.** Anything an agent can see, a human can
  reproduce in the UI, and vice versa. There is no second implementation to
  drift.

What is missing, specifically:

- **No schema or discovery.** Method names and parameter shapes exist only in
  Go. Nothing can enumerate them.
- **Positional array params.** Order-dependent and unnamed. `getMetric` takes
  between 3 and 14 of them.
- **Undocumented invariants that bite.** Timestamps must be *strings*;
  passing numbers returns `-32602 invalid params` with no hint. This cost real
  time during ordinary development, by someone who could read the handler.
  An agent cannot.
- **Unbounded responses.** See section 2.
- **No result identity.** Responses carry data, never "where to look at this".

**Position: keep `/rpc` as the plumbing, do not have agents speak it directly.**
It is shaped for a client that ships alongside the server and already knows the
rules — which the UI does, and an agent does not. The agent layer adds names,
schemas, bounds and links on top.

The cheaper alternative — agents on raw `/rpc` plus a skill documenting the
quirks — is not unreasonable. It fails silently rather than loudly, which is
the argument against it.

## 2. Discoverability, and whether it costs anything

It does not, because it is out-of-band.

- **`rpc.discover`** returning a schema document costs nothing unless called.
  OpenRPC is the existing convention for exactly this. Existing paths are
  untouched.
- **Named params.** JSON-RPC 2.0 already permits `params` as an object as well
  as an array. Accepting `{"startTime": "…"}` beside the positional form is one
  unmarshal branch, backwards compatible, and removes the single worst trap.
  It is the difference between `getMetric` being callable and not.

### The real cheat: summarise in SQL, not in the model

An agent asking "what happened in this trace" currently fetches ~171KB and
reads it — roughly 40k tokens and seconds of model time — to arrive at
something like *"159 spans, 12 services, 3 errors, slowest was checkout at
812ms."*

That sentence is ~30 tokens. DuckDB computes it in about a millisecond;
aggregating a few hundred rows is what the engine is for.

So today the summarisation happens in the most expensive available place: in
the model, over serialised JSON, after a network hop. Moving it into SQL makes
the agent path **cheaper than the UI path**, because the UI wants rows to draw
and an agent wants conclusions to reason about.

Shape of it: `describeTrace(id)` returns facts plus a link; `findSlowest(window,
n)` returns n rows and n links. Full payloads get fetched only when something
has a reason to want them.

**Rule: agent methods must be projections of the same queries, never
reimplementations.** If `describeTrace` computes "slowest span" differently
from what the waterfall draws, the human follows the link, sees something else,
and stops believing the agent. Same SQL, different projection.

### Settled: summaries are queries, not an agent surface

They live in SQL, like every other query, with the same thin Go wrapper and the
same `/rpc` exposure. **The store has no business knowing what is querying it.**

That framing does more work than it looks:

- It removes the forked-API risk entirely. "Same SQL, different projection"
  stops being a discipline to maintain and becomes structurally true, because
  there is only one implementation to be right.
- **The UI gets them too.** If `describeTrace` is a query, a trace list card can
  show "159 spans · 12 services · 3 errors" without fetching the trace. Today
  the only way to know a trace's shape is to fetch all of it. A human-facing win
  falling out of the agent-facing design is usually the sign the abstraction is
  in the right place.
- MCP becomes a *client*, not a path. It translates tool calls into the same
  methods a browser calls. If it were deleted tomorrow, nothing in the store
  would change.

**Where the Go layer earns its place: link assembly.** SQL owns the facts —
trace id, span id, the window. It cannot know the base URL, the browser port,
or whether this is reached through a `kubectl port-forward`. That is runtime
configuration and presentation. So the seam is: SQL produces facts, Go builds
the URL.

---

## Measurements this rests on

Taken during query optimisation on 2026-08-21, quiet machine, 14-core Apple
Silicon, store of 244,942 spans / 45,981 traces:

| thing | number |
|---|---|
| one trace fetch (159 spans) | ~28ms, 171KB |
| full trace list, unbounded | 116.6ms, **9.40MB**, 45,269 summaries |
| trace list extrapolated to 434k traces | ~1.3s, ~90MB |
| viewer cold start to first answered query | 0.05s in-memory, 0.07s on-disk |

The 9.40MB figure is why bounded results are a prerequisite rather than a
nicety: it is one request, and it does not fit in a context window.

## 3. The link handoff

The division of labour: **the agent works in queries, the UI is the payoff.**
The agent narrows headlessly, cheaply, without rendering anything; the human
clicks once and lands in the real view, where they can disagree with the
agent's conclusion and keep exploring past it. A claim with a link is
checkable. A wall of JSON is not.

Cheap to build, too — the reveal reuses views that already exist. There is no
agent-specific UI to design.

### What a link must encode

Trace id, span id, **and the time window**.

The window is the non-obvious one. `/traces/{id}` links generated during this
session worked, and then an hour later showed *"No traces in this time range"*
— the default window had rolled forward past the data. The trace was still
there; the view was not looking at it.

For a human clicking their own link immediately, that is rare. For an agent
handing over a link read after lunch, or pasted into an incident channel, it is
the normal case. `start`/`end` already exist as route params; they simply are
not what anyone reaches for by default.

`?span=` matters for the same reason `cyclePoint` does: landing on the trace is
useful, landing on *the span being discussed*, revealed and selected, is the
difference between "look at this" and "look at this thing here".

### Link rot, and why silent degradation stops working

Retention prunes at the size cap, so links die. For metrics it is worse: the
existing metrics plan records that datapoint ids are `uuid()` minted per row at
ingest, so a shared link dies when that row ages out — and **degrades silently
to "no selection"**.

Silent is survivable when you made the link yourself thirty seconds ago. When
an agent made it, the human lands on an empty view and cannot tell whether the
agent was wrong, the data is gone, or the UI is broken. Those need
distinguishing. *"This trace was pruned by retention at 14:32"* preserves trust;
*"no selection"* spends it.

This is a product improvement that agents motivate and humans get for free —
anyone who bookmarks a trace hits the same wall.

### Metrics links are second-class today

Metric view params are `agg`, `htab`, `hscope`, `dp` — there is no way to name
a *series*. Selection is derived backwards by scanning for whichever series
contains the selected datapoint. So an agent can say "this trace, this span"
precisely, but cannot say "this series" at all; it can only point at a
datapoint that may not survive the week.

The `series_id` work in the storage plan fixes this, and is a prerequisite for
metrics links being worth handing over.

### Mechanics

**One shared link builder**, not per-tool assembly. Otherwise half the tools
forget the timestamps, and they will be the half nobody tests.

**Open question:** whether a link can encode *why* the agent is pointing there.
The cycle work added `cyclePoint` for "this span specifically, and here is what
is wrong with it". An agent pointing at a slow span wants the same affordance,
and there is currently no way to say "highlight this, for this reason" in a
URL.

## 4. Do we need MCP?

Probably not, and not first.

**MCP is packaging, not capability.** Once it is a client rather than a backend
path (section 2), it buys nothing the API cannot express. What it does buy is
host ergonomics: tool listing with schemas in the host UI, a transport hosts
already speak so nobody writes glue, per-tool permissioning, and a known
install gesture.

All real. None of them a capability the store lacks.

**The API work erases most of it.** With `rpc.discover`, named params, bounded
results and links, an agent holding a generic HTTP tool is about as capable as
one holding MCP tools — and all four improvements benefit the UI too.

**For our own use it is already unnecessary.** Claude Code has Bash and can
call `/rpc` directly; that is how every measurement in these notes was taken. A
skill teaching the grammar and the timestamp-as-string trap makes that
reliable, at the cost of a markdown file rather than a subcommand, a transport
and a versioned tool surface.

**Sequencing: API first, MCP if and when someone asks.** Being able to defer it
is a property of the architecture, not an accident — because MCP is a client,
deciding later costs nothing. If it were a backend path, it would have to be
decided now.

Where it eventually earns its place is distribution: someone who is not us
pointing their agent at the viewer without reading the API docs first. Real
audience, later problem — and by then the tool set can be carved from
experience instead of guessed at.

**The expensive mistake would be guessing the tool set now.** The transport is
cheap; the carving is not. A `--mcp` flag on the existing binary, months from
now, once a skill has shown which handful of tools people actually reach for.

## 5. The API traps, and what to fix

Found by using the API rather than by reading it, which is the point: every one
of these cost time to someone who had the source open.

| trap | what happens | fix |
|---|---|---|
| **Timestamps must be strings** | `params: [1787…, 1787…]` returns `-32602 invalid params`, no hint which argument or why. `parseTimestampParam` requires `param.(string)` then `ParseInt`. | Accept numbers as well as strings. A JSON number large enough for ns is exact to 2^53, and ns timestamps exceed that — so accept both and document that strings are safe above 2^53. |
| **`searchTraces` is unbounded** | Returns every summary in the window: 9.40MB / 45,269 traces measured. One request does not fit in a context window. | A `limit` in the search grammar, plus a default. |
| **Positional params** | `getMetric` takes 3–14 positional arguments. Unnamed, order-dependent, impossible to call correctly without the source. | Accept `params` as an object. JSON-RPC 2.0 already permits it. |
| **No discovery** | Method names and shapes exist only in Go. | `rpc.discover`. |
| **Store is single-writer** | Opening the db file while the viewer runs fails outright: `Conflicting lock is held`. Agents cannot go around the process. | Nothing to fix — document it. Arguably a feature: one door, one set of rules. |

### Batching: no

Tested — the endpoint rejects a JSON-RPC batch array with `-32700 parse error`.
The handler decodes a single message via `jsonrpc2.DecodeMessage` and casts to
`*jsonrpc2.Request`; supporting arrays is perhaps 50 lines.

**Not worth doing.** Batching amortises network cost, and on localhost a round
trip is under a millisecond. What an agent actually pays per call is tokens and
turns in its own framework — and that is fixed by *composite queries*
(`describeTrace` doing three aggregates in one statement) rather than by three
payloads in one envelope. The composite returns a sentence; the batch returns
three full responses.

It would also import partial-failure semantics, ordering and notification
handling, for a benefit that would be hard to measure.

### Order to land

1. **Timestamps** — accept numbers as well as strings. Smallest change,
   removes the trap most likely to be hit first.
2. **Bounded search** — a limit in the grammar with a sensible default. This is
   the prerequisite for everything else; 9.40MB is not a response, it is a
   denial of service against a context window.
3. `rpc.discover` and named params — after the two above, because both change
   the shapes that discovery would describe.
