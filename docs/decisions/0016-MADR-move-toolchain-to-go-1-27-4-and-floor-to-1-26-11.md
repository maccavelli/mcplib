---
status: accepted
date: 2026-11-17
decision-makers: mcplib maintainers
consulted: "govulncheck v1.9.0 (vulnerability database updated 2026-11-11 02:51 UTC) under four toolchains, 2026-11-17; go-selfupdate-lib's 0045-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md as the model; the consumers' go.mod files, read-only"
informed: the fleet's MCP servers, which require this module
---
# Build and test with Go 1.27.4 through the `toolchain` line, and raise the floor to `go 1.26.11`

## Context and Problem Statement

The owner's instruction of 2026-11-17, relayed by the coordinator, verbatim:

> "go-llmprovider-sdk, go-tui-lib, acp-go-sdk2, gobble-cli, magic-cli-remote, go-selfupdate-lib, mcp-server-duckduckgo, mcp-server-magictools, mcp-server-recall, mcp-server-sequential-thinking, mcplib, mcp-buntdb, and mcp-server-socratic-thinker all need to have their go floor pinned to 1.26.11, and go version/toolchain pinned to 1.27.4."

"Floor" is the `go` directive; "version/toolchain" is the `toolchain` line. A
read-only survey found that four of the thirteen need Go 1.27 and cannot
build under 1.26.11; mcplib is not one of them. The owner then ruled, by
label, on the survey: "One short record pair per repository, all approved
now through CI (Recommended)". As put: a pair modelled on go-selfupdate-lib's
is drafted in each ready repository, committed, executed and pushed to its
`main` on this approval, libraries first; the coordinator stops and returns
to the owner on any failure; tags and releases wait for the owner's word.

Everything in this record other than the quoted instruction and the quoted
label is evidence or the author's text.

**Evidence index** (HEAD `5f463da`, branch `main`, clean, level with its
upstream, 2026-11-17 unless stated).

* **E1, the pins [read].** `go.mod:3` is `go 1.26.6`; there is no `toolchain`
  line. `README.md:17` and `AGENTS.md:9` say "Requires Go 1.26.6." No other
  file names a 1.26 or 1.27 patch outside the records (a whole-tree `grep`).
  There is one module (`go.mod`), no `go.work`, no fixture modules, no
  Makefile Go-version variable and no changelog.
* **E2, CI [read].** `.github/workflows/ci.yml:19` sets up Go with
  `go-version-file: go.mod` (`actions/setup-go` pinned to v7.0.0). Its
  README at that pin: "If the `toolchain` directive is present, its version
  is used; otherwise, the action falls back to the `go` directive"
  (as quoted in go-selfupdate-lib's `0018-MADR-move-toolchain-to-go-1-27-2.md`
  from the README at commit `b7ad1da`; not re-fetched here). So after the
  change CI builds with 1.27.4, on ubuntu, macos and windows. CI has **no job
  that builds and tests on the floor**: a named gap, see Consequences.
* **E3, `govulncheck ./...` [observed, this Mac, `GOPROXY=off`].** v1.9.0,
  database updated 2026-11-11 02:51:11 UTC, at HEAD:

  | `GOTOOLCHAIN` | Exit | Result |
  | :--- | :--- | :--- |
  | `go1.27.3` | 3 | GO-2026-6859, found in `net/http@go1.27.3`, fixed in `go1.27.4` |
  | `go1.26.10` | 3 | GO-2026-6859, found in `net/http@go1.26.10`, fixed in `go1.26.11` |
  | `go1.27.4` | 0 | "No vulnerabilities found." |
  | `go1.26.11` | 0 | "No vulnerabilities found." |
* **E4, the reusable workflow [read in full].**
  `.github/workflows/publish-selfupdate-release.yml` (`workflow_call`) names
  no Go version and sets up no Go: its steps are tag checks, a checkout of
  the called workflow commit into `.mcplib-release-tools` (`:61-70`),
  shell scripts (`refuse-existing-release.sh`, `verify-selfupdate-release.sh`),
  `actions/download-artifact`, `gh release` calls and
  `actions/attest-build-provenance`. The only `uses:` lines are checkout,
  download-artifact and attest. No script it calls invokes `go`. It reads
  neither this repository's `go.mod` nor the caller's. So **callers' release
  builds do not change toolchain with this record or with any mcplib
  release**: a caller's Go version comes from the caller's own build jobs.
  This differs from go-selfupdate-lib, whose publish workflow builds tools
  from its own `go.mod`.
