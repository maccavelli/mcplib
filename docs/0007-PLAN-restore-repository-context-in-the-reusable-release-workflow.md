---
status: in-progress
date: 2026-09-02
associated-madr: 0007-MADR-restore-repository-context-in-the-reusable-release-workflow.md
decision-makers: mcplib maintainers
---

# Implement repository context in the reusable release workflow

Associated MADR: [0007-MADR-restore-repository-context-in-the-reusable-release-workflow.md](0007-MADR-restore-repository-context-in-the-reusable-release-workflow.md)

<!-- markdownlint-disable MD013 MD024 MD029 -->

## Goal

A consumer tag run completes the reusable workflow end to end and produces an
immutable release with the expected assets, with no `gh` step relying on an
inferred repository and no guard capable of reporting success without checking.

## Scope

**In scope.** `.github/workflows/publish-selfupdate-release.yml`; an mcplib
patch release; the workflow SHA pin in all six consumers; the PLAN 0005 Gate G2
record.

**Out of scope.** `scripts/verify-selfupdate-release.sh` and its validation
rules, which ran correctly (step 7 passed in both failed runs); the
`selfupdate` Go package; the canonical asset contract; the consumers' own build
or staging steps; the two already-pushed tags, which are handled in Section 6.

## Verified baseline

Established 2026-09-02; re-confirm before editing.

| Fact | How it was verified |
|---|---|
| Both tag runs failed at step 8, steps 1-7 green | `gh api .../actions/runs/<id>/jobs`, step-level conclusions |
| Error is `fatal: not a git repository` from `gh release create` | `gh run view --log-failed` |
| `GH_REPO` appears **0** times in the workflow | `grep -c GH_REPO` on the file |
| `GH_TOKEN` appears 4 times | same file |
| No top-level checkout; paths are `.mcplib-release-tools` and `staging` | `grep -n "path:"` lines 79, 85 |
| Nine `gh` occurrences, at lines 51, 55, 56, 65, 113, 116, 129, 140, 141 | `grep -n '\bgh '` — note a `^\s+gh` regex misses lines 65, 140 and 141 |
| Both pinned SHAs carry the defect | `git show <sha>:<file> \| grep -c GH_REPO` → 0 for `3e64e30` and `3389f79` |
| No release or draft was created | `gh release view` → `release not found` in both repositories |
| Five consumers pin `3389f79`; prepare-commit-msg pins `3e64e30` | `grep -rn "publish-selfupdate-release.yml@"` across the six |

## Implementation Steps

1. **Add repository context.** Add `GH_REPO: ${{ github.repository }}` beside
   `GH_TOKEN` in each step that calls `gh` against a repository: *Refuse an
   existing release*, *Create a draft release*, *Publish the draft*, and *Wait
   for immutability and release attestation*. Leave *Check gh release
   capabilities* alone — it invokes only `--help` and needs no repository — but
   add a short comment there recording why it is exempt, so a later reader does
   not "fix" it or copy the omission.

2. **Make the existence guard fail closed.** Replace the two-outcome test at
   line 65 with three explicit branches:

   * the release exists → refuse, as today;
   * `gh` reported "release not found" → proceed;
   * any other failure → refuse, printing the captured `gh` output.

   Capture output instead of discarding it to `/dev/null`, so a diagnosable
   error is never read as a negative result. This is the defect that let the
   workflow report a passing guard while checking nothing.

3. **Do not add a checkout.** `staging/` holds the downloaded, validated
   assets, and `actions/checkout` defaults to `clean: true`. Record this as a
   comment near the artifact download.

4. **Run `actionlint`** and resolve anything it reports in the changed file.

## Required tests

The workflow itself cannot run outside GitHub, so test what is testable and be
explicit about what is not.

* **Guard logic.** Extract the three-branch decision so it can be exercised by
  `scripts/` shell tests with a stubbed `gh` on `PATH` returning, in turn: a
  release, a "not found" error, and an unrelated failure. Require refuse,
  proceed, refuse.
* **Negative test, mandatory.** Run the guard test against the *current*
  two-outcome implementation and record that the third case (unrelated failure)
  **wrongly proceeds**. A guard test that has only ever been seen passing is
  exactly the failure being fixed. Do this against a scratch copy; do not dirty
  the tree.
* **Static check.** Assert every step that calls `gh` against a repository sets
  `GH_REPO`, so a future step cannot reintroduce the omission. Use the complete
  `\bgh ` inventory, not a line-anchored regex.

