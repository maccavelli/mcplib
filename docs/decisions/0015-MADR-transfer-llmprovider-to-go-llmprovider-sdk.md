---
status: accepted
date: 2026-09-29
decision-makers: mcplib maintainers
consulted: go-llmprovider-sdk maintainers
informed: all mcplib consumers; mcp-server-magicdev, mcp-server-magictools, prepare-commit-msg
---
# Transfer `llmprovider`, `wizard` and Their Records to go-llmprovider-sdk, and Remove Them from mcplib at v1.7.0

## Context and Problem Statement

`mcplib` holds two packages that are not about MCP:
* `llmprovider/`, the LLM provider adapters, OAuth and model discovery;
* `wizard/`, the provider configuration flow.

They also carry most of this repository's decision records. The repository
`github.com/maccavelli/go-llmprovider-sdk` has accepted a decision to become
their home:
* go-llmprovider-sdk `docs/decisions/0002-MADR-migrate-llmprovider-from-mcplib.md`
  (`status: accepted`, 2026-09-29), and its PLAN
  `docs/decisions/0002-PLAN-migrate-llmprovider-from-mcplib.md`.
* The feasibility evidence is in go-llmprovider-sdk
  `docs/reports/0001-REPORT-llmprovider-extraction-feasibility.md`.

Those records cannot be linked from here; they are cited by repository and
name.

That decision moves the code, its history and its records out, but it cannot
authorise changes to *this* repository. This record decides what `mcplib`
does:
* the freeze;
* the releases;
* which records leave and which stay;
* how a reader of `mcplib` finds what moved;
* what is left behind, and in what state.

Facts, read at `4e1f9a5` (= `v1.6.0`) on 2026-09-29:

* **Nothing in `mcplib` depends on the two packages.**
  * No Go file outside `llmprovider/` and `wizard/` imports or names them.
  * The only other reference is a test comment citing
    `0012-MADR-conform-providers-to-reference-clients.md` revision 4
    (`backplane_test.go:254-255`).
  * `README.md` describes them (lines 10-11, 22-28, 36-37, 57, 75, 90-91 and
    the whole "LLM providers" section, lines 98-174).
  * `AGENTS.md` names them (lines 8-9).
* **The packages depend on `mcplib`** only through `mcplib/logging` (for
  `RedactString` and `MaskSecret`) and one call to `IsOrchestratorOwned()`.
  Both stay here; the SDK carries its own copy of the two logging functions
  and takes orchestration from its caller.
* **Importers.** Of the 13 fleet repositories that require `mcplib`, only
  three import these packages: `mcp-server-magicdev`,
  `mcp-server-magictools` and `prepare-commit-msg`. Each keeps importing
  other `mcplib` packages. Each migrates under its own record, before this
  repository removes anything.
* **Records.** Of the 37 records under `docs/`:
  * 26 move to go-llmprovider-sdk and are renumbered there. The full table
    is in §4.
  * `docs/decisions/0010-*-windows-stdio-oauth-tokenstore-ci.md` is mixed:
    its D1–D2, D14–D18, P1 and P8 are this repository's Windows stdio and CI
    work. It is copied, and each copy is amended to its own scope.
  * `docs/0012-PLAN-circuit-breaker-test.md` changes only
    `backplane_test.go`, so it stays, although its MADR moves.
  * The 0002, 0005, 0006 and 0007 pairs are `mcplib`-only and stay.
* **Open work in the moving records** (the ranking plan, the OAuth-loopback
  plan, the LLM part of the Windows/token-store plan) is transferred by the
  go-llmprovider-sdk PLAN. None of it is executed in `mcplib`.
* **Numbering history.** This repository numbered two different records
  0009 and two 0010 (`docs/` versus `docs/decisions/`). After the move only
  `docs/decisions/0010-*` remains here with either number.
* **Process rules in the way.** `AGENTS.md` lines 74-79 say "never leave a
  gap deliberately" and "do not relocate historical files as part of writing
  a new pair". Deleting the moved records frees eight numbers entirely:
  0001, 0003, 0004, 0008, 0009, 0011, 0013 and 0014. That is a
  deliberate, recorded exception, and `AGENTS.md` must say so. Line 76 also
  still says legacy `docs/` holds 0001–0008; it holds 0001–0013.

