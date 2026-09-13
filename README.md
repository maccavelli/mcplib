> **Migration notice.** This repository was previously a one-way published
> export of a privately hosted project; early history contains squashed
> “Sync from internal source …” commits from that period. `origin` is now
> [`github.com/maccavelli/mcplib`](https://github.com/maccavelli/mcplib),
> `main` tracks it directly, and normal commits/PRs land here.

# mcplib

Shared Go library for a fleet of [MCP](https://modelcontextprotocol.io) servers
and related CLIs. It exists so those programs do not each invent stdio
transport, tool hardening, LLM access, configuration, logging, or self-update.

The module path is `github.com/maccavelli/mcplib`. It is a library only: no
binary, no `build`/`install` target. Consumers import packages; they do not
vendor copies of the same primitives.

Requires Go 1.26.6.

## Why

The fleet (MagicTools and its sub-servers, MagicDev, Recall, Socratic Thinker,
prepare-commit-msg, and siblings) used to re-implement the same MCP and LLM
plumbing. The copies drifted: different provider menus, stale model catalogs,
incompatible stdio shutdown, two CLI updaters, and none at all in other
products.

mcplib is the ownership boundary for that shared behavior. Adding a provider,
a redaction pattern, or an asset-naming rule here is how every consumer gets
it. Re-implementing those pieces in an app is the defect this library exists
to prevent.

## How it is designed to work

Two runtime modes, one codebase.

**Standalone.** A process talks MCP over stdio to an IDE or agent. LLM calls
go through `llmprovider` with the process’s own credentials. Large payloads
stay in JSON-RPC (bounded). Self-update, when wired, talks to GitHub Releases.

**Orchestrated.** MagicTools spawns the same binary as a sub-server and injects
environment:

| Variable | Meaning |
| --- | --- |
| `MCP_ORCHESTRATOR_OWNED=true` | This process is fleet-managed. |
| `MCP_LLM_ENABLED=true` | Shared LLM backplane is available. |
| `MCP_LLM_ADDR` / `MCP_LLM_TOKEN` | Backplane endpoint and bearer token. |

`IsOrchestratorOwned()` and `NewBackplaneClient` are the only supported
detectors. When the env is absent they report “not available”; callers degrade,
they do not fail the process. Individual servers must not re-bind these
variables through their own config stacks.

The rest of the design is the same in both modes:

* **Canonical implementations, injected seams.** The library owns data and
  flow. Consumers own rendering and product lifecycle. `wizard.Prompter`,
  `selfupdate.Reporter` / `Confirmer` / `Installer`, and
  `WithSchemaDerival` exist so mcplib never imports a TUI toolkit, a CLI
  framework, or a service manager.
* **Hardened handlers.** `HardenedAddTool` always recovers panics, keeps
  result content non-nil, and, when orchestrated, appends an
  `__orchestrator_signal` for pipeline scoring. Schema derivation and
  serialized calls are opt-in.
* **Payload bypasses for size.** JSON-RPC is the default. Under the
  orchestrator, `fastpath` can write a tool result to a bounded artifact
  file, and `hfsc` can stream a large body as session log chunks, so the
  RPC frame never holds the payload.
* **Secrets stay in one place.** `logging.Redact` is the single redaction
  path for log buffers, sanitizing writers, and MCP log notifications.
  `logging.MaskSecret` is the opposite: a UI fingerprint (`••••••••a75y`),
  never for logs.
* **Optional dependencies stay in subpackages.** Root `mcplib` wraps the
  official MCP Go SDK. `schema` is the only package that imports
  `invopop/jsonschema`. `llmprovider` speaks HTTP itself — no vendor SDKs.

A typical server constructs `NewMCPServer`, wraps stdin/stdout with
`NewStdioPipeline` (128 KiB buffers, real-close on EOF, mutex-flushed
writes), registers tools through `HardenedAddTool`, and optionally attaches
a `LogBuffer` plus `RegisterDiagnosticTool` for `get_internal_logs`.
Peer-server access uses `RecallClient` / `SocraticClient` (Streamable HTTP,
circuit breaker, reconnect). Shared LLM, when orchestrated, uses
`BackplaneClient` (nil if the backplane is off).

## Packages

| Package | Role |
| --- | --- |
| `mcplib` | Server wrapper, stdio pipeline, hardened tools/prompts/resources, orchestrator detection, backplane client, Recall/Socratic clients, diagnostic log tool, pagination, async stderr writer |
| `mcplib/llmprovider` | SDK-free LLM adapters, descriptors, static catalogs, live discovery, health probes |
| `mcplib/wizard` | Canonical “choose provider / key / model / fallbacks” flow behind `Prompter` |
| `mcplib/selfupdate` | GitHub Releases discovery, exact assets, SHA-256 integrity, locked binary replacement |
| `mcplib/schema` | Opt-in JSON Schema derivation for tool inputs |
| `mcplib/logging` | Secret redaction, sanitizing writer, UI key masking |
| `mcplib/fastpath` | Orchestrator artifact writes that skip JSON-RPC size limits |
| `mcplib/hfsc` | High-Fidelity Smart Chunking: stream extreme payloads as session logs |

## LLM providers

`NewProvider(name, apiKey, model, opts...)` is the factory. Every provider
implements `Generate`; optional interfaces add tools, extended thinking, and
the sealed `Item` contract (`MessageItem`, `FunctionCallItem`,
`FunctionCallOutputItem`, `ReasoningItem`) so callers switch on types instead
of parsing vendor JSON.

Registered names: `gemini`, `openai`, `claude`, `grok`, `opencode-zen`,
`opencode-go`, `huggingface`, `kilo`, `ollama`. Ollama is the only one that
needs no API key. Gateways that speak Chat Completions share one encoder;
OpenCode still routes some models onto Responses-shaped or Anthropic/Gemini
wires.

`Descriptors()` is what a configuration UI reads — labels, env vars, default
base URLs, static model lists. Adding a provider is one descriptor plus a
constructor; wizards built on `wizard.ConfigureLLM` pick it up without a
downstream edit. `ConfigureLLM` never writes config and never logs a key.
Consumers persist the returned `Result` in their own schema.

Retries are opt-in (`GenerateWithRetry`). Typed sentinels
(`ErrRateLimited`, `ErrAuthFailure`, `ErrInvalidRequest`,
`ErrProviderUnavailable`) classify failures; 401/403 and other 4xx are not
retried.

## Self-update

`selfupdate` discovers GitHub Releases, selects the exact asset
`<product>-<goos>-<goarch>[.exe]`, checks it against `SHA256SUMS` (and the
GitHub digest when present), and replaces the running binary under a lock.
It does not prove publisher signature authenticity.

The library never reads flags or calls `os.Exit`. The consumer binds:

* a **source** (GitHub),
* a **version policy** (strict `vMAJOR.MINOR.PATCH`),
* an **asset selector**,
* an **installer** (`StandaloneInstaller` for a plain binary;
  `ManagedInstaller` when a service must stop/start around the replace),
* a **reporter** and **confirmer**.

Consumers publish through the reusable workflow
`.github/workflows/publish-selfupdate-release.yml`, pinned to the mcplib
module-tag commit. It accepts only a complete staged set, attests the files,
and publishes an immutable release. It never `--clobber`s. That workflow is
the only supported publication path for the asset contract.

## Develop

```bash
make test          # go test ./...
make lint          # golangci-lint via .golangci.yml
make fmt vet tidy
```

Opt-in: `make vuln` (govulncheck; non-zero when findings exist, including
stdlib fixes in newer Go patches) and `make test-sum` (gotestsum).

Architectural decisions live in [`docs/`](docs/). After changing this
module, re-test the consumers that import the packages you touched.
