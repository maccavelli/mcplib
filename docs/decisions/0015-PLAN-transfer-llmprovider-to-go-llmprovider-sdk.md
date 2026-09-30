---
status: in-progress
date: 2026-09-29
associated-madr: "0015-MADR-transfer-llmprovider-to-go-llmprovider-sdk.md"
decision-makers: mcplib maintainers
---

# Implement 0015: Transfer `llmprovider` and `wizard` to go-llmprovider-sdk

Associated MADR: [0015-MADR-transfer-llmprovider-to-go-llmprovider-sdk.md](0015-MADR-transfer-llmprovider-to-go-llmprovider-sdk.md)

This plan is the `mcplib` side of go-llmprovider-sdk
`docs/decisions/0002-PLAN-migrate-llmprovider-from-mcplib.md`:

| This plan's phase | go-llmprovider-sdk 0002-PLAN phase |
|---|---|
| R0 | Phase 1 |
| R1 | Phase 9 |
| R2 | Phase 13 |

It executes the MADR's §1–§5 and nothing else. If a fact contradicts the
MADR or this plan, stop and prompt. Add a dated entry to the deviation log,
amend the MADR if a decision or an asserted fact changes, and only then
continue.

## Goal

1. `llmprovider/` and `wizard/` are frozen at `F` = `4e1f9a5` (`v1.6.0`).
2. `v1.6.1` marks both packages `Deprecated`, pointing at go-llmprovider-sdk.
3. `v1.7.0` no longer contains the packages or the 26 moved records.
4. A `docs/README.md` relocation index resolves every moved record.
5. `README.md`, `AGENTS.md`, `backplane_test.go`,
   `docs/0012-PLAN-circuit-breaker-test.md` and `docs/decisions/0010-*` are
   true after the move.

## Scope

### In scope (the only files any phase may touch)

* **R0:** this pair.
* **R1:** the package doc comments in `llmprovider/provider.go` and
  `wizard/prompter.go`.
* **R2:**
  * deletion of `llmprovider/**` and `wizard/**`;
  * `go.mod`, `go.sum` (tidy only);
  * deletion of the 26 records in MADR §4;
  * `docs/decisions/0010-MADR-windows-stdio-oauth-tokenstore-ci.md` and
    `docs/decisions/0010-PLAN-windows-stdio-oauth-tokenstore-ci.md`
    (appended amendment only);
  * `docs/0012-PLAN-circuit-breaker-test.md` (frontmatter, link, appended
    note);
  * new `docs/README.md`;
  * `README.md`, `AGENTS.md`;
  * `backplane_test.go` (comment only).
* **R3:** this pair.

### Out of scope

* `logging/`, `selfupdate/`, `schema/`, `fastpath/`, `hfsc/`, the root
  package's code, and CI.
* `docs/decisions/0010` P1 and P8, which remain open under that record.
* Records 0002, 0005, 0006 and 0007.

## 0. Preconditions and conventions

* **Gate, every phase:**
  * `go build ./...`, `go vet ./...`, `go test ./...`;
  * `test -z "$(gofmt -l .)"`, `go mod tidy -diff`, `make lint`;
  * `markdownlint-cli2` for changed non-record Markdown.
* **Commits:** `git commit --no-edit` at the end of each phase. The global
  hook writes the message.
* **No push or tag** without the owner's explicit request in that turn.
* **Freeze check at the start of R1 and R2:**
  `git log --oneline 4e1f9a5..HEAD -- llmprovider wizard` must list only this
  plan's own commits.

## Phase R0: start

1. Confirm the tree is clean on `main`, and HEAD is `4e1f9a5` = `v1.6.0^{commit}`.
   * Done 2026-09-29: `v1.6.0` is an annotated tag whose commit is
     `4e1f9a5`, zero commits ahead.
   * `git log v1.6.0..HEAD -- llmprovider wizard logging/redact.go logging/mask.go`
     printed nothing.
2. Present this pair and wait for the owner's approval.
3. On approval, set the MADR to `accepted` and this PLAN to `in-progress`,
   then commit this pair only (bootstrap exception: docs only).

## Phase R1: `v1.6.1` deprecation

**Precondition:**
`go list -m github.com/maccavelli/go-llmprovider-sdk@v1.0.0` resolves.

1. Add the deprecation paragraph to each package's doc comment, as the last
   paragraph:

   ```go
   // Deprecated: use github.com/maccavelli/go-llmprovider-sdk/llmprovider.
   ```

   and, for `wizard`:

   ```go
   // Deprecated: use github.com/maccavelli/go-llmprovider-sdk/wizard.
   ```

   *Amended 2026-09-29:* each paragraph ends
   `; see its docs/guides/migrating-from-mcplib.md.` in place of the
   final period. See the MADR's amendment of the same date.

   The package docs are at `llmprovider/provider.go:1-3` and in
   `wizard/prompter.go` (`// Package wizard …`, the only file with one at
   `4e1f9a5`; neither package has a `doc.go`).
2. **Red first, in a scratch module outside the tree.** Import
   `mcplib/llmprovider` through a `replace` to a scratch copy of this tree:
   * before the edit, `staticcheck` reports nothing;
   * after the edit, it reports `SA1019`.

   Record both outputs.
3. Run the gate, then commit.
4. On the owner's request, tag `v1.6.1`.

## Phase R2: `v1.7.0` removal

**Preconditions:**

* go-llmprovider-sdk `0002-PLAN` Phase 6 is committed. The moved records
  and the SDK copy of `decisions/0010` exist there, so every relocation row
  can be checked.