## Decision Drivers

* **`mcplib` becomes MCP-only.** It keeps the server, stdio, hardening,
  backplane, orchestrator detection, logging, schema, fastpath, hfsc and
  self-update. No LLM-provider surface stays.
* **No known importer breaks.** Every fleet importer migrates first, and an
  unknown importer is warned before it is broken.
* **A reader of `mcplib` can still find every moved decision** from its old
  number.
* **What stays is left true.** Records that remain cite the moved ones by
  repository and name, and never by a path that no longer resolves.
* **This repository's rules are changed explicitly** where the move
  contradicts them, not ignored.

## Considered Options

* **A. Freeze now, deprecate in `v1.6.1`, remove in `v1.7.0`, and delete the
  moved records behind a relocation table.**
* **B. Keep forwarding packages** (type aliases, re-exported functions and
  sentinels) for the whole v1 line.
* **C. Remove in `mcplib` `v2.0.0`.**
* **D. Remove the code but keep every record here,** frozen, as history.

## Decision Outcome

Chosen option: "A", because it leaves `mcplib` MCP-only and keeps every
fleet importer working. The go-llmprovider-sdk migration sequences all three
consumers before `v1.7.0`. An unknown importer is told by `v1.6.1`'s
deprecation notice, and later by a compile error, never by a silent
behaviour change.

### 1. Freeze

From `F` = `4e1f9a5` (`v1.6.0`), no commit changes `llmprovider/` or
`wizard/` except the two changes this record makes:
* the deprecation notice (§2);
* the removal (§3).

`logging/` is **not** frozen. It stays here and may evolve;
go-llmprovider-sdk's `internal/redact` is a separate copy and is not kept in
step.

### 2. `v1.6.1`: deprecation only

`llmprovider` and `wizard` each gain a package-doc paragraph:
`Deprecated: use github.com/maccavelli/go-llmprovider-sdk/<pkg>.` Nothing
else changes. Tagged after go-llmprovider-sdk `v1.0.0` exists, so the notice
points somewhere real.

### 3. `v1.7.0`: removal

* `llmprovider/` and `wizard/` are deleted once `mcp-server-magicdev`,
  `mcp-server-magictools` and `prepare-commit-msg` have completed their
  migration records, and a fleet-wide grep finds no importer.
* `go.mod` keeps `golang.org/x/term` for `selfupdate`.
* This is a deliberate exception to semantic versioning inside v1. Consumers
  pinned to `v1.6.x` or earlier are unaffected; the module proxy keeps
  serving those versions.

### 4. Records

**Deleted from `mcplib`** (history remains in git and is imported, with
authorship, into go-llmprovider-sdk):

