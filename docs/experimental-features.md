# Experimental features

**Experimental and unsupported.** These features are opt-in. They may change or
be removed without notice. Their availability does not promise a stable API,
compatibility, migration support, or a future supported release.

Experiments ship disabled. Normal viewer startup does not read a flag file or
start a flag provider. To use a local configuration:

```sh
otel-desktop-viewer --feature-flags-file ./feature-flags.json
```

The repository's `feature-flags.json` also defaults every experiment to off.
For a local opt-in, change the relevant flag's `defaultVariant` to `"on"`.
`state: "ENABLED"` means the flag is evaluated; the boolean variant determines
whether the feature is enabled.

The viewer uses OpenFeature with flagd's in-process file resolver. It polls the
file every five seconds. No dashboard, account, flag server or network access is
needed for evaluation. Paths are relative to the viewer's working directory.
An unreadable or invalid initial file fails startup. Missing flags and evaluation
errors default to off. flagd can retain its last valid configuration after an
invalid file edit; to disable an experiment, save a valid `"off"` configuration
or restart without `--feature-flags-file`.

| Flag | Default | Status |
| --- | --- | --- |
| `evals.enabled` | `false` | Experimental and unsupported; reserved for the Evals feature under development |

This change provides flag infrastructure. Result import, the kit integration and
run launching will land together. Enabling the flag does not yet provide an Evals
screen or run launcher.

## Integration contract

- `GET /api/features` returns evaluated capabilities and their display labels.
  It is uncached. The Evals label is always `Experimental and unsupported`.
- The UI must hide disabled features and display that label when an experiment
  is enabled. Refresh capabilities when entering the feature; frontend state is
  not permission to start backend work.
- `/evals` and `/api/evals` paths are gated on each request. New eval handlers
  must use the same gate; any future eval RPC methods must check the flag too.
- Disabled features must not start eval workers or other feature-specific work.
  Disabling a flag prevents new actions; cancellation of an already-running eval
  will be specified with the launcher.
- Keep defaults off in release builds and the checked-in configuration. Flags
  are rollout controls, not authentication or a way to conceal shipped source.

For Collector embedding, set `extensions.duckdb.feature_flags_file` to the local
JSON file. Each extension owns an isolated OpenFeature API and closes its provider
on shutdown, without changing providers used by other Collector components.
