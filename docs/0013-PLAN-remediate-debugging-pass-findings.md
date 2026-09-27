---
status: in-progress
date: 2026-09-27
associated-madr: "0013-MADR-remediate-debugging-pass-findings.md"
decision-makers: mcplib maintainers
---

# Implement Remediation of the Post-0010 Debugging-Pass Defects

Associated MADR: [0013-MADR-remediate-debugging-pass-findings.md](0013-MADR-remediate-debugging-pass-findings.md)
(accepted, revision 3, 2026-09-27).

> **Revision 2 of this plan (2026-09-27): rebased onto `e219e11`.** `ba92db1`
> left `main` unbuildable; the OAuth pair's phase R1 repaired it as
> `e219e11`. Every red run, gate, mutant, live run and diff below was
> repeated on that base. The rebase changed three things:
> * one import anchor in Phase 4's fix;
> * one Phase 4 mutant, retargeted to the branch its test reaches;
> * two Phase 4 coverage tests, added for the listing branches the
>   repair split.
> The owner's answers to Q1–Q4 are recorded (§0.1).

This plan executes that MADR and nothing else. If execution finds a fact that
contradicts the MADR or this plan, **stop and prompt**. Record a dated entry
in §9, amend the MADR when a decision or an asserted fact changes, and only
then continue. Do not absorb a deviation silently, and do not widen scope.

## How this plan was proven

Every phase below was executed on 2026-09-27, in scratch copies of the tree
made from `git archive e219e11`, outside the repository. The same proof
was first made on `ca29b81` and repeated in full after the repair:
* **Red.** Each phase's test diff was applied, and each named test was seen to
  fail on the unfixed code. Appendix A quotes each failure message.
* **Green.** The fix diff was applied, and the full gate passed (§0.3).
* **Mutants.** A test that cannot fail on `e219e11` (new API, coverage or a
  relocated live test) was seen to fail on a named mutant instead.
* **Live.** The live suite ran on the final tree.
* **Diffs.** Appendix B's diffs were generated mechanically from that proof.
  Applying them in order to a fresh `e219e11` archive with `git apply`
  reproduces the proven tree byte for byte: 229 files, 0 mismatches.

Executing this plan therefore repeats a proven sequence. It does not design
anything.

## Goal

Fix the MADR's confirmed defects that change a recommended model, fail or
degrade a thinking request, or let the wizard save a wrong configuration.
Wire the options that do not reach their code, and prove each fix with a test
that has been seen to fail. Specifically:
* **Ranking (A1–A3):** no duplicate ids, and blank or infinite prices are
  unknown.
* **Discovery (A4–A5):** `DiscoverModels` ranks with the provider's profile and
  metadata URL, within 10 s.
* **Metadata (A6):** a dead metadata host costs one bounded wait per minute,
  not one per call; a stale document survives a failed refresh.
* **Thinking (B9, B1, B2):** Claude 4.7 and later get adaptive thinking (today
  HTTP 400). `low` reaches the budget-based wires. `""` is documented as each
  provider's default.
* **Catalog (B10):** the two retired `StaticClaude` ids go.
* **Wizard (C1, C2, C4, C6, C7):**
  * no duplicate fallbacks;
  * a listing failure names its cause (`ModelCatalog.Err`);
  * no cross-provider default at the Other and "No models found" prompts;
  * case-insensitive exclusion;
  * no blank ids.
* **Live suite (D1):** no FAIL. The OpenCode generation tests run on paid
  OpenCode Go models.
* **Records:** 0010 and 0012 carry pointers; the README describes the
  changes.

## Scope

**In scope:** the findings the MADR's §1 lists, and the coverage gaps it lists
(§0.4 records what the tests can and cannot pin).

**Out of scope (do not implement "while here"):**
* everything MADR §2 routes to 0012: B3, B4, B6, B7, D5, D6, and the typed
  `FreeTierError`;
* everything MADR §4 accepts: A7–A10, B5, B8, C3, C5, C8, C9, C10, and D2, D4;
* 0010 PLAN's open items: Phase 6's DeepSeek live check (D2) and Phase 7's
  README section for 0010's API (D3);
* re-verifying `StaticOpenAI`, `StaticGrok` or `StaticGemini` beyond what B10's
  evidence covers.

## 0. Preconditions and conventions

### 0.1 Decisions (accepted 2026-09-27)

The owner accepted the MADR's recommended answers (MADR revision 3). A
later change to any answer means stopping before the phase it governs,
amending this plan, and having it re-approved.

| Question | Answer | Governs |
|---|---|---|
| Q1 | (c) per-model wire shapes | Phase 5, the `low` mappings. B9's adaptive shape is needed under every answer. |
| Q2 | (a) `""` is each provider's documented default | Phase 5 (doc comments), Phase 7 (0010 §1 note) |
| Q3 | (b) keep free Zen ids; 0012 types `FreeTierError` | Phase 7 (0012 pointer). Phase 6 moves the live tests under either answer. |
| Q4 | (b) 10 s fixed; surface the cause | Phase 4 (`ModelCatalog.Err`, `defaultDiscoverLimit`, `DiscoverLimit` doc), Phase 7 (README) |

### 0.2 Baseline

* **Branch and base.** `main` at `e219e11`, or a descendant whose only
  changes since `e219e11` are under `docs/`. Confirm that
  `git diff --stat e219e11 HEAD -- . ':!docs'` prints nothing and the
  working tree is clean. `ba92db1` and `6e19cdf` do not build; never start
  from them. Every diff in Appendix B is against the state left by the
  phase before it.
* **Apply order.** Diffs apply strictly in order: B.1.1, B.1.2, B.2.1, … B.6.2.
  Phase 4's fix edits text that Phase 1 introduced (`catalogFrom`).
* **Applying a diff.** Save the block to a file outside the repository and run
  `git apply <file>` from the repository root. A diff that does not apply
  cleanly is a deviation (§9); do not hand-merge it.

### 0.3 Gate (every phase)

After the fix diff, all of these must pass. Each check's exit status decides
by itself; no pipeline sits between a check and its status.
1. `gofmt -l <each .go file the phase touched>` prints nothing.
2. `golint -set_exit_status <file>` for each of those files.
3. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
4. `make lint`.
5. `go test -count=1 ./...`.
6. `go test -race -count=1 ./llmprovider ./wizard`.

### 0.4 Tests that cannot be red on `e219e11`