| `mcplib` path | go-llmprovider-sdk path |
|---|---|
| `docs/0001-MADR-add-grok-xai-llm-provider.md` | `docs/decisions/0003-MADR-add-grok-xai-llm-provider.md` |
| `docs/0001-PLAN-add-grok-xai-llm-provider.md` | `docs/decisions/0003-PLAN-add-grok-xai-llm-provider.md` |
| `docs/0003-MADR-add-gateway-llm-providers.md` | `docs/decisions/0004-MADR-add-gateway-llm-providers.md` |
| `docs/0003-PLAN-add-gateway-llm-providers.md` | `docs/decisions/0004-PLAN-add-gateway-llm-providers.md` |
| `docs/0004-MADR-canonicalize-llm-provider-configuration.md` | `docs/decisions/0005-MADR-canonicalize-llm-provider-configuration.md` |
| `docs/0004-PLAN-canonicalize-llm-provider-configuration.md` | `docs/decisions/0005-PLAN-canonicalize-llm-provider-configuration.md` |
| `docs/0008-MADR-subscription-auth-for-llm-providers.md` | `docs/decisions/0006-MADR-subscription-auth-for-llm-providers.md` |
| `docs/0008-PLAN-subscription-auth-for-llm-providers.md` | `docs/decisions/0006-PLAN-subscription-auth-for-llm-providers.md` |
| `docs/0009-MADR-live-catalog-model-search.md` | `docs/decisions/0007-MADR-live-catalog-model-search.md` |
| `docs/0009-PLAN-live-catalog-model-search.md` | `docs/decisions/0007-PLAN-live-catalog-model-search.md` |
| `docs/decisions/0009-MADR-repair-oauth-loopback-and-session-wiring.md` | `docs/decisions/0008-MADR-repair-oauth-loopback-and-session-wiring.md` |
| `docs/decisions/0009-PLAN-repair-oauth-loopback-and-session-wiring.md` | `docs/decisions/0008-PLAN-repair-oauth-loopback-and-session-wiring.md` |
| `docs/0010-MADR-use-case-aware-default-model-ranking.md` | `docs/decisions/0009-MADR-use-case-aware-default-model-ranking.md` |
| `docs/0010-PLAN-use-case-aware-default-model-ranking.md` | `docs/decisions/0009-PLAN-use-case-aware-default-model-ranking.md` |
| `docs/0011-REPORT-provider-source-compatibility-audit.md` | `docs/reports/0011-REPORT-provider-source-compatibility-audit.md` |
| `docs/0012-MADR-conform-providers-to-reference-clients.md` | `docs/decisions/0012-MADR-conform-providers-to-reference-clients.md` |
| `docs/0012-PLAN-chatgpt-backend.md` | `docs/decisions/0012-PLAN-chatgpt-backend.md` |
| `docs/0012-PLAN-gateway-conventions.md` | `docs/decisions/0012-PLAN-gateway-conventions.md` |
| `docs/0012-PLAN-grok.md` | `docs/decisions/0012-PLAN-grok.md` |
| `docs/0012-PLAN-item-fidelity.md` | `docs/decisions/0012-PLAN-item-fidelity.md` |
| `docs/0012-PLAN-oauth-hygiene.md` | `docs/decisions/0012-PLAN-oauth-hygiene.md` |
| `docs/0012-PLAN-shared-transport.md` | `docs/decisions/0012-PLAN-shared-transport.md` |
| `docs/0013-MADR-remediate-debugging-pass-findings.md` | `docs/decisions/0013-MADR-remediate-debugging-pass-findings.md` |
| `docs/0013-PLAN-remediate-debugging-pass-findings.md` | `docs/decisions/0013-PLAN-remediate-debugging-pass-findings.md` |
| `docs/decisions/0014-MADR-gemini-wire-fidelity.md` | `docs/decisions/0014-MADR-gemini-wire-fidelity.md` |
| `docs/decisions/0014-PLAN-gemini-wire-fidelity.md` | `docs/decisions/0014-PLAN-gemini-wire-fidelity.md` |

**Copied, and both copies amended to scope:**
`docs/decisions/0010-MADR-windows-stdio-oauth-tokenstore-ci.md` and its
PLAN.
* **Here**, a dated amendment says: D1–D2 (stdio), D14–D18 (CI), P1 and P8
  remain in scope. D3–D13 and P2–P7 are transferred to go-llmprovider-sdk
  `docs/decisions/0010-*`.
* The go-llmprovider-sdk copy says the reverse.
* Nothing above the amendment is rewritten.

**Kept, with the link to its moved MADR made a repository-named citation:**
`docs/0012-PLAN-circuit-breaker-test.md`.
* `associated-madr` becomes
  `"go-llmprovider-sdk docs/decisions/0012-MADR-conform-providers-to-reference-clients.md"`.
* Its body link becomes text, since a relative link cannot reach another
  repository.
* A dated note says why.

**Kept unchanged:** the 0002, 0005, 0006 and 0007 MADR/PLAN pairs.
* They cite no moved record.
* 0005-PLAN's G1 deviation (lines 1366-1398) edited
  `wizard/configure_test.go` in commit `3e64e30`. That history stays true
  and is not rewritten.

**Relocation index.** A new `docs/README.md` indexes the records that stay.
It reproduces the table above and notes that 0009 and 0010 were each
used twice. It lists the numbers now unused: 0001, 0003, 0004, 0008, 0009,
0011, 0013 and 0014. It also lists the two numbers kept only in part:
* 0010, by `docs/decisions/0010-*`;
* 0012, by `docs/0012-PLAN-circuit-breaker-test.md`.

