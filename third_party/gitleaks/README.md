# Gitleaks pattern attribution

`collector_redaction.yaml` adapts selected token regexes from
[Gitleaks v8.30.1, config/gitleaks.toml](https://github.com/gitleaks/gitleaks/blob/83d9cd684c87d95d656c1458ef04895a7f1cbd8e/config/gitleaks.toml).
The pinned release commit is `83d9cd684c87d95d656c1458ef04895a7f1cbd8e`.
The upstream MIT license is included in this directory.

Source rule IDs:

- `jwt`
- `openai-api-key`
- `anthropic-api-key`, `anthropic-admin-api-key`
- `github-pat`, `github-oauth`, `github-app-token`, `github-refresh-token`
- `github-fine-grained-pat`
- `slack-bot-token`, `slack-user-token`, `slack-app-token`

Adaptations: extract the token expression, remove capture groups used only for
secret extraction and trailing context delimiters, combine equivalent GitHub
prefixes and Anthropic prefixes. Trailing delimiters are unnecessary for masking
the token and can consume punctuation belonging to adjacent telemetry fields.

This is a pinned regex subset, not the Gitleaks detector. Contrib redaction
replaces the entire regex match. It has no Gitleaks `secretGroup`, entropy,
keyword prefilter, path allowlist, stopword or per-rule allowlist semantics.
These distinctive token shapes deliberately mask even low-entropy examples;
upstream entropy checks and allowlists can suppress such examples in Gitleaks.
No general high-entropy detector or Gitleaks `generic-api-key` rule is imported:
using its regex alone would overmatch identifiers and ordinary telemetry.
The secret-key assignments, authorization schemes and PEM rule are viewer
policy, not imported Gitleaks rules.

Updates require reviewing the upstream rule and its accompanying metadata,
updating the pinned provenance and fixtures, and running the redaction tests.
There is no runtime download or automatic ruleset update.