A test of new API (`ModelCatalog.Err`, the metadata cache's `failed` field)
does not compile on `e219e11`. A coverage test, a guard on a shape that
already works, or a relocated live test passes there. Each is proven by a
named mutant instead. Apply the mutant to a scratch copy of the phase's green
tree, made from `git archive HEAD` outside the repository, **never to the
working tree**. The mutant must fail the named test at run time; a build
failure proves nothing.

One mutant is expected to **survive**: deleting the branch at
`wizard/configure.go:309-311`. That branch is observably equivalent to
falling through, so no test can pin it. The survival is the evidence, and is
recorded, not treated as a defect.

### 0.5 Credentials for live steps

Live steps read credentials from the environment and never print them:
* `ANTHROPIC_API_KEY` (Phases 5 and 6);
* `GEMINI_API_KEY` (Phase 5);
* `OPENCODE_API_KEY` (Phases 5, 6 and 7);
* `KILO_API_KEY` and `HF_TOKEN` (Phase 7's full suite).

A live test may skip only on 429 or an unset credential. A transport timeout
is re-run once, and both runs are recorded (MADR D6).

### 0.6 Commits

* **When.** At the end of each phase, after the gate passes.
* **Pre-add.** Run `gofmt -l` and per-file `golint` on every staged `.go` file.
* **Commit.** Use `git commit --no-edit`; the global `prepare-commit-msg` hook
  writes the message. No `-m`, no `--amend`.
* **No push.** Nothing is pushed or tagged unless asked in the same turn.

## Phase 0 — Start execution

The MADR already records the answers and `status: accepted` (revision 3).
1. Check §0.2's base.
2. In this plan, set `status: in-progress` and update `date`.
3. Commit this plan only.

## Phase 1 — Ranking correctness (A1–A3)

**Changes.**
* **A1.**
  * `catalogFrom` passes the usable ids through `uniqueIDs`, keeping the first
    of each. `ModelCatalog.Usable` is documented as "once each".
  * `rankRecommended` skips an id already chosen.
* **A2–A3.** `kiloPrice` parses every string. A blank (`ParseFloat` error),
  negative, `+Inf` or unparseable price is unknown.
* **Existing test updated.** `TestKiloCandidate_Fields`'s `org/blank` row
  pinned blank-is-a-known-0. A2 reverses that rule, so the row becomes
  unknown (`0, false`).

**Files:** `llmprovider/model_ranking.go`, `llmprovider/discovery.go`,
`llmprovider/ranking_edge_test.go` (new), `llmprovider/model_ranking_test.go`.

**Steps.**
1. Apply B.1.1 (tests).
2. Red: `go test -count=1 -run '^(TestListModelCatalog_DuplicateIDs|TestRankRecommended_SkipsDuplicateCandidates|TestKiloCandidate_AbsentPriceIsUnknown|TestKiloPrice_NonFiniteIsUnknown|TestRankRecommended_InfinitePriceNotCheapest|TestKiloCandidate_Fields)$' ./llmprovider`. Every named test fails as in A.1.
3. Apply B.1.2 (fix).
4. Gate (§0.3). The golden test `TestListModelCatalog_Snapshot20260926` must
   pass unchanged. The 2026-09-26 captures contain no duplicate, blank or
   non-finite entry (MADR A).
5. Commit.

## Phase 2 — Discovery wiring (A4–A5)

**Changes.**
* **A5.**
  * `modelListingTimeout = 10 * time.Second` replaces
    `ListModelCatalogWithSource`'s literal.
  * `boundedListing` wraps the six `list*Models` helpers `DiscoverModels`
    uses: Gemini, Claude, Ollama, OpenCode, Hugging Face and Kilo.
* **A4.**
  * `KiloProvider` keeps `modelProfile`. `HuggingFaceProvider` keeps
    `modelProfile` and `metadataURL`. `OpencodeProvider` keeps `modelProfile`;
    it already keeps `metadataURL`.
  * Each passes them to its listing.
  * The `ModelProfile` field and `WithModelProfile` docs now say
    `DiscoverModels` ranks with the profile. This replaces "Ignored by
    provider constructors", and is the contract change MADR A4 records.

**Files:** `llmprovider/discovery.go`, `opencode.go`, `huggingface.go`,
`kilo.go`, `options.go`, `llmprovider/discovery_wiring_test.go` (new).

**Steps.**
1. Apply B.2.1.
2. Red: `go test -count=1 -run '^(TestDiscoverModels_ListingBounded|TestDiscoverModels_HonoursRankingOptions)$' ./llmprovider`. All six `ListingBounded` subtests and all three
   `HonoursRankingOptions` subtests fail (A.2).
3. Apply B.2.2.
4. Gate.
5. Commit.

## Phase 3 — Metadata on the request path (A6)

**Changes.**
* **Cache entries** hold the last good document, when it was fetched, the
  last failure and its time.
* **`loadModelMetadata`:**
  * answers a fresh document from the cache;
  * within `modelMetadataRetryAfter` (1 minute) of a failure, answers from
    the cache without fetching: the stale document if there is one, else the
    failure;
  * on a failed refresh, keeps the stale document.
* **`chatReasoningEffort`** bounds its lookup with `metadataLookupTimeout`
  (5 s).
* **Test replacements.** Removed: `TestLoadModelMetadata_FailureNotCached`,
  which pinned 0010 PLAN §1.11 item 3, the rule A6 supersedes. Added in its
  place:
  * `TestLoadModelMetadata_FailureCachedBriefly` (red);
  * `TestLoadModelMetadata_FailureRetriedAfterBackoff` (new API, proven by a
    mutant).

**Files:** `llmprovider/model_metadata.go`, `opencode.go`,
`llmprovider/metadata_request_test.go` (new), `llmprovider/model_metadata_test.go`.

**Steps.**
1. Apply B.3.1.
2. Red: `go test -count=1 -run '^(TestChatReasoningEffort_FailureBackoff|TestChatReasoningEffort_LookupHasDeadline|TestLoadModelMetadata_StaleOnRefreshFailure|TestLoadModelMetadata_FailureCachedBriefly)$' ./llmprovider` (A.3).
3. Apply B.3.2.
4. Gate.
5. Mutants (§0.4): `p3-backoff-longer` (kills `TestLoadModelMetadata_FailureRetriedAfterBackoff`) (A.3).
6. Commit.

## Phase 4 — Wizard and the listing's cause (C1, C2, C4, C6, C7; coverage)

**Governed by Q4 (b).**

**Changes.**
* **C2.**
  * `ModelCatalog.Err` carries the fetch error when a listing degrades.
  * The wizard's static-catalog notice becomes a warning that names the
    cause. `TestConfigureLLM_StaticCatalogNotice`'s expected text gains
    `(opencode: models endpoint returned HTTP 500)`.
  * `defaultDiscoverLimit` becomes 10 s, and `DiscoverLimit` is documented as
    unable to extend the lister's bound.
* **C4.** `existingModel(o, provider)` gives the Other and "No models found"
  prompts their default: the saved model, only for its own provider.
* **C7.** Both prompts trim and refuse a blank id.
* **C1.** `appendPicks` skips an id already chosen, ignoring case, and the
  TextPrompter's MultiSelect counts a repeated index once.
* **C6.** `excludedIDs` and `without` compare lower-cased ids.
* **Coverage.** The MADR's listed gaps get tests. They include both arms of
  `discoverModels`' listing-error branch: a provider with no static catalog
  (Ollama unreachable), and one that falls back to its static catalog (a
  kept Grok session that cannot refresh). They also cover the static notice
  with no cause (a Claude listing with nothing usable). Each passes on
  `e219e11` and is proven by a mutant.

**Files:**
* `llmprovider/discovery.go`;
* `wizard/configure.go`, `wizard/model_select.go`, `wizard/text_prompter.go`;
* new tests: `wizard/model_select_edge_test.go`,
  `llmprovider/matcher_coverage_test.go`, `llmprovider/catalog_err_test.go`
  (in B.4.2, because it uses the new field);
* `wizard/model_select_test.go`.

**Steps.**
1. Apply B.4.1.
2. Red: `go test -count=1 -run '^(TestConfigureLLM_FallbackPicksDeduped|TestMultiSelect_RepeatedIndexCountsOnce|TestConfigureLLM_StaticCatalogNotice|TestConfigureLLM_OtherDefaultsOnlyToSameProvider|TestConfigureLLM_FallbackExclusionIgnoresCase|TestConfigureLLM_BlankModelIDRefused)$' ./wizard` (A.4). In `OtherDefaultsOnlyToSameProvider`, the "same
   provider keeps its model" subtest passes on `e219e11` by design, as a
   guard.
3. Apply B.4.2.
4. Gate.
5. Mutants: `p4-err-dropped` (kills `TestListModelCatalog_ErrExplainsDegrade`), `p4-err-dropped-wizard` (kills `TestConfigureLLM_StaticCatalogNotice`), `p4-no-listing-warning` (kills `TestConfigureLLM_ListingErrorWarns`), `p4-no-static-listing-warning` (kills `TestConfigureLLM_ListingTokenFailureUsesStaticCatalog`), `p4-unusable-listing-silent` (kills `TestConfigureLLM_UnusableListingNotice`), `p4-empty-listing-sentinel` (kills `TestConfigureLLM_OllamaEmptyListing`), `p4-empty-listing-deleted` (must survive `TestConfigureLLM_OllamaEmptyListing`), `p4-default-row` (kills `TestConfigureLLM_DefaultRowIsExistingModel`), `p4-blank-round-nothing-left` (kills `TestConfigureLLM_BlankFallbackRoundWithNothingLeft`), `p4-fallback-no-match-silent` (kills `TestConfigureLLM_FallbackSearchNoMatches`), `p4-glob-question-literal` (kills `TestSearchModels_GlobQuestionMark`), `p4-no-label-tier` (kills `TestSearchModels_LabelSubstringTier`) (A.4).
6. Commit.

## Phase 5 — Thinking shapes on the budget wires (B9; B1 under Q1 (c); B2 under Q2 (a))

**Changes.**
* **New `llmprovider/thinking_wire.go`:**
  * `claudeAdaptiveOnly` uses OpenCode's version pattern: Claude 4.7 and
    later, or a `claude-` id with no readable version.
  * `addMessagesThinking` sends adaptive thinking, plus
    `output_config.effort` when an effort is set, to those models. Every
    other model gets an enabled budget: the configured one, else 1024 for
    `low`, else 4096. It raises `max_tokens` when needed.
  * `geminiThinkingConfig`: a configured budget wins. Otherwise `low` is
    `thinkingLevel: "low"` on Gemini 3 and later and a 1,024 budget on 1.x and
    2.x (OpenCode's `GEMINI_LEGACY_RE`), and any other effort keeps the
    dynamic budget.
* **Callers.**
  * `ClaudeProvider` and `GeminiProvider` gain `reasoningEffort`, and
    `thinkingParams` is removed.
  * OpenCode's `messagesBody` and `googleBody` call the shared functions.
* **Docs (Q2 (a)).** `ProviderConfig.ReasoningEffort`, `WithReasoningEffort`
  and `ModelProfile.ReasoningEffort` document each provider's default.
* **Existing tests updated.**
  * `TestClaudeThinking_RequestBody` used the placeholder `claude-x`. An
    unversioned `claude-` id now counts as current and gets the adaptive
    shape, so the test moves to `claude-haiku-4-5`.
  * `TestOpencode_Thinking_PerRoute`'s messages subtest used
    `claude-sonnet-5`. Anthropic refuses a budget there (B9), so it moves to
    `claude-haiku-4-5`.
  * Both keep their budget assertions unchanged.

**Files:**
* source: `llmprovider/thinking_wire.go` (new), `claude.go`, `gemini.go`,
  `opencode.go`, `options.go`, `model_profile.go`;
* new tests: `llmprovider/thinking_wire_test.go`,
  `llmprovider/live_thinking_test.go` (build tag `live_gateways`);
* updated tests: `llmprovider/thinking_test.go`, `llmprovider/opencode_test.go`.

**Steps.**
1. Apply B.5.1.
2. Red, offline: `go test -count=1 -run '^(TestThinkingWire_Claude|TestThinkingWire_Gemini|TestThinkingWire_OpencodeRoutes)$' ./llmprovider`. Every Claude case except the two budget
   guards fails, as do both Gemini `low` cases and all three OpenCode routes
   (A.5).
3. Red, live (`ANTHROPIC_API_KEY`):
   `go test -count=1 -tags live_gateways -run '^TestLive_ClaudeThinkingShapes$' ./llmprovider`.
   `claude-sonnet-5` and `claude-opus-4-8` must fail with
   `llm: invalid request: claude HTTP 400` at both efforts. This is B9
   through mcplib itself.
4. Apply B.5.2.
5. Gate.
6. Live:
   `go test -count=1 -tags live_gateways -run 'TestLive_(ClaudeThinkingShapes|GeminiThinkingShapes|OpencodeMessagesThinking)$' -v ./llmprovider`.
   All 13 tests and subtests pass.
7. Mutants (live): `p5-gemini-legacy-ignored` (kills `TestLive_GeminiThinkingShapes`), `p5-messages-bad-type` (kills `TestLive_OpencodeMessagesThinking`) (A.5).
8. Commit.

**Not provable live while MADR D2 stands:** Zen's messages route for Claude
4.7 and later, and Zen's google route. Their bodies are pinned by
`TestThinkingWire_OpencodeRoutes`. The same shapes are proven live on the
upstream APIs (Anthropic, Gemini) and on Go's messages route.

## Phase 6 — Static Claude catalog and live-test relocation (B10, D1)

**Changes.**
* **B10.** `StaticClaude` drops `claude-3-5-haiku-latest` and
  `claude-sonnet-4-20250514`; the Messages API answers both with 404. Five
  wizard tests hard-coded six Claude rows:
  * `BlankSearchShowsRecommended`, `CurrentModelListed` and
    `CurrentModelOnlyForSameProvider` now derive the count from
    `StaticClaude`;
  * `SearchAgain` and `OtherFromSearchResults` expect one `haiku` match.
* **D1.** Three OpenCode live tests move from free Zen models to paid
  OpenCode Go models, and the live file's header notes why:
  * `TestLive_OpencodeChatCompletions` → Go `hy3`;
  * `TestLive_OpencodeResponses` → Go `gpt-6-luna`;
  * `TestLive_OpencodeRouteStillEnforced` → Go `gpt-6-luna`.
* **New live test.** `TestLive_StaticClaudeServed` requires every
  `StaticClaude` id to answer.

**Files:** `llmprovider/models_catalog.go`,
`llmprovider/live_static_test.go` (new), `llmprovider/live_gateways_test.go`,
`wizard/model_select_test.go`.

**Steps.**
1. Apply B.6.1.
2. Red, live (`ANTHROPIC_API_KEY`): `go test -count=1 -tags live_gateways -run '^(TestLive_StaticClaudeServed)$' ./llmprovider`. The two retired ids fail
   with `claude HTTP 404` (A.6).
3. Apply B.6.2.
4. Gate.
5. Live:
   `go test -count=1 -tags live_gateways -run 'TestLive_(StaticClaudeServed|OpencodeChatCompletions|OpencodeResponses|OpencodeRouteStillEnforced)$' -v ./llmprovider`.
   All pass.
6. Mutants (live): `p6-bad-key-chat` (kills `TestLive_OpencodeChatCompletions`), `p6-bad-key-responses` (kills `TestLive_OpencodeResponses`), `p6-route-override-ignored` (kills `TestLive_OpencodeRouteStillEnforced`) (A.6).
7. Commit.

## Phase 7 — Records, README and close-out

1. Apply these insertions, each immediately after its anchor. Each anchor
   occurs exactly once; this was checked against the current files on
   2026-09-27.

   * **`docs/0010-MADR-use-case-aware-default-model-ranking.md`.** After:

     ```text
       fixed value would override vendor intent. Revision 2's `"medium"` is
       withdrawn.
     ```

     insert:

     ```markdown
     * **Amended by [0013-MADR-remediate-debugging-pass-findings.md](0013-MADR-remediate-debugging-pass-findings.md) (Q2):** `""` means each
       provider's documented default. On the wire that is `medium` on the effort
       APIs, `high` on Grok 4.5, `{"enabled": true}` on Kilo, adaptive thinking
       with no effort on Claude 4.7 and later, a 4,096-token budget on older Claude,
       and dynamic thinking on Gemini.
     ```

   * **`docs/0010-PLAN-use-case-aware-default-model-ranking.md`.** After:

     ```text
     3. **Metadata failures are not cached.** A later listing in the same process
        retries. A success is cached for 10 minutes per URL.
     ```

     insert:

     ```markdown
        *Amended by [0013-MADR-remediate-debugging-pass-findings.md](0013-MADR-remediate-debugging-pass-findings.md) (A6):* a failure is now remembered
        for one minute, a stale document is kept when a refresh fails, and a lookup
        inside a request waits at most 5 seconds.
     ```

   * **`docs/0012-MADR-conform-providers-to-reference-clients.md`.** After:

     ```text
       * its Go gates handle defaults, and §1.1's `ErrNotPermitted` handles
         requests.
     ```

     insert:

     ```markdown
     * **`0013-MADR-remediate-debugging-pass-findings.md`:** routes six findings here:
       * B3 to §1.1: every 4xx other than 401, 403 and 429 becomes
         `ErrInvalidRequest` and its body is discarded. §1.1's table also needs
         OpenCode's 403 `FreeTierError` (Zen's free tier refuses other clients) and
         its 402 "Upstream request failed: Insufficient account funds";
       * B4 (408 is terminal) and B6 (`GenerateItemsWithRetry` keeps its own loop)
         to §1.2;
       * B7 (no `x-opencode-session` on listings, and a new id per health probe) to
         §1.4;
       * D5 (the live suite's `skipIfTransient` skips every `ErrInvalidRequest`, so
         a wire regression answering 400 is skipped) to §1.1, whose typed errors
         let it skip only the transient classes;
       * D6 (a 30-second response-header timeout on OpenCode Go failed one live
         run) to §1.3.
     ```

   * **`README.md`.** After:

     ```text
     Scripts that drive the wizard need one extra (blank) line before each model
     and fallback selection.
     ```

     insert:

     ```markdown

     A live listing is bounded at 10 seconds; `Options.DiscoverLimit` can shorten
     that bound but not extend it. When the listing fails, the wizard's notice names
     the cause, and `ModelCatalog.Err` carries it for other callers.
     ```

   * **`README.md`.** After:

     ```text
     forced-refresh retry after a 401; static-key 401/403 responses and other 4xx
     responses are not retried.
     ```

     insert:

     ```markdown

     `WithReasoningEffort` sets the effort for every provider's thinking path.
     Effort APIs send it as given. Claude 4.7 and later use adaptive thinking with
     `output_config.effort`. Older Claude models map `low` to a 1,024-token budget.
     Gemini maps `low` to `thinkingLevel` on Gemini 3 and to a 1,024-token budget on
     Gemini 2.x. `DiscoverModels` on Kilo, OpenCode and Hugging Face ranks with the
     provider's `WithModelProfile` and `WithModelMetadataURL`.
     ```


2. Full live suite: `go test -count=1 -tags live_gateways -run Live -v ./llmprovider`.
   * Acceptance: no FAIL.
   * Skips only on 429 or an unset credential.
   * A transport timeout is re-run once, and both runs are recorded.
3. Final gate (§0.3) on the whole tree.
4. Fill §10 (execution record) with each phase's commit, red output, gate
   output, mutant results and live results.
5. Set this plan to `status: complete`. The MADR stays `accepted`.
6. Commit.

## 7. Acceptance criteria

1. Every test named in a red step failed before its fix, with the message
   recorded in §10. Every mutant listed was killed at run time, and the one
   equivalence mutant survived.
2. The gate (§0.3) passes after every phase.
3. `TestListModelCatalog_Snapshot20260926` passes unchanged. MADR 0010 §7's
   sixes do not move.
4. The full live suite has no FAIL (Phase 7, step 2).
5. The 0010 MADR, the 0010 PLAN, 0012 and the README carry Phase 7's text.

## 8. Rollout and rollback

**Consumer-visible changes** (prepare-commit-msg, magictools, magicdev pick
them up on their next mcplib bump):
* `ModelCatalog` gains `Err`. This is additive; `ModelCatalog` holds slices,
  so no consumer compares it with `==`.
* A provider constructed with `WithModelProfile(ProfileCapable)` now discovers
  in capable order. Before, it discovered in utility order.
* `DiscoverModels` returns within 10 s of listing, on every provider.
* `GenerateThinking` works on Claude 4.7 and later (HTTP 400 before), and
  honours `WithReasoningEffort` on Claude and Gemini.
* The wizard warns with the listing's cause, never offers another provider's
  model as a default, and refuses blank ids.
* `StaticClaude` lists four ids, down from six.

**Rollback.** Each phase is one commit. Revert phases in reverse order, since
Phase 4 builds on Phase 1's `catalogFrom`. There is no data or configuration
migration.

**Risks.**
* **A future Claude id outside the version pattern.** An unversioned
  `claude-` id is treated as current (adaptive). A budget-only model with such
  an id would get HTTP 400. `TestLive_ClaudeThinkingShapes` and
  `TestLive_StaticClaudeServed` catch it for every static id.
* **Anthropic or Google changing which shapes they accept.** The Phase 5 live
  tests report it as a failure, not a skip.
* **A recovered metadata host is ignored for up to a minute.** This is
  accepted in exchange for bounding a dead one.

## 9. Deviation log

*(Empty. Add a dated entry for every departure from this plan, before
continuing.)*

## 10. Execution record

*(Filled during execution: per phase, the commit, the red output, the gate
output, the mutant results and the live results.)*

## Appendix A — Proof record (2026-09-27, scratch copies of `e219e11`)

Each table quotes the failure message(s) the named test printed on the
unfixed code. Mutant rows show the exact anchor, the replacement, and the
result.

### A.1 Phase 1 — Ranking correctness

| Test | Failed (with subtests) | Failure on `e219e11` |
|---|---|---|
| `TestListModelCatalog_DuplicateIDs` | 1 | `Recommended lists a/pro 2 times: [a/pro a/pro b/pro]`<br>`Usable lists a/pro 2 times: [a/pro a/pro b/pro]` |
| `TestRankRecommended_SkipsDuplicateCandidates` | 1 | `ranked = [x/a x/a y/b]` |
| `TestKiloCandidate_AbsentPriceIsUnknown` | 1 | `no pricing block: costKnown=true eligible=false, want unknown and eligible` |
| `TestKiloPrice_NonFiniteIsUnknown` | 1 | `kiloPrice("") = 0, known; want unknown`<br>`kiloPrice("  ") = 0, known; want unknown`<br>`kiloPrice("Infinity") = +Inf, known; want unknown`<br>`kiloPrice("+Inf") = +Inf, known; want unknown`<br>`kiloPrice("inf") = +Inf, known; want unknown` |
| `TestRankRecommended_InfinitePriceNotCheapest` | 1 | `ranked = [z/infinite a/cheap b/mid]` |
| `TestKiloCandidate_Fields` | 1 | `org/blank: cost = 0/true, want 0/false` |

Gate after the fix: gofmt PASS, golint-discovery.go PASS, golint-model_ranking.go PASS, golint-model_ranking_test.go PASS, golint-ranking_edge_test.go PASS, vet PASS, vet-live PASS, lint PASS, test PASS, race PASS.

### A.2 Phase 2 — Discovery wiring

| Test | Failed (with subtests) | Failure on `e219e11` |
|---|---|---|
| `TestDiscoverModels_ListingBounded` | 7 | `GET /v1/models ran without a deadline, want one within 10s`<br>`GET /models ran without a deadline, want one within 10s`<br>`GET /api/tags ran without a deadline, want one within 10s`<br>`GET /models ran without a deadline, want one within 10s`<br>`GET /models ran without a deadline, want one within 10s`<br>`GET /models ran without a deadline, want one within 10s` |
| `TestDiscoverModels_HonoursRankingOptions` | 4 | `ranked = [b/flash d/mid c/pro a/flash-lite e/mini f/large]`<br>`ranked = [v/no-reason a/large b/mid c/flash d/uncovered]`<br>`DiscoverModels fetched the environment's metadata URL 1 times, want 0`<br>`ranked = [qwen3.8-flash glm-5.3-flash mimo-v2.6-flash gpt-6-luna hy3 kimi-k2.6]`<br>`DiscoverModels fetched the environment's metadata URL 2 times, want 0` |

Gate after the fix: gofmt PASS, golint-discovery.go PASS, golint-discovery_wiring_test.go PASS, golint-huggingface.go PASS, golint-kilo.go PASS, golint-opencode.go PASS, golint-options.go PASS, vet PASS, vet-live PASS, lint PASS, test PASS, race PASS.

### A.3 Phase 3 — Metadata on the request path

| Test | Failed (with subtests) | Failure on `e219e11` |
|---|---|---|
| `TestChatReasoningEffort_FailureBackoff` | 1 | `3 thinking calls made 3 metadata fetches, want 1` |
| `TestChatReasoningEffort_LookupHasDeadline` | 1 | `metadata lookup ran without a deadline, want one within 5s` |
| `TestLoadModelMetadata_StaleOnRefreshFailure` | 1 | `refresh failed: doc=map[] err=model metadata: http://127.0.0.1:<port> returned HTTP 500, want the stale document` |
| `TestLoadModelMetadata_FailureCachedBriefly` | 1 | `requests = 2, want 1 (the failure is cached)` |

Gate after the fix: gofmt PASS, golint-metadata_request_test.go PASS, golint-model_metadata.go PASS, golint-model_metadata_test.go PASS, golint-opencode.go PASS, vet PASS, vet-live PASS, lint PASS, test PASS, race PASS.

Mutants:

| Mutant | File | Anchor → replacement | Test | Result |
|---|---|---|---|---|
| `p3-backoff-longer` | `llmprovider/model_metadata.go` | `` time.Since(e.failed) < modelMetadataRetryAfter `` → `` time.Since(e.failed) < 2*modelMetadataRetryAfter `` | `TestLoadModelMetadata_FailureRetriedAfterBackoff` | killed: `load after the backoff: doc=map[] err=model metadata: http://127.0.0.1:<port> returned HTTP 500, want a fresh fetch` |

### A.4 Phase 4 — Wizard and the listing's cause

| Test | Failed (with subtests) | Failure on `e219e11` |
|---|---|---|
| `TestConfigureLLM_FallbackPicksDeduped` | 1 | `Fallbacks = ["claude-sonnet-5" "claude-sonnet-5" "claude-sonnet-4-6"], want ["claude-sonnet-5" "claude-sonnet-4-6"]` |
| `TestMultiSelect_RepeatedIndexCountsOnce` | 1 | `MultiSelect(1,1,2) = [0 0 1], want [0 1]` |
| `TestConfigureLLM_StaticCatalogNotice` | 1 | `static notice seen 0 times, want 1: [live model listing for OpenCode Zen is unavailable; search covers the built-in catalog only]` |
| `TestConfigureLLM_OtherDefaultsOnlyToSameProvider` | 3 | `Enter at Other saved provider=gemini model="claude-opus-5", want an error (no default from another provider)`<br>`Enter at No models found saved provider=ollama model="claude-opus-5", want an error` |
| `TestConfigureLLM_FallbackExclusionIgnoresCase` | 1 | `fallback menu has 6 rows, want 5 (claude-haiku-4-5 is the primary)` |
| `TestConfigureLLM_BlankModelIDRefused` | 1 | `Other with "   ": model="   ", want an error`<br>`No models found with blanks: model="   ", want an error` |

Gate after the fix: gofmt PASS, golint-catalog_err_test.go PASS, golint-discovery.go PASS, golint-matcher_coverage_test.go PASS, golint-configure.go PASS, golint-model_select.go PASS, golint-model_select_edge_test.go PASS, golint-model_select_test.go PASS, golint-text_prompter.go PASS, vet PASS, vet-live PASS, lint PASS, test PASS, race PASS.

Mutants:

| Mutant | File | Anchor → replacement | Test | Result |
|---|---|---|---|---|
| `p4-err-dropped` | `llmprovider/discovery.go` | `` \t\tcat.Err = fetchErr\n `` → `` \t\tcat.Err = nil\n `` | `TestListModelCatalog_ErrExplainsDegrade` | killed: `failed listing: Live=false Err=<nil>, want Live false and an HTTP 500 cause` |
| `p4-err-dropped-wizard` | `llmprovider/discovery.go` | `` \t\tcat.Err = fetchErr\n `` → `` \t\tcat.Err = nil\n `` | `TestConfigureLLM_StaticCatalogNotice` | killed: `static notice seen 0 times, want 1: [live model listing for OpenCode Zen is unavailable; search covers the built-in catalog only]` |
| `p4-no-listing-warning` | `wizard/configure.go` | `` \t\t\tp.Notify(LevelWarn, \"could not list models for %s (%v)\", d.Label, err)\n `` → `` \t\t\t_ = err\n `` | `TestConfigureLLM_ListingErrorWarns` | killed: `listing warning seen 0 times, want 1: [cannot reach http://127.0.0.1:<port>: could not reach Ollama at http://127.0.0.1:<port>: Get "http://127.0.0.1:<port>/api` |
| `p4-no-static-listing-warning` | `wizard/configure.go` | `` \t\tp.Notify(LevelWarn, \"could not list models for %s (%v); using the built-in catalog\", d.Label, err)\n `` → `` \t\t_ = err\n `` | `TestConfigureLLM_ListingTokenFailureUsesStaticCatalog` | killed: `listing warning seen 0 times, want 1: []` |
| `p4-unusable-listing-silent` | `wizard/configure.go` | `` \tdefault:\n\t\tp.Notify(LevelInfo, \"live model listing for %s is unavailable; search covers the built-in catalog only\", d.Label)\n `` → `` \tdefault:\n `` | `TestConfigureLLM_UnusableListingNotice` | killed: `notice "live model listing for Claude (Anthropic) is unavailable; search covers the built-in catalog only" seen 0 times, want 1: []` |
| `p4-empty-listing-sentinel` | `wizard/configure.go` | `` \tif len(cat.Recommended) == 0 {\n\t\treturn fallback\n\t}\n `` → `` \tif len(cat.Recommended) == 0 {\n\t\treturn llmprovider.ModelCatalog{Recommended: []string{\"sentinel\"}}\n\t}\n `` | `TestConfigureLLM_OllamaEmptyListing` | killed: `ConfigureLLM: model="" err=select model: fakePrompter: unexpected Select("Choose a Ollama (local) model:"), want llama3` |
| `p4-empty-listing-deleted` | `wizard/configure.go` | `` \tif len(cat.Recommended) == 0 {\n\t\treturn fallback\n\t}\n `` → *(deleted)* | `TestConfigureLLM_OllamaEmptyListing` | survived (as required) |
| `p4-default-row` | `wizard/model_select.go` | `` \t\t\tdefaultIdx, listed = i, true\n `` → `` \t\t\tdefaultIdx, listed = i-i, true\n `` | `TestConfigureLLM_DefaultRowIsExistingModel` | killed: `default = 0, model = "claude-sonnet-4-6"; want 2 and "claude-sonnet-4-6"` |
| `p4-blank-round-nothing-left` | `wizard/model_select.go` | `` \t\t\tif len(recs) == 0 {\n\t\t\t\treturn chosen, nil\n\t\t\t}\n `` → *(deleted)* | `TestConfigureLLM_BlankFallbackRoundWithNothingLeft` | killed: `ConfigureLLM: select fallbacks: fakePrompter: unexpected MultiSelect("Choose fallback models (optional):")` |
| `p4-fallback-no-match-silent` | `wizard/model_select.go` | `` \t\tmatches := llmprovider.SearchModels(d.ID, usable, q)\n\t\tif len(matches) == 0 {\n\t\t\tp.Notify(LevelWarn, \"no %s models match %q\", d.Label, q)\n `` → `` \t\tmatches := llmprovider.SearchModels(d.ID, usable, q)\n\t\tif len(matches) == 0 {\n `` | `TestConfigureLLM_FallbackSearchNoMatches` | killed: `no-match warning seen 0 times, want 1: []` |
| `p4-glob-question-literal` | `llmprovider/model_matcher.go` | `` \t\tcase '?':\n\t\t\tb.WriteString(\".\")\n `` → `` \t\tcase '?':\n\t\t\tb.WriteString(`\\?`)\n `` | `TestSearchModels_GlobQuestionMark` | killed: `claude-sonnet-? = [], want only claude-sonnet-5` |
| `p4-no-label-tier` | `llmprovider/model_matcher.go` | `` \tcase strings.Contains(lowerLabel, q):\n\t\treturn scoreLabelSubstring, true\n `` → *(deleted)* | `TestSearchModels_LabelSubstringTier` | killed: `balanced speed = [], want claude-sonnet-5 at scoreLabelSubstring` |

### A.5 Phase 5 — Thinking shapes

| Test | Failed (with subtests) | Failure on `e219e11` |
|---|---|---|
| `TestThinkingWire_Claude` | 6 | `thinking = map[budget_tokens:4096 type:enabled], want map[type:adaptive]`<br>`output_config = <nil>, want map[effort:low]`<br>`thinking = map[budget_tokens:4096 type:enabled], want map[type:adaptive]`<br>`thinking = map[budget_tokens:9000 type:enabled], want map[type:adaptive]`<br>`max_tokens = 13096, want 8192`<br>`thinking = map[budget_tokens:4096 type:enabled], want map[budget_tokens:1024 type:enabled]`<br>`thinking = map[budget_tokens:4096 type:enabled], want map[budget_tokens:1024 type:enabled]` |
| `TestThinkingWire_Gemini` | 3 | `thinkingConfig = map[thinkingBudget:-1], want map[thinkingLevel:low]`<br>`thinkingConfig = map[thinkingBudget:-1], want map[thinkingBudget:1024]` |
| `TestThinkingWire_OpencodeRoutes` | 4 | `thinking = map[budget_tokens:4096 type:enabled], want map[type:adaptive]`<br>`output_config = <nil>, want map[effort:low]`<br>`thinking = map[budget_tokens:4096 type:enabled], want map[budget_tokens:1024 type:enabled]`<br>`thinkingConfig = map[thinkingBudget:-1], want map[thinkingLevel:low]` |
| `TestLive_ClaudeThinkingShapes` | 5 | `GenerateThinking: llm: invalid request: claude HTTP 400`<br>`GenerateThinking: llm: invalid request: claude HTTP 400`<br>`GenerateThinking: llm: invalid request: claude HTTP 400`<br>`GenerateThinking: llm: invalid request: claude HTTP 400` |

Gate after the fix: gofmt PASS, golint-claude.go PASS, golint-gemini.go PASS, golint-live_thinking_test.go PASS, golint-model_profile.go PASS, golint-opencode.go PASS, golint-opencode_test.go PASS, golint-options.go PASS, golint-thinking_test.go PASS, golint-thinking_wire.go PASS, golint-thinking_wire_test.go PASS, vet PASS, vet-live PASS, lint PASS, test PASS, race PASS.

Mutants:

| Mutant | File | Anchor → replacement | Test | Result |
|---|---|---|---|---|
| `p5-gemini-legacy-ignored` | `llmprovider/thinking_wire.go` | `` \tcase geminiLegacyRE.MatchString(model):\n `` → `` \tcase false && geminiLegacyRE.MatchString(model):\n `` | `TestLive_GeminiThinkingShapes` | killed: `GenerateThinking: llm: invalid request: gemini HTTP 400` |
| `p5-messages-bad-type` | `llmprovider/thinking_wire.go` | `` body[jsonKeyThinking] = map[string]any{jsonKeyType: jsonKeyEnabled, \"budget_tokens\": budget} `` → `` body[jsonKeyThinking] = map[string]any{jsonKeyType: \"bogus\", \"budget_tokens\": budget} `` | `TestLive_OpencodeMessagesThinking` | killed: `GenerateThinking: llm: invalid request: opencode-go/messages HTTP 400` |

### A.6 Phase 6 — Static Claude catalog and live tests

| Test | Failed (with subtests) | Failure on `e219e11` |
|---|---|---|
| `TestLive_StaticClaudeServed` | 3 | `claude-3-5-haiku-latest: llm: invalid request: claude HTTP 404`<br>`claude-sonnet-4-20250514: llm: invalid request: claude HTTP 404` |

Gate after the fix: gofmt PASS, golint-live_gateways_test.go PASS, golint-live_static_test.go PASS, golint-models_catalog.go PASS, golint-model_select_test.go PASS, vet PASS, vet-live PASS, lint PASS, test PASS, race PASS.

Mutants:

| Mutant | File | Anchor → replacement | Test | Result |
|---|---|---|---|---|
| `p6-bad-key-chat` | `llmprovider/opencode.go` | `` \t\treturn oauthAuthorizationHeader, \"Bearer \" + key\n `` → `` \t\treturn oauthAuthorizationHeader, \"Bearer x\" + key\n `` | `TestLive_OpencodeChatCompletions` | killed: `Generate: llm: authentication failed: opencode-go/chat_completions HTTP 401` |
| `p6-bad-key-responses` | `llmprovider/opencode.go` | `` \t\treturn oauthAuthorizationHeader, \"Bearer \" + key\n `` → `` \t\treturn oauthAuthorizationHeader, \"Bearer x\" + key\n `` | `TestLive_OpencodeResponses` | killed: `GenerateItems: llm: authentication failed: opencode-go/responses HTTP 401` |
| `p6-route-override-ignored` | `llmprovider/opencode.go` | `` route, err := resolveOpencodeRoute(gateway, model, cfg.OpencodeRoute) `` → `` route, err := resolveOpencodeRoute(gateway, model, \"\") `` | `TestLive_OpencodeRouteStillEnforced` | killed: `DRIFT (probed 2026-08-28): gpt-6-luna now succeeds on /chat/completions. Routes were measured as non-interchangeable; if that changed, the route table may no lo` |

### A.7 Live runs on the proven tree

* Phase 5 live step: 13 PASS, 0 FAIL, 0 SKIP.
* Phase 6 live step: 8 PASS, 0 FAIL, 0 SKIP.
* Full live suite on the final tree: 51 PASS, 0 FAIL, 1 SKIP; SKIP: `TestLive_KiloChatCompletions`.
  The SKIP is a Kilo 429 (MADR D4). The one transient failure of the `ca29b81` proof (MADR D6) did not recur.

### A.8 Coverage and reproduction

* Before the plan, a coverage profile at `e219e11` (`-coverpkg` over `llmprovider` and `wizard`) showed zero hits on every block the MADR lists as uncovered under *Coverage gaps*. On the final tree every one of those blocks is hit, including both arms of the listing-error branch.
* Applying Appendix B in order to a fresh `e219e11` archive with `git apply` reproduced the proven tree: 229 files compared, 0 mismatches, 0 extra files.
* A clean end-to-end re-run of red and green for Phases 1–6 on 2026-09-27 ended `FINAL=OK`.

## Appendix B — Diffs

Apply in order (§0.2). Each `.1` diff holds the phase's tests, and each `.2`
diff its fix. A `.2` diff also carries any test that needs the fix to compile
or that the fix changes.

### B.1.1 Phase 1 tests

```diff
diff --git a/llmprovider/model_ranking_test.go b/llmprovider/model_ranking_test.go
--- a/llmprovider/model_ranking_test.go
+++ b/llmprovider/model_ranking_test.go
@@ -317,7 +317,7 @@
 		costKnown bool
 	}{
 		{`{"id":"org/free","pricing":{"prompt":"0","completion":"0"}}`, 0, true},
-		{`{"id":"org/blank","pricing":{"prompt":"","completion":""}}`, 0, true},
+		{`{"id":"org/blank","pricing":{"prompt":"","completion":""}}`, 0, false},
 		{`{"id":"org/bad","pricing":{"prompt":"abc","completion":"0.000001"}}`, 0, false},
 	} {
 		c := kiloCandidate(decodeKiloEntry(t, tc.js), refNow)
diff --git a/llmprovider/ranking_edge_test.go b/llmprovider/ranking_edge_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/ranking_edge_test.go
@@ -0,0 +1,93 @@
+package llmprovider
+
+import (
+	"context"
+	"fmt"
+	"strings"
+	"testing"
+)
+
+// countID reports how many times id appears in ids.
+func countID(ids []string, id string) int {
+	n := 0
+	for _, v := range ids {
+		if v == id {
+			n++
+		}
+	}
+	return n
+}
+
+// TestListModelCatalog_DuplicateIDs pins MADR 0013 A1: a listing that repeats
+// an id yields it once in Usable and once in Recommended.
+func TestListModelCatalog_DuplicateIDs(t *testing.T) {
+	pinRankingNow(t, refNow)
+	a := kiloRankEntry("a/pro", "A Pro", "0.000001", "0.000004", 20, true, "")
+	b := kiloRankEntry("b/pro", "B Pro", "0.000002", "0.000004", 20, true, "")
+	srv := serveBody(t, `{"data":[`+strings.Join([]string{a, a, b}, ",")+`]}`)
+	cat := listCatalog(context.Background(), t, ProviderKilo, WithBaseURL(srv.URL))
+	if n := countID(cat.Recommended, "a/pro"); n != 1 {
+		t.Errorf("Recommended lists a/pro %d times: %v", n, cat.Recommended)
+	}
+	if n := countID(cat.Usable, "a/pro"); n != 1 {
+		t.Errorf("Usable lists a/pro %d times: %v", n, cat.Usable)
+	}
+}
+
+// TestRankRecommended_SkipsDuplicateCandidates pins MADR 0013 A1 at the
+// ranker: two candidates with one id take one slot.
+func TestRankRecommended_SkipsDuplicateCandidates(t *testing.T) {
+	c := good("x/a", "x", 1, 10)
+	got := rankRecommended(ProfileUtility, ProviderKilo, []rankCandidate{c, c, good("y/b", "y", 2, 10)}, nil)
+	assertRanked(t, got, []string{"x/a", "y/b"})
+}
+
+// kiloPricedEntry is one reasoning Kilo entry created ten days before refNow;
+// pricing is a JSON member (with its leading comma) or "".
+func kiloPricedEntry(t *testing.T, id, pricing string) kiloCatalogEntry {
+	t.Helper()
+	return decodeKiloEntry(t, fmt.Sprintf(`{"id":%q,"name":%q,"created":%d,"context_length":200000,`+
+		`"supported_parameters":["tools","reasoning"]%s}`, id, id, refNow.AddDate(0, 0, -10).Unix(), pricing))
+}
+
+// TestKiloCandidate_AbsentPriceIsUnknown pins MADR 0013 A2 (MADR 0010 §3: an
+// absent field never excludes). An explicit "0" is still free and excluded.
+func TestKiloCandidate_AbsentPriceIsUnknown(t *testing.T) {
+	absent := kiloCandidate(kiloPricedEntry(t, "x/y", ""), refNow)
+	if absent.costKnown || !absent.eligible(ProviderKilo) {
+		t.Errorf("no pricing block: costKnown=%v eligible=%v, want unknown and eligible",
+			absent.costKnown, absent.eligible(ProviderKilo))
+	}
+	free := kiloCandidate(kiloPricedEntry(t, "x/free", `,"pricing":{"prompt":"0","completion":"0"}`), refNow)
+	if !free.costKnown || free.eligible(ProviderKilo) {
+		t.Errorf(`pricing "0": costKnown=%v eligible=%v, want known and excluded`,
+			free.costKnown, free.eligible(ProviderKilo))
+	}
+}
+
+// TestKiloPrice_NonFiniteIsUnknown pins MADR 0013 A2–A3: blank and
+// non-finite prices are unknown; finite non-negative prices parse.
+func TestKiloPrice_NonFiniteIsUnknown(t *testing.T) {
+	for _, s := range []string{"", "  ", "Infinity", "+Inf", "inf", "NaN", "-1", "abc"} {
+		if v, ok := kiloPrice(s); ok {
+			t.Errorf("kiloPrice(%q) = %v, known; want unknown", s, v)
+		}
+	}
+	for s, want := range map[string]float64{"0": 0, "0.000001": 0.000001, " 2e-6 ": 2e-6} {
+		if v, ok := kiloPrice(s); !ok || v != want {
+			t.Errorf("kiloPrice(%q) = %v/%v, want %v/true", s, v, ok, want)
+		}
+	}
+}
+
+// TestRankRecommended_InfinitePriceNotCheapest pins MADR 0013 A3: before the
+// fix an "Infinity" price made the blend NaN, which cmp.Compare orders first.
+func TestRankRecommended_InfinitePriceNotCheapest(t *testing.T) {
+	mk := func(id, price string) rankCandidate {
+		return kiloCandidate(kiloPricedEntry(t, id,
+			fmt.Sprintf(`,"pricing":{"prompt":%q,"completion":%q}`, price, price)), refNow)
+	}
+	got := rankRecommended(ProfileUtility, ProviderKilo,
+		[]rankCandidate{mk("a/cheap", "0.0000001"), mk("b/mid", "0.000001"), mk("z/infinite", "Infinity")}, nil)
+	assertRanked(t, got, []string{"a/cheap", "b/mid", "z/infinite"})
+}
```

### B.1.2 Phase 1 fix

```diff
diff --git a/llmprovider/discovery.go b/llmprovider/discovery.go
--- a/llmprovider/discovery.go
+++ b/llmprovider/discovery.go
@@ -33,8 +33,8 @@
 	// Recommended is what ListAvailableModels returns: at most
 	// MaxListedModels ids, curated against the static catalog.
 	Recommended []string
-	// Usable is every id the provider's usability filters admit, in listing
-	// order, uncapped. It equals Recommended when Live is false.
+	// Usable is every id the provider's usability filters admit, once each,
+	// in listing order, uncapped. It equals Recommended when Live is false.
 	Usable []string
 	// Live reports whether Usable came from the provider's listing rather
 	// than the static catalog.
@@ -96,6 +96,7 @@
 // Ollama has always had: a failed fetch, or one that yields no usable id,
 // substitutes the static catalog.
 func catalogFrom(usable []string, fetchErr error, static []string, curate func([]string) []string) ModelCatalog {
+	usable = uniqueIDs(usable)
 	if fetchErr != nil || len(usable) == 0 {
 		return staticCatalog(static)
 	}
@@ -104,6 +105,19 @@
 		return staticCatalog(static)
 	}
 	return ModelCatalog{Recommended: recommended, Usable: usable, Live: true}
+}
+
+// uniqueIDs drops repeated ids, keeping the first of each (MADR 0013 A1).
+func uniqueIDs(ids []string) []string {
+	seen := make(map[string]struct{}, len(ids))
+	out := make([]string, 0, len(ids))
+	for _, id := range ids {
+		if _, dup := seen[id]; !dup {
+			seen[id] = struct{}{}
+			out = append(out, id)
+		}
+	}
+	return out
 }
 
 // staticCatalog wraps a caller-owned copy of a static catalog.
diff --git a/llmprovider/model_ranking.go b/llmprovider/model_ranking.go
--- a/llmprovider/model_ranking.go
+++ b/llmprovider/model_ranking.go
@@ -197,9 +197,9 @@
 	return profile != ProfileCapable && provider == ProviderKilo && strings.HasPrefix(id, "kilo-auto/")
 }
 
-// rankRecommended returns at most MaxListedModels ids: the eligible
+// rankRecommended returns at most MaxListedModels distinct ids: the eligible
 // candidates in profile order, at most maxPerRankGroup per group, then fill in
-// order, skipping ids already chosen or excluded (MADR 0010 §4).
+// order, skipping ids already chosen or excluded (MADR 0010 §4, MADR 0013 A1).
 func rankRecommended(profile ModelProfile, provider string, cands []rankCandidate, fill []string) []string {
 	var eligible []rankCandidate
 	for _, c := range cands {
@@ -219,7 +219,7 @@
 		if len(out) == MaxListedModels {
 			break
 		}
-		if perGroup[c.group] >= maxPerRankGroup {
+		if perGroup[c.group] >= maxPerRankGroup || slices.Contains(out, c.id) {
 			continue
 		}
 		perGroup[c.group]++
@@ -300,13 +300,10 @@
 	return c
 }
 
-// kiloPrice parses one Kilo per-token price. An empty price is 0; a negative
-// ("-1", the variable-priced kilo-auto tiers) or unparseable one is unknown.
+// kiloPrice parses one Kilo per-token price. A blank, negative ("-1", the
+// variable-priced kilo-auto tiers), non-finite or unparseable price is unknown
+// (MADR 0010 §3; MADR 0013 A2–A3).
 func kiloPrice(s string) (float64, bool) {
-	s = strings.TrimSpace(s)
-	if s == "" {
-		return 0, true
-	}
-	v, err := strconv.ParseFloat(s, 64)
-	return v, err == nil && v >= 0
-}
+	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
+	return v, err == nil && v >= 0 && !math.IsInf(v, 1)
+}
```

### B.2.1 Phase 2 tests

```diff
diff --git a/llmprovider/discovery_wiring_test.go b/llmprovider/discovery_wiring_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/discovery_wiring_test.go
@@ -0,0 +1,178 @@
+package llmprovider
+
+import (
+	"context"
+	"net/http"
+	"net/http/httptest"
+	"slices"
+	"strings"
+	"sync"
+	"testing"
+	"time"
+)
+
+// deadlineTransport records, for the first request of each method and path,
+// how long its context had left (-1 without a deadline), then forwards it.
+type deadlineTransport struct {
+	mu   sync.Mutex
+	seen map[string]time.Duration
+}
+
+func (d *deadlineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
+	left := time.Duration(-1)
+	if dl, ok := r.Context().Deadline(); ok {
+		left = time.Until(dl)
+	}
+	key := r.Method + " " + r.URL.Path
+	d.mu.Lock()
+	if d.seen == nil {
+		d.seen = map[string]time.Duration{}
+	}
+	if _, ok := d.seen[key]; !ok {
+		d.seen[key] = left
+	}
+	d.mu.Unlock()
+	return http.DefaultTransport.RoundTrip(r)
+}
+
+func (d *deadlineTransport) left(key string) (time.Duration, bool) {
+	d.mu.Lock()
+	defer d.mu.Unlock()
+	v, ok := d.seen[key]
+	return v, ok
+}
+
+// discoverer is the DiscoverModels half of a provider.
+type discoverer interface {
+	DiscoverModels(context.Context) ([]string, error)
+}
+
+// TestDiscoverModels_ListingBounded pins MADR 0013 A5: every DiscoverModels
+// listing runs under the 10 s bound ListModelCatalogWithSource applies, so a
+// slow listing or metadata host cannot hold discovery past it.
+func TestDiscoverModels_ListingBounded(t *testing.T) {
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
+		w.WriteHeader(http.StatusInternalServerError)
+	}))
+	t.Cleanup(srv.Close)
+	for _, tc := range []struct {
+		name, listing string
+		build         func(opts ...ProviderOption) (discoverer, error)
+	}{
+		{"claude", "GET /v1/models", func(o ...ProviderOption) (discoverer, error) { return NewClaude("k", "m", o...) }},
+		{"gemini", "GET /models", func(o ...ProviderOption) (discoverer, error) {
+			return NewGemini(context.Background(), "k", "m", o...)
+		}},
+		{"ollama", "GET /api/tags", func(o ...ProviderOption) (discoverer, error) { return NewOllama("", "m", o...) }},
+		{"opencode", "GET /models", func(o ...ProviderOption) (discoverer, error) {
+			return NewOpencode(ProviderOpencodeZen, "k", "glm-5.3-flash", o...)
+		}},
+		{"huggingface", "GET /models", func(o ...ProviderOption) (discoverer, error) { return NewHuggingFace("k", "m", o...) }},
+		{"kilo", "GET /models", func(o ...ProviderOption) (discoverer, error) { return NewKilo("k", "m", o...) }},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			rec := &deadlineTransport{}
+			p, err := tc.build(WithHTTPClient(&http.Client{Transport: rec}), WithBaseURL(srv.URL))
+			if err != nil {
+				t.Fatalf("construct: %v", err)
+			}
+			_, _ = p.DiscoverModels(context.Background())
+			left, ok := rec.left(tc.listing)
+			if !ok {
+				t.Fatalf("no %s request was made", tc.listing)
+			}
+			switch {
+			case left < 0:
+				t.Errorf("%s ran without a deadline, want one within 10s", tc.listing)
+			case left > 10*time.Second:
+				t.Errorf("%s ran with %v left, want at most 10s", tc.listing, left.Round(time.Second))
+			}
+		})
+	}
+}
+
+// getOnly answers GET with body and anything else (the health probes) with 500.
+func getOnly(t *testing.T, body string) *httptest.Server {
+	t.Helper()
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.Method != http.MethodGet {
+			w.WriteHeader(http.StatusInternalServerError)
+			return
+		}
+		_, _ = w.Write([]byte(body))
+	}))
+	t.Cleanup(srv.Close)
+	return srv
+}
+
+// TestDiscoverModels_HonoursRankingOptions pins MADR 0013 A4: with every
+// probe failing, DiscoverModels returns exactly what ListModelCatalog
+// recommends for the same profile and metadata URL, and never reads the
+// environment's metadata URL when an option names one.
+func TestDiscoverModels_HonoursRankingOptions(t *testing.T) {
+	pinRankingNow(t, refNow)
+	enableModelMetadata(t)
+	envMeta, envHits := metadataServer(t, http.StatusInternalServerError, "")
+	t.Setenv(envModelMetadataURL, envMeta.URL)
+
+	kiloListing := `{"data":[` + strings.Join([]string{
+		kiloRankEntry("a/flash-lite", "A Flash Lite", "0.0000001", "0.0000004", 20, true, ""),
+		kiloRankEntry("b/flash", "B Flash", "0.0000003", "0.0000012", 10, true, `"terminalBench":{"overallScore":0.75}`),
+		kiloRankEntry("c/pro", "C Pro", "0.000005", "0.000025", 40, true, `"terminalBench":{"overallScore":0.80}`),
+		kiloRankEntry("d/mid", "D Mid", "0.000001", "0.000004", 60, true, `"preferredIndex":2`),
+		kiloRankEntry("e/mini", "E Mini", "0.0000002", "0.0000008", 90, true, ""),
+		kiloRankEntry("f/large", "F Large", "0.000002", "0.000008", 30, true, ""),
+		kiloRankEntry("kilo-auto/efficient", "Auto Efficient", "-1", "-1", -1, true, `"preferredIndex":0`),
+	}, ",") + `]}`
+	zenIDs := []string{"glm-5.3-flash", "qwen3.8-flash", "kimi-k2.6", "gpt-6-luna", "hy3", "mimo-v2.6-flash", "deepseek-v4-pro"}
+	zenMeta := `{` + mdSection("opencode",
+		mdModel("glm-5.3-flash", "glm-flash", true, 0.15, 0.5, "2026-08-26"),
+		mdModel("qwen3.8-flash", "qwen-flash", true, 0.2, 0.8, "2026-08-20"),
+		mdModel("kimi-k2.6", "kimi", true, 0.6, 2.5, "2026-07-01"),
+		mdModel("gpt-6-luna", "gpt-luna", true, 0.1, 0.5, "2026-09-22"),
+		mdModel("hy3", "hy", true, 0.3, 1.2, "2026-08-01"),
+		mdModel("mimo-v2.6-flash", "mimo", true, 0.14, 0.28, "2026-09-22"),
+		mdModel("deepseek-v4-pro", "deepseek-pro", true, 2, 8, "2026-09-10"),
+	) + `}`
+
+	for _, tc := range []struct {
+		provider, listing, meta string
+		build                   func(opts ...ProviderOption) (discoverer, error)
+	}{
+		{ProviderKilo, kiloListing, "", func(o ...ProviderOption) (discoverer, error) { return NewKilo("k", "m", o...) }},
+		{ProviderHuggingFace, hfRankListing, hfRankMetadata, func(o ...ProviderOption) (discoverer, error) {
+			return NewHuggingFace("k", "m", o...)
+		}},
+		{ProviderOpencodeZen, zenStyleListing(zenIDs...), zenMeta, func(o ...ProviderOption) (discoverer, error) {
+			return NewOpencode(ProviderOpencodeZen, "k", "glm-5.3-flash", o...)
+		}},
+	} {
+		t.Run(tc.provider, func(t *testing.T) {
+			listing := getOnly(t, tc.listing)
+			opts := []ProviderOption{WithBaseURL(listing.URL)}
+			if tc.meta != "" {
+				meta, _ := metadataServer(t, http.StatusOK, tc.meta)
+				opts = append(opts, WithModelMetadataURL(meta.URL))
+			}
+			utility := listCatalog(context.Background(), t, tc.provider, opts...).Recommended
+			opts = append(opts, WithModelProfile(ProfileCapable))
+			want := listCatalog(context.Background(), t, tc.provider, opts...).Recommended
+			if slices.Equal(want, utility) {
+				t.Fatalf("fixture does not separate the profiles: both %v", want)
+			}
+			resetModelMetadataCache()
+			p, err := tc.build(opts...)
+			if err != nil {
+				t.Fatalf("construct: %v", err)
+			}
+			got, err := p.DiscoverModels(context.Background())
+			if err != nil {
+				t.Fatalf("DiscoverModels: %v", err)
+			}
+			assertRanked(t, got, want)
+			if n := envHits.Load(); n != 0 {
+				t.Errorf("DiscoverModels fetched the environment's metadata URL %d times, want 0", n)
+			}
+		})
+	}
+}
```

### B.2.2 Phase 2 fix

```diff
diff --git a/llmprovider/discovery.go b/llmprovider/discovery.go
--- a/llmprovider/discovery.go
+++ b/llmprovider/discovery.go
@@ -28,6 +28,11 @@
 	claudeListPageLimit = "1000"
 )
 
+// modelListingTimeout bounds one model listing, its metadata fetch included
+// (MADR 0010 §2). ListModelCatalogWithSource and every DiscoverModels listing
+// apply it (MADR 0013 A5).
+const modelListingTimeout = 10 * time.Second
+
 // ModelCatalog is the result of one model listing, viewed two ways.
 type ModelCatalog struct {
 	// Recommended is what ListAvailableModels returns: at most
@@ -69,7 +74,7 @@
 func ListModelCatalogWithSource(ctx context.Context, providerName string, src TokenSource, opts ...ProviderOption) (ModelCatalog, error) {
 	cfg := ApplyOptions(opts)
 
-	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
+	ctx, cancel := context.WithTimeout(ctx, modelListingTimeout)
 	defer cancel()
 	if strings.EqualFold(providerName, ProviderOpenAI) && isChatGPTTokenSource(src) {
 		return listChatGPTModels(ctx, src, cfg)
@@ -82,6 +87,14 @@
 		return ModelCatalog{}, fmt.Errorf("model listing: acquire token: %w", err)
 	}
 	return modelCatalogFor(ctx, providerName, token.Value, cfg)
+}
+
+// boundedListing runs one DiscoverModels listing under modelListingTimeout
+// and returns its recommendation (MADR 0013 A5).
+func boundedListing(ctx context.Context, list func(context.Context) (ModelCatalog, error)) ([]string, error) {
+	ctx, cancel := context.WithTimeout(ctx, modelListingTimeout)
+	defer cancel()
+	return recommendedOf(list(ctx))
 }
 
 // recommendedOf adapts a catalog result to the ListAvailableModels contract.
@@ -262,7 +275,9 @@
 
 // listGeminiModels lists Gemini models and returns a short curated production set.
 func listGeminiModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
-	return recommendedOf(modelCatalogFor(ctx, ProviderGemini, apiKey, cfg))
+	return boundedListing(ctx, func(ctx context.Context) (ModelCatalog, error) {
+		return modelCatalogFor(ctx, ProviderGemini, apiKey, cfg)
+	})
 }
 
 // geminiModelsPage is one page of Gemini's GET {base}/models.
@@ -358,7 +373,9 @@
 // listClaudeModels uses Anthropic's Models API when available; otherwise returns
 // the curated static catalog (Anthropic historically lacked a public list endpoint).
 func listClaudeModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
-	return recommendedOf(modelCatalogFor(ctx, ProviderClaude, apiKey, cfg))
+	return boundedListing(ctx, func(ctx context.Context) (ModelCatalog, error) {
+		return modelCatalogFor(ctx, ProviderClaude, apiKey, cfg)
+	})
 }
 
 // claudeModelsPage is one page of Anthropic's GET /v1/models.
@@ -436,7 +453,9 @@
 
 // listOllamaModels fetches installed models from a local Ollama instance.
 func listOllamaModels(ctx context.Context, cfg ProviderConfig) ([]string, error) {
-	return recommendedOf(ollamaCatalog(ctx, cfg))
+	return boundedListing(ctx, func(ctx context.Context) (ModelCatalog, error) {
+		return ollamaCatalog(ctx, cfg)
+	})
 }
 
 // ollamaCatalog lists every installed model. Ollama has no static catalog, so
@@ -588,7 +607,9 @@
 // owned_by "opencode"), so route selection cannot be derived from it; see
 // opencode_route.go.
 func listOpencodeModels(ctx context.Context, gateway, apiKey string, cfg ProviderConfig) ([]string, error) {
-	return recommendedOf(opencodeCatalog(ctx, gateway, apiKey, cfg))
+	return boundedListing(ctx, func(ctx context.Context) (ModelCatalog, error) {
+		return opencodeCatalog(ctx, gateway, apiKey, cfg)
+	})
 }
 
 // opencodeCatalog lists one OpenCode gateway. An unknown gateway is an error,
@@ -641,7 +662,9 @@
 // (tokens/sec) and first_token_latency_ms per provider offering. The sorted
 // order is handed to curateFromCatalog with a nil rankFn, which preserves it.
 func listHuggingFaceModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
-	return recommendedOf(modelCatalogFor(ctx, ProviderHuggingFace, apiKey, cfg))
+	return boundedListing(ctx, func(ctx context.Context) (ModelCatalog, error) {
+		return modelCatalogFor(ctx, ProviderHuggingFace, apiKey, cfg)
+	})
 }
 
 // fetchHuggingFaceUsable returns the usable router models, fastest first: input
@@ -832,7 +855,9 @@
 // Models flagged mayTrainOnYourPrompts are excluded. That is a POLICY decision,
 // not a capability filter — see isUsableKiloModel's comment.
 func listKiloModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
-	return recommendedOf(modelCatalogFor(ctx, ProviderKilo, apiKey, cfg))
+	return boundedListing(ctx, func(ctx context.Context) (ModelCatalog, error) {
+		return modelCatalogFor(ctx, ProviderKilo, apiKey, cfg)
+	})
 }
 
 // kiloUsable returns the usable Kilo models, cheapest first: input must include
diff --git a/llmprovider/huggingface.go b/llmprovider/huggingface.go
--- a/llmprovider/huggingface.go
+++ b/llmprovider/huggingface.go
@@ -41,6 +41,8 @@
 	client          *http.Client
 	maxTokens       int
 	reasoningEffort string
+	modelProfile    ModelProfile
+	metadataURL     string
 }
 
 // NewHuggingFace creates a Hugging Face Inference Providers router client.
@@ -60,6 +62,8 @@
 		client:          cfg.HTTPClient,
 		maxTokens:       cfg.MaxTokens,
 		reasoningEffort: cfg.ReasoningEffort,
+		modelProfile:    cfg.ModelProfile,
+		metadataURL:     cfg.ModelMetadataURL,
 	}, nil
 }
 
@@ -172,8 +176,10 @@
 // Falls back to the static catalog.
 func (p *HuggingFaceProvider) DiscoverModels(ctx context.Context) ([]string, error) {
 	listed, err := listHuggingFaceModels(ctx, p.apiKey, ProviderConfig{
-		HTTPClient: p.client,
-		BaseURL:    p.baseURL,
+		HTTPClient:       p.client,
+		BaseURL:          p.baseURL,
+		ModelProfile:     p.modelProfile,
+		ModelMetadataURL: p.metadataURL,
 	})
 	if err != nil || len(listed) == 0 {
 		listed = StaticModels(ProviderHuggingFace)
diff --git a/llmprovider/kilo.go b/llmprovider/kilo.go
--- a/llmprovider/kilo.go
+++ b/llmprovider/kilo.go
@@ -44,6 +44,7 @@
 	client          *http.Client
 	maxTokens       int
 	reasoningEffort string
+	modelProfile    ModelProfile
 	// caps is the model's supported_parameters set, from WithKiloCapabilities.
 	// nil means "unknown" — send the standard request rather than guessing a
 	// model lacks a capability.
@@ -74,6 +75,7 @@
 		client:          cfg.HTTPClient,
 		maxTokens:       cfg.MaxTokens,
 		reasoningEffort: cfg.ReasoningEffort,
+		modelProfile:    cfg.ModelProfile,
 		caps:            caps,
 	}, nil
 }
@@ -216,8 +218,9 @@
 // than sending everything.
 func (p *KiloProvider) DiscoverModels(ctx context.Context) ([]string, error) {
 	listed, err := listKiloModels(ctx, p.apiKey, ProviderConfig{
-		HTTPClient: p.client,
-		BaseURL:    p.baseURL,
+		HTTPClient:   p.client,
+		BaseURL:      p.baseURL,
+		ModelProfile: p.modelProfile,
 	})
 	if err != nil || len(listed) == 0 {
 		listed = StaticModels(ProviderKilo)
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -35,6 +35,7 @@
 	reasoningEffort string
 	route           OpencodeRoute
 	metadataURL     string
+	modelProfile    ModelProfile
 	// sessionID is sent as x-opencode-session on every request, fixed for the
 	// provider's lifetime (MADR 0012 §1.4, pulled forward by 0010 Phase 6).
 	sessionID string
@@ -76,6 +77,7 @@
 		reasoningEffort: cfg.ReasoningEffort,
 		route:           route,
 		metadataURL:     cfg.ModelMetadataURL,
+		modelProfile:    cfg.ModelProfile,
 		sessionID:       rand.Text(),
 	}, nil
 }
@@ -352,8 +354,10 @@
 // format and 500 on most of them.
 func (p *OpencodeProvider) DiscoverModels(ctx context.Context) ([]string, error) {
 	listed, err := listOpencodeModels(ctx, p.gateway, p.apiKey, ProviderConfig{
-		HTTPClient: p.client,
-		BaseURL:    p.baseURL,
+		HTTPClient:       p.client,
+		BaseURL:          p.baseURL,
+		ModelProfile:     p.modelProfile,
+		ModelMetadataURL: p.metadataURL,
 	})
 	if err != nil || len(listed) == 0 {
 		listed = StaticModels(p.gateway)
diff --git a/llmprovider/options.go b/llmprovider/options.go
--- a/llmprovider/options.go
+++ b/llmprovider/options.go
@@ -44,7 +44,7 @@
 	KiloCapabilities []string
 	// ModelProfile selects how the recommended models of the open catalogs
 	// (Kilo, OpenCode Zen and Go, Hugging Face) are ranked. The zero value is
-	// ProfileUtility. Ignored by provider constructors.
+	// ProfileUtility. Those providers' DiscoverModels ranks with it too.
 	ModelProfile ModelProfile
 	// ModelMetadataURL overrides the models.dev-format document the open
 	// catalogs are ranked with, and OpenCode's chat route reads
@@ -118,9 +118,9 @@
 	}
 }
 
-// WithModelProfile selects how ListAvailableModels and ListModelCatalog rank
-// the recommended models of the open catalogs (MADR 0010 §1). Ignored by
-// provider constructors.
+// WithModelProfile selects how ListAvailableModels, ListModelCatalog and the
+// open catalogs' DiscoverModels rank the recommended models (MADR 0010 §1,
+// MADR 0013 A4).
 func WithModelProfile(p ModelProfile) ProviderOption {
 	return func(cfg *ProviderConfig) {
 		cfg.ModelProfile = p
```

### B.3.1 Phase 3 tests

```diff
diff --git a/llmprovider/metadata_request_test.go b/llmprovider/metadata_request_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/metadata_request_test.go
@@ -0,0 +1,104 @@
+package llmprovider
+
+import (
+	"context"
+	"net/http"
+	"net/http/httptest"
+	"sync/atomic"
+	"testing"
+	"time"
+)
+
+// TestChatReasoningEffort_FailureBackoff pins MADR 0013 A6: with the metadata
+// host failing, three chat-route thinking calls make one fetch, not three.
+func TestChatReasoningEffort_FailureBackoff(t *testing.T) {
+	enableModelMetadata(t)
+	srv, hits := metadataServer(t, http.StatusServiceUnavailable, "")
+	p, err := NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash",
+		WithModelMetadataURL(srv.URL), WithReasoningEffort(effortLow))
+	if err != nil {
+		t.Fatal(err)
+	}
+	for range 3 {
+		if got := p.chatReasoningEffort(context.Background(), true); got != "" {
+			t.Fatalf("effort = %q with metadata down, want none", got)
+		}
+	}
+	if n := hits.Load(); n != 1 {
+		t.Errorf("3 thinking calls made %d metadata fetches, want 1", n)
+	}
+}
+
+// TestChatReasoningEffort_LookupHasDeadline pins MADR 0013 A6: the lookup
+// inside a request carries its own deadline of at most 5 s, whatever the
+// caller's context.
+func TestChatReasoningEffort_LookupHasDeadline(t *testing.T) {
+	enableModelMetadata(t)
+	srv, _ := metadataServer(t, http.StatusOK, smallMetadataDoc)
+	rec := &deadlineTransport{}
+	p, err := NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash", WithModelMetadataURL(srv.URL+"/api.json"),
+		WithReasoningEffort(effortLow), WithHTTPClient(&http.Client{Transport: rec}))
+	if err != nil {
+		t.Fatal(err)
+	}
+	p.chatReasoningEffort(context.Background(), true)
+	left, ok := rec.left("GET /api.json")
+	switch {
+	case !ok:
+		t.Fatal("no metadata request was made")
+	case left < 0:
+		t.Error("metadata lookup ran without a deadline, want one within 5s")
+	case left > 5*time.Second:
+		t.Errorf("metadata lookup had %v left, want at most 5s", left.Round(time.Second))
+	}
+}
+
+// TestLoadModelMetadata_StaleOnRefreshFailure pins MADR 0013 A6: when the
+// cached document has expired and the refresh fails, the stale copy is used.
+func TestLoadModelMetadata_StaleOnRefreshFailure(t *testing.T) {
+	enableModelMetadata(t)
+	var fail atomic.Bool
+	var hits atomic.Int32
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
+		hits.Add(1)
+		if fail.Load() {
+			w.WriteHeader(http.StatusInternalServerError)
+			return
+		}
+		_, _ = w.Write([]byte(smallMetadataDoc))
+	}))
+	t.Cleanup(srv.Close)
+	cfg := ApplyOptions([]ProviderOption{WithModelMetadataURL(srv.URL)})
+	if _, err := loadModelMetadata(context.Background(), cfg); err != nil {
+		t.Fatalf("first load: %v", err)
+	}
+	modelMetadataMu.Lock()
+	e := modelMetadataCache[srv.URL]
+	e.fetched = time.Now().Add(-modelMetadataTTL - time.Minute)
+	modelMetadataCache[srv.URL] = e
+	modelMetadataMu.Unlock()
+	fail.Store(true)
+	doc, err := loadModelMetadata(context.Background(), cfg)
+	if err != nil || doc[metadataKeyZen] == nil {
+		t.Errorf("refresh failed: doc=%v err=%v, want the stale document", doc, err)
+	}
+	if n := hits.Load(); n != 2 {
+		t.Errorf("requests = %d, want 2 (one fetch, one failed refresh)", n)
+	}
+}
+
+// TestLoadModelMetadata_FailureCachedBriefly pins MADR 0013 A6: a failed
+// fetch is remembered, so an immediate second load does not fetch again.
+func TestLoadModelMetadata_FailureCachedBriefly(t *testing.T) {
+	enableModelMetadata(t)
+	srv, hits := metadataServer(t, http.StatusInternalServerError, "")
+	cfg := ApplyOptions([]ProviderOption{WithModelMetadataURL(srv.URL)})
+	for i := range 2 {
+		if _, err := loadModelMetadata(context.Background(), cfg); err == nil {
+			t.Fatalf("load %d: want the HTTP 500 error", i+1)
+		}
+	}
+	if n := hits.Load(); n != 1 {
+		t.Errorf("requests = %d, want 1 (the failure is cached)", n)
+	}
+}
```

### B.3.2 Phase 3 fix

```diff
diff --git a/llmprovider/model_metadata.go b/llmprovider/model_metadata.go
--- a/llmprovider/model_metadata.go
+++ b/llmprovider/model_metadata.go
@@ -21,6 +21,12 @@
 	envModelMetadataURL     = "MCPLIB_MODELS_METADATA_URL"
 	envDisableModelMetadata = "MCPLIB_DISABLE_MODELS_METADATA"
 	modelMetadataTTL        = 10 * time.Minute
+	// modelMetadataRetryAfter is how long a failed fetch is remembered before
+	// the next load tries again (MADR 0013 A6).
+	modelMetadataRetryAfter = time.Minute
+	// metadataLookupTimeout bounds the lookup a generation request makes
+	// (OpenCode's chat route) whatever the caller's context (MADR 0013 A6).
+	metadataLookupTimeout = 5 * time.Second
 
 	metadataKeyZen = "opencode"
 	metadataKeyGo  = "opencode-go"
@@ -88,9 +94,12 @@
 	return ""
 }
 
+// modelMetadataCacheEntry is one URL's last good document and last failure.
 type modelMetadataCacheEntry struct {
-	doc     modelMetadataDoc
-	fetched time.Time
+	doc     modelMetadataDoc // nil until a fetch succeeds; kept when a refresh fails
+	fetched time.Time        // when doc was fetched
+	failed  time.Time        // when the last fetch failed; zero after a success
+	err     error            // that failure
 }
 
 var (
@@ -118,26 +127,43 @@
 }
 
 // loadModelMetadata returns the document, from the in-process cache while it
-// is younger than modelMetadataTTL. A failure is returned, never cached.
+// is younger than modelMetadataTTL. A failure is remembered for
+// modelMetadataRetryAfter, and while it is, loads answer from the cache
+// without fetching: the stale document when there is one, else the failure
+// (MADR 0013 A6).
 func loadModelMetadata(ctx context.Context, cfg ProviderConfig) (modelMetadataDoc, error) {
 	if modelMetadataDisabled() {
 		return nil, errModelMetadataDisabled
 	}
 	url := modelMetadataURL(cfg)
 	modelMetadataMu.Lock()
-	e, ok := modelMetadataCache[url]
+	e := modelMetadataCache[url]
 	modelMetadataMu.Unlock()
-	if ok && time.Since(e.fetched) < modelMetadataTTL {
+	switch {
+	case e.doc != nil && time.Since(e.fetched) < modelMetadataTTL:
 		return e.doc, nil
+	case !e.failed.IsZero() && time.Since(e.failed) < modelMetadataRetryAfter:
+		return e.cached()
 	}
 	doc, err := fetchModelMetadata(ctx, url, cfg.HTTPClient)
+	modelMetadataMu.Lock()
+	defer modelMetadataMu.Unlock()
 	if err != nil {
-		return nil, err
-	}
-	modelMetadataMu.Lock()
+		e.failed, e.err = time.Now(), err
+		modelMetadataCache[url] = e
+		return e.cached()
+	}
 	modelMetadataCache[url] = modelMetadataCacheEntry{doc: doc, fetched: time.Now()}
-	modelMetadataMu.Unlock()
 	return doc, nil
+}
+
+// cached answers from a cache entry after a failure: the stale document when
+// there is one, else the failure.
+func (e modelMetadataCacheEntry) cached() (modelMetadataDoc, error) {
+	if e.doc != nil {
+		return e.doc, nil
+	}
+	return nil, e.err
 }
 
 // fetchModelMetadata performs one GET. Go's transport requests gzip itself.
diff --git a/llmprovider/model_metadata_test.go b/llmprovider/model_metadata_test.go
--- a/llmprovider/model_metadata_test.go
+++ b/llmprovider/model_metadata_test.go
@@ -10,6 +10,7 @@
 	"strings"
 	"sync/atomic"
 	"testing"
+	"time"
 )
 
 // resetModelMetadataCache empties the in-process metadata cache. httptest
@@ -94,7 +95,10 @@
 	}
 }
 
-func TestLoadModelMetadata_FailureNotCached(t *testing.T) {
+// TestLoadModelMetadata_FailureRetriedAfterBackoff pins MADR 0013 A6: once
+// modelMetadataRetryAfter has passed since a failure, the next load fetches.
+// It replaces TestLoadModelMetadata_FailureNotCached (0010 PLAN §1.11 item 3).
+func TestLoadModelMetadata_FailureRetriedAfterBackoff(t *testing.T) {
 	enableModelMetadata(t)
 	var hits atomic.Int32
 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
@@ -109,12 +113,17 @@
 	if _, err := loadModelMetadata(context.Background(), cfg); err == nil {
 		t.Fatal("first load: want an error for HTTP 500")
 	}
+	modelMetadataMu.Lock()
+	e := modelMetadataCache[srv.URL]
+	e.failed = time.Now().Add(-modelMetadataRetryAfter - time.Second)
+	modelMetadataCache[srv.URL] = e
+	modelMetadataMu.Unlock()
 	doc, err := loadModelMetadata(context.Background(), cfg)
 	if err != nil || doc[metadataKeyZen] == nil {
-		t.Fatalf("second load: doc=%v err=%v, want a fresh fetch", doc, err)
+		t.Fatalf("load after the backoff: doc=%v err=%v, want a fresh fetch", doc, err)
 	}
 	if n := hits.Load(); n != 2 {
-		t.Errorf("requests = %d, want 2 (failure not cached)", n)
+		t.Errorf("requests = %d, want 2", n)
 	}
 }
 
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -237,11 +237,15 @@
 // chatReasoningEffort returns the reasoning_effort for the chat route: the
 // configured effort, when this is a thinking call and the model's published
 // reasoning_options list it (MADR 0010 §6); otherwise "". Metadata that is
-// unavailable, disabled or silent on the model sends nothing.
+// unavailable, disabled or silent on the model sends nothing. The lookup waits
+// at most metadataLookupTimeout, and a failed fetch is not retried for
+// modelMetadataRetryAfter (MADR 0013 A6).
 func (p *OpencodeProvider) chatReasoningEffort(ctx context.Context, thinking bool) string {
 	if !thinking || p.reasoningEffort == "" {
 		return ""
 	}
+	ctx, cancel := context.WithTimeout(ctx, metadataLookupTimeout)
+	defer cancel()
 	doc, err := loadModelMetadata(ctx, ProviderConfig{HTTPClient: p.client, ModelMetadataURL: p.metadataURL})
 	if err != nil || !slices.Contains(doc.reasoningEfforts(p.gateway, p.model), p.reasoningEffort) {
 		return ""
```

### B.4.1 Phase 4 tests

```diff
diff --git a/llmprovider/matcher_coverage_test.go b/llmprovider/matcher_coverage_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/matcher_coverage_test.go
@@ -0,0 +1,21 @@
+package llmprovider
+
+import "testing"
+
+// TestSearchModels_GlobQuestionMark covers model_matcher.go's '?' glob rune:
+// it matches exactly one character.
+func TestSearchModels_GlobQuestionMark(t *testing.T) {
+	got := SearchModels(ProviderClaude, []string{"claude-sonnet-5", "claude-sonnet-45", "claude-opus-5"}, "claude-sonnet-?")
+	if len(got) != 1 || got[0].ID != "claude-sonnet-5" {
+		t.Errorf("claude-sonnet-? = %v, want only claude-sonnet-5", got)
+	}
+}
+
+// TestSearchModels_LabelSubstringTier covers model_matcher.go's label tier: a
+// query found only in a curated label scores scoreLabelSubstring.
+func TestSearchModels_LabelSubstringTier(t *testing.T) {
+	got := SearchModels(ProviderClaude, []string{"claude-sonnet-5", "claude-opus-4-8"}, "balanced speed")
+	if len(got) != 1 || got[0].ID != "claude-sonnet-5" || got[0].Score != scoreLabelSubstring {
+		t.Errorf("balanced speed = %+v, want claude-sonnet-5 at scoreLabelSubstring", got)
+	}
+}
diff --git a/wizard/model_select_edge_test.go b/wizard/model_select_edge_test.go
new file mode 100644
--- /dev/null
+++ b/wizard/model_select_edge_test.go
@@ -0,0 +1,273 @@
+package wizard
+
+import (
+	"context"
+	"io"
+	"net/http"
+	"net/http/httptest"
+	"slices"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/maccavelli/mcplib/llmprovider"
+)
+
+// TestConfigureLLM_FallbackPicksDeduped pins MADR 0013 C1: a repeated index
+// in a fallback MultiSelect adds that model once.
+func TestConfigureLLM_FallbackPicksDeduped(t *testing.T) {
+	withEnv(t, nil)
+	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
+	f := &fakePrompter{
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey},
+		inputs: []string{"", ""}, multiSelects: [][]int{{0, 0, 1}},
+	}
+	res, err := ConfigureLLM(context.Background(), f, Options{NeedFallbacks: true})
+	if err != nil {
+		t.Fatalf("ConfigureLLM: %v", err)
+	}
+	if want := []string{static[1], static[2]}; !slices.Equal(res.Fallbacks, want) {
+		t.Errorf("Fallbacks = %q, want %q", res.Fallbacks, want)
+	}
+}
+
+// TestMultiSelect_RepeatedIndexCountsOnce pins MADR 0013 C1 in TextPrompter.
+func TestMultiSelect_RepeatedIndexCountsOnce(t *testing.T) {
+	p, _ := pipePrompter(t, "1,1,2\n")
+	got, err := p.MultiSelect("Pick any", []Choice{{Label: "a"}, {Label: "b"}, {Label: "c"}}, nil)
+	if err != nil {
+		t.Fatalf("MultiSelect: %v", err)
+	}
+	if !slices.Equal(got, []int{0, 1}) {
+		t.Errorf("MultiSelect(1,1,2) = %v, want [0 1]", got)
+	}
+}
+
+// TestConfigureLLM_OtherDefaultsOnlyToSameProvider pins MADR 0013 C4 (MADR
+// 0009 §4.3): the Other and "No models found" prompts default to the saved
+// model only when it belongs to the chosen provider.
+func TestConfigureLLM_OtherDefaultsOnlyToSameProvider(t *testing.T) {
+	withEnv(t, nil)
+	saved := Result{Provider: llmprovider.ProviderClaude, Model: "claude-opus-5"}
+	t.Run("other provider, Other", func(t *testing.T) {
+		gemini := len(llmprovider.StaticModels(llmprovider.ProviderGemini))
+		f := &fakePrompter{
+			t: t, selects: []int{providerIdx(t, llmprovider.ProviderGemini), gemini},
+			secrets: []string{testKey}, inputs: []string{""},
+		}
+		res, err := ConfigureLLM(context.Background(), f, Options{Existing: saved})
+		if err == nil {
+			t.Errorf("Enter at Other saved provider=%s model=%q, want an error (no default from another provider)",
+				res.Provider, res.Model)
+		}
+	})
+	t.Run("other provider, no models found", func(t *testing.T) {
+		f := &fakePrompter{
+			t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
+			inputs: []string{"http://127.0.0.1:1"}, confirms: []bool{false},
+		}
+		res, err := ConfigureLLM(context.Background(), f, Options{Existing: saved})
+		if err == nil {
+			t.Errorf("Enter at No models found saved provider=%s model=%q, want an error", res.Provider, res.Model)
+		}
+	})
+	t.Run("same provider keeps its model", func(t *testing.T) {
+		claude := len(llmprovider.StaticModels(llmprovider.ProviderClaude))
+		f := &fakePrompter{
+			t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), claude + 1},
+			secrets: []string{testKey}, inputs: []string{""},
+		}
+		res, err := ConfigureLLM(context.Background(), f, Options{
+			Existing: Result{Provider: llmprovider.ProviderClaude, Model: "my-model"},
+		})
+		if err != nil || res.Model != "my-model" {
+			t.Errorf("Enter at Other: model=%q err=%v, want my-model", res.Model, err)
+		}
+	})
+}
+
+// TestConfigureLLM_FallbackExclusionIgnoresCase pins MADR 0013 C6: a primary
+// typed in another case is not offered back as a fallback.
+func TestConfigureLLM_FallbackExclusionIgnoresCase(t *testing.T) {
+	withEnv(t, nil)
+	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
+	f := &fakePrompter{
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), len(static)},
+		secrets: []string{testKey}, inputs: []string{"", "Claude-Haiku-4-5", ""}, multiSelects: [][]int{{}},
+	}
+	if _, err := ConfigureLLM(context.Background(), f, Options{NeedFallbacks: true}); err != nil {
+		t.Fatalf("ConfigureLLM: %v", err)
+	}
+	if n := len(f.seenMultiSelectItems[0]); n != len(static)-1 {
+		t.Errorf("fallback menu has %d rows, want %d (claude-haiku-4-5 is the primary)", n, len(static)-1)
+	}
+}
+
+// TestConfigureLLM_BlankModelIDRefused pins MADR 0013 C7 and covers
+// model_select.go's empty-id branch: a blank or whitespace-only id is refused
+// at Other and at "No models found".
+func TestConfigureLLM_BlankModelIDRefused(t *testing.T) {
+	withEnv(t, nil)
+	claude := len(llmprovider.StaticModels(llmprovider.ProviderClaude))
+	for _, id := range []string{"", "   "} {
+		f := &fakePrompter{
+			t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), claude},
+			secrets: []string{testKey}, inputs: []string{"", id},
+		}
+		if res, err := ConfigureLLM(context.Background(), f, Options{}); err == nil {
+			t.Errorf("Other with %q: model=%q, want an error", id, res.Model)
+		}
+	}
+	f := &fakePrompter{
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
+		inputs: []string{"http://127.0.0.1:1", "   "}, confirms: []bool{false},
+	}
+	if res, err := ConfigureLLM(context.Background(), f, Options{}); err == nil {
+		t.Errorf("No models found with blanks: model=%q, want an error", res.Model)
+	}
+}
+
+// TestConfigureLLM_ListingErrorWarns covers configure.go's listing-error
+// warning: Ollama unreachable with Discover set.
+func TestConfigureLLM_ListingErrorWarns(t *testing.T) {
+	withEnv(t, nil)
+	f := &fakePrompter{
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
+		inputs: []string{"http://127.0.0.1:1", "llama3"}, confirms: []bool{false},
+	}
+	res, err := ConfigureLLM(context.Background(), f, Options{Discover: true})
+	if err != nil || res.Model != "llama3" {
+		t.Fatalf("ConfigureLLM: model=%q err=%v, want llama3", res.Model, err)
+	}
+	if n := countContaining(f.seenNotify, "could not list models for Ollama"); n != 1 {
+		t.Errorf("listing warning seen %d times, want 1: %v", n, f.seenNotify)
+	}
+}
+
+// TestConfigureLLM_OllamaEmptyListing covers configure.go's empty live
+// listing: a reachable Ollama with nothing installed falls through to manual
+// entry without a warning.
+func TestConfigureLLM_OllamaEmptyListing(t *testing.T) {
+	withEnv(t, nil)
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if r.URL.Path == "/api/tags" {
+			_, _ = w.Write([]byte(`{"models":[]}`))
+		}
+	}))
+	t.Cleanup(srv.Close)
+	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)}, inputs: []string{srv.URL, "llama3"}}
+	res, err := ConfigureLLM(context.Background(), f, Options{Discover: true})
+	if err != nil || res.Model != "llama3" {
+		t.Fatalf("ConfigureLLM: model=%q err=%v, want llama3", res.Model, err)
+	}
+	if len(f.seenNotify) != 0 {
+		t.Errorf("notices = %v, want none for an empty install", f.seenNotify)
+	}
+}
+
+// TestConfigureLLM_DefaultRowIsExistingModel covers model_select.go's default
+// row: a saved model in the recommended list is the menu default.
+func TestConfigureLLM_DefaultRowIsExistingModel(t *testing.T) {
+	withEnv(t, nil)
+	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
+	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 2}, secrets: []string{testKey}}
+	res, err := ConfigureLLM(context.Background(), f, Options{
+		Existing: Result{Provider: llmprovider.ProviderClaude, Model: static[2]},
+	})
+	if err != nil {
+		t.Fatalf("ConfigureLLM: %v", err)
+	}
+	if f.seenSelectDefault[1] != 2 || res.Model != static[2] {
+		t.Errorf("default = %d, model = %q; want 2 and %q", f.seenSelectDefault[1], res.Model, static[2])
+	}
+}
+
+// TestConfigureLLM_BlankFallbackRoundWithNothingLeft covers model_select.go's
+// blank round after search has taken every recommended model: it ends the
+// loop without an empty MultiSelect.
+func TestConfigureLLM_BlankFallbackRoundWithNothingLeft(t *testing.T) {
+	withEnv(t, nil)
+	srv := zenServer(t, http.StatusOK, zenListing(zenSearchIDs))
+	f := &fakePrompter{
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpencodeZen), 0}, secrets: []string{testKey},
+		inputs:       []string{srv.URL, "", "flash", ""},
+		multiSelects: [][]int{{0, 1, 2, 3, 4}}, confirms: []bool{true},
+	}
+	opts := zenOptions()
+	opts.NeedFallbacks = true
+	res, err := ConfigureLLM(context.Background(), f, opts)
+	if err != nil {
+		t.Fatalf("ConfigureLLM: %v", err)
+	}
+	if len(res.Fallbacks) != 5 || len(f.seenMultiSelectItems) != 1 {
+		t.Errorf("Fallbacks = %v after %d MultiSelects, want 5 after 1", res.Fallbacks, len(f.seenMultiSelectItems))
+	}
+}
+
+// TestConfigureLLM_FallbackSearchNoMatches covers model_select.go's no-match
+// fallback search: it warns and asks again.
+func TestConfigureLLM_FallbackSearchNoMatches(t *testing.T) {
+	withEnv(t, nil)
+	f := &fakePrompter{
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey},
+		inputs: []string{"", "zzz-no-match", ""}, multiSelects: [][]int{{}},
+	}
+	if _, err := ConfigureLLM(context.Background(), f, Options{NeedFallbacks: true}); err != nil {
+		t.Fatalf("ConfigureLLM: %v", err)
+	}
+	if n := countContaining(f.seenNotify, `match "zzz-no-match"`); n != 1 {
+		t.Errorf("no-match warning seen %d times, want 1: %v", n, f.seenNotify)
+	}
+}
+
+// TestConfigureLLM_ListingTokenFailureUsesStaticCatalog covers configure.go's
+// listing-error branch for a provider that has a static catalog: a kept Grok
+// session that cannot refresh fails the listing, so the wizard warns and
+// offers the built-in catalog.
+func TestConfigureLLM_ListingTokenFailureUsesStaticCatalog(t *testing.T) {
+	withEnv(t, nil)
+	static := llmprovider.StaticModels(llmprovider.ProviderGrok)
+	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 1, 0}, confirms: []bool{true}}
+	res, err := ConfigureLLM(context.Background(), f, Options{
+		Existing: Result{
+			Provider:    llmprovider.ProviderGrok,
+			Kind:        CredOAuth,
+			AccessToken: "expired-access-abcd",
+			TokenExpiry: time.Now().Add(-time.Hour),
+		},
+		TokenStore: newMemoryTokenStore(),
+		Discover:   true,
+	})
+	if err != nil {
+		t.Fatalf("ConfigureLLM: %v", err)
+	}
+	if res.Model != static[0] {
+		t.Errorf("Model = %q, want the first built-in model %q", res.Model, static[0])
+	}
+	if n := countContaining(f.seenNotify, "no refresh token); using the built-in catalog"); n != 1 {
+		t.Errorf("listing warning seen %d times, want 1: %v", n, f.seenNotify)
+	}
+}
+
+// TestConfigureLLM_UnusableListingNotice covers the static-catalog notice with
+// no cause: a listing that succeeds but offers no usable model degrades
+// without a ModelCatalog.Err.
+func TestConfigureLLM_UnusableListingNotice(t *testing.T) {
+	withEnv(t, nil)
+	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		return &http.Response{
+			StatusCode: http.StatusOK,
+			Header:     make(http.Header),
+			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"not-a-claude-model"}],"has_more":false}`)),
+			Request:    r,
+		}, nil
+	})}
+	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey}}
+	if _, err := ConfigureLLM(context.Background(), f, Options{Discover: true, HTTPClient: client}); err != nil {
+		t.Fatalf("ConfigureLLM: %v", err)
+	}
+	want := "live model listing for Claude (Anthropic) is unavailable; search covers the built-in catalog only"
+	if n := countContaining(f.seenNotify, want); n != 1 {
+		t.Errorf("notice %q seen %d times, want 1: %v", want, n, f.seenNotify)
+	}
+}
diff --git a/wizard/model_select_test.go b/wizard/model_select_test.go
--- a/wizard/model_select_test.go
+++ b/wizard/model_select_test.go
@@ -241,7 +241,8 @@
 	if err != nil {
 		t.Fatalf("ConfigureLLM: %v", err)
 	}
-	notice := "live model listing for OpenCode Zen is unavailable; search covers the built-in catalog only"
+	notice := "live model listing for OpenCode Zen is unavailable (opencode: models endpoint returned HTTP 500); " +
+		"search covers the built-in catalog only"
 	if n := countContaining(f.seenNotify, notice); n != 1 {
 		t.Errorf("static notice seen %d times, want 1: %v", n, f.seenNotify)
 	}
```

### B.4.2 Phase 4 fix

```diff
diff --git a/llmprovider/catalog_err_test.go b/llmprovider/catalog_err_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/catalog_err_test.go
@@ -0,0 +1,26 @@
+package llmprovider
+
+import (
+	"context"
+	"net/http"
+	"strings"
+	"testing"
+)
+
+// TestListModelCatalog_ErrExplainsDegrade pins MADR 0013 C2: a listing that
+// degrades to the static catalog because the fetch failed carries the cause.
+func TestListModelCatalog_ErrExplainsDegrade(t *testing.T) {
+	failing, _ := metadataServer(t, http.StatusInternalServerError, "")
+	cat := listCatalog(context.Background(), t, ProviderOpencodeZen, WithBaseURL(failing.URL))
+	if cat.Live || cat.Err == nil || !strings.Contains(cat.Err.Error(), "HTTP 500") {
+		t.Errorf("failed listing: Live=%v Err=%v, want Live false and an HTTP 500 cause", cat.Live, cat.Err)
+	}
+	ok := serveBody(t, opencodeListingFixture)
+	if cat := listCatalog(context.Background(), t, ProviderOpencodeZen, WithBaseURL(ok.URL)); !cat.Live || cat.Err != nil {
+		t.Errorf("good listing: Live=%v Err=%v, want Live and no error", cat.Live, cat.Err)
+	}
+	empty := serveBody(t, `{"object":"list","data":[]}`)
+	if cat := listCatalog(context.Background(), t, ProviderOpencodeZen, WithBaseURL(empty.URL)); cat.Live || cat.Err != nil {
+		t.Errorf("empty listing: Live=%v Err=%v, want the static catalog with no error", cat.Live, cat.Err)
+	}
+}
diff --git a/llmprovider/discovery.go b/llmprovider/discovery.go
--- a/llmprovider/discovery.go
+++ b/llmprovider/discovery.go
@@ -44,6 +44,10 @@
 	// Live reports whether Usable came from the provider's listing rather
 	// than the static catalog.
 	Live bool
+	// Err is the listing failure that made Live false. It is nil when Live is
+	// true, and when a listing that succeeded yielded no usable id (MADR 0013
+	// C2).
+	Err error
 }
 
 // ListAvailableModels fetches models from a provider listing API when available,
@@ -111,7 +115,9 @@
 func catalogFrom(usable []string, fetchErr error, static []string, curate func([]string) []string) ModelCatalog {
 	usable = uniqueIDs(usable)
 	if fetchErr != nil || len(usable) == 0 {
-		return staticCatalog(static)
+		cat := staticCatalog(static)
+		cat.Err = fetchErr
+		return cat
 	}
 	recommended := curate(usable)
 	if len(recommended) == 0 {
diff --git a/wizard/configure.go b/wizard/configure.go
--- a/wizard/configure.go
+++ b/wizard/configure.go
@@ -5,6 +5,7 @@
 	"fmt"
 	"net/http"
 	"os"
+	"strings"
 	"time"
 
 	"github.com/maccavelli/mcplib/llmprovider"
@@ -12,8 +13,9 @@
 )
 
 // defaultDiscoverLimit bounds a live model listing so a slow or unreachable
-// provider cannot stall a wizard indefinitely.
-const defaultDiscoverLimit = 20 * time.Second
+// provider cannot stall a wizard indefinitely. It equals the lister's own
+// 10 s bound, which no caller deadline extends (MADR 0013 Q4).
+const defaultDiscoverLimit = 10 * time.Second
 
 // Result is what ConfigureLLM produces. It is deliberately data, not config:
 // each consumer persists it in its own schema. Unifying configuration storage
@@ -51,6 +53,7 @@
 	// session has no static catalog, so the user is asked for a model id.
 	Discover bool
 	// DiscoverLimit bounds the listing call. Zero uses defaultDiscoverLimit.
+	// The lister caps every listing at 10 s, so a larger value has no effect.
 	DiscoverLimit time.Duration
 	// NeedFallbacks collects additional models after the primary.
 	NeedFallbacks bool
@@ -143,10 +146,11 @@
 		// Ollama with nothing installed, or a provider whose listing failed
 		// and which has no static catalog. Let the user type an id rather
 		// than dead-ending the wizard.
-		manual, inputErr := p.Input("No models found; enter a model id", o.Existing.Model)
+		manual, inputErr := p.Input("No models found; enter a model id", existingModel(o, d.ID))
 		if inputErr != nil {
 			return Result{}, fmt.Errorf("enter model: %w", inputErr)
 		}
+		manual = strings.TrimSpace(manual)
 		if manual == "" {
 			// Returning Result{Model: ""} would hand the caller a
 			// configuration that cannot generate anything.
@@ -309,7 +313,12 @@
 	if len(cat.Recommended) == 0 {
 		return fallback
 	}
-	if !cat.Live && !chatGPT {
+	switch {
+	case cat.Live || chatGPT:
+	case cat.Err != nil:
+		p.Notify(LevelWarn, "live model listing for %s is unavailable (%v); search covers the built-in catalog only",
+			d.Label, cat.Err)
+	default:
 		p.Notify(LevelInfo, "live model listing for %s is unavailable; search covers the built-in catalog only", d.Label)
 	}
 	return cat
diff --git a/wizard/model_select.go b/wizard/model_select.go
--- a/wizard/model_select.go
+++ b/wizard/model_select.go
@@ -2,6 +2,7 @@
 
 import (
 	"fmt"
+	"slices"
 	"strings"
 
 	"github.com/maccavelli/mcplib/llmprovider"
@@ -57,7 +58,7 @@
 		case idx == len(shown):
 			continue
 		default:
-			return enterModelID(p, o)
+			return enterModelID(p, d.ID, o)
 		}
 	}
 }
@@ -90,20 +91,31 @@
 	case current != "" && idx == len(models):
 		return current, nil
 	default:
-		return enterModelID(p, o)
-	}
-}
-
-// enterModelID is the Other escape hatch: the user types a model id.
-func enterModelID(p Prompter, o Options) (string, error) {
-	manual, err := p.Input("Model id", o.Existing.Model)
+		return enterModelID(p, d.ID, o)
+	}
+}
+
+// enterModelID is the Other escape hatch: the user types a model id. The
+// saved model is the default only for its own provider (MADR 0009 §4.3), and a
+// blank id is refused (MADR 0013 C4, C7).
+func enterModelID(p Prompter, provider string, o Options) (string, error) {
+	manual, err := p.Input("Model id", existingModel(o, provider))
 	if err != nil {
 		return "", fmt.Errorf("enter model: %w", err)
 	}
+	manual = strings.TrimSpace(manual)
 	if manual == "" {
 		return "", fmt.Errorf("wizard: no model entered")
 	}
 	return manual, nil
+}
+
+// existingModel returns the saved model when it belongs to provider, else "".
+func existingModel(o Options, provider string) string {
+	if o.Existing.Provider == provider {
+		return o.Existing.Model
+	}
+	return ""
 }
 
 // selectFallbacks offers fallback models, never the primary or one already
@@ -156,12 +168,13 @@
 	}
 }
 