The next number stays the highest ever used plus one (after this record:
0016).

### 5. Text that stays is corrected

* **`README.md`:**
  * Remove the `llmprovider` and `wizard` rows and text (lines 36-37, 57,
    75, 90-91) and the "LLM providers" section (lines 98-174).
  * Reword lines 10-11, 22-24 and 27-28 so the library's stated purpose no
    longer includes LLM access.
  * Add one sentence naming go-llmprovider-sdk for provider access.
  * The orchestrated-mode text and backplane (lines 34-52, 77-83, 89),
    logging (69-72), self-update and develop sections stay.
* **`AGENTS.md`:**
  * Remove the sentence at lines 8-9.
  * Correct line 76's legacy range to 0001–0013.
  * Add, beside lines 74-79, the relocation exception this record makes,
    citing it.
* **`backplane_test.go:254-255`:** cite "go-llmprovider-sdk
  `0012-MADR-conform-providers-to-reference-clients.md` revision 4".

### 6. What does not change

* `backplane.go`, `IsOrchestratorOwned`, `logging/` (including `Redact` and
  `MaskSecret`), `selfupdate/`, `schema/`, `fastpath/`, `hfsc/` and CI are
  untouched by this decision.
* The open `decisions/0010` P1 and P8 stay open here.

### Consequences

* Good, because `mcplib`'s packages, README and records then describe one
  purpose.
* Good, because every moved record is reachable from its old path through
  `docs/README.md`, and its git history is still here.
* Good, because the one test comment and the one plan that cite a moved
  record keep pointing at it by name.
* Neutral, because `mcplib` does not depend on go-llmprovider-sdk at any
  point. The dependency runs only from consumers to both.
* Bad, because `v1.7.0` removes public packages in a minor release, which is
  a recorded semantic-versioning exception.
* Bad, because the record sequence here gets eight deliberate gaps, which
  contradicts `AGENTS.md` until §5 amends it.
* Bad, because the Windows/token-store record now exists in two
  repositories. Each copy is authoritative only for its amended scope.

### Confirmation

* **After `v1.6.1`,** a scratch program importing `mcplib/llmprovider` is
  flagged by staticcheck `SA1019`.
* **After `v1.7.0`:**
  * `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` (empty),
    `go mod tidy -diff` and `make lint` pass.
  * `grep -rn 'llmprovider\|wizard' --include='*.go' .` is empty.
  * A link check over `docs/**` and `README.md` is clean.
  * Every row of the relocation table names a file that exists in
    go-llmprovider-sdk.

## Pros and Cons of the Options

### A. Freeze, deprecate in v1.6.1, remove in v1.7.0, relocate records

* Good, because it gives an MCP-only library with a warning release first.
* Good, because a reader lands on the new location from any old number.
* Bad, because it takes a semantic-versioning exception and leaves numbering
  gaps.

### B. Forwarding packages for the whole v1 line

* Good, because nothing ever breaks.
* Bad, because `mcplib` would depend on go-llmprovider-sdk indefinitely.
* Bad, because the forwarding `wizard` would have to re-inject
  `IsOrchestratorOwned`, keeping the LLM surface this decision exists to
  remove.

### C. Remove in v2.0.0

* Good, because it is strictly semantic-versioned.
* Bad, because all 13 importing repositories would rewrite to `/v2` for a
  change that concerns three of them.

### D. Remove the code, keep all records here

* Good, because it needs no gaps and no relocation.
* Bad, because the rationale would live in a repository that no longer has
  the code, while the code's citations point at the other repository's
  numbers. The two would disagree.

## More Information

* **Implementation:**
  [0015-PLAN-transfer-llmprovider-to-go-llmprovider-sdk.md](0015-PLAN-transfer-llmprovider-to-go-llmprovider-sdk.md).
* **Governing cross-repository record:** go-llmprovider-sdk
  `docs/decisions/0002-MADR-migrate-llmprovider-from-mcplib.md`. Its §10 is
  the record mapping, and §13 assigns this pair.
* **Revisit** if an importer outside the fleet surfaces before `v1.7.0`, in
  which case option B for one release is the fallback. Revisit also if a
  consumer's migration record is abandoned.
