---
status: in-progress
date: 2026-11-17
associated-madr: "0016-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md"
---
# Implement Go 1.27.4 as the toolchain and `go 1.26.11` as the floor

Associated MADR: [0016-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md](0016-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md)
(option A, as the owner instructed).

## Goal

`go.mod` says `go 1.26.11` and `toolchain go1.27.4`, and every current
statement of the version agrees. The plan stops before any tag.

## Scope

In scope: the three lines below. Out of scope: other repositories, a floor CI
job (a named gap in the MADR), `ci.yml` beyond line 28, the reusable workflow,
history (records, including 0006's own text), and any tag or release.

## Implementation Steps

Numbers are at `5f463da`.

| File | Line | Change |
| :--- | :--- | :--- |
| `go.mod` | 3 | `go 1.26.6` becomes `go 1.26.11` |
| `go.mod` | after 3 | new `toolchain go1.27.4` line (blank line either side) |
| `README.md` | 17 | "Requires Go 1.26.6." becomes "Requires Go 1.26.11; built and tested with Go 1.27.4." |
| `AGENTS.md` | 9 | "Requires Go 1.26.6." becomes "Requires Go 1.26.11; built and tested with Go 1.27.4." |
| `.github/workflows/ci.yml` | 28 | `golangci-lint@v2.13.1` becomes `golangci-lint@v2.15.0` (MADR A1) |
| `selfupdate/target_test.go` | 66-75 and new | filesystem-root test derives the root without `os.Getenv`; new `TestCanonicalizeRootContract` (MADR A1) |
| `docs/0006-MADR-raise-go-toolchain-floor-to-1-26-6.md` | end | dated note pointing to 0016-MADR |
| `docs/0006-PLAN-raise-go-toolchain-floor-to-1-26-6.md` | end | dated note pointing to 0016-PLAN |

1. Edit by a script with `assert` on each old string's count; read each
   result back.
2. `go mod tidy -diff` shows nothing; `go.sum` is unchanged.
3. Version-agreement check (scratch script, not committed): reads the
   target from `go.mod` and asserts every pin and statement agrees; it is
   seen to fail on a planted wrong toolchain, a wrong floor and a stray old
   pin in a doc or workflow.
4. Gates (heavy runs through `run-heavy`): `go vet`, `gofmt -l`, `go mod
   tidy -diff`, `make lint`, `go test ./...`, the floor run under
   `GOTOOLCHAIN=go1.26.11 GOPROXY=off`, `govulncheck ./...` under
   `go1.27.4` and `go1.26.11`, cross-builds `GOOS=linux` and `GOOS=windows`,
   the repository's scripts tests, markdownlint and a disclosure check.
5. Stage the record files only; the change files stay unstaged; no commit.

## Verification

* The agreement check passes on the tree and fails on all three plants.
* `govulncheck` exits 0 under both toolchains.
* The floor build, vet and tests pass under `go1.26.11`.
* `git grep` for `1.26.6` outside records finds nothing.

## Rollout and Rollback

Revert the change commit; no history is rewritten. No consumer changes until
it moves its `require` to a later tag (MADR E5). Release and tag: on the
owner's word only; a release is not needed for callers' release builds
(MADR E4).

## Execution Record

### 2026-11-17

Pair drafted from `5f463da` (clean, level with upstream); 0016 chosen by a
whole-repository scan (highest was 0015; no `check_records.py`). The change
was made by one script with `assert` on each old string: `go.mod:3` and a new
`toolchain go1.27.4` line, `README.md:17`, `AGENTS.md:9`, and dated
amendments A1 on 0006's MADR and PLAN. `go mod tidy -diff` exit 0; `go.sum`
unchanged. Nothing committed, pushed or tagged.

Red re-run (this Mac, govulncheck v1.9.0, database 2026-11-11 02:51 UTC,
`GOPROXY=off`): `go1.27.3` exit 3 and `go1.26.10` exit 3 (GO-2026-6859);
`go1.27.4` and `go1.26.11` exit 0.

Version-agreement check (scratch script): exit 0 on the tree (`F=1.26.11
T=1.27.4`). Planted on copies: wrong toolchain exit 1 (`README.md:17` and
`AGENTS.md:9` fail); wrong floor exit 1 (same lines); stray old pin in
`AGENTS.md:9` exit 1 ("stray 1.26.6"); stray pin in `ci.yml` exit 1
(`ci.yml:19` "stray 1.26.6").

Gates (this Mac, exit status before any filter):

| Gate | Start | End | Exit |
| :--- | :--- | :--- | :--- |
| `go vet ./...` | 13:34:11 | 13:34:12 | 0 |
| `gofmt -l .` empty | 13:34:12 | 13:34:12 | 0 |
| `go mod tidy -diff` | 13:34:12 | 13:34:12 | 0 |
| `make lint` (golangci-lint v2.14.0, local) | 13:34:12 | 13:34:22 | 2, see Deviations |
| `go test -count=1 -timeout 20m ./...` via `run-heavy` | 13:34:22 | 13:37:35 | 0 |
| floor: `GOTOOLCHAIN=go1.26.11 GOPROXY=off` build, vet, test via `run-heavy` | 13:37:35 | 13:39:27 | 0 |
| `govulncheck ./...` under `go1.27.4` | 13:39:27 | 13:39:28 | 0 |
| `govulncheck ./...` under `go1.26.11` | 13:39:28 | 13:39:30 | 0 |
| `GOOS=linux CGO_ENABLED=0 go build ./...` | 13:39:30 | 13:39:33 | 0 |
| `GOOS=windows CGO_ENABLED=0 go build ./...` | 13:39:33 | 13:39:38 | 0 |
| the three `scripts/` tests | 13:39:38 | 13:39:43 | 0 |
| `actionlint` on both workflows | 13:39:43 | 13:39:43 | 0 |
| `markdownlint-cli2 README.md AGENTS.md` | 13:39:43 | 13:39:43 | 1, 198 findings, same count at HEAD (README.md outside line 17; `docs/0011-REPORT-...`); not caused by this change |

Not done: no CI floor job (named gap), no tag, no release, no workflow edit.

### 2026-11-17: the floor job (MADR A2)

By the owner's ruling D4 (MADR A2), `.github/workflows/ci.yml` gains the
`go126` job, after `validate`; no other line of `ci.yml` changes. The
version-agreement check also asserts the job's `go-version` equals `go.mod`'s
`go` line, its `GOVERSION` test names the same version and `GOTOOLCHAIN` is
`local`; planted `go-version: '1.26.10'` and a planted `= go1.26.10` test each
fail it (exit 1, "go126: go-version is not exactly 1.26.11", "go126: GOVERSION
test is not go1.26.11"). `actionlint` on `ci.yml` exits 0 and
`scripts/check-workflow-gh-repo.sh` passes. The job's steps ran locally
under `GOTOOLCHAIN=go1.26.11 GOPROXY=off` (the 14:27 to 15:03 gate above; the
edit changes no Go code); `GOTOOLCHAIN=local` on a 1.26.11 SDK was not run,
because no 1.26.11 SDK is installed on this Mac. The run on `6a139d6`
succeeded on all three operating systems (MADR A2 quotes it); the first run
that includes the new job is its proof and is not yet observed.

## Deviations

### 2026-11-17: the lint gate fails, and CI's lint step is expected to fail

* **Evidence.** `make lint` with the local golangci-lint v2.14.0 exits 2:
  `selfupdate/target.go:127:23: G703: Path traversal via taint analysis
  (gosec)`. The same exit and finding occur on a `git archive HEAD` copy,
  so it is pre-existing and not caused by this change; the line is
  unchanged since 2026-09-02. CI installs v2.13.1 (`ci.yml:28`). Built
  from the module cache, v2.13.1 exits non-zero under Go 1.27.4 with 4
  typecheck findings ("export data version 5 is greater than maximum
  supported version 4") and exits 0 ("0 issues") under
  `GOTOOLCHAIN=go1.26.11`. v2.14.0 and v2.15.0 type-check under 1.27.4 and
  report the G703 finding.
* **Consequence of doing nothing.** After the push, CI's Linux lint step
  fails on the `toolchain go1.27.4` line.
* **Resolutions (owner's choice; none applied):** (1) bump the pinned
  golangci-lint to v2.14.0 or later and fix the G703 finding at
  `selfupdate/target.go:127` in code under its own record (recommended; a
  source change, so a new scope); (2) keep v2.13.1 and drop the `toolchain`
  line (contradicts the instruction); (3) bump the pin and exclude the
  finding (a loosened gate: not offered as a fix).
* **Status.** Resolved 2026-11-17 by the owner's rulings D2 and D3 (MADR
  Decided list): the pin is v2.15.0 and the finding was analysed and
  resolved in the test (MADR A1). Resolution (1) was applied in the form
  the analysis allowed: no source change was needed, because the finding's
  only taint source is `os.Getenv("SystemDrive")` in
  `selfupdate/target_test.go:70`.

### 2026-11-17: execution after the rulings

* `ci.yml:28` is `golangci-lint@v2.15.0`. Lint run as CI will run it: a
  v2.15.0 binary built from the module cache into a scratch directory (no
  download; `GOPROXY=off`); `golangci-lint version` prints "golangci-lint
  has version (devel) built with go1.27.4". `make lint` with it exits 0,
  "0 issues.", under Go 1.27.4 and under `GOTOOLCHAIN=go1.26.11`. Before
  the test change it reported one finding in both: the G703 at
  `selfupdate/target.go:127:23` (the only one).
* The version-agreement check now also asserts the single
  `golangci-lint@` pin is v2.15.0; a planted `v2.13.1` in `ci.yml` fails it
  ("golangci-lint pins ['v2.13.1'], want [v2.15.0]"), and the earlier
  plants still fail.
* Planted breaks on the new test (scratch copy, `go test -run
  CanonicalizeRoot ./selfupdate/`, each exit 1): filesystem-root check
  disabled, `target_test.go:72: accepted filesystem root`; symlinks to
  directories refused, `accepts_a_symlink_to_a_directory_and_resolves_it`
  fails; resolved-directory check disabled, `rejects_a_symlink_to_a_file`
  fails with `accepted`; absolute-path check disabled,
  `rejects_a_relative_path_that_names_an_existing_directory` fails.
* Gates after the last Go change (this Mac, 2026-11-17, exit status before
  any filter; lint is the scratch v2.15.0 binary through
  `GOLANGCI_LINT=`; heavy runs through `run-heavy`, which queued behind other
  agents' runs and was waited out):

| Gate | Start | End | Exit |
| :--- | :--- | :--- | :--- |
| `go vet ./...` | 13:51:10 | 13:51:10 | 0 |
| `GOOS=windows go vet ./...` | 13:51:10 | 13:51:11 | 0 |
| `gofmt -l .` empty | 13:51:11 | 13:51:11 | 0 |
| `go mod tidy -diff` | 13:51:11 | 13:51:11 | 0 |
| `make lint`, Go 1.27.4 | 13:51:11 | 13:51:16 | 0 ("0 issues.") |
| `make lint`, `GOTOOLCHAIN=go1.26.11` | 13:51:16 | 13:51:20 | 0 ("0 issues.") |
| `go test -count=1 -timeout 20m ./...` | 13:51:20 | 14:14:00 | 0 |
| `go test -race -count=1 -timeout 20m ./selfupdate/...` | 14:14:00 | 14:27:17 | 0 |
| floor: `GOTOOLCHAIN=go1.26.11 GOPROXY=off` build, vet, test | 14:27:17 | 15:03:33 | 0 |
| `govulncheck ./...`, go1.27.4 | 15:03:33 | 15:03:34 | 0 |
| `govulncheck ./...`, go1.26.11 | 15:03:34 | 15:03:36 | 0 |
| `GOOS=linux`, `GOOS=windows` builds, `CGO_ENABLED=0` | 15:03:36 | 15:03:36 | 0, 0 |
| the three `scripts/` tests | 15:03:36 | 15:03:38 | 0 |
| `actionlint` on both workflows | 15:03:38 | 15:03:39 | 0 |
| version-agreement check | 15:03:39 | 15:03:39 | 0 |

  The earlier markdownlint result (198 findings, same at HEAD) is unchanged
  by this work.