-// excludedIDs is the set a fallback round must not offer.
+// excludedIDs is the set a fallback round must not offer, keyed by lower-case
+// id because SearchModels compares ids case-insensitively (MADR 0013 C6).
 func excludedIDs(primary string, chosen []string) map[string]struct{} {
 	exclude := make(map[string]struct{}, len(chosen)+1)
-	exclude[primary] = struct{}{}
+	exclude[strings.ToLower(primary)] = struct{}{}
 	for _, c := range chosen {
-		exclude[c] = struct{}{}
+		exclude[strings.ToLower(c)] = struct{}{}
 	}
 	return exclude
 }
@@ -170,21 +183,24 @@
 func without(models []string, exclude map[string]struct{}) []string {
 	out := make([]string, 0, len(models))
 	for _, m := range models {
-		if _, skip := exclude[m]; !skip {
+		if _, skip := exclude[strings.ToLower(m)]; !skip {
 			out = append(out, m)
 		}
 	}
 	return out
 }
 
-// appendPicks appends ids[i] for each valid index. chosen becomes non-nil
-// even when nothing was picked, because a MultiSelect was shown.
+// appendPicks appends ids[i] for each valid index, once each (MADR 0013 C1).
+// chosen becomes non-nil even when nothing was picked, because a MultiSelect
+// was shown.
 func appendPicks(chosen, ids []string, idxs []int) []string {
 	if chosen == nil {
 		chosen = []string{}
 	}
 	for _, i := range idxs {
-		if i >= 0 && i < len(ids) {
+		if i >= 0 && i < len(ids) && !slices.ContainsFunc(chosen, func(c string) bool {
+			return strings.EqualFold(c, ids[i])
+		}) {
 			chosen = append(chosen, ids[i])
 		}
 	}
diff --git a/wizard/text_prompter.go b/wizard/text_prompter.go
--- a/wizard/text_prompter.go
+++ b/wizard/text_prompter.go
@@ -6,6 +6,7 @@
 	"fmt"
 	"io"
 	"os"
+	"slices"
 	"strconv"
 	"strings"
 
@@ -166,8 +167,9 @@
 	}
 }
 
-// MultiSelect implements Prompter. Input is a comma-separated list of indices;
-// an empty line accepts the preselection.
+// MultiSelect implements Prompter. Input is a comma-separated list of indices,
+// a repeated index counting once (MADR 0013 C1); an empty line accepts the
+// preselection.
 func (p *TextPrompter) MultiSelect(title string, choices []Choice, preselected []int) ([]int, error) {
 	if len(choices) == 0 {
 		return nil, nil
@@ -191,7 +193,9 @@
 				ok = false
 				break
 			}
-			out = append(out, n-1)
+			if !slices.Contains(out, n-1) {
+				out = append(out, n-1)
+			}
 		}
 		if ok {
 			return out, p.flushErr()
```

### B.5.1 Phase 5 tests

```diff
diff --git a/llmprovider/live_thinking_test.go b/llmprovider/live_thinking_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_thinking_test.go
@@ -0,0 +1,94 @@
+//go:build live_gateways
+
+// Live checks of the thinking shapes on the budget-based wires (MADR 0013 Q1,
+// B9). They REQUIRE ANTHROPIC_API_KEY and GEMINI_API_KEY (and OPENCODE_API_KEY
+// for the Go route) and skip without them. Only rate limiting skips: a 400
+// here is the regression these tests exist to catch.
+package llmprovider
+
+import (
+	"errors"
+	"os"
+	"strings"
+	"testing"
+)
+
+// liveEnvKey returns a credential from the environment or skips.
+func liveEnvKey(t *testing.T, name string) string {
+	t.Helper()
+	key := os.Getenv(name)
+	if key == "" {
+		t.Skipf("%s unset", name)
+	}
+	return key
+}
+
+// assertAlpha fails unless a thinking call returned text containing ALPHA.
+func assertAlpha(t *testing.T, out string, err error) {
+	t.Helper()
+	if errors.Is(err, ErrRateLimited) {
+		t.Skipf("rate limited: %v", err)
+	}
+	if err != nil {
+		t.Fatalf("GenerateThinking: %v", err)
+	}
+	if !strings.Contains(strings.ToUpper(out), "ALPHA") {
+		t.Errorf("output %q does not contain ALPHA", out)
+	}
+}
+
+// TestLive_ClaudeThinkingShapes: a budget-only model (Haiku 4.5) and two
+// adaptive-only models accept the utility and capable efforts.
+func TestLive_ClaudeThinkingShapes(t *testing.T) {
+	key := liveEnvKey(t, "ANTHROPIC_API_KEY")
+	for _, model := range []string{"claude-haiku-4-5", "claude-sonnet-5", "claude-opus-4-8"} {
+		for _, effort := range []string{effortLow, ""} {
+			t.Run(model+"/"+effort, func(t *testing.T) {
+				ctx, cancel := liveCtx(t)
+				defer cancel()
+				p, err := NewClaude(key, model, WithReasoningEffort(effort), WithMaxTokens(2048))
+				if err != nil {
+					t.Fatal(err)
+				}
+				out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
+				assertAlpha(t, out, err)
+			})
+		}
+	}
+}
+
+// TestLive_GeminiThinkingShapes: Gemini 2.5 (budget) and 3.x (thinkingLevel)
+// accept the utility and capable efforts.
+func TestLive_GeminiThinkingShapes(t *testing.T) {
+	key := liveEnvKey(t, "GEMINI_API_KEY")
+	for _, model := range []string{"gemini-2.5-flash", "gemini-3.7-flash"} {
+		for _, effort := range []string{effortLow, ""} {
+			t.Run(model+"/"+effort, func(t *testing.T) {
+				ctx, cancel := liveCtx(t)
+				defer cancel()
+				p, err := NewGemini(ctx, key, model, WithReasoningEffort(effort), WithMaxTokens(2048))
+				if err != nil {
+					t.Fatal(err)
+				}
+				out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
+				if errors.Is(err, ErrProviderUnavailable) {
+					t.Skipf("Gemini overloaded: %v", err)
+				}
+				assertAlpha(t, out, err)
+			})
+		}
+	}
+}
+
+// TestLive_OpencodeMessagesThinking: OpenCode Go's messages route accepts the
+// low-effort budget on qwen3.8-flash (one of Go's utility six).
+func TestLive_OpencodeMessagesThinking(t *testing.T) {
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "qwen3.8-flash", WithReasoningEffort(effortLow))
+	if err != nil {
+		t.Fatal(err)
+	}
+	out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
+	assertAlpha(t, out, err)
+}
diff --git a/llmprovider/opencode_test.go b/llmprovider/opencode_test.go
--- a/llmprovider/opencode_test.go
+++ b/llmprovider/opencode_test.go
@@ -191,7 +191,7 @@
 	t.Run("messages uses thinking.budget_tokens and raises max_tokens", func(t *testing.T) {
 		var body map[string]any
 		srv := captureServer(t, &body, fxOpencodeMessages)
-		p, _ := NewOpencode(ProviderOpencodeZen, "k", "claude-sonnet-5",
+		p, _ := NewOpencode(ProviderOpencodeZen, "k", "claude-haiku-4-5",
 			WithBaseURL(srv.URL), WithMaxTokens(4096), WithThinkingBudget(8000))
 		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
 			t.Fatalf("GenerateThinking: %v", err)
diff --git a/llmprovider/thinking_test.go b/llmprovider/thinking_test.go
--- a/llmprovider/thinking_test.go
+++ b/llmprovider/thinking_test.go
@@ -32,7 +32,7 @@
 	srv := captureServer(t, &body, `{"content":[{"type":"text","text":"ok"}]}`)
 
 	// maxTokens (4096) <= budget (8000) must force the ceiling above the budget.
-	p, err := NewClaude("k", "claude-x", WithBaseURL(srv.URL), WithMaxTokens(4096), WithThinkingBudget(8000))
+	p, err := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL), WithMaxTokens(4096), WithThinkingBudget(8000))
 	if err != nil {
 		t.Fatal(err)
 	}
diff --git a/llmprovider/thinking_wire_test.go b/llmprovider/thinking_wire_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/thinking_wire_test.go
@@ -0,0 +1,125 @@
+package llmprovider
+
+import (
+	"context"
+	"reflect"
+	"testing"
+)
+
+// thinkingCase is one GenerateThinking request and the thinking fields its
+// body must carry. A nil want entry means the key must be absent.
+type thinkingCase struct {
+	model, effort string
+	budget        int
+	want          map[string]any
+}
+
+func assertThinkingFields(t *testing.T, body map[string]any, want map[string]any) {
+	t.Helper()
+	for k, v := range want {
+		got, present := body[k]
+		switch {
+		case v == nil && present:
+			t.Errorf("%s = %v, want it absent", k, got)
+		case v != nil && !reflect.DeepEqual(got, v):
+			t.Errorf("%s = %v, want %v", k, got, v)
+		}
+	}
+}
+
+// TestThinkingWire_Claude pins MADR 0013 Q1 and B9 on the Anthropic wire:
+// Claude 4.7 and later take adaptive thinking with output_config.effort
+// (thinking.type "enabled" is HTTP 400 there); older models take a budget,
+// 1024 for "low". An explicit budget wins where a budget is accepted.
+func TestThinkingWire_Claude(t *testing.T) {
+	adaptive := map[string]any{"type": "adaptive"}
+	for _, tc := range []thinkingCase{
+		{"claude-sonnet-5", effortLow, 0, map[string]any{"thinking": adaptive,
+			"output_config": map[string]any{"effort": "low"}}},
+		{"claude-opus-4-8", "", 0, map[string]any{"thinking": adaptive, "output_config": nil}},
+		{"claude-sonnet-5", "", 9000, map[string]any{"thinking": adaptive, "max_tokens": float64(8192)}},
+		{"claude-haiku-4-5", effortLow, 0, map[string]any{"output_config": nil,
+			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(1024)}}},
+		{"claude-haiku-4-5", "", 0, map[string]any{
+			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(4096)}}},
+		{"claude-haiku-4-5", effortLow, 2000, map[string]any{
+			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(2000)}}},
+		{"claude-sonnet-4-20250514", effortLow, 0, map[string]any{
+			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(1024)}}},
+	} {
+		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
+			var body map[string]any
+			srv := captureServer(t, &body, `{"content":[{"type":"text","text":"ok"}]}`)
+			p, err := NewClaude("k", tc.model, WithBaseURL(srv.URL),
+				WithReasoningEffort(tc.effort), WithThinkingBudget(tc.budget))
+			if err != nil {
+				t.Fatal(err)
+			}
+			if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
+				t.Fatalf("GenerateThinking: %v", err)
+			}
+			assertThinkingFields(t, body, tc.want)
+		})
+	}
+}
+
+// TestThinkingWire_Gemini pins MADR 0013 Q1 on the Gemini wire: "low" is
+// thinkingLevel on Gemini 3 and later and a 1024 budget on 2.x (thinkingLevel
+// is HTTP 400 there); other efforts keep the dynamic budget; a budget wins.
+func TestThinkingWire_Gemini(t *testing.T) {
+	for _, tc := range []thinkingCase{
+		{"gemini-3.7-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingLevel": "low"}}},
+		{"gemini-2.5-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(1024)}}},
+		{"gemini-3.7-flash", "", 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(-1)}}},
+		{"gemini-3.7-flash", effortHigh, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(-1)}}},
+		{"gemini-2.5-flash", effortLow, 512, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(512)}}},
+	} {
+		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
+			var body map[string]any
+			srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
+			p, err := NewGemini(context.Background(), "k", tc.model, WithBaseURL(srv.URL),
+				WithReasoningEffort(tc.effort), WithThinkingBudget(tc.budget))
+			if err != nil {
+				t.Fatal(err)
+			}
+			if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
+				t.Fatalf("GenerateThinking: %v", err)
+			}
+			gc, _ := body["generationConfig"].(map[string]any)
+			assertThinkingFields(t, gc, tc.want)
+		})
+	}
+}
+
+// TestThinkingWire_OpencodeRoutes pins the same shapes on OpenCode's messages
+// and google routes, which forward to the same upstream APIs.
+func TestThinkingWire_OpencodeRoutes(t *testing.T) {
+	for _, tc := range []struct {
+		gateway, model, fixture string
+		inGenCfg                bool
+		want                    map[string]any
+	}{
+		{ProviderOpencodeZen, "claude-sonnet-5", fxOpencodeMessages, false, map[string]any{
+			"thinking": map[string]any{"type": "adaptive"}, "output_config": map[string]any{"effort": "low"}}},
+		{ProviderOpencodeGo, "qwen3.8-flash", fxOpencodeMessages, false, map[string]any{
+			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(1024)}}},
+		{ProviderOpencodeZen, "gemini-3.8-flash", fxOpencodeGoogle, true, map[string]any{
+			"thinkingConfig": map[string]any{"thinkingLevel": "low"}}},
+	} {
+		t.Run(tc.gateway+"/"+tc.model, func(t *testing.T) {
+			var body map[string]any
+			srv := captureServer(t, &body, tc.fixture)
+			p, err := NewOpencode(tc.gateway, "k", tc.model, WithBaseURL(srv.URL), WithReasoningEffort(effortLow))
+			if err != nil {
+				t.Fatal(err)
+			}
+			if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
+				t.Fatalf("GenerateThinking: %v", err)
+			}
+			if tc.inGenCfg {
+				body, _ = body["generationConfig"].(map[string]any)
+			}
+			assertThinkingFields(t, body, tc.want)
+		})
+	}
+}
```

### B.5.2 Phase 5 fix

```diff
diff --git a/llmprovider/claude.go b/llmprovider/claude.go
--- a/llmprovider/claude.go
+++ b/llmprovider/claude.go
@@ -16,12 +16,13 @@
 // permanent limitation of Anthropic's current API design, not a TODO.
 // Callers must replay prior items as messages on every call.
 type ClaudeProvider struct {
-	apiKey         string
-	model          string
-	baseURL        string // For testing
-	client         *http.Client
-	maxTokens      int
-	thinkingBudget int // extended-thinking token budget for the GenerateThinking path
+	apiKey          string
+	model           string
+	baseURL         string // For testing
+	client          *http.Client
+	maxTokens       int
+	thinkingBudget  int    // extended-thinking token budget for the GenerateThinking path
+	reasoningEffort string // effort for the GenerateThinking path (see addMessagesThinking)
 }
 
 // defaultClaudeThinkingBudget is used by GenerateThinking when no budget is configured.
