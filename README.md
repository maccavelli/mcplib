> **Mirror notice:** This repository is a one-way published export of a
> privately hosted project. History is squashed into sync snapshots, and pull
> requests cannot be merged here directly — open an issue instead. Changes
> land in the private source and are re-exported.

# mcplib

Shared Go library for MCP servers and related tools in this monorepo.

## Packages

| Package | Role |
|---------|------|
| `mcplib` (root) | Server helpers, stdio transport, prompts, diagnostics, orchestrator utilities |
| `mcplib/llmprovider` | LLM provider adapters (OpenAI / Claude / Gemini style discovery) |
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

## When you change mcplib

1. `make -C scripts/go/mcplib test lint`
2. Re-test consumers that import the changed packages (or `make -C scripts/go test-mcp` for a full MCP sweep).

## Lint config

Uses the fleet file `scripts/go/.golangci.yml` (same as app modules).
