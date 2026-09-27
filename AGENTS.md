# AGENTS.md

Instructions for AI coding agents working in this repository. All agents read
this file. A repository-local `CLAUDE.md` / `.claude/rules/` / `.grok/rules/`
wins only where it is more specific than this file.

`mcplib` is the shared Go library (`github.com/maccavelli/mcplib`) for the
fleet. It is a library only: no packaged binary. LLM providers live in
`llmprovider/`; the configuration wizard lives in `wizard/`. Requires Go 1.26.6.

## MADR and PLAN before mutating work

**Whenever the user asks for an MADR and a plan, load the
`madr-and-plan-writing` skill first** and follow it for authoring, naming
(`NNNN-MADR-*` / `NNNN-PLAN-*`), and review. This applies both to writing a
fresh pair and to amending an existing one.

The name is exact — it is the `name:` field of the skill. A mistyped call
returns `Unknown skill`, and an agent that proceeds without the skill writes
something shaped like a MADR while missing the required headings and the
directory rule below.

**Read-only investigation is allowed with no pair.** Reading, searching,
`git log` / `git show` / `git diff`, and existing tests or diagnostics that
do not write the tree do not need a MADR.

**Mutating work is not.** Before the first write, name the
`docs/decisions/NNNN-MADR-*` / `docs/decisions/NNNN-PLAN-*` pair being
executed, or stop and write one.

Mutating means: creating, editing, or deleting files; staging or committing
(except the bootstrap exception below); dependency or lockfile changes;
CI / config / hook changes; builds or installers that write the tree,
`$HOME`, or a live service; generating committed artifacts.

Order:

1. Investigate (read-only).
2. Write or amend the MADR (`status: proposed` unless the owner already
   decided). Present it. Do not implement.
3. Write or amend the PLAN. Present it.
4. Mutate **only after** the owner explicitly approves execution
   (`proceed`, `execute the plan`, `do phase N`). Stay inside that PLAN.
5. Anything discovered mid-execution that is out of scope waits: amend the
   pair, re-approve, then continue. Completing a phase is not permission to
   invent the next unwritten one.

Follow-up vs greenfield:

- **Same topic** (debug, leftover phase, bug found in that plan's live
  run): amend that number. Add a PLAN phase or an Observed / amendment in
  the MADR. Do not silently rewrite historical rationale.
- **Greenfield**: next unused `NNNN`, new MADR, new PLAN, same slug. No
  mutation until that PLAN is approved.

Bootstrap exception: authoring `docs/decisions/NNNN-MADR-*`,
`docs/decisions/NNNN-PLAN-*`, and this file does not require a *prior* pair.
Putting source, tests, CI, or product config in that same commit is a
violation.

`git push` and tags still need an explicit ask in the same turn.

## File naming: MADR and plan files

New pairs are written here (create the directory if it is missing):

```text
docs/decisions/NNNN-MADR-short-slug.md
docs/decisions/NNNN-PLAN-short-slug.md
```

- `NNNN` is a zero-padded 4-digit number. A MADR and its PLAN share the same
  number and the same slug.
- **Next number** is the highest `NNNN` among `NNNN-MADR-*.md` and
  `NNNN-PLAN-*.md` anywhere under `docs/`, plus one. That includes
  `docs/decisions/` and legacy locations (`docs/` for 0001–0008). Never reuse
  a number, never renumber an existing record, never leave a gap deliberately.
- **Amend in place.** An existing pair stays in the directory it already
  occupies. Do not relocate historical files as part of writing a new pair.

## Commits

`git commit --no-edit`. Never pass `-m` / `--message` / `-F`. The global
`prepare-commit-msg` hook writes the message from the staged diff.

This repository has no `make pre-add-check`. Before staging Go files, `gofmt`
must be clean; `go vet` and `go test` on the touched packages must exit 0.
`make lint` must be clean before a release-shaped change.