@@ -39,12 +40,13 @@
 		baseURL = cfg.BaseURL
 	}
 	return &ClaudeProvider{
-		apiKey:         apiKey,
-		model:          model,
-		baseURL:        baseURL,
-		client:         cfg.HTTPClient,
-		maxTokens:      cfg.MaxTokens,
-		thinkingBudget: cfg.ThinkingBudget,
+		apiKey:          apiKey,
+		model:           model,
+		baseURL:         baseURL,
+		client:          cfg.HTTPClient,
+		maxTokens:       cfg.MaxTokens,
+		thinkingBudget:  cfg.ThinkingBudget,
+		reasoningEffort: cfg.ReasoningEffort,
 	}, nil
 }
 
@@ -119,20 +121,6 @@
 // GenerateItemsWithToolThinking sends items with both tool calling and thinking enabled.
 func (p *ClaudeProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
 	return p.doGenerateItems(ctx, input, &tool, true)
-}
-
-// thinkingParams returns the budget and the effective max_tokens for a thinking request.
-// Anthropic requires max_tokens > budget_tokens, so the ceiling is raised when needed.
-func (p *ClaudeProvider) thinkingParams() (budget, maxTokens int) {
-	budget = p.thinkingBudget
-	if budget <= 0 {
-		budget = defaultClaudeThinkingBudget
-	}
-	maxTokens = p.maxTokens
-	if maxTokens <= budget {
-		maxTokens = budget + defaultClaudeThinkingBudget
-	}
-	return budget, maxTokens
 }
 
 func claudeItemsToMessages(items []Item) []map[string]any {
@@ -175,9 +163,7 @@
 	}
 
 	if thinking {
-		budget, effMax := p.thinkingParams()
-		body["max_tokens"] = effMax
-		body["thinking"] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
+		body["max_tokens"] = addMessagesThinking(body, p.model, p.reasoningEffort, p.thinkingBudget, maxTokens)
 	}
 
 	if tool != nil {
diff --git a/llmprovider/gemini.go b/llmprovider/gemini.go
--- a/llmprovider/gemini.go
+++ b/llmprovider/gemini.go
@@ -11,12 +11,13 @@
 
 // GeminiProvider implements Provider using the Google Gemini API via standard http client.
 type GeminiProvider struct {
-	apiKey         string
-	model          string
-	baseURL        string // For testing
-	client         *http.Client
-	maxTokens      int
-	thinkingBudget int // thinkingConfig budget for the GenerateThinking path
+	apiKey          string
+	model           string
+	baseURL         string // For testing
+	client          *http.Client
+	maxTokens       int
+	thinkingBudget  int    // thinkingConfig budget for the GenerateThinking path
+	reasoningEffort string // effort for the GenerateThinking path (see geminiThinkingConfig)
 }
 
 // dynamicGeminiThinkingBudget (-1) lets the model size its own thinking budget.
@@ -31,25 +32,22 @@
 		baseURL = cfg.BaseURL
 	}
 	return &GeminiProvider{
-		apiKey:         apiKey,
-		model:          model,
-		baseURL:        baseURL,
-		client:         cfg.HTTPClient,
-		maxTokens:      cfg.MaxTokens,
-		thinkingBudget: cfg.ThinkingBudget,
+		apiKey:          apiKey,
+		model:           model,
+		baseURL:         baseURL,
+		client:          cfg.HTTPClient,
+		maxTokens:       cfg.MaxTokens,
+		thinkingBudget:  cfg.ThinkingBudget,
+		reasoningEffort: cfg.ReasoningEffort,
 	}, nil
 }
 
 // genConfig builds the generationConfig map, adding a thinkingConfig when the thinking
