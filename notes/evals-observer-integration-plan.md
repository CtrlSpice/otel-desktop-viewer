# Evals in otel-desktop-viewer

Date: 17 October 2026

Status: draft for discussion and annotation. The steps below are proposed;
the open decisions still need answers.

## Goal

The viewer receives an evaluation, shows its results, and lets someone follow
a result into the traces that explain it.

Our eval kit is the first integration. Other people's evaluations should be
able to use the same viewer, whatever application or behaviour they test.
Fixture setup belongs to each evaluation tool and is not part of the viewer's
general observer requirements.

## Direction already agreed

- Build importing first, then launching. Ship both together.
- Keep Evals off by default and label it "Experimental and unsupported."
- Reuse the report design and shared components we have polished.
- Preserve recorded results and evidence. Importing does not run an evaluation.
- Keep links from results to investigator, tool-call and grader spans.

## 1. Agree on what an evaluation contains

Describe these things consistently:

- **Evaluation:** its identity, name, source tool and progress.
- **Case:** the question, example or scenario being tested.
- **Configuration:** the model, agent, prompt or application version being tested.
- **Attempt:** one execution of a case against a configuration.
- **Assessment:** a score or label, who produced it and which attempt it assesses.
- **Trace references:** the trace and span IDs associated with the work.

An attempt can have several assessments, such as correctness, relevance and
safety. A grader can be a model, a deterministic check or a person.

Different tools use different score scales. Our pass/partial/fail scheme is one
supported scheme. The import must carry enough information to explain what a
score means and how any summary was calculated.

Keep planned work separate from recorded attempts. For our kit, means and bars
continue to use the full plan; missing and error grades remain unscored. When
another tool does not supply a plan, the viewer must say that coverage is unknown.

## 2. Make our existing results the first import

Transfer the completed evaluation into the viewer and reproduce the current
report from the imported records.

Preserve:

- Cases, configurations, planned repetitions and completed attempts.
- Original grades, execution errors, corrections and human signoff.
- Timing, token usage and costs, including unknown or incomplete values.
- Original trace/span IDs and their association with each attempt or assessment.
- The distinction between retained work, continuations and recovery attempts.

The viewer should not need our kit's directory layout to understand an imported
evaluation. A small converter reads those files and supplies the common records.

## 3. Check the design against another tool

Try a small, real export from a second framework. Inspect is the proposed choice
because its results and scoring structure differ from our kit's.

Use it to find assumptions that only work for us before committing to the shared
format. Each supported tool gets a converter; the viewer uses the same result
model and presentation.

Checking a second format does not yet commit us to shipping a complete second
integration in the first release.

## 4. Handle results and traces arriving separately

Support these situations:

- Results arrive without traces.
- Matching traces arrive later.
- Results arrive while an evaluation is still running.
- Grading happens later, or in a different trace from the evaluated work.
- Someone imports the same file again or reconnects after a transfer stops.

Stable identities must prevent duplicate results. Updates must extend the right
evaluation and preserve earlier completed work. Conflicting records need a clear
reported outcome rather than silently replacing a grade.

A stopped transfer must leave an accurate record of what was saved and what is
still missing. Receiving results and receiving telemetry are separate operations;
the interface must make partial completion understandable.

Use the viewer's existing telemetry intake and trace navigation. OpenTelemetry's
evaluation-result event is a useful starting point for live scores, but we also
need to describe the surrounding evaluation, cases and planned work.

## 5. Move the report into an Evals page

Add a list of evaluations and a detail page using the polished report components.

Preserve these distinctions throughout the interface:

- An incorrect answer versus a failed execution.
- Missing scores versus zero scores.
- Completed work versus planned work.
- Original assessments versus later corrections.
- Estimated costs versus recorded charges, with an explicit currency.
- Available traces versus references whose telemetry is missing.

Large evaluations should load in manageable pieces. Opening one attempt should
not require downloading every other attempt's tool calls. Live updates should
preserve expanded sections, focus and scroll position.

## 6. Add launching through our kit

Once receiving and displaying results works, connect the launch controls.

The kit runs the evaluation and sends results and progress back. The viewer
displays that progress and offers the agreed controls. Other integrations can
have their own execution requirements.

Before implementing launching, specify exactly what starts, who owns the running
processes, how stopping works and what happens if the viewer closes. Importing or
reopening results must never launch work. Completed attempts must not be rerun
implicitly.

## Decisions to settle before coding

### A. Saving and retention

The viewer currently removes older telemetry when its store reaches a size limit.
An evaluation's scores may still matter after that point.

Questions:

- Does saving an evaluation also preserve its associated traces?
- Can results remain after their traces have been removed?
- What does deleting an evaluation remove, especially when traces are shared?
- How do these choices interact with the viewer's existing size limit?
- What should survive restarting the viewer in its normal and persistent modes?

Decision / annotations:

>

### B. Imported contents

The current report contains summary data and trace links. It does not contain
full prompts, answers, grader explanations or private evidence files.

Questions:

- Keep that scope for the first version, or include detailed inputs and outputs?
- If detailed content is included, where is it displayed and how is it exported?
- Should results and optional telemetry be packaged together for sharing?

Decision / annotations:

>

### C. Stopping and disconnection

Questions:

- Does closing the viewer stop an evaluation that it launched?
- What happens when the experimental feature is switched off during a run?
- How does the viewer distinguish a disconnected runner from a finished run?
- How do we reconnect to ongoing work without starting it again?

Decision / annotations:

>

## How we know the design works

- Our completed evaluation imports with the same results, accounting and links.
- A second tool's example fits without inventing scores or losing their meaning.
- Repeated imports and interrupted transfers do not duplicate or erase work.
- Late grading and late telemetry connect to the correct attempt.
- Results without traces remain useful and accurately labelled.
- Saving, deletion, restart and storage limits behave as agreed above.
- The page remains usable for large runs, on desktop and mobile, in both themes.
- Launch, stop and reconnect behaviour is tested with a local fake runner before
  any paid evaluation is used for verification.

## Suggested discussion order

1. Decide saving and retention.
2. Decide imported contents and sharing.
3. Confirm the common records using examples from our kit and Inspect.
4. Write down transfer, update and failure behaviour.
5. Decide launch and stop behaviour.
6. Turn the agreed design into implementation steps and acceptance checks.

## General annotations

>

## References

- Earlier idea: `notes/evals-in-viewer-idea.md` (historical; later decisions supersede it).
- Decisions tracker: `notes/agents-integration-todo.md`.
- Existing report: `evals/viewer-usability/presentation/README.md`.
- Experimental feature policy: `docs/experimental-features.md`.
- Promptfoo outputs: https://www.promptfoo.dev/docs/configuration/outputs/
- Inspect logs: https://inspect.aisi.org.uk/eval-logs.html
- OpenTelemetry evaluation events:
  https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/gen-ai-events.md
