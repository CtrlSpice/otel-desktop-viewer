# Experimental features

**Experimental and unsupported.** These features are opt-in. They may change or
be removed without notice. Their availability does not promise a stable API,
compatibility, migration support, or a future supported release.

Evals is selected at build time. Public builds omit its frontend implementation,
HTTP handlers, decoder, projection package and flag provider. A runtime flag file
cannot enable it in a public binary.

```sh
make build             # public build
make build EVALS=1     # includes Evals
```

The enabled build uses Go's `evals` build tag and a matching frontend build.
Its generated assets live in `internal/server/static-evals` under
`desktopexporter`, separately from the committed public `static` assets. Direct
`go build` and `go install` use the public variant by default. For a direct tagged
Go build, first generate its assets with `make build-ts EVALS=1`.

Both builds retain the same database schema, including Evals table definitions
and cleanup when source logs are deleted. Switching build variants does not
change the database version or bypass its compatibility checks.

Within an Evals build, the local runtime flag still controls activation. Normal
startup does not read a flag file or start a provider. To use a local configuration:

```sh
./otel-desktop-viewer --feature-flags-file ./feature-flags.json
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
| `evals.enabled` | `false` | Requires an Evals build; experimental and unsupported |

The enabled workspace imports and inspects Promptfoo v3 results. See
[Evals ingestion](eval-ingestion.md) for supported input and storage behaviour.

## Integration contract

- `GET /api/features` returns evaluated capabilities and their display labels.
  It is uncached. The Evals label is always `Experimental and unsupported`.
- The public UI omits Evals navigation and import handling. Its capability store
  stays off even if a server reports Evals enabled.
- Public builds return 404 for `/evals` and `/api/evals` paths. Enabled builds
  check the runtime flag on each request. Frontend state cannot enable backend work.
- The runtime flag prevents new Evals actions when off; it does not cancel an
  import already in progress.
- Release builds exclude Evals. The checked-in runtime configuration also keeps
  it off. Build selection is not authentication or a way to conceal public source.

For Collector embedding, set `extensions.duckdb.feature_flags_file` to the local
JSON file. Each extension owns an isolated OpenFeature API and closes its provider
on shutdown, without changing providers used by other Collector components.