-// path is requested. A non-positive configured budget maps to -1 (dynamic thinking).
+// path is requested (see geminiThinkingConfig).
 func (p *GeminiProvider) genConfig(thinking bool) map[string]any {
 	cfg := map[string]any{"maxOutputTokens": p.maxTokens}
 	if thinking {
-		budget := p.thinkingBudget
-		if budget <= 0 {
-			budget = dynamicGeminiThinkingBudget
-		}
-		cfg["thinkingConfig"] = map[string]any{"thinkingBudget": budget}
+		cfg["thinkingConfig"] = geminiThinkingConfig(p.model, p.reasoningEffort, p.thinkingBudget)
 	}
 	return cfg
 }
diff --git a/llmprovider/model_profile.go b/llmprovider/model_profile.go
--- a/llmprovider/model_profile.go
+++ b/llmprovider/model_profile.go
@@ -13,8 +13,9 @@
 )
 
 // ReasoningEffort is the recommended request effort for the profile: "low"
-// for ProfileUtility, and "" for ProfileCapable, meaning the model's own
-// default. A value outside the two profiles is treated as ProfileUtility.
+// for ProfileUtility, and "" for ProfileCapable, meaning each provider's
+// documented default (see ProviderConfig.ReasoningEffort; MADR 0013 Q2). A
+// value outside the two profiles is treated as ProfileUtility.
 func (p ModelProfile) ReasoningEffort() string {
 	if p == ProfileCapable {
 		return ""
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -178,14 +178,7 @@
 		jsonKeyMessages: claudeItemsToMessages(input),
 	}
 	if thinking {
-		budget := p.thinkingBudget
-		if budget <= 0 {
-			budget = defaultClaudeThinkingBudget
-		}
-		if maxTokens <= budget {
-			maxTokens = budget + defaultClaudeThinkingBudget
-		}
-		body["thinking"] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
+		maxTokens = addMessagesThinking(body, p.model, p.reasoningEffort, p.thinkingBudget, maxTokens)
 	}
 	body[jsonKeyMaxTokens] = maxTokens
 	if tool != nil {
@@ -208,11 +201,7 @@
 func (p *OpencodeProvider) googleBody(input []Item, tool *Tool, thinking bool) map[string]any {
 	genCfg := map[string]any{"maxOutputTokens": p.maxTokens}
 	if thinking {
-		budget := p.thinkingBudget
-		if budget <= 0 {
-			budget = dynamicGeminiThinkingBudget
-		}
-		genCfg["thinkingConfig"] = map[string]any{"thinkingBudget": budget}
+		genCfg["thinkingConfig"] = geminiThinkingConfig(p.model, p.reasoningEffort, p.thinkingBudget)
 	}
 	body := map[string]any{
 		"contents":         geminiItemsToContents(input),
diff --git a/llmprovider/options.go b/llmprovider/options.go
--- a/llmprovider/options.go
+++ b/llmprovider/options.go
@@ -30,8 +30,13 @@
 	// (Claude "thinking", Gemini "thinkingConfig"). Zero leaves the per-provider
 	// default in effect.
 	ThinkingBudget int
-	// ReasoningEffort selects OpenAI reasoning effort ("low"|"medium"|"high") for
-	// the GenerateThinking path. Empty leaves the per-provider default in effect.
+	// ReasoningEffort selects the reasoning effort ("low"|"medium"|"high") for
+	// the GenerateThinking path of every provider. Effort APIs send it as is;
+	// Claude 4.7 and later send output_config.effort; older Claude and Gemini
+	// map "low" to a small budget or thinkingLevel (MADR 0013 Q1). Empty leaves
+	// each provider's documented default: medium on the effort APIs, high on
+	// Grok 4.5, the model's own on Kilo and Claude 4.7+, a 4096 budget on older
+	// Claude, and dynamic thinking on Gemini (MADR 0013 Q2).
 	ReasoningEffort string
 	// OpencodeRoute overrides the wire format the OpenCode gateway providers use
 	// for the configured model. Empty means "resolve from the built-in route
@@ -86,8 +91,9 @@
 	}
 }
 
-// WithReasoningEffort sets the OpenAI reasoning effort ("low"|"medium"|"high") used by
-// the provider's GenerateThinking path. An empty value leaves the default in effect.
+// WithReasoningEffort sets the reasoning effort ("low"|"medium"|"high") used by the
+// provider's GenerateThinking path; see ProviderConfig.ReasoningEffort. An empty value
+// leaves each provider's default in effect.
 func WithReasoningEffort(s string) ProviderOption {
 	return func(cfg *ProviderConfig) {
 		cfg.ReasoningEffort = s
diff --git a/llmprovider/thinking_wire.go b/llmprovider/thinking_wire.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/thinking_wire.go
@@ -0,0 +1,108 @@
+package llmprovider
+
+import (
+	"maps"
+	"regexp"
+	"strconv"
+	"strings"
+)
+
+// Thinking request shapes for the budget-based wires, the Anthropic Messages
+// API and Gemini's generateContent, shared by ClaudeProvider, GeminiProvider
+// and OpenCode's messages and google routes (MADR 0013 Q1, B9). The model id
+// selects the shape, as in OpenCode's own client
+// (packages/opencode/src/provider/transform.ts).
+
+// lowEffortThinkingBudget is the budget "low" maps to where a model takes only
+// a budget. It is Anthropic's minimum: 1023 is refused with HTTP 400
+// (measured 2026-09-27).
+const lowEffortThinkingBudget = 1024
+
+// Request keys of the two budget-based wires.
+const (
+	jsonKeyThinking       = "thinking"
+	jsonKeyThinkingBudget = "thinkingBudget"
+)
+
+// claudeVersionRE reads a Claude version, family-first (claude-opus-4-8) or
+// version-first (claude-4.7-opus). A minor has at most two digits, so the date
+// in claude-sonnet-4-20250514 is not read as one. OpenCode's
+// anthropicUsesModernAdaptiveThinking uses the same pattern.
+var claudeVersionRE = regexp.MustCompile(`(?i)claude-(?:[a-z]+-)?(\d+)(?:[.-](\d{1,2}))?(?:[.@-]|$)`)
+
+// geminiLegacyRE matches Gemini 1.x and 2.x ids, which take thinkingBudget but
+// not thinkingLevel (HTTP 400 on gemini-2.5-flash, measured 2026-09-27). It is
+// OpenCode's GEMINI_LEGACY_RE.
+var geminiLegacyRE = regexp.MustCompile(`(?i)gemini-(?:(?:flash|pro)-)?[12](?:[.-]|$)`)
+
+// claudeAdaptiveOnly reports whether model refuses thinking.type "enabled":
+// Claude 4.7 and later, and a claude- id with no readable version (HTTP 400 on
+// claude-sonnet-5 and claude-opus-4-8, measured 2026-09-27). Other ids,
+// including the non-Claude models OpenCode serves on its messages route, take
+// a budget.
+func claudeAdaptiveOnly(model string) bool {
+	lower := strings.ToLower(model)
+	if !strings.Contains(lower, "claude-") {
+		return false
+	}
+	m := claudeVersionRE.FindStringSubmatch(lower)
+	if m == nil {
+		return true
+	}
+	major, err := strconv.Atoi(m[1])
+	if err != nil {
+		return true
+	}
+	minor := 0
+	if m[2] != "" {
+		if minor, err = strconv.Atoi(m[2]); err != nil {
+			return true
+		}
+	}
+	return major > 4 || (major == 4 && minor >= 7)
+}
+
+// addMessagesThinking adds the thinking fields of an Anthropic Messages
+// request to body and returns its max_tokens. Adaptive-only models get
+// thinking.type "adaptive", plus output_config.effort when an effort is set; a
+// budget does not apply to them. Other models get an enabled budget: the
+// configured one, else lowEffortThinkingBudget for "low", else
+// defaultClaudeThinkingBudget, with max_tokens raised above it when needed
+// (Anthropic requires max_tokens > budget_tokens).
+func addMessagesThinking(body map[string]any, model, effort string, budget, maxTokens int) int {
+	if claudeAdaptiveOnly(model) {
+		fields := map[string]any{jsonKeyThinking: map[string]any{jsonKeyType: "adaptive"}}
+		if effort != "" {
+			fields["output_config"] = map[string]any{jsonKeyEffort: effort}
+		}
+		maps.Copy(body, fields)
+		return maxTokens
+	}
+	if budget <= 0 {
+		budget = defaultClaudeThinkingBudget
+		if effort == effortLow {
+			budget = lowEffortThinkingBudget
+		}
+	}
+	if maxTokens <= budget {
+		maxTokens = budget + defaultClaudeThinkingBudget
+	}
+	body[jsonKeyThinking] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
+	return maxTokens
+}
+
+// geminiThinkingConfig returns a Gemini thinkingConfig. A configured budget
+// wins. Otherwise "low" is thinkingLevel "low" on Gemini 3 and later and a
+// lowEffortThinkingBudget budget on 1.x and 2.x, and any other effort keeps
+// the dynamic budget.
+func geminiThinkingConfig(model, effort string, budget int) map[string]any {
+	switch {
+	case budget > 0:
+		return map[string]any{jsonKeyThinkingBudget: budget}
+	case effort != effortLow:
+		return map[string]any{jsonKeyThinkingBudget: dynamicGeminiThinkingBudget}
+	case geminiLegacyRE.MatchString(model):
+		return map[string]any{jsonKeyThinkingBudget: lowEffortThinkingBudget}
+	}
+	return map[string]any{"thinkingLevel": effortLow}
+}
```

### B.6.1 Phase 6 tests

```diff
diff --git a/llmprovider/live_gateways_test.go b/llmprovider/live_gateways_test.go
--- a/llmprovider/live_gateways_test.go
+++ b/llmprovider/live_gateways_test.go
@@ -14,7 +14,9 @@
 // OpenCode tests REQUIRE OPENCODE_API_KEY (plan deviation D3). Its free models
 // answer 200 with NO Authorization header but 401 with a bogus one, and
 // NewOpencode requires a non-empty key and always sends it — so a placeholder
-// is strictly worse than none there.
+// is strictly worse than none there. The generation tests use paid OpenCode Go
+// models: Zen's free tier refuses clients other than OpenCode (403
+// FreeTierError, measured 2026-09-26/27; MADR 0013 D1).
 //
 // The Hugging Face test REQUIRES HF_TOKEN: HF reports is_free:false for all
 // provider offerings, so no credential-free path exists (verified 2026-08-29).
@@ -107,7 +109,7 @@
 func TestLive_OpencodeChatCompletions(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeZen, opencodeKey(t), "hy3-free")
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "hy3")
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
@@ -124,7 +126,7 @@
 func TestLive_OpencodeResponses(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeZen, opencodeKey(t), "muse-spark-1.2-contributor-free")
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "gpt-6-luna")
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
@@ -151,12 +153,12 @@
 // route table rests on: routes are NOT interchangeable. If this fails, OpenCode
 // has become a translating gateway and the table is no longer necessary.
 func TestLive_OpencodeRouteStillEnforced(t *testing.T) {
-	const model = "muse-spark-1.2-contributor-free"
+	const model = "gpt-6-luna"
 	ctx, cancel := liveCtx(t)
 	defer cancel()
 
 	key := opencodeKey(t)
-	onResponses, err := NewOpencode(ProviderOpencodeZen, key, model)
+	onResponses, err := NewOpencode(ProviderOpencodeGo, key, model)
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
@@ -166,7 +168,7 @@
 		t.Fatalf("%s must still succeed on its documented /responses route: %v", model, err)
 	}
 
-	onChat, err := NewOpencode(ProviderOpencodeZen, key, model,
+	onChat, err := NewOpencode(ProviderOpencodeGo, key, model,
 		WithOpencodeRoute(OpencodeRouteChatCompletions))
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
diff --git a/llmprovider/live_static_test.go b/llmprovider/live_static_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_static_test.go
@@ -0,0 +1,33 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"errors"
+	"testing"
+)
+
+// TestLive_StaticClaudeServed pins MADR 0013 B10: every StaticClaude id
+// answers on the Messages API. The static catalog is what the wizard offers
+// when the listing fails, so a retired id there is a dead end. It REQUIRES
+// ANTHROPIC_API_KEY and skips without it.
+func TestLive_StaticClaudeServed(t *testing.T) {
+	key := liveEnvKey(t, "ANTHROPIC_API_KEY")
+	for _, model := range StaticClaude {
+		t.Run(model, func(t *testing.T) {
+			ctx, cancel := liveCtx(t)
+			defer cancel()
+			p, err := NewClaude(key, model, WithMaxTokens(16))
+			if err != nil {
+				t.Fatal(err)
+			}
+			_, err = p.Generate(ctx, "Reply with only the word ALPHA")
+			if errors.Is(err, ErrRateLimited) {
+				t.Skipf("rate limited: %v", err)
+			}
+			if err != nil {
+				t.Errorf("%s: %v", model, err)
+			}
+		})
+	}
+}
```

### B.6.2 Phase 6 fix

```diff
diff --git a/llmprovider/models_catalog.go b/llmprovider/models_catalog.go
--- a/llmprovider/models_catalog.go
+++ b/llmprovider/models_catalog.go
@@ -44,14 +44,13 @@
 		"o4-mini",
 	}
 
-	// StaticClaude: current aliases first, then widely available older IDs.
+	// StaticClaude: current aliases, each answering on the Messages API
+	// (verified 2026-09-27, MADR 0013 B10).
 	StaticClaude = []string{
 		"claude-haiku-4-5",
 		"claude-sonnet-5",
 		"claude-sonnet-4-6",
 		"claude-opus-4-8",
-		"claude-3-5-haiku-latest",
-		"claude-sonnet-4-20250514",
 	}
 
 	// StaticOpencodeZen: MADR 0010 §7's utility six, ranked from the
diff --git a/wizard/model_select_test.go b/wizard/model_select_test.go
--- a/wizard/model_select_test.go
+++ b/wizard/model_select_test.go
@@ -83,9 +83,9 @@
 	if !slices.Contains(f.seenInput, searchModelsPrompt) {
 		t.Errorf("inputs = %v, want the search prompt", f.seenInput)
 	}
-	menu := f.seenSelectItems[1]
-	if len(menu) != 7 || menu[6].Label != otherModelLabel {
-		t.Errorf("model menu = %v, want 6 recommended rows then Other", labels(menu))
+	menu, n := f.seenSelectItems[1], len(llmprovider.StaticClaude)
+	if len(menu) != n+1 || menu[n].Label != otherModelLabel {
+		t.Errorf("model menu = %v, want %d recommended rows then Other", labels(menu), n)
 	}
 }
 
@@ -130,7 +130,7 @@
 func TestConfigureLLM_SearchAgain(t *testing.T) {
 	withEnv(t, nil)
 	f := &fakePrompter{
-		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 2, 1},
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 1, 1},
 		inputs: []string{"haiku", ""}, secrets: []string{testKey},
 	}
 	res, err := ConfigureLLM(context.Background(), f, Options{})