* **E5, who requires this module [read, read-only; no consumer touched].**

  | Repository | `go` line | requires mcplib | Calls the workflow |
  | :--- | :--- | :--- | :--- |
  | mcp-server-duckduckgo | `go 1.27.4` | `v1.6.0` | yes, pinned to commit `8d4d89f` (v1.4.1) |
  | mcp-server-magictools | `go 1.26.6` | `v1.4.1` | yes, same commit |
  | mcp-server-recall | `go 1.26.6` | `v1.4.1` | yes, same commit |
  | mcp-server-socratic-thinker | `go 1.26.6` | `v1.4.1` | yes, same commit |
  | mcp-server-sequential-thinking | `go 1.26.6` | `v1.2.0` | no |
  | mcp-server-evolve-plan, -filesystem, -magicskills, -go-modernizer, -magicdev, -brainstorm | `go 1.26.6` | `v1.2.0` | no |

  None has a `toolchain` line. No other repository under the fleet's Go
  directory requires this module. Every consumer pins a published tag, so
  none sees a `go.mod` change until it moves its `require`.
* **E6, records [scan].** A whole-repository scan for
  `NNNN-{MADR,PLAN,REPORT,GATES}-*.md` (the repository has no
  `check_records.py`) finds records in `docs/` (0001 to 0013) and
  `docs/decisions/` (0009, 0010, 0014, 0015). The highest is 0015, so this
  pair is 0016.
* **E7, the earlier floor record [read].**
  [0006-MADR-raise-go-toolchain-floor-to-1-26-6.md](../0006-MADR-raise-go-toolchain-floor-to-1-26-6.md)
  raised the floor to 1.26.6 for four advisories. It is history; this record
  amends it by a dated note.

The question: how should this module's builds and declared floor follow the
owner's instruction, given GO-2026-6859 in 1.26.10 and 1.27.3?

## Decision Drivers