* The migration records of `prepare-commit-msg` (0008),
  `mcp-server-magictools` (0005) and `mcp-server-magicdev` (0001) are
  `complete`.
* This finds nothing:

  ```bash
  grep -rlE 'github.com/maccavelli/mcplib/(llmprovider|wizard)"' \
    --include='*.go' <fleet root>/*/ | grep -v '^<fleet root>/mcplib/'
  ```

Steps:

1. **Code.**
   * `git rm -r llmprovider wizard`.
   * `go mod tidy`. Assert `golang.org/x/term` is still required, because
     `selfupdate` uses it, and that the `go.mod` diff only removes or is
     empty.
2. **Records.** `git rm` the 26 files listed in MADR §4. Assert with a
   script that exactly those 26 paths were removed and no other path under
   `docs/`.
3. **`docs/decisions/0010` pair.** Append to each file a section headed
   `## Amendment — <date>: scope after the transfer to go-llmprovider-sdk`.
   It says:
   * D1–D2, D14–D18, P1 and P8 remain in scope here;
   * D3–D13 and P2–P7 are transferred to go-llmprovider-sdk
     `docs/decisions/0010-*` under go-llmprovider-sdk
     `0002-MADR-migrate-llmprovider-from-mcplib.md`;
   * the P8 cross-compile step's `./llmprovider` target no longer exists
     here.

   Nothing above it changes.
4. **`docs/0012-PLAN-circuit-breaker-test.md`.**
   * Set `associated-madr` to
     `"go-llmprovider-sdk docs/decisions/0012-MADR-conform-providers-to-reference-clients.md"`.
   * Replace the body link at line 10 with the same text as inline code.
   * Append a dated note saying the MADR moved under 0015.
5. **`docs/README.md`** (new):
   * the records that stay, and this pair;
   * the relocation table from MADR §4;
   * the numbering note from MADR §4;
   * the next number (0016).
6. **`README.md`,** per MADR §5:
   * remove lines 36-37, 57, 75, 90-91 and 98-174;
   * reword lines 10-11, 22-24 and 27-28;
   * add the go-llmprovider-sdk sentence.

   Line numbers are at `4e1f9a5`. Re-locate each by its quoted content
   before editing, and assert each edit landed.
7. **`AGENTS.md`,** per MADR §5:
   * lines 8-9;
   * line 76's range;
   * the relocation exception beside lines 74-79.
8. **`backplane_test.go:254-255`,** comment only: `(go-llmprovider-sdk
   0012-MADR-conform-providers-to-reference-clients.md revision 4)`.
9. **Verify:**
   * the gate;
   * `grep -rn 'llmprovider\|wizard' --include='*.go' .` is empty;
   * a link resolver over `docs/**` and `README.md`. It must first be seen
     to fail on a scratch copy with one planted bad link, and on
     `docs/0001-MADR-add-grok-xai-llm-provider.md` referenced from the new
     index;
   * each relocation row's target exists in a fresh clone of
     go-llmprovider-sdk.
10. Commit. On the owner's request, tag `v1.7.0`.

## Phase R3: close-out

1. Fill the execution record with commits, gate output and red-first
   evidence.
2. Set this PLAN to `complete`.
3. Report `v1.6.1` and `v1.7.0` to go-llmprovider-sdk `0002-PLAN`'s
   execution record.

## Acceptance criteria

| # | Criterion | Check | Phase |
|---|---|---|---|
| C1 | No commit changes `llmprovider/` or `wizard/` after `4e1f9a5` except R1 and R2 | freeze check | R1, R2 |
| C2 | Importers are warned | `SA1019` in the scratch module | R1 |
| C3 | Packages gone, library builds and tests | gate; the Go grep is empty | R2 |
| C4 | Exactly the 26 records removed | removal assertion script | R2 |
| C5 | Every relocation row resolves in go-llmprovider-sdk | row check against a fresh clone | R2 |
| C6 | Remaining docs have no broken link | link resolver (seen to fail first) | R2 |
| C7 | `README.md` and `AGENTS.md` describe an MCP-only library and the relocation exception | reading of the final files, quoted in the execution record | R2 |

## Rollout and rollback

* **R0:** docs only. Revert the commit.
* **R1:** additive doc comments. Revert before tagging. After tagging
  `v1.6.1`, fix forward with `v1.6.2`; tags are never moved.
* **R2:** before tagging, revert the commit; the packages and records return
  intact. After tagging `v1.7.0`, restoring the packages would be a new
  decision.
  * Importers can stay on `v1.6.x`, which the module proxy keeps serving.
  * Moved records remain reachable in git history here and in
    go-llmprovider-sdk.

## Deviation log

None.

## Execution record

* **R0 step 1:** done 2026-09-29 (read-only), with the result quoted in the
  step.
* **R0 step 2:** the pair was presented on 2026-09-29.
* **R0 step 3:** done 2026-09-29.
  * The owner answered "approve mcplib 0015 pair for execution". That
    accepts the MADR and approves this PLAN, so the MADR is now
    `status: accepted` and this PLAN `status: in-progress`.
  * HEAD was still `4e1f9a5`, and
    `git log --oneline v1.6.0..HEAD -- llmprovider wizard logging/redact.go logging/mask.go`
    printed nothing.
  * This pair was scanned with the disclosure guard's own deny list
    before the commit: no match.
  * The commit contains this pair only (bootstrap exception).
* **R1:** waits for go-llmprovider-sdk `v1.0.0`, per its precondition.
  Since 2026-09-29, that tag follows the SDK's
  `0015-PLAN-canonical-sdk-api-and-module-layout.md` (see the MADR's
  amendment).