@@ -139,7 +139,6 @@
 	}
 	want := []string{
 		llmprovider.ModelLabel(llmprovider.ProviderClaude, "claude-haiku-4-5"),
-		llmprovider.ModelLabel(llmprovider.ProviderClaude, "claude-3-5-haiku-latest"),
 		searchAgainLabel, otherModelLabel,
 	}
 	if got := labels(f.seenSelectItems[1]); !slices.Equal(got, want) {
@@ -153,7 +152,7 @@
 func TestConfigureLLM_OtherFromSearchResults(t *testing.T) {
 	withEnv(t, nil)
 	f := &fakePrompter{
-		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 3},
+		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 2},
 		inputs: []string{"haiku", "my-id"}, secrets: []string{testKey},
 	}
 	res, err := ConfigureLLM(context.Background(), f, Options{})
@@ -189,7 +188,8 @@
 
 func TestConfigureLLM_CurrentModelListed(t *testing.T) {
 	withEnv(t, nil)
-	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 6}, secrets: []string{testKey}}
+	n := len(llmprovider.StaticClaude)
+	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), n}, secrets: []string{testKey}}
 	res, err := ConfigureLLM(context.Background(), f, Options{
 		Existing: Result{Provider: llmprovider.ProviderClaude, Model: "claude-opus-5"},
 	})
