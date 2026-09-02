---
status: proposed
date: 2026-09-02
decision-makers: mcplib maintainers
consulted: failed tag runs in prepare-commit-msg and mcp-server-socratic-thinker
informed: all six self-update consumers
---

# Restore repository context in the reusable release workflow, and make its guard fail closed

<!-- markdownlint-disable MD013 MD024 -->

> Paired with [0007-PLAN-restore-repository-context-in-the-reusable-release-workflow.md](0007-PLAN-restore-repository-context-in-the-reusable-release-workflow.md).
> Fixes a defect in the workflow introduced by
> [MADR 0005](0005-MADR-canonicalize-cli-self-update-in-mcplib.md) Phase 5.

## Context and Problem Statement

The first two canonical release tags both failed. `prepare-commit-msg v1.1.4`
(run 33690128008) and `mcp-server-socratic-thinker v1.1.0` (run 33692726660)
failed at the identical step:

```text
8  Create a draft release: failure
   gh release create "$tag" --draft --verify-tag --generate-notes
   failed to run git: fatal: not a git repository (or any of the parent directories): .git
```

`.github/workflows/publish-selfupdate-release.yml` never checks out the calling
repository at the top level. It checks mcplib's own tools into a subdirectory
(`path: .mcplib-release-tools`, line 79) and downloads the staged assets into
another (`path: staging`, line 85). The job's working directory therefore
contains no `.git`, and every `gh` subcommand that acts on a repository has no
way to resolve which repository it means.

`gh` documents the remedy explicitly:

> `GH_REPO`: specify the GitHub repository in the `[HOST/]OWNER/REPO` format for
> commands that otherwise operate on a local repository.

The workflow sets `GH_TOKEN` in four steps and **`GH_REPO` in none**: the string
does not appear in the file at all. Both currently pinned versions carry the
defect — `3e64e30` (v1.3.0) and `3389f79` (v1.4.0) each contain zero occurrences.

This is a regression of knowledge the fleet already had. The hand-rolled job
this workflow replaced set it deliberately, with the reason attached:

```yaml
GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
# Explicit repo: avoid relying solely on checkout remote for gh.
GH_REPO: ${{ github.repository }}
```

### The loud failure is the lucky one

Four steps invoke `gh` against a repository. Only one of them fails visibly.

| Line | Step | Call | Behaviour without repository context |
|---:|---|---|---|
| 55-56 | Check gh release capabilities | `gh release verify --help`, `gh release view --help` | unaffected; `--help` needs no repository |
| 65 | Refuse an existing release | `if gh release view "$tag" >/dev/null 2>&1` | **silently inert** |
| 113, 116 | Create a draft release | `gh release create`, `gh release upload` | fails loudly — this is what CI reported |
| 129 | Publish the draft | `gh release edit --draft=false` | would fail |
| 140-141 | Wait for immutability | `gh release view --json isImmutable ... \|\| true`, `gh release verify` | **silently degrades**; polls to its 120s timeout |

Two of these fail quietly, and one of those is a safety guard.

**F1 — the "refuse an existing release" guard cannot fire.** Line 65 redirects
both streams to `/dev/null` and tests the exit status. With no repository
context the call errors, the `if` is false, and the workflow concludes that no
release exists — every time, for every tag. The step reports success while
checking nothing. It also runs *before* the checkout at line 70, so its working
directory is unambiguously empty. This guard is the workflow's protection
against publishing over an existing or draft release, and it has never been
capable of detecting one.

**F2 — the immutability wait cannot confirm.** Line 140 ends in `|| true`, so a
failing call yields an empty string rather than an error; `immutable` never
equals `true` and the loop runs to its deadline before failing. The eventual
failure is real but the message would describe a propagation timeout rather
than a missing repository.

### What it cost

Nothing, this time. Steps 1-7 succeeded and step 8 failed before creating
anything:

* `gh release view v1.1.4 -R maccavelli/prepare-commit-msg` → `release not found`
* `gh release view v1.1.0 -R maccavelli/mcp-server-socratic-thinker` → `release not found`
* zero draft releases in either repository.

The draft-first ordering chosen in MADR 0005 held: no partial release exists, no
immutable release was produced, and both tags remain unused and re-runnable.

## Decision Drivers

* Every `gh` call in the workflow must resolve the calling repository.
* A guard that cannot fail is worse than no guard; failures must be
  distinguishable from negative results.
