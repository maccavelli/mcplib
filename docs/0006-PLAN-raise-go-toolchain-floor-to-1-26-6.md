---
status: accepted
date: 2026-09-02
associated-madr: 0006-MADR-raise-go-toolchain-floor-to-1-26-6.md
decision-makers: mcplib maintainers
---

# Implement the Go 1.26.6 Toolchain Floor Across the Fleet

> Paired with [0006-MADR-raise-go-toolchain-floor-to-1-26-6.md](0006-MADR-raise-go-toolchain-floor-to-1-26-6.md).
> This plan raises fifteen modules, eleven Makefile pins, one container image,
> and the developer toolchain to Go 1.26.6, publishes mcplib v1.4.0, and amends
> MADR/PLAN 0005.
> It does not authorize implementation, pushes, tags, or releases; each
> external mutation remains an explicit gate.

<!-- markdownlint-disable MD013 MD024 MD029 MD036 -->

## 0. Execution Contract

1. Do not change source, tooling, or the developer environment until both this
   plan and MADR 0006 are accepted.
2. Record each repository's HEAD and short status before touching it. Stop on an
   overlapping change; preserve unrelated owner work exactly.
3. End every mutating phase with a green repository and one commit per touched
   repository. Read-only gates record evidence in Section 9 without empty commits.
4. Do not push, tag, publish a release, or open a pull request without explicit
   authorization in the same turn. Plan approval is not that authorization.
5. Run each repository's own complete check target before staging. Where a Go
   pre-commit wrapper exists, run it for every changed file.
6. Record command output, commit IDs, and deviations in Section 9 **during**
   execution, not afterwards. PLAN 0005 Section 23 documents what retrospective
   backfill costs; do not repeat it.
7. No dependency upgrade other than the mcplib version bump belongs in these
   commits. A `go.sum` change not explained by that bump stops the phase.

## 1. Verified Baseline

Established read-only on 2026-09-02.

### 1.1 Exposure

`govulncheck` v1.7.0 against unmodified trees:

| Repository | Result |
|---|---|
| mcplib | `make vuln` -> Error 3; 4 reachable stdlib advisories |
| prepare-commit-msg at a 1.26.5 pin | `make verify` -> Error 3 at the vuln step |

Advisories: GO-2026-6218 (`net/url`), GO-2026-6090 (`crypto/tls`),
GO-2026-5972 (`encoding/asn1`), GO-2026-5026 (`net/http`). All fixed in go1.26.6.

### 1.2 Module inventory

Fifteen Go modules; twelve import mcplib. Fourteen declare `go 1.26.5`; only
prepare-commit-msg declares `go 1.26.6`. The three non-importers are mcplib
itself, magic-cli-remote, and mcp-buntdb.

| Module | `go` | mcplib pin | Phase |
|---|---|---|---|
| mcplib | 1.26.5 | self | 2 |
| magic-cli-remote | 1.26.5 | none | 3 |
| mcp-server-recall | 1.26.5 | v1.2.0 | 4 |
| mcp-server-magictools | 1.26.5 | v1.2.0 | 4 |
| mcp-server-socratic-thinker | 1.26.5 | v1.2.0 | 4 |
| mcp-server-duckduckgo | 1.26.5 | v1.2.0 | 4 |
| mcp-buntdb | 1.26.5 | none | 5 |
| mcp-server-brainstorm | 1.26.5 | v1.2.0 | 5 |
| mcp-server-evolve-plan | 1.26.5 | v1.2.0 | 5 |
| mcp-server-filesystem | 1.26.5 | v1.2.0 | 5 |
| mcp-server-go-modernizer | 1.26.5 | v1.2.0 | 5 |
| mcp-server-magicdev | 1.26.5 | v1.2.0 | 5 |
| mcp-server-magicskills | 1.26.5 | v1.2.0 | 5 |
| mcp-server-sequential-thinking | 1.26.5 | v1.2.0 | 5 |
| prepare-commit-msg | 1.26.6 | v1.3.0 | 6 |

`docs/` and `scripts/` are the only other workspace directories and contain no
`go.mod`. Confirm both facts at execution time and record the result.

### 1.3 Non-`go.mod` assertions