@@ -197,11 +197,11 @@
 		t.Fatalf("ConfigureLLM: %v", err)
 	}
 	menu := f.seenSelectItems[1]
-	if len(menu) != 8 || menu[6] != (Choice{Label: "claude-opus-5", Detail: currentModelDetail}) || menu[7].Label != otherModelLabel {
-		t.Errorf("menu = %+v, want 6 recommended rows, the current row, then Other", menu)
-	}
-	if f.seenSelectDefault[1] != 6 {
-		t.Errorf("default = %d, want 6 (the current row)", f.seenSelectDefault[1])
+	if len(menu) != n+2 || menu[n] != (Choice{Label: "claude-opus-5", Detail: currentModelDetail}) || menu[n+1].Label != otherModelLabel {
+		t.Errorf("menu = %+v, want %d recommended rows, the current row, then Other", menu, n)
+	}
+	if f.seenSelectDefault[1] != n {
+		t.Errorf("default = %d, want %d (the current row)", f.seenSelectDefault[1], n)
 	}
 	if res.Model != "claude-opus-5" {
 		t.Errorf("Model = %q, want claude-opus-5", res.Model)
@@ -217,8 +217,8 @@
 		t.Fatalf("ConfigureLLM: %v", err)
 	}
 	menu := f.seenSelectItems[1]
-	if len(menu) != 7 {
-		t.Errorf("menu = %+v, want 7 rows (no current row for another provider)", menu)
+	if n := len(llmprovider.StaticClaude) + 1; len(menu) != n {
+		t.Errorf("menu = %+v, want %d rows (no current row for another provider)", menu, n)
 	}
 	for _, c := range menu {
 		if c.Detail == currentModelDetail {
```

