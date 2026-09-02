---
status: accepted
date: 2026-09-02
decision-makers: mcplib maintainers
consulted: mcplib, magic-cli-remote, prepare-commit-msg, and the twelve sibling modules that import mcplib
informed: all mcplib consumers
---

# Raise the Fleet Go Toolchain Floor to 1.26.6 and Assert It in One Place per Repository

<!-- markdownlint-disable MD013 MD024 -->

> Paired with [0006-PLAN-raise-go-toolchain-floor-to-1-26-6.md](0006-PLAN-raise-go-toolchain-floor-to-1-26-6.md).
> Amends the compatibility-floor assertion in
> [0005-MADR-canonicalize-cli-self-update-in-mcplib.md](0005-MADR-canonicalize-cli-self-update-in-mcplib.md).

## Context and Problem Statement

Go 1.26.5 ships four standard-library vulnerabilities that `govulncheck` reports as
*reachable* from this fleet's own code. All four are fixed in Go 1.26.6:

| Advisory | Package | Summary | Fixed in |
|---|---|---|---|
| GO-2026-6218 | `net/url` | quadratic complexity in `resolvePath` | go1.26.6 |
| GO-2026-6090 | `crypto/tls` | unbounded post-handshake messages accepted | go1.26.6 |
| GO-2026-5972 | `encoding/asn1` | missing maximum recursion depth | go1.26.6 |
| GO-2026-5026 | `net/http` | `x/net/idna` fails to reject ASCII-only Punycode labels | go1.26.6 |

Reachability was confirmed on 2026-09-02 against unmodified working trees:

```text
mcplib:              make vuln  -> Error 3, 4 reachable stdlib vulnerabilities
prepare-commit-msg:  make verify -> Error 3 at the vuln step (when pinned to 1.26.5)
```

mcplib's exposure is the most consequential. Its v1.3.0 `selfupdate` package is a
security-sensitive updater built directly on `net/http`, `crypto/tls`, and `net/url` —
precisely the three packages carrying reachable advisories. An updater that fetches and
verifies release binaries is the wrong component to compile against a standard library with
known reachable TLS and URL defects.

The exposure is fleet-wide but the response so far has not been. Of the fifteen Go modules
in this workspace, fourteen declare `go 1.26.5`. Only `prepare-commit-msg` declares
`go 1.26.6`, raised in commit `faea9f3` ("fix(cicd): restore secure green quality baseline",
2026-08-30) — which was exactly this security fix, applied to one repository and never
propagated. Twelve of the fifteen modules import `github.com/maccavelli/mcplib`; the three
that do not are mcplib itself, `magic-cli-remote`, and `mcp-buntdb`.

### The vulnerability is in the building toolchain, not the module graph

No dependency upgrade fixes this. The defects are in the standard library that compiles the
binary, so the fix is to *build with* Go 1.26.6. A module's `go` directive is its minimum
required toolchain version; raising it is what makes building with a vulnerable toolchain
impossible rather than merely discouraged. That distinction drives the decision: the security
benefit arrives when each repository builds on 1.26.6, and the directive raise is the
enforcement mechanism that keeps it there.

### Current version assertions

`go.mod` is already the single source of truth for every repository's CI. All seven
release-bearing repositories' workflows select Go with `go-version-file: go.mod`; none pins a
literal version in a workflow. Version assertions outside `go.mod` are:

| Location | Current value | Kind |
|---|---|---|
| Eleven repositories' `Makefile:1` `MOD_VERSION` | `1.26.5` | declared; referenced nowhere in those Makefiles |
| `prepare-commit-msg/Makefile:1` `MOD_VERSION` | `1.26.6` | already correct |
| mcplib, `magic-cli-remote`, `mcp-buntdb` | no `MOD_VERSION` | nothing to raise |
| `magic-cli-remote/scripts/test-linux-arm64.sh:33` | `GO_IMAGE="${GO_IMAGE:-golang:1.26.5}"` | container image for native ARM64 tests |
| `prepare-commit-msg/scripts/bootstrap-tools.sh:10` | `GO_VERSION="go1.26.6"` | hard equality gate; exits 1 on mismatch |
| `prepare-commit-msg/README.md:240` | prose "(Go 1.26.6)" | documentation |
| `~/Library/Application Support/go/env` | `GOTOOLCHAIN=local` | developer environment |
| `~/.local/bin/go` | symlink to `~/.local/go1.26.5/bin/go` | developer environment |

`GOTOOLCHAIN=local` is deliberate and is retained by this decision: it forbids silent
toolchain switching, so the declared floor is the toolchain that actually builds. Its
consequence is that the developer environment must be moved explicitly; nothing upgrades
itself.

Only mcplib, magic-cli-remote, and prepare-commit-msg gate on `govulncheck`, and only through
their Makefiles. The four MCP servers have no vulnerability gate, so they are equally exposed
and silent about it.