| Location | Current | Target |
|---|---|---|
| eleven `Makefile:1` `MOD_VERSION` | `1.26.5` | `1.26.6` |
| `prepare-commit-msg/Makefile:1` | `1.26.6` | unchanged |
| mcplib, magic-cli-remote, mcp-buntdb Makefiles | no `MOD_VERSION` | nothing to raise |
| `magic-cli-remote/scripts/test-linux-arm64.sh:33` | `golang:1.26.5` | `golang:1.26.6` |
| `prepare-commit-msg/scripts/bootstrap-tools.sh:10` | `go1.26.6` | unchanged |
| `prepare-commit-msg/README.md:240` | `(Go 1.26.6)` | unchanged |
| `~/.local/bin/go` symlink | `~/.local/go1.26.5/bin/go` | `~/.local/go1.26.6/bin/go` |
| `~/Library/Application Support/go/env` | `GOTOOLCHAIN=local` | unchanged, deliberately |

No CI workflow pins a literal Go version; all seven use `go-version-file: go.mod`.
**No workflow edit is required by this plan.** A workflow diff in any phase is a
deviation.

### 1.4 Already correct

* prepare-commit-msg's floor, bootstrap pin, `MOD_VERSION`, and README prose are
  at 1.26.6 and are not edited.
* Its published v1.1.1-v1.1.3 binaries were built on 1.26.6 and need no action.
* A `go1.26.6` toolchain is already in the module cache at
  `~/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.6.darwin-arm64`.

## 2. Goal, Scope, and Non-goals

**Goal.** All fifteen modules, every tooling pin, and the developer toolchain
build on Go 1.26.6; no reachable standard-library advisory remains in any repository that
measures for one; mcplib v1.4.0 carries the new floor.

**In scope.** `go` directives; `MOD_VERSION`; the ARM64 container image; the
developer toolchain install and symlink; mcplib v1.4.0; the mcplib repin in
prepare-commit-msg; MADR/PLAN 0005 amendments; a repeatable future-bump procedure.

**Out of scope.** Rebuilding or replacing published releases; any CI workflow
change; any dependency upgrade other than mcplib; adding vulnerability gates to
repositories that lack them; PLAN 0005's own remaining phases 7-12 and Gate G2;
raising any module above 1.26.6.

## 3. Phase 0 — Accept the Decision Artifacts

**Files.** `docs/0006-MADR-raise-go-toolchain-floor-to-1-26-6.md`,
`docs/0006-PLAN-raise-go-toolchain-floor-to-1-26-6.md`.

**Steps.** Set both statuses to `accepted` on owner approval. Commit in mcplib.

**Acceptance.** Both files committed; identifiers and slugs mirror; the MADR
links this plan and this plan links the MADR.

## 4. Phase 1 — Move the Developer Toolchain

This phase is first. `GOTOOLCHAIN=local` means nothing can build at 1.26.6 until
it completes, so every later phase's verification depends on it.

**Steps.**

1. Record the current state: `go version`, `go env GOROOT GOTOOLCHAIN`, and
   `ls -l ~/.local/bin/go`.
2. Install the go1.26.6 SDK to `~/.local/go1.26.6`, matching the existing
   `~/.local/go1.26.5` layout. Do not remove `~/.local/go1.26.5`; it is the
   rollback path.
3. Repoint the symlink: `~/.local/bin/go` -> `~/.local/go1.26.6/bin/go`.
4. Leave `GOTOOLCHAIN=local` in `~/Library/Application Support/go/env` unchanged.

**Verification.**

    go version                      # expect go1.26.6 darwin/arm64
    go env GOVERSION GOTOOLCHAIN    # expect go1.26.6 and local
    ls -l ~/.local/bin/go

**Acceptance.** `go version` reports go1.26.6, `GOTOOLCHAIN` is still `local`,
and `~/.local/go1.26.5` remains installed and untouched.

**Note.** This mutates the developer environment rather than a repository and
therefore produces no commit. Record its evidence in Section 9.

## 5. Phase 2 — Raise mcplib and Amend the 0005 Documents

**Files.** `go.mod`,
`docs/0005-MADR-canonicalize-cli-self-update-in-mcplib.md`,
`docs/0005-PLAN-canonicalize-cli-self-update-in-mcplib.md`.

**Steps.**

1. `go.mod:3` — `go 1.26.5` -> `go 1.26.6`. mcplib's Makefile has no
   `MOD_VERSION`; there is nothing else to raise in this repository.