## Verification

    grep -n '\bgh ' .github/workflows/publish-selfupdate-release.yml
    grep -c 'GH_REPO' .github/workflows/publish-selfupdate-release.yml
    go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/publish-selfupdate-release.yml
    sh scripts/verify-selfupdate-release_test.sh
    go build ./... && go vet ./... && go test ./...
    make lint
    git diff --check

## Acceptance criteria

1. Every `gh` step acting on a repository sets `GH_REPO`; the capabilities step
   is exempt with a comment saying why.
2. The existence guard refuses on a query failure, and that branch is proven to
   fail against the current implementation before the fix.
3. `actionlint` is clean.
4. No checkout of the calling repository was added, and `staging/` handling is
   unchanged.
5. mcplib v1.4.1 is published and resolves through the proxy.
6. All six consumers pin the same new workflow SHA, including
   prepare-commit-msg, which currently pins the older `3e64e30`.
7. One consumer tag run reaches *Publish the draft* and *Wait for immutability*
   and produces an immutable release with the expected asset set.

## Rollout and Rollback

**Order matters.**

1. Fix and commit in mcplib. No push without same-turn authorization.
2. Gate: publish **v1.4.1**. Do not move v1.4.0.
3. Repin all six consumers to the v1.4.1 commit SHA with a `# mcplib v1.4.1`
   comment. One commit per repository.
4. Only then resume PLAN 0005 Gate G2 §18.2.

**The two pushed tags.** `prepare-commit-msg v1.1.4` and
`mcp-server-socratic-thinker v1.1.0` exist and produced no release. Two routes,
and the choice belongs to the owner:

* **Re-run the existing tag's workflow** after the repin. The tag is unchanged
  and immutability is unaffected, because no release was ever created.
* **Cut a fresh tag.** Cleaner history, and the opportunity to reconsider
  `v1.1.4` for prepare-commit-msg: PLAN 0005 §18 proposed **v1.2.0**, and the
  release adds an `update` command and a new asset contract, which is
  minor-shaped rather than patch-shaped.

Do not delete a tag to reuse it.

**Rollback.** Revert the mcplib commit and leave consumers pinned to the
previous SHA; they return to the current broken-but-fails-closed state, which
creates no releases. There is no partial-release state to unwind.

## Execution Record

Populate during execution.

| Step | Status | Commit | Evidence | Deviation |
|---|---|---|---|---|
| 1-4 workflow fix | complete | (this commit) | `GH_REPO: ${{ github.repository }}` added to exactly the four repository-scoped steps (Refuse, Create, Publish, Wait), verified with `grep -cF` = 4 and a per-step listing; the capabilities step is exempt with a comment; a comment records why no caller checkout is added | the guard moved to run **after** the tools checkout so it can call the shared script — still before the artifact download and before any mutation. Step order is now Require, Capabilities, Checkout, Refuse, Download, Validate, Create |
| Guard tests | complete | (this commit) | `scripts/refuse-existing-release.sh` extracted with three explicit outcomes; `scripts/refuse-existing-release_test.sh` drives it with a stubbed gh across 6 cases: existing release, two absent wordings, two undiagnosed failures, and a usage error. 6 passed / 0 failed | **the first harness stubbed gh by prefixing PATH, and this environment's shell startup files re-order PATH — so the tests silently ran the REAL gh and 3 cases failed for the wrong reason.** Fixed by adding an explicit `GH` seam to the script; a PATH-prefix stub is not reliable here |
| Negative test of the guard | complete | (this commit) | The same test file was run against a scratchpad reconstruction of the pre-fix two-outcome guard. The two undiagnosed-failure cases **fail** there (it proceeds where it must refuse) and pass against the fix — including the exact CI string `failed to run git: fatal: not a git repository`. The tree was never modified | none |
| Static workflow check | complete | (this commit) | `scripts/check-workflow-gh-repo.sh` asserts every repository-scoped `gh` step sets `GH_REPO`. Proven both ways: passes the real workflow, exits 1 on a fixture with **one** setting removed. Wired into mcplib CI beside the existing fixture test | the first version false-positived on the step *name* "Check **gh release** capabilities", which matched the command pattern; fixed by skipping `- name:` and comment lines |
| Verification suite | complete | (this commit) | actionlint on both workflows; guard tests; static check; the untouched release-validator tests; shellcheck on all three new scripts; `go build`/`go vet`/`go test ./...`; `make lint`; `git diff --check` — all pass | none |
| Gate: mcplib v1.4.1 | pending authorization | | | |
| Repin six consumers | pending | | | |
| Consumer release proves the path | pending | | | |