### Constraint inherited from MADR 0005

[MADR 0005](0005-MADR-canonicalize-cli-self-update-in-mcplib.md) states "The requested
compatibility floor is Go 1.26.5" and treats prepare-commit-msg's 1.26.6 as tolerable because
a library requiring 1.26.5 remains consumable by it. That assertion was correct when written
and is superseded here. MADR 0005's decision — canonical self-update behind policy seams — is
untouched; only its floor constraint changes.

## Decision Drivers

* No reachable standard-library vulnerability in any shipped binary.
* One asserted Go version per repository, in `go.mod`, with every other reference derived
  from it or matching it exactly.
* A uniform fleet: the same toolchain builds mcplib, its twelve consumers, and the developer
  environment, so a green local build predicts a green CI build.
* No mutation of published releases; superseding a vulnerable artifact means a new tag.
* Explicit, reviewable toolchain movement — `GOTOOLCHAIN=local` stays, so no build silently
  changes compiler.
* Honest gates: a vulnerability check that cannot pass is fixed, never filtered.

## Considered Options

* Raise every module, tooling pin, and the developer toolchain to Go 1.26.6
* Raise only the three repositories that gate on `govulncheck`
* Keep `go 1.26.5` and add a `toolchain go1.26.6` directive to each module
* Keep Go 1.26.5 everywhere and accept the reachable advisories

## Decision Outcome

Chosen option: "Raise every module, tooling pin, and the developer toolchain to Go 1.26.6",
because it is the only option that removes the reachable advisories from every artifact this
fleet ships rather than from the subset that happens to run a vulnerability gate, and because
a single uniform floor is the only state in which `GOTOOLCHAIN=local` gives a meaningful
guarantee about which compiler produced a binary.

Concretely:

* every `go.mod` in the workspace declares `go 1.26.6`;
* every `Makefile` `MOD_VERSION` declares `1.26.6`;
* `magic-cli-remote`'s native ARM64 container image becomes `golang:1.26.6`;
* `prepare-commit-msg`'s bootstrap gate keeps demanding an exact match and demands `go1.26.6`;
* the developer toolchain becomes `~/.local/go1.26.6` with `~/.local/bin/go` repointed, and
  `GOTOOLCHAIN=local` is retained;
* CI needs no change anywhere, because every workflow already derives its Go version from
  `go.mod`.

### Release consequence for mcplib

mcplib v1.3.0 is published declaring `go 1.26.5`. Raising the floor is visible to consumers —
a module requiring mcplib must itself allow 1.26.6 — so the raise ships as a new minor
release, **v1.4.0**, not a patch. Under MADR 0005's canonical release contract, v1.3.0 is
immutable and is not rebuilt; v1.4.0 supersedes it through the normal release path.

The twelve importing modules are unaffected until each upgrades mcplib. Eleven currently pin
mcplib v1.2.0; only prepare-commit-msg is on v1.3.0. Raising a consumer's own floor is
therefore independent of, and does not require, the mcplib release — which is what makes the
rollout order in the associated plan safe to interrupt at any phase boundary.

### Interaction with PLAN 0005

PLAN 0005 is in-progress: Phases 0-6 and Gate G1 are complete, Phases 7-12 and Gate G2 are
not. Its consumer phases instruct each repository to pin mcplib v1.3.0, and its Global
Acceptance criterion 1 requires the package to build and test with Go 1.26.5. Both change:
remaining consumer phases pin v1.4.0, and the criterion becomes Go 1.26.6. prepare-commit-msg,
already migrated at v1.3.0, takes the bump as ordinary dependency maintenance. The associated
plan owns those amendments; this record owns only the floor decision.

### Bump procedure

An exact pin means the fleet does not drift onto a newer patch by itself, which is the point,
but it also means a future Go security release requires a deliberate sweep. The associated
plan defines that sweep as a repeatable procedure rather than a one-off, so the next raise is
a re-run and not a rediscovery.

### Consequences

* Good, because four reachable standard-library advisories leave every binary this fleet
  builds, including the self-update client that fetches and verifies other binaries.
* Good, because one version is asserted per repository and every other reference matches it
  exactly, so there is no configuration in which CI and a developer build differ.
* Good, because retaining `GOTOOLCHAIN=local` keeps toolchain changes explicit and reviewable
  instead of implicit in whatever a machine happens to download.
* Good, because CI requires no edit: `go-version-file: go.mod` already makes `go.mod`
  authoritative in all seven release-bearing repositories.
* Neutral, because published releases are not rebuilt. prepare-commit-msg v1.1.1 through
  v1.1.3 were already built on 1.26.6; every other repository's next release is its first
  1.26.6 artifact.
* Bad, because mcplib needs a v1.4.0 release before any consumer can take the new floor
  together with the shared `selfupdate` package.