2. Run `go mod tidy`. Any `go.sum` change stops the phase as a deviation.
3. Amend MADR 0005: add a dated addendum recording that its "compatibility floor
   is Go 1.26.5" assertion is superseded by MADR 0006, with the reason. Do not
   rewrite the original sentence.
4. Amend PLAN 0005: annotate Section 1.1's Go-directive column, change Global
   Acceptance criterion 1 to Go 1.26.6, and add a dated deviation entry stating
   that remaining consumer phases pin mcplib v1.4.0 rather than v1.3.0. Strike
   through rather than delete the superseded text.

**Verification.**

    go build ./... && go vet ./... && go test ./...
    make vuln
    git diff --check

`make vuln` must now report no reachable standard-library advisory. It failed
with Error 3 on this tree at 1.26.5 on 2026-09-02, so a pass here is a state
change and not an untested assertion.

**Acceptance.** mcplib builds, tests, and reports clean; the 0005 documents
record the superseded floor without losing their original text.

## 6. Gate G1 — Publish mcplib v1.4.0

External gate; blocks Phase 6 only.

**Preconditions.** Phase 2 committed, mcplib clean, CI green on Linux, macOS, and
Windows, and `v1.4.0` absent locally and remotely.

**Commands, only after same-turn authorization.**

    git fetch origin --tags
    git ls-remote --tags origin refs/tags/v1.4.0
    git push origin main
    # after the pushed commit passes CI on all three runners:
    test "$(git rev-parse HEAD)" = "$(git ls-remote origin refs/heads/main | awk '{print $1}')"
    git tag -a v1.4.0 -m "mcplib v1.4.0"
    git push origin refs/tags/v1.4.0
    GOPROXY=https://proxy.golang.org go list -m github.com/maccavelli/mcplib@v1.4.0

**Rollback.** Do not move the tag. A defect after publication is fixed by
reverting on main and publishing v1.4.1 after review.

**Note.** v1.3.0 stays published and immutable. It is superseded, not replaced.

## 7. Phases 3-6 — Raise the Consumers

Each repository is its own phase commit. Raising a consumer's floor does not
require mcplib v1.4.0, so Phases 3, 4, and 5 may run before or after Gate G1.
Only Phase 6 depends on it.

**Per-repository steps.**

1. Record HEAD and `git status --short`. Stop on unrelated modification.
2. `go.mod` — `go 1.26.5` -> `go 1.26.6`.
3. `Makefile:1` — `MOD_VERSION := 1.26.5` -> `1.26.6` where present.
4. Run `go mod tidy`; an unexplained `go.sum` change stops the phase.
5. Run the repository's complete check target — `make verify` where it exists,
   otherwise `go build ./... && go vet ./... && go test ./...`.
6. Commit. Do not push.

**Phase 3 — magic-cli-remote.** Additionally set
`scripts/test-linux-arm64.sh:33` to `GO_IMAGE="${GO_IMAGE:-golang:1.26.6}"`.
Per its repository rules, run `make pre-add-check` with the explicit changed-file
list before staging, run `make race`, and commit with `git commit --no-edit`.
This repository gates on `govulncheck`; `make vuln` must report clean.

**Phase 4 — the four PLAN 0005 MCP servers.** mcp-server-recall,
mcp-server-magictools, mcp-server-socratic-thinker, mcp-server-duckduckgo. None
gates on `govulncheck`; run each one's `govulncheck ./...` manually and record
the result as evidence even though no gate enforces it. Socratic Thinker is on
branch `ci/harden-release-pipeline`; honour PLAN 0005 Section 16's branch
precondition before editing.

**Phase 5 — the eight remaining sibling modules.** mcp-buntdb,
mcp-server-brainstorm, mcp-server-evolve-plan, mcp-server-filesystem,
mcp-server-go-modernizer, mcp-server-magicdev, mcp-server-magicskills,
mcp-server-sequential-thinking. These are outside PLAN 0005 entirely; touch only
the floor and `MOD_VERSION`. mcp-buntdb has no `MOD_VERSION` and no mcplib
dependency, so its floor is its only change.

**Phase 6 — prepare-commit-msg.** Its floor, bootstrap pin, `MOD_VERSION`, and
README are already correct. The only change is `go get
github.com/maccavelli/mcplib@v1.4.0` plus `go mod tidy`, then `make verify`.
Requires Gate G1.

