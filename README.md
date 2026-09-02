> **Migration notice:** This repository was previously a one-way published
> export of a privately hosted project; early history contains squashed
> "Sync from internal source ..." commits from that period. It has since been
> migrated: `origin` is now `github.com/maccavelli/mcplib`, `main` tracks it
> directly, and normal commits/PRs land here going forward.

# mcplib

Shared Go library for MCP servers and related tools in this monorepo.

## Packages

| Package | Role |
|---------|------|
| `mcplib` (root) | Server helpers, stdio transport, prompts, diagnostics, orchestrator utilities |
| `mcplib/llmprovider` | LLM provider adapters: OpenAI, Claude, Gemini, Grok, plus the OpenCode Zen/Go, Hugging Face and Kilo gateways |
| `mcplib/selfupdate` | Canonical CLI self-update: GitHub Releases discovery, exact assets, SHA-256 integrity, and locked replacement |
| `mcplib/schema` | Schema helpers |
| `mcplib/logging` | Redaction / sanitization helpers |
| `mcplib/fastpath` | Fast-path helpers |
| `mcplib/hfsc` | HFSC-related dispatch helpers |

Module path is the short name `mcplib` (not a public module path). Consumers in this repo depend on it via workspace + replace (see below).

## How resolution works

Two mechanisms keep local multi-module development working:

1. **Repo-root `go.work`** — lists `./scripts/go/mcplib` so tools opened at the monorepo root see the live tree.
2. **Per-consumer `replace mcplib => ../mcplib`** in each app `go.mod` — so builds still resolve when a single module is built outside the workspace (CI/subtree layouts).

Do not remove one without understanding the other. Both are intentional.

## Quality targets

```bash
# From repo root:
make -C scripts/go/mcplib test
make -C scripts/go/mcplib lint
make -C scripts/go/mcplib fmt vet tidy

# Opt-in (requires tools in GOBIN / PATH):
make -C scripts/go/mcplib vuln      # govulncheck (exits non-zero when findings exist)
make -C scripts/go/mcplib test-sum  # gotestsum
```

`make vuln` is informational: `govulncheck` returns a non-zero status when it reports issues (including stdlib fixes in newer Go patch releases). It is not part of the default `test`/`lint` gate.

Fleet shortcuts:

```bash
make -C scripts/go test-lib lint-lib
```

There is no `build` / `install` / `vendor` target: this module is a library only.

## Self-update releases

`mcplib/selfupdate` is the shared CLI updater. Consumers publish GitHub
Releases through the reusable workflow
`.github/workflows/publish-selfupdate-release.yml`, pinned to the mcplib
module tag's commit SHA. The workflow validates exact
`<product>-<goos>-<goarch>[.exe]` assets plus `SHA256SUMS`, attests the
staged files, and publishes only an immutable complete release. It never
uses `--clobber`. Runtime verification is release-asset integrity, not
publisher signature authenticity.

## When you change mcplib

1. `make -C scripts/go/mcplib test lint`
2. Re-test consumers that import the changed packages (or `make -C scripts/go test-mcp` for a full MCP sweep).

## Lint config

Uses the fleet file `scripts/go/.golangci.yml` (same as app modules).