* The owner's instruction: floor 1.26.11, toolchain 1.27.4.
* Ship nothing built on a toolchain with a published advisory (E3).
* A library's floor is a promise to every consumer (E5).
* No gate loosened; no compatibility engineering (the owner is the only
  user of the fleet's repositories).

## Considered Options

* **A. `go 1.26.11` and `toolchain go1.27.4`, as ruled.**
* **B. A different floor** (keep `go 1.26.6`, add `toolchain go1.27.4`).
* **C. Do nothing.**

## Decision Outcome

Chosen option: "A", because it is what the owner instructed, moves CI onto a
standard library `govulncheck` reports clean (E3), and keeps the floor equal
to the first advisory-free 1.26 patch. The library needs nothing from 1.27,
so the floor stays on the 1.26 line.

0. **Amended 2026-11-17 (A1, below):** CI's `golangci-lint` pin moves from
   v2.13.1 to v2.15.0 (`ci.yml:28`), and `selfupdate/target_test.go` is
   changed so the newer linter's one finding is resolved without a
   suppression.
1. `go.mod:3` becomes `go 1.26.11`, and a `toolchain go1.27.4` line is added.
   No `require` changes; `go mod tidy -diff` must stay clean.
2. `README.md:17` and `AGENTS.md:9` say "Requires Go 1.26.11; built and
   tested with Go 1.27.4."
3. `ci.yml` is **not** edited: `go-version-file: go.mod` follows the
   `toolchain` line (E2).
4. **No release is required for any consumer's builds** (E4). A release is
   needed only for a consumer to receive this `go.mod`: a consumer that
   later runs `go get github.com/maccavelli/mcplib@<new tag>` has its `go`
   line raised to 1.26.11 by the go command. That is the only bump a
   consumer must make, and it is already their target floor under the
   owner's instruction. The tag and the release need the owner's word; the
   plan stops before them.
5. 0006-MADR gets a dated note pointing here; nothing is rewritten.

### Consequences

* Good, because CI builds with Go 1.27.4 and `govulncheck` is clean under
  both 1.27.4 and 1.26.11 (E3).
* Good, because the reusable workflow is unaffected (E4): nothing a caller
  builds changes.
* Bad, because the floor of the next mcplib release rises by five patches
  for consumers. Would any break? Consumers on `go 1.26.6` (E5) that take
  the new release get their floor raised by the go command to 1.26.11;
  that is the owner's own target for them. They break only if built with
  `GOTOOLCHAIN=local` on a Go older than 1.26.11. `mcp-server-duckduckgo`
  (`go 1.27.4`) is above it. Nothing breaks until a consumer moves its
  `require`.
* Bad, because **CI has no floor job** (named gap): nothing in CI builds
  and tests on 1.26.11, so the declared floor is proven only by a local run
  on this Mac. Adding such a job is the owner's decision, fleet-wide; it is
  not added here.
* Bad, because **CI's lint step is expected to fail after the change**
  (found 2026-11-17, see the PLAN's deviation entry). `ci.yml:28` installs
  `golangci-lint` v2.13.1, which cannot type-check the Go 1.27.4 standard
  library ("export data version 5 is greater than maximum supported
  version 4", 4 typecheck findings, observed locally with the v2.13.1
  source built from the module cache); v2.14.0 and v2.15.0 type-check it
  but report one `gosec` G703 finding at `selfupdate/target.go:127`
  (`os.Lstat(abs)`), code unchanged since 2026-09-02. At HEAD, under the
  1.26.11 toolchain, v2.13.1 reports 0 issues. **Resolved by A1 below.**
  Observed in CI on a sibling repository the same day: mcp-buntdb's lint
  step under v2.13.1 with Go 1.27.4 failed with "could not import os"
  and `typecheck: 4`, exit 2 (reported by the coordinator; not re-read
  here).
* Neutral, because every host's `GOTOOLCHAIN` already names 1.27.4, so the
  `toolchain` line changes nothing locally; only CI's `setup-go` follows it.

### Confirmation

* `govulncheck ./...` exits 0 under `go1.27.4` and `go1.26.11` on the changed
  tree (E3 is the red).
* A version-agreement check reads `go.mod` and asserts every pin and current
  statement agrees; it passes on the tree and fails on planted wrong
  toolchain, wrong floor and a stray old pin in a doc or workflow.
* `GOTOOLCHAIN=go1.26.11 GOPROXY=off` build, vet and test pass; the
  repository's tests, vet, gofmt, tidy and lint pass on 1.27.4.
* CI on `main` passes; its `setup-go` logs name 1.27.4.

## Pros and Cons of the Options

### A. Floor 1.26.11 and toolchain 1.27.4 (chosen)

* Good, because it follows the instruction and 0045 in go-selfupdate-lib.
* Bad, because consumers' minimum rises on their next bump.

### B. Keep `go 1.26.6`, add the toolchain line

* Good, because the floor does not move for consumers.
* Bad, because it contradicts the instruction, and a floor of 1.26.6 builds
  on a standard library with known advisories (0006-MADR's own reasoning).

### C. Do nothing

* Good, because nothing changes.
* Bad, because CI follows `go.mod` and, with no `toolchain` line, builds
  with the `go` line's version, which the survey measured as an affected
  line; it also contradicts the instruction.

## More Information

### Decided

The owner's ruling, label quoted exactly and only as a label: "One short
record pair per repository, all approved now through CI (Recommended)". It
is the approval of the accompanying plan, and the authority for commits and
pushes to `main` for this plan only. Tags and releases wait.

* **D2, the lint version, fleet-wide.** Chosen label: "v2.15.0 everywhere,
  in each repository's Go-pins record (Recommended)". Asked: six
  repositories pin v2.13.1 in CI, which cannot type-check under Go 1.27.4;
  each record is amended to include the CI pin; any new finding the newer
  linter reports is brought to the owner with the code rather than
  suppressed.
* **D3, the G703 finding.** Chosen label: "Analyse it; restructure the code
  if that satisfies the linter honestly; come back if not (Recommended)".
  Asked: establish whether a caller can really steer that path somewhere
  it should not go, and whether the function can be written so the
  analysis passes without suppression, with tests, under an amendment to
  this record; if the only way is a suppression, stop and bring the owner
  the evidence.

### Amendments

#### A1 (2026-11-17): the lint pin is v2.15.0, and the G703 finding is a test-only taint

* **Pin.** `.github/workflows/ci.yml:28` installs `golangci-lint@v2.15.0`
  (D2). v2.15.0 was built from the module cache for the checks (no download).
  Its `version` output is "golangci-lint has version (devel) built with
  go1.27.4": a source build without a tag, so the version is the source
  directory's. It uses gosec v2.29.0 (`go.mod:120` of that source). Under
  both `GOTOOLCHAIN=go1.27.4` and `go1.26.11`, `make lint` at HEAD reports
  exactly one finding, `selfupdate/target.go:127:23: G703`; v2.14.0
  reported the same one.
* **What the rule sees.** gosec v2.29.0's `analyzers/pathtraversal.go` lists
  as taint sources `os.Args`, `os.Getenv`, `os.ReadFile` and the types
  `*http.Request`, `*url.URL`, `*bufio.Reader` and `*bufio.Scanner`;
  `os.Lstat` and `os.Stat` are sinks; `filepath.Base`, `filepath.Rel`,
  `path.Base` and the `strconv` parsers are sanitisers, and `filepath.Clean`
  and `filepath.Abs` are deliberately not.
* **Where the taint comes from.** Linting with `--tests=false` reports no
  G703. With tests, replacing `os.Getenv("SystemDrive")` at
  `selfupdate/target_test.go:70` by a constant makes it disappear. So the
  only source is that test line, which feeds the Windows branch of
  `TestCanonicalizeRootRejectsFilesystemRoot` into `canonicalizeRoot`
  (`:72`). Production code never reaches a listed source: replacing the
  home-directory and `AllowedRoots` arguments by constants (scratch
  copies) leaves the finding in place.
* **Is there a real traversal? No.** `canonicalizeRoot` is called only from
  `allowedRoots` (`target.go:99`, `:105`), which `resolveTarget`
  (`target.go:52`) calls with `os.UserHomeDir()` and
  `TargetPolicy.AllowedRoots`. The roots are the allow-list: the target
  must resolve under one of them (`underAnyRoot`, `target.go:53`). They
  are chosen by the program that embeds the library (for example
  `AllowedRoots: []string{filepath.Dir(opts.TargetPath)}` in
  magic-cli-remote's `internal/updateclient/client.go:124`, and `{dir}` in
  prepare-commit-msg's `migration_test.go:63`), or are the user's own home.
  An attacker who chooses an allowed root has already chosen where the
  executable may be replaced, so a `Lstat` of it adds nothing. The
  function rejects an empty, relative, missing, non-directory and
  filesystem-root path, resolves symlinks, and accepts a symlink to a
  directory (as the `IsDir` or symlink test at `:131` and the resolved
  `IsDir` test at `:142` require).
* **Conclusion: a false positive, from a test, resolved honestly.**
  `TestCanonicalizeRootRejectsFilesystemRoot` now derives the volume root
  from `filepath.VolumeName(t.TempDir())` instead of an environment
  variable (a better test: it no longer depends on `SystemDrive` being
  set). No production line changes, no `//nolint`, no `.golangci.yml`
  exclusion. A new `TestCanonicalizeRootContract` holds the function's
  contract (accepts a directory and a symlink to one, resolved; rejects
  empty, a relative path that names an existing directory, a file, a
  missing path and a symlink to a file). It was seen to fail on four
  planted breaks on a scratch copy: filesystem-root check disabled
  ("accepted filesystem root"), symlinks to directories refused
  ("allowed root is not a directory"), the resolved-directory check
  disabled ("accepted ...", symlink to a file), and the absolute-path check
  disabled (the relative-directory case).
* **Consumers.** Only a `_test.go` file changed under `selfupdate/`;
  `target.go` and the exported API are unchanged, so no consumer is
  affected.
* **Restructurings of `target.go` were not needed**, so `os.OpenRoot`,
  pre-validation and handle-passing were not built: the analyser's only
  source is in the test, and changing production code to satisfy it would
  be changing the code to move the analyser, not to fix anything.

### Relation to earlier records

Amends [0006-MADR-raise-go-toolchain-floor-to-1-26-6.md](../0006-MADR-raise-go-toolchain-floor-to-1-26-6.md)
(the floor is now 1.26.11) by a dated note; supersedes nothing.

### Not verified

* That `actions/setup-go` v7.0.0 installs 1.27.4 on the runners: no run yet.
* That CI's `govulncheck` (not used in `ci.yml`; none runs there) would
  report GO-2026-6859: only local v1.9.0 ran.
* What GO-2026-6859 is beyond its title and fixed versions.
* Whether any consumer builds with `GOTOOLCHAIN=local` on an older Go.
* The setup-go README quote is taken from go-selfupdate-lib's record, not
  re-fetched.

### Links

* [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859)
* go-selfupdate-lib's `0045-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md`, the model.