* Bad, because eight modules outside PLAN 0005's scope must also be raised — seven of them
  mcplib importers, plus `mcp-buntdb` — widening this work beyond the self-update programme.
* Bad, because an exact pin must be swept by hand on every future Go patch release. The plan's
  documented procedure reduces but does not remove that cost.

### Confirmation

* `govulncheck ./...` reports no reachable standard-library advisory in mcplib,
  magic-cli-remote, and prepare-commit-msg, and `make verify` is green where it exists.
* Every `go.mod` in the workspace declares `go 1.26.6`; a repository-wide search finds no
  remaining `1.26.5` assertion outside historical documentation and execution records.
* `go env GOVERSION` reports `go1.26.6` and `go env GOTOOLCHAIN` still reports `local`.
* `prepare-commit-msg/scripts/bootstrap-tools.sh` still exits 1 when the running toolchain
  differs from its pin, demonstrated against a scratch copy rather than the working tree.
* mcplib v1.4.0 resolves through the module proxy and declares `go 1.26.6`.
* PLAN 0005's remaining consumer phases and Global Acceptance criterion 1 name v1.4.0 and
  Go 1.26.6.

## Pros and Cons of the Options

### Raise every module, tooling pin, and the developer toolchain to Go 1.26.6

* Good, because no artifact this fleet builds retains a reachable advisory.
* Good, because uniformity makes `GOTOOLCHAIN=local` a guarantee rather than a coincidence.
* Good, because it generalizes the fix one repository already proved in `faea9f3`.
* Bad, because it requires an mcplib minor release and touches fifteen modules.
* Bad, because it commits to a manual sweep on each future Go patch.

### Raise only the three repositories that gate on `govulncheck`

* Good, because it fixes every repository that would currently report a failure.
* Good, because it is the smallest change that turns the visible gates green.
* Bad, because it fixes the repositories that *measure* exposure rather than those that
  *have* it; the four ungated MCP servers stay vulnerable and silent.
* Bad, because it entrenches a split fleet, which is the condition that let one repository's
  security fix go unpropagated for three days.

### Keep `go 1.26.5` and add a `toolchain go1.26.6` directive

* Good, because the consumer-visible floor stays 1.26.5, so mcplib needs no minor release.
* Good, because local and CI builds would select 1.26.6 where toolchain switching is allowed.
* Neutral, because `setup-go` honours a `toolchain` line read through `go-version-file`.
* Bad, because `GOTOOLCHAIN=local` forbids the switch the directive requests, so the fleet
  would have to abandon its deliberate no-silent-switching policy for the directive to work.
* Bad, because it is a request, not a requirement: a consumer building with 1.26.5 still
  compiles the vulnerable standard library. It documents an intent it cannot enforce.

### Keep Go 1.26.5 everywhere and accept the reachable advisories

* Good, because nothing changes and no release is needed.
* Bad, because four reachable advisories stay in every shipped binary, including the updater.
* Bad, because `make verify` cannot pass in three repositories; the only way to green it is to
  filter or delete the vulnerability check, which ships the defects and hides them behind a
  gate that no longer looks.

## More Information

### Primary Evidence

* `govulncheck` v1.7.0 run against mcplib and prepare-commit-msg on 2026-09-02.
* `prepare-commit-msg` commit `faea9f3`, 2026-08-30, raising that repository alone to 1.26.6.
* Workspace survey of fifteen `go.mod` files, twelve Makefiles, and seven CI workflows, 2026-09-02.
* [Go module reference — the `go` directive as minimum required toolchain](https://go.dev/ref/mod#go-mod-file-go)
* [Go toolchain selection and `GOTOOLCHAIN`](https://go.dev/doc/toolchain)
* [Go vulnerability database](https://pkg.go.dev/vuln/)

### Repository Evidence Index

* `mcplib/go.mod`, `mcplib/Makefile`, `mcplib/.github/workflows/ci.yml`
* `magic-cli-remote/go.mod`, `magic-cli-remote/scripts/test-linux-arm64.sh`
* `prepare-commit-msg/go.mod`, `prepare-commit-msg/Makefile`,
  `prepare-commit-msg/scripts/bootstrap-tools.sh`, `prepare-commit-msg/README.md`
* `mcp-server-recall/go.mod`, `mcp-server-magictools/go.mod`,
  `mcp-server-socratic-thinker/go.mod`, `mcp-server-duckduckgo/go.mod`
* `mcp-buntdb/go.mod`
* `mcp-server-brainstorm/go.mod`, `mcp-server-evolve-plan/go.mod`,
  `mcp-server-filesystem/go.mod`, `mcp-server-go-modernizer/go.mod`,
  `mcp-server-magicdev/go.mod`, `mcp-server-magicskills/go.mod`,
  `mcp-server-sequential-thinking/go.mod`