**Acceptance for Phases 3-6.** Every repository builds and tests green on
go1.26.6; every `govulncheck` run recorded; no workflow file modified.

## 8. Phase 7 — Fleet Audit and Bump Procedure

**Audit.** From the common parent directory:

    for d in */; do [ -f "$d/go.mod" ] && printf "%-32s %s\n" "$d" "$(grep -E '^go ' $d/go.mod)"; done
    grep -rn "1\.26\.5" */Makefile */scripts/*.sh */.github/workflows/*.yml 2>/dev/null

The first must show `go 1.26.6` for every module. The second must return no hit
outside historical documentation and execution records. Record both outputs.

**Negative test.** Confirm the tooling gate still gates, against a scratch copy
and never the working tree: copy `prepare-commit-msg/scripts/bootstrap-tools.sh`
to the scratch directory, assert the edit landed, set its `GO_VERSION` to a
version the local toolchain is not, run it, and require exit 1 with
`expected <x>, got go1.26.6`. This was demonstrated on 2026-09-02 in the inverse
direction (`expected go1.26.99, got go1.26.5`, exit 1).

**Bump procedure.** Record in this section, for reuse on the next Go patch:

1. install the SDK to `~/.local/go<version>` and repoint `~/.local/bin/go`;
2. raise every `go.mod` `go` directive and every `Makefile` `MOD_VERSION`;
3. raise `magic-cli-remote/scripts/test-linux-arm64.sh`'s `GO_IMAGE`;
4. raise `prepare-commit-msg/scripts/bootstrap-tools.sh`'s `GO_VERSION`;
5. run each repository's complete check target and `govulncheck ./...`;
6. publish a new mcplib minor release and repin consumers;
7. leave CI untouched — `go-version-file: go.mod` follows automatically.

**Acceptance.** The audit is clean, the negative test fails as designed, and the
procedure is recorded.

## 9. Execution Record

Populate **during** execution.

| Phase or gate | Status | Commit/release | Verification evidence | Deviation |
|---|---|---|---|---|
| 0 Decision artifacts | complete | (this commit) | both statuses set to accepted; filenames mirror; MADR links PLAN and PLAN links MADR; counts cross-checked against the workspace: 15 modules, 14 at go 1.26.5, 12 importing mcplib, 11 MOD_VERSION at 1.26.5 | none |
| 1 Developer toolchain | pending | | | |
| 2 mcplib floor + 0005 amendments | pending | | | |
| G1 mcplib v1.4.0 | pending authorization | | | |
| 3 magic-cli-remote | pending | | | |
| 4 PLAN 0005 MCP servers | pending | | | |
| 5 Sibling modules | pending | | | |
| 6 prepare-commit-msg repin | pending | | | |
| 7 Audit and bump procedure | pending | | | |

## 10. Global Acceptance Criteria

1. Every workspace `go.mod` declares `go 1.26.6`.
2. `go version` reports go1.26.6 and `go env GOTOOLCHAIN` still reports `local`.
3. `govulncheck ./...` reports no reachable standard-library advisory in mcplib,
   magic-cli-remote, and prepare-commit-msg, and the four PLAN 0005 MCP servers'
   manual runs are recorded.
4. `make verify` is green in every repository that defines it.
5. No CI workflow file was modified.
6. mcplib v1.4.0 resolves through the proxy and declares `go 1.26.6`.
7. MADR 0005's superseded floor assertion and PLAN 0005's criterion 1 and v1.4.0
   repin are recorded without deleting the original text.
8. No published release was rebuilt, replaced, or re-uploaded.
9. Every mutating phase has one green commit per touched repository, and every
   phase's evidence was recorded during execution.

## 11. Rollback and Recovery

**Developer toolchain.** `~/.local/go1.26.5` is retained; repoint
`~/.local/bin/go` back to it. No repository change is needed to build again.

**Before any push.** Revert only the affected phase commit and re-run that
repository's checks. Never reset or discard owner work.

**After mcplib v1.4.0.** Do not move the tag. Publish v1.4.1 after review.
Consumers may stay on v1.3.0, which remains published and immutable.

**If a repository fails on 1.26.6.** Stop that phase and report it as a
deviation with the failure output. Do not lower that repository alone; a split
floor is the condition this plan exists to remove.