* Do not disturb `staging/`, which holds the validated release assets.
* Consumers pin this workflow by commit SHA, so any fix implies a new mcplib
  release and a coordinated repin.

## Considered Options

* Set `GH_REPO: ${{ github.repository }}` on every step that calls `gh`, and
  make the existence guard distinguish an error from a negative result
* Add a top-level `actions/checkout` of the calling repository
* Pass `--repo ${{ github.repository }}` to each `gh` invocation
* Set `GH_REPO` only on the step that failed

## Decision Outcome

Chosen option: "Set `GH_REPO` on every step that calls `gh`, and make the
existence guard distinguish an error from a negative result", because it
restores repository context everywhere without touching the working directory,
and because fixing only the visible symptom would leave a safety guard that
reports success while checking nothing.

The guard becomes explicit about the three outcomes it can observe: the release
exists (refuse), the release does not exist (proceed), or the query itself
failed (refuse, and say why). A query failure must never be read as "no release
exists".

### Consequences

* Good, because all four affected steps regain repository context, including
  the two that were failing quietly.
* Good, because the existence guard becomes capable of failing, which is the
  precondition for trusting it at all.
* Good, because `staging/` is untouched: no checkout is added near the
  validated assets.
* Good, because it is a small, reviewable diff to one file.
* Neutral, because the two already-pushed tags stay in place; they created no
  release and can be re-run once consumers repin.
* Bad, because it requires an mcplib release (v1.4.1) and a repin in all six
  consumers before any product release can proceed. Gate G2 is blocked until
  then.
* Bad, because it confirms the workflow's publication path had never been
  exercised end to end before the first real tag. Runtime behaviour on a live
  tag remains unproven until a release actually completes.

### Confirmation

* `grep -c GH_REPO .github/workflows/publish-selfupdate-release.yml` equals the
  number of steps that call `gh` against a repository.
* The existence guard has three explicit branches and a test that exercises the
  query-failure branch, proving it refuses rather than proceeds.
* `actionlint` is clean.
* A consumer tag run reaches "Publish the draft" and "Wait for immutability" and
  produces an immutable release with the expected asset set.
* All six consumers pin the same new SHA. Note that `prepare-commit-msg`
  currently pins `3e64e30` (v1.3.0) while the other five pin `3389f79`
  (v1.4.0); the repin brings all six into agreement.

## Pros and Cons of the Options

### Set `GH_REPO` everywhere and harden the guard

* Good, because it fixes the visible failure and both silent ones together.
* Good, because it needs no change to the job's filesystem layout.
* Good, because it restores a convention the replaced job already had.
* Bad, because it still requires a coordinated release and repin.

### Add a top-level checkout of the calling repository

* Good, because it would give `gh` a real repository to infer from.
* Bad, because `actions/checkout` defaults to `clean: true`, which removes
  untracked files — `staging/` holds the downloaded, validated release assets,
  and the job would have to order the checkout before the download and hope no
  later step re-runs it. The job this workflow replaced carried an explicit
  comment warning about exactly this hazard.
* Bad, because it fetches an entire repository to answer a question that one
  environment variable answers.

### Pass `--repo` to each invocation

* Good, because it is explicit at every call site.
* Neutral, because it is equivalent in effect to `GH_REPO`.
* Bad, because it must be remembered at every future call site, whereas a
  step-level env var covers the whole step. The defect being fixed is precisely
  a per-call-site omission.

### Set `GH_REPO` only on the failing step

* Good, because it is the minimum change to make the observed failure stop.
* Bad, because it leaves the existence guard permanently inert and the
  immutability wait unable to confirm. The workflow would appear to work while
  two of its checks did nothing — the exact condition that let this ship.

## More Information

* Failed runs
  [33690128008](https://github.com/maccavelli/prepare-commit-msg/actions/runs/33690128008)
  and
  [33692726660](https://github.com/maccavelli/mcp-server-socratic-thinker/actions/runs/33692726660),
  both failing at step 8 with steps 1-7 green.
* `.github/workflows/publish-selfupdate-release.yml` lines 55-56, 65, 113, 116,
  129, 140-141; checkout paths at lines 79 and 85.
* [`gh` environment variables](https://cli.github.com/manual/gh_help_environment),
  documenting `GH_REPO`.
* [`actions/checkout` `clean` input](https://github.com/actions/checkout#usage).
* MADR 0005's draft-first publication ordering, which contained the blast radius
  of this defect.
