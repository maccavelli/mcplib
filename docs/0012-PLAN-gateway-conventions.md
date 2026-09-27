---
status: complete
date: 2026-09-27
associated-madr: "0012-MADR-conform-providers-to-reference-clients.md"
decision-makers: mcplib maintainers
---

# Implement 0012 §3 — Gateway Conventions (OpenCode and Kilo)

Associated MADR: [0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
(accepted 2026-09-27, revision 3). This is the third of that MADR's six plans
(§8).

This plan executes MADR §3 as amended in revision 3, and nothing else. If a
fact contradicts the MADR or this plan, **stop and prompt**. Add a dated
entry to §9 of this plan, amend the MADR when a decision or an asserted fact
changes, and only then continue.

## How this plan was proven

Every phase below was executed on 2026-09-27, in scratch copies of
`git archive 7ea0ad4` with `0012-PLAN-item-fidelity.md` applied, outside the
repository:
* **Red.** Each phase's tests diff was applied, and each named test was seen
  to fail on the unfixed code. Appendix A quotes each failure.
* **Green.** The fix diff was applied, and the full gate (§0.2) passed.
* **Mutants.** A guard that holds today, or a test of new API, was seen to
  fail on a named mutant.
* **Live.** Each live test ran against the real gateways.
* **Gates.** G-O and G-K ran first; their transcripts are in the MADR's
  revision 3.
* **Diffs.** Appendix B's diffs were generated mechanically from that proof.
  Applying all five plans' diffs in §8's order to a fresh `7ea0ad4` archive
  reproduces the proven tree byte for byte: 308 files, 0 mismatches.

## Goal

* **OpenCode routes (§3.1).** An OpenCode Zen or Go request uses the route
  its model's `provider.npm` names in models.opencode.ai's document, as
  OpenCode's client does:
  * an explicit `WithOpencodeRoute` still wins;
  * the built-in table, refreshed from the same document, is the fallback;
  * `jev-*` (the `systemone` route) is not usable.
* **OpenCode conventions (§3.2):**
  * the Responses route sends `store: false`;
  * MiniMax-M3 on the Messages route thinks adaptively.
* **Kilo conventions (§3.3):**
  * **Data collection.** Requests send `provider.data_collection: "deny"`
    unless `WithKiloDataCollection(true)`. A model that requires collection
    is refused with a terminal `ErrNotPermitted`.
  * **Tokens and organizations.** A URL-prefixed token selects its backend,
    and an organization, from the token or from `WithKiloOrganization`,
    scopes generation and the listing.
* **Live suite (§3.4):** each OpenCode and Kilo live test picks, at run
  time, the first of its candidate models that the metadata lists as active,
  and skips when none is.

## Scope

**In scope:** MADR §3.1–§3.4 as amended: G1–G5 below.

**Out of scope:**
* The Kilo `max_tokens` clamp (K3) and the `/models` 401 anonymous retry.
  The owner dropped both on 2026-09-27; the evidence is in the MADR's
  revision 3.
* §3.2's chat `reasoning_effort`, and adaptive thinking for Claude 4.7+:
  already shipped (MADR revision 2).
* §3.3's Kilo `reasoning` object: already shipped (0010 §6).

## 0. Preconditions and conventions

### 0.1 Baseline

* `0012-PLAN-item-fidelity.md` complete on `main`. The diffs were generated
  on that tree. On any other base they need rebasing, which is a §9
  deviation.
* `go test -count=1 ./...` passes before G1.

### 0.2 Gate (every phase)

1. `gofmt -l` prints nothing, and `golint -set_exit_status` passes, on each
   `.go` file the phase touched.
2. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
3. `make lint`.
4. `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

### 0.3 Red first

As in `0012-PLAN-item-fidelity.md` §0.3: tests diff, red run, fix diff,
gate, then mutants on scratch copies.

### 0.4 Credentials for live steps

`OPENCODE_API_KEY` (a funded Go key) and `KILO_API_KEY`. Each live test
skips without its credential.

### 0.5 Commits

One `git commit --no-edit` per phase, after the gate. No push and no tag.

## Phase G0 — Start

1. Confirm §0.1.
2. Set this plan to `status: in-progress`.
3. Commit the documents only.

## Phase G1 — OpenCode routes from the model metadata (§3.1, G-O)

**Today.**
* `resolveOpencodeRoute` (`opencode_route.go`) fixes each model's route at
  construction, from a table dated 2026-08-28 and a prefix heuristic. On
  2026-09-27 the metadata disagreed with it:
  * Go's `qwen3.6-plus`, `qwen3.7-max`, `qwen3.7-plus` and `qwen3.8-max`
    route to chat, where the table says messages;
  * Zen's `qwen3.8-max` routes to chat, where the heuristic says messages;
  * 27 listed, non-deprecated models are missing from the table: 19 on Zen
    and 8 on Go.
* `isUsableOpencodeModel` admits `jev-1.13` and `jev-1.13-free`, which use
  the `systemone` route that `mcplib` cannot encode.

**Changes** (Appendix B.G1):
* **Metadata:**
  * `modelMetadata.Provider.NPM` is decoded;
  * `opencodeRouteForNPM` maps `@ai-sdk/openai` to responses,
    `@ai-sdk/anthropic` to messages, `@ai-sdk/google` to google, and
    anything else, or unset, to chat (`provider.ts:1274-1278`).
* **Per-request route.** `OpencodeProvider.requestRoute` picks each
  request's route in order:
  1. the pinned `WithOpencodeRoute`;
  2. the metadata route, looked up within `metadataLookupTimeout`;
  3. the construction-time table route.

  `Route()` keeps reporting the construction-time route, and its comment
  says so.
* **Table.** It is regenerated from the 2026-09-27 document: every
  non-deprecated `opencode` and `opencode-go` model. The testdata snapshot
  `testdata/opencode-routes.json` pins it.
* **Usability:** `jev-*` is denied.
* **Test updates:**
  * `TestOpencodeRoute_Heuristic` swaps `claude-sonnet-4` for an untabled
    id, because `claude-sonnet-4` is now tabled;
  * the 2026-09-26 ranking snapshot's Zen usable count goes from 80 to 78,
    because of `jev-*` (MADR revision 3 notes the change to 0010 §7).

**Verification.**
* Red:
  * `TestOpencode_RoutesFromMetadata`, over the four packages, an unset
    package, and a model the table routes differently;
  * `TestOpencodeRouteTable_MatchesMetadataSnapshot`;
  * `TestIsUsableOpencodeModel_DeniesSystemone`;
  * live `TestLive_OpencodeRoutesFromMetadata` on Go `qwen3.8-max`.
* Guards proven by mutants: the override beats the metadata; missing or
  disabled metadata leaves the table.
* Gate G-O showed both routes answer for Go qwen, so following the metadata
  is safe.

## Phase G2 — OpenCode Responses store and MiniMax-M3 thinking (§3.2)

**Today.**
* `responsesBody` omits `store`, where OpenCode's client sends `false` for
  every `@ai-sdk/openai` model (`transform.ts:1235-1243`).
* `addMessagesThinking` gives MiniMax-M3 an enabled budget, where OpenCode
  sends `thinking: {type: adaptive}` (`transform.ts:1293-1296`).

**Changes** (Appendix B.G2):
* `responsesBody` sends `store: false`.
* `minimaxAdaptiveThinking` sends `{type: adaptive}` for `minimax-m3`, with
  no budget and no effort.

**Verification.**
* Two unit tests and two live tests are red.
* The guard `TestOpencodeMessages_OtherModelsKeepBudget` is proven by a
  mutant.
* Live, both forms were accepted, and adaptive thinking returned reasoning.
  This is conformance, not a failure avoided (MADR revision 3).

## Phase G3 — Kilo data collection (§3.3, G-K)

**Today.**
* Kilo requests never send `provider.data_collection`.
* `parseAPIErrorBody` ignores Kilo's top-level `error_type`, so gate G-K's
  `data_collection_required` would surface as a plain `ErrInvalidRequest`.

**Changes** (Appendix B.G3):
* **Option:** `ProviderConfig.KiloDataCollection` and
  `WithKiloDataCollection(allow bool)`.
* **Request:** `KiloProvider` sends `{"data_collection": "deny"}` unless
  collection is allowed.
* **Errors:**
  * `parseAPIErrorBody` reads `error_type`;
  * Kilo's `data_collection_required` is a terminal `ErrNotPermitted` that
    still matches `ErrInvalidRequest` for a 400 (MADR §7).
* **Live-test updates.** `TestLive_KiloChatCompletions` and
  `TestLive_KiloToolCall` target `kilo-auto/free`, which requires
  collection, so they opt in with `WithKiloDataCollection(true)`.

**Verification.**
* Red:
  * `TestKilo_DeniesDataCollectionByDefault`;
  * `TestClassify_KiloDataCollectionRequired`, with gate G-K's verbatim
    body;
  * live `TestLive_KiloDataCollectionDenied` on `kilo-auto/free`.
* `TestKilo_DataCollectionAllowed` names the new option, so a mutant proves
  it.
* Live, the non-training `deepseek/deepseek-v4.1-flash` still completes a
  tool round trip with the default `deny`.

## Phase G4 — Kilo URL-prefixed tokens and organizations (§3.3, K4)

**Today.**
* **URL tokens.** A token `"https://host/prefix:secret"` is sent, as a
  bearer, to `api.kilo.ai`: the wrong backend.
* **Organizations.** They are unsupported.

**Changes** (Appendix B.G4):
* **URL tokens.** `resolveKiloEndpoints` applies Kilo's token rule
  (`auth/token.ts:9`) and its `route()` (`api/url.ts:9-30`). A URL-prefixed
  token sends generation and the listing to `{origin}{prefix}/api/gateway`.
  The whole token remains the bearer.
* **Organizations:**
  * an organization comes from `WithKiloOrganization`, else from a token
    path `…/api/organizations/{id}`;
  * it adds `X-KILOCODE-ORGANIZATIONID` to generation and the listing;
  * it lists `{origin}{prefix}/api/organizations/{id}/models`
    (`api/models.ts:219`).
* **Plumbing.** `DiscoverModels`, `fetchKiloCatalog` and
  `KiloModelCapabilities` share the same endpoints.

**Verification.**
* Red: `TestKilo_URLTokenSelectsBase` (three token forms) and
  `TestKilo_URLTokenOrganization`. A recording transport observes both
  without a network.
* Guards proven by mutants:
  * a plain key, including one whose secret contains a URL, keeps the
    default gateway and sends no organization header;
  * the option scopes both requests.
* **No live run.** No organization account or URL-prefixed token was
  available. The rules are verified against the Kilo client's source
  (MADR revision 3 records where `mcplib` deliberately differs).

## Phase G5 — The live suite picks active models at run time (§3.4)

**Today.** Each OpenCode and Kilo live test names one fixed model. A model
the metadata deprecates or drops is still targeted, and the test fails (O6).

**Changes** (Appendix B.G5, tests only):
* **Logic.** In an untagged test file:
  * `decodePickerDoc` reads the `opencode`, `opencode-go` and `kilo`
    sections of models.opencode.ai's document, with each model's status.
    `mcplib`'s own decoder keeps no `kilo` section;
  * `firstActiveModel(doc, provider, candidates)` returns the first
    candidate listed for the provider with a status other than
    `deprecated`.
* **Picker.** `liveModel(t, provider, candidates...)` fetches the document
  once per run, directly, so the providers under test still run without
  metadata. It then applies three rules:
  * it picks the first active candidate;
  * with the document unreachable, it keeps the first candidate;
  * with no candidate active, it skips the test.
* **Call sites.** Every OpenCode and Kilo live call site uses the picker.
  Candidates share what each test needs:

  | Test needs | Candidates |
  |---|---|
  | a Go chat-routed model | `hy3`, `glm-5.3-flash`, `kimi-k2.6` |
  | a Go responses-routed model | `gpt-6-luna`, `grok-4.6` |
  | a Go messages-routed model | `qwen3.8-flash`, `minimax-m3` |
  | a free Kilo model that requires data collection | `kilo-auto/free`, `nvidia/nemotron-3.5-lightning:free`, `poolside/laguna-s-2.1:free` |
  | a paid Kilo model that does not train on prompts | `deepseek/deepseek-v4.1-flash`, `z-ai/glm-5.3-flash` |

  Every Kilo candidate listed tools and reasoning on 2026-09-27. A test that
  needs one particular model (`qwen3.8-max`'s metadata route,
  `minimax-m3`'s adaptive thinking, `kimi-k2.6`'s interleaved field, the
  reasoning-effort pair) names only that model, and so skips when it is
  deprecated.
* **Table-driven tests.** The builders in `live_items_test.go` and
  `live_system_test.go` receive the subtest's `t`, so the picker can skip a
  single subtest.
* **`TestLive_ModelPickerSkipsDeprecated`** asks for Go `glm-5`, then
  `glm-5.3-flash`. `glm-5` was deprecated on 2026-09-27.
* **`TestLive_ModelPickerKilo`** asks for an unlisted Kilo id, then
  `deepseek/deepseek-v4.1-flash`. It fails, rather than skips, when the
  document has no `kilo` section.

**Verification.**
* This phase is test infrastructure, so nothing is red.
* Six mutants prove it:
  * accepting a deprecated model;
  * reversing the candidate order;
  * reading another provider's section;
  * dropping the `kilo` section;
  * accepting a deprecated model, seen live;
  * dropping the `kilo` section, seen live.
* Every OpenCode and Kilo live test passed through the picker; Appendix A.L
  lists what ran.

## Phase G6 — Records and close-out

1. Record each phase's result in §10.
2. Set this plan to `status: complete` once §7 holds.
3. Commit the documents only.

## 7. Acceptance criteria

* Every Appendix A red test fails before its fix and passes after it.
* Every mutant is killed.
* The gate passes after each phase.
* The live tests pass, or skip only for a missing credential or a transient
  error.
* The MADR's §3.1 confirmation holds:
  * metadata with `@ai-sdk/anthropic` routes to messages;
  * absent metadata falls back to the table;
  * `WithOpencodeRoute` wins.

## 8. Rollout and rollback

* **Behaviour changes:**
  * an OpenCode request may take a different route when the metadata says
    so (G1);
  * Kilo requests deny data collection by default (G3). A consumer that
    relied on free Kilo models must pass `WithKiloDataCollection(true)`. The
    wizard's listing never offered them.
* **Additive API:** `WithKiloDataCollection` and `WithKiloOrganization`.
* **Rollback:** revert the phase commit. G1 and G3 are independent. G4
  builds on G3's option plumbing only textually.

## 9. Deviation log

None yet.

## 10. Execution record

Executed on `main`, 2026-09-27, after `0012-PLAN-circuit-breaker-test.md`
(`05a1fcf`). Each phase applied Appendix B with `git apply`, taken from this
document, and checked equal to the proven diff.

* **G1**, commit `6f06349`: 4 red tests failed as required; 2 base guards passed; gate passed; 3/3 mutants killed.
* **G2**, commit `b37caec`: 4 red tests failed as required; 1 base guard passed; gate passed; 1/1 mutants killed.
* **G3**, commit `af649ea`: 3 red tests failed as required; 0 base guards passed; gate passed; 1/1 mutants killed.
* **G4**, commit `76fdae7`: 2 red tests failed as required; 1 base guard passed; gate passed; 4/4 mutants killed.
* **G5**, commit `6a75c48`: nothing red (see Appendix A); 4 base guards passed; gate passed; 6/6 mutants killed.

**Live** (`-tags live_gateways`, this plan's tests, on the executed tree): 35 passed, 0 skipped, 0 failed.

## Appendix A — Proof record (2026-09-27)

### A.G1 Phase G1 — OpenCode routes from the model metadata (§3.1, G-O)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestOpencode_RoutesFromMetadata` | unit | path = "/chat/completions", want "/messages" |
| `TestOpencodeRouteTable_MatchesMetadataSnapshot` | unit | opencode-zen qwen3.8-max (npm ""): route = "messages", <nil> |
| `TestIsUsableOpencodeModel_DeniesSystemone` | unit | isUsableOpencodeModel("jev-1.13") = true, want false |
| `TestLive_OpencodeRoutesFromMetadata` | live | request paths = [/zen/go/v1/messages], want one /chat/completions |

**Guards before the fix:**

* `TestOpencode_RouteOverrideBeatsMetadata`: passed on the unfixed code.
* `TestOpencode_RouteWithoutMetadataUsesTable`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 8 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gw1-metadata-before-override` | `TestOpencode_RouteOverrideBeatsMetadata` | killed | path = "/messages", want the override's /chat/completions |
| `gw1-absent-model-is-chat` | `TestOpencode_RouteWithoutMetadataUsesTable` | killed | path = "/chat/completions", want the table's /messages |
| `gw1-no-metadata-is-chat` | `TestOpencode_RouteWithoutMetadataUsesTable` | killed | path = "/chat/completions", want the table's /messages |

### A.G2 Phase G2 — OpenCode Responses store and MiniMax-M3 thinking (§3.2)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestOpencodeResponses_SendsStoreFalse` | unit | store = <nil> (present false), want false |
| `TestOpencodeMessages_MinimaxM3ThinksAdaptive` | unit | thinking = map[budget_tokens:4096 type:enabled], want exactly {type: adaptive} |
| `TestLive_OpencodeResponsesStoreFalse` | live | request did not send store:false: {"input":[{"content":"Reply with only the word ALPHA","role":"user"}],"max_output_tokens":8192,"model":"gpt-6-luna"} |
| `TestLive_OpencodeMinimaxM3Adaptive` | live | request did not think adaptively: {"max_tokens":8192,"messages":[{"content":"What is 17 * 23? Reply with the number.","role":"user"}],"model":"minimax-m3","thinking":{"budget_tokens":4096,"type":"enabled"}} |

**Guards before the fix:**

* `TestOpencodeMessages_OtherModelsKeepBudget`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 4 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gw2-adaptive-everywhere` | `TestOpencodeMessages_OtherModelsKeepBudget` | killed | thinking = map[type:adaptive], want an enabled budget |

### A.G3 Phase G3 — Kilo data collection (§3.3, G-K)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestKilo_DeniesDataCollectionByDefault` | unit | provider = map[], want data_collection deny |
| `TestClassify_KiloDataCollectionRequired` | unit | err = llm: invalid request: kilo HTTP 400: Data collection is required for this model. Please enable data collection to use this model or choose another model. (terminal true), want terminal ErrNotPermitted |
| `TestLive_KiloDataCollectionDenied` | live | err = <nil>, want ErrNotPermitted with type data_collection_required |

**Guards before the fix:**

* `TestKilo_DataCollectionAllowed`: added with the fix.

**Gate** (fix applied): all passed — per-file `golint` on 7 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gw3-option-ignored` | `TestKilo_DataCollectionAllowed` | killed | provider = map[data_collection:deny], want absent |

### A.G4 Phase G4 — Kilo URL-prefixed tokens and organizations (§3.3, K4)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestKilo_URLTokenSelectsBase` | unit | requests: |
| `TestKilo_URLTokenOrganization` | unit | requests: |

**Guards before the fix:**

* `TestKilo_PlainTokenUsesDefaults`: passed on the unfixed code.
* `TestKilo_OrganizationOption`: added with the fix.

**Gate** (fix applied): all passed — per-file `golint` on 5 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gw4-org-header-always` | `TestKilo_PlainTokenUsesDefaults` | killed | requests: |
| `gw4-unanchored` | `TestKilo_PlainTokenUsesDefaults` | killed | requests: |
| `gw4-option-ignored` | `TestKilo_OrganizationOption` | killed | requests: |
| `gw4-listing-ignores-org` | `TestKilo_OrganizationOption` | killed | requests: |

### A.G5 Phase G5 — The live suite picks active models at run time (§3.4)

**Red:** none. This phase is test infrastructure with no fix, so its tests pass as soon as they exist; mutants prove each one (below).

**Guards before the fix:**

* `TestFirstActiveModel`: passed as soon as it existed.
* `TestDecodePickerDoc`: passed as soon as it existed.
* `TestLive_ModelPickerSkipsDeprecated`: passed as soon as it existed.
* `TestLive_ModelPickerKilo`: passed as soon as it existed.

**Gate** (tests applied): all passed — per-file `golint` on 10 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gw5-accepts-deprecated` | `TestFirstActiveModel` | killed | firstActiveModel(opencode-go, [old live1]) = "old", true; want "live1", true |
| `gw5-last-not-first` | `TestFirstActiveModel` | killed | firstActiveModel(opencode-go, [gone live2 live1]) = "live1", true; want "live2", true |
| `gw5-any-provider` | `TestFirstActiveModel` | killed | firstActiveModel(opencode-go, [vendor/model]) = "vendor/model", true; want "", false |
| `gw5-drops-kilo-section` | `TestDecodePickerDoc` | killed | decodePickerDoc = map[opencode-go:map[go-model:deprecated] opencode-zen:map[zen-model:beta]], want map[kilo:map[vendor/kilo-model:] opencode-go:map[go-model:deprecated] opencode-zen:map[zen-model:beta]] |
| `gw5-live-accepts-deprecated` | `TestLive_ModelPickerSkipsDeprecated` | killed | picked glm-5 from [glm-5 glm-5.3-flash] |
| `gw5-live-drops-kilo-section` | `TestLive_ModelPickerKilo` | killed | picked "" (false) from 0 kilo models, want deepseek/deepseek-v4.1-flash |

### A.L Live runs on this plan's final tree (`-tags live_gateways`, 2026-09-27)

| Test | Result |
|---|---|
| `TestLive_OpencodeChatCompletions` | PASS |
| `TestLive_OpencodeResponses` | PASS |
| `TestLive_OpencodeRouteStillEnforced` | PASS |
| `TestLive_KiloChatCompletions` | PASS |
| `TestLive_KiloToolCall` | PASS |
| `TestLive_KiloReasoningSpelling` | PASS |
| `TestLive_KiloSupportedParameters` | PASS |
| `TestLive_OpencodeKeyHeaderPerRoute` | PASS |
| `TestLive_OpencodeKeyHeaderPerRoute/messages` | PASS |
| `TestLive_OpencodeKeyHeaderPerRoute/google` | PASS |
| `TestLive_OpencodeChatReasoningEffort` | PASS |
| `TestLive_OpencodeChatReasoningEffort/glm-5.3-flash` | PASS |
| `TestLive_OpencodeChatReasoningEffort/hy3` | PASS |
| `TestLive_KiloReasoningShapes` | PASS |
| `TestLive_KiloReasoningShapes/enabled` | PASS |
| `TestLive_KiloReasoningShapes/effort_low` | PASS |
| `TestLive_ToolRoundTrip` | PASS |
| `TestLive_ToolRoundTrip/claude` | PASS |
| `TestLive_ToolRoundTrip/gemini` | PASS |
| `TestLive_ToolRoundTrip/grok` | PASS |
| `TestLive_ToolRoundTrip/kilo` | PASS |
| `TestLive_ToolRoundTrip/go-chat` | PASS |
| `TestLive_ToolRoundTrip/go-messages` | PASS |
| `TestLive_ToolRoundTrip/go-responses` | PASS |
| `TestLive_KiloDataCollectionDenied` | PASS |
| `TestLive_ModelPickerSkipsDeprecated` | PASS |
| `TestLive_ModelPickerKilo` | PASS |
| `TestLive_OpencodeResponsesStoreFalse` | PASS |
| `TestLive_OpencodeMinimaxM3Adaptive` | PASS |
| `TestLive_OpencodeRoutesFromMetadata` | PASS |
| `TestLive_InterleavedReasoningReplay` | PASS |
| `TestLive_SystemMessage` | PASS |
| `TestLive_SystemMessage/claude` | PASS |
| `TestLive_SystemMessage/go-messages` | PASS |
| `TestLive_OpencodeMessagesThinking` | PASS |


Totals: 35 passed, 0 skipped, 0 failed.


## Appendix B — Diffs

Generated from the proof. Apply each phase's **Tests** diff, then its **Fix**
diff, with `git apply`, in order, on the §0.1 baseline.

### B.G1 Phase G1 — OpenCode routes from the model metadata (§3.1, G-O)

**Tests** (`gw1-tests.diff`, 361 lines):

```diff
diff --git a/llmprovider/live_opencode_route_test.go b/llmprovider/live_opencode_route_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_opencode_route_test.go
@@ -0,0 +1,49 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"net/http"
+	"strings"
+	"sync"
+	"testing"
+)
+
+// TestLive_OpencodeRoutesFromMetadata: on OpenCode Go, qwen3.8-max follows its
+// metadata (no provider.npm, so chat_completions) where the 2026-08-28 table
+// said messages. Gate G-O (2026-09-27) measured both routes answering for every
+// Go qwen model, so the metadata route is safe to follow.
+func TestLive_OpencodeRoutesFromMetadata(t *testing.T) {
+	t.Setenv(envDisableModelMetadata, "0") // the real models.opencode.ai document decides
+	var (
+		mu    sync.Mutex
+		paths []string
+	)
+	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		if strings.Contains(r.URL.Host, "opencode.ai") && r.Method == http.MethodPost {
+			mu.Lock()
+			paths = append(paths, r.URL.Path)
+			mu.Unlock()
+		}
+		return http.DefaultTransport.RoundTrip(r)
+	})}
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "qwen3.8-max", WithHTTPClient(client))
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("Generate: %v", err)
+	}
+	mu.Lock()
+	defer mu.Unlock()
+	if len(paths) != 1 || !strings.HasSuffix(paths[0], "/chat/completions") {
+		t.Fatalf("request paths = %v, want one /chat/completions", paths)
+	}
+	if strings.TrimSpace(out) == "" {
+		t.Error("empty output")
+	}
+}
diff --git a/llmprovider/opencode_metadata_route_test.go b/llmprovider/opencode_metadata_route_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/opencode_metadata_route_test.go
@@ -0,0 +1,181 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"net/http"
+	"net/http/httptest"
+	"os"
+	"strings"
+	"sync"
+	"testing"
+)
+
+// routeServer serves a metadata document listing model under gateway's
+// section, with provider.npm set when npm is not empty (listed=false serves a
+// document without the model), and answers every generation route with "ok",
+// recording the request paths.
+func routeServer(t *testing.T, gateway, model, npm string, listed bool) (*httptest.Server, func() []string) {
+	t.Helper()
+	t.Setenv(envDisableModelMetadata, "0") // TestMain turns the fetch off
+	var (
+		mu    sync.Mutex
+		paths []string
+	)
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if strings.HasSuffix(r.URL.Path, "/api.json") {
+			entry := `"other-model":{"id":"other-model"}`
+			if listed {
+				entry = `"` + model + `":{"id":"` + model + `"`
+				if npm != "" {
+					entry += `,"provider":{"npm":"` + npm + `"}`
+				}
+				entry += "}"
+			}
+			_, _ = w.Write([]byte(`{"` + modelMetadataKey(gateway) + `":{"models":{` + entry + `}}}`))
+			return
+		}
+		mu.Lock()
+		paths = append(paths, r.URL.Path)
+		mu.Unlock()
+		switch {
+		case strings.HasSuffix(r.URL.Path, "/messages"):
+			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
+		case strings.HasSuffix(r.URL.Path, "/responses"):
+			_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
+		case strings.HasSuffix(r.URL.Path, ":generateContent"):
+			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
+		default:
+			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
+		}
+	}))
+	t.Cleanup(srv.Close)
+	return srv, func() []string {
+		mu.Lock()
+		defer mu.Unlock()
+		return append([]string(nil), paths...)
+	}
+}
+
+// generatePath runs one Generate and returns the single request path it sent.
+func generatePath(t *testing.T, srv *httptest.Server, paths func() []string, gateway, model string, opts ...ProviderOption) string {
+	t.Helper()
+	opts = append([]ProviderOption{WithBaseURL(srv.URL), WithModelMetadataURL(srv.URL + "/api.json")}, opts...)
+	p, err := NewOpencode(gateway, "k", model, opts...)
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	if _, err := p.Generate(context.Background(), "hi"); err != nil {
+		t.Fatalf("Generate: %v", err)
+	}
+	got := paths()
+	if len(got) != 1 {
+		t.Fatalf("requests = %v, want exactly one", got)
+	}
+	return got[0]
+}
+
+// TestOpencode_RoutesFromMetadata: the metadata's provider.npm picks the
+// route, as OpenCode's client does (provider.ts:1274-1278), for a model the
+// table does not know and for one it routes differently.
+func TestOpencode_RoutesFromMetadata(t *testing.T) {
+	tests := []struct {
+		name, gateway, model, npm, want string
+	}{
+		{"anthropic", ProviderOpencodeGo, "brand-new-model", "@ai-sdk/anthropic", "/messages"},
+		{"openai", ProviderOpencodeGo, "brand-new-model", "@ai-sdk/openai", "/responses"},
+		{"google", ProviderOpencodeZen, "brand-new-model", "@ai-sdk/google", "/models/brand-new-model:generateContent"},
+		{"compatible", ProviderOpencodeGo, "brand-new-model", "@ai-sdk/openai-compatible", "/chat/completions"},
+		{"unset", ProviderOpencodeGo, "brand-new-model", "", "/chat/completions"},
+		// The table says messages; the metadata wins.
+		{"over table", ProviderOpencodeGo, "minimax-m3", "@ai-sdk/openai-compatible", "/chat/completions"},
+	}
+	for _, tc := range tests {
+		t.Run(tc.name, func(t *testing.T) {
+			srv, paths := routeServer(t, tc.gateway, tc.model, tc.npm, true)
+			if got := generatePath(t, srv, paths, tc.gateway, tc.model); got != tc.want {
+				t.Errorf("path = %q, want %q", got, tc.want)
+			}
+		})
+	}
+}
+
+// TestOpencode_RouteOverrideBeatsMetadata: WithOpencodeRoute wins over the
+// metadata.
+func TestOpencode_RouteOverrideBeatsMetadata(t *testing.T) {
+	srv, paths := routeServer(t, ProviderOpencodeGo, "brand-new-model", "@ai-sdk/anthropic", true)
+	got := generatePath(t, srv, paths, ProviderOpencodeGo, "brand-new-model",
+		WithOpencodeRoute(OpencodeRouteChatCompletions))
+	if got != "/chat/completions" {
+		t.Errorf("path = %q, want the override's /chat/completions", got)
+	}
+}
+
+// TestOpencode_RouteWithoutMetadataUsesTable: metadata that is disabled, or
+// that does not list the model, leaves the table's route.
+func TestOpencode_RouteWithoutMetadataUsesTable(t *testing.T) {
+	t.Run("absent from document", func(t *testing.T) {
+		srv, paths := routeServer(t, ProviderOpencodeGo, "minimax-m3", "", false)
+		if got := generatePath(t, srv, paths, ProviderOpencodeGo, "minimax-m3"); got != "/messages" {
+			t.Errorf("path = %q, want the table's /messages", got)
+		}
+	})
+	t.Run("disabled", func(t *testing.T) {
+		srv, paths := routeServer(t, ProviderOpencodeGo, "minimax-m3", "@ai-sdk/openai-compatible", true)
+		t.Setenv(envDisableModelMetadata, "1")
+		if got := generatePath(t, srv, paths, ProviderOpencodeGo, "minimax-m3"); got != "/messages" {
+			t.Errorf("path = %q, want the table's /messages", got)
+		}
+	})
+}
+
+// TestOpencodeRouteTable_MatchesMetadataSnapshot: every active Zen and Go
+// model in testdata/opencode-routes.json (provider.npm per model, from
+// models.opencode.ai/api.json on 2026-09-27, deprecated models omitted)
+// resolves without metadata to the route its npm selects. It pins the table's
+// refresh; the oracle below is written independently of the production map.
+func TestOpencodeRouteTable_MatchesMetadataSnapshot(t *testing.T) {
+	raw, err := os.ReadFile("testdata/opencode-routes.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	var snapshot map[string]map[string]string
+	if err := json.Unmarshal(raw, &snapshot); err != nil {
+		t.Fatal(err)
+	}
+	oracle := map[string]OpencodeRoute{
+		"@ai-sdk/openai":    OpencodeRouteResponses,
+		"@ai-sdk/anthropic": OpencodeRouteMessages,
+		"@ai-sdk/google":    OpencodeRouteGoogle,
+	}
+	gateways := map[string]string{"opencode": ProviderOpencodeZen, "opencode-go": ProviderOpencodeGo}
+	for section, models := range snapshot {
+		gateway, ok := gateways[section]
+		if !ok || len(models) == 0 {
+			t.Fatalf("snapshot section %q: unknown or empty", section)
+		}
+		for model, npm := range models {
+			want, ok := oracle[npm]
+			if !ok {
+				want = OpencodeRouteChatCompletions
+			}
+			got, err := resolveOpencodeRoute(gateway, model, "")
+			if err != nil || got != want {
+				t.Errorf("%s %s (npm %q): route = %q, %v; want %q", gateway, model, npm, got, err, want)
+			}
+		}
+	}
+}
+
+// TestIsUsableOpencodeModel_DeniesSystemone: jev-* models use OpenCode's
+// "systemone" route, which has no encoder here (MADR 0012 §3.1).
+func TestIsUsableOpencodeModel_DeniesSystemone(t *testing.T) {
+	for _, id := range []string{"jev-1.13", "jev-1.13-free", "JEV-2"} {
+		if isUsableOpencodeModel(id) {
+			t.Errorf("isUsableOpencodeModel(%q) = true, want false", id)
+		}
+	}
+	if !isUsableOpencodeModel("kimi-k2.6") {
+		t.Error("isUsableOpencodeModel(kimi-k2.6) = false, want true")
+	}
+}
diff --git a/llmprovider/testdata/opencode-routes.json b/llmprovider/testdata/opencode-routes.json
new file mode 100644
--- /dev/null
+++ b/llmprovider/testdata/opencode-routes.json
@@ -0,0 +1,116 @@
+{
+ "opencode": {
+  "big-pickle": "",
+  "claude-fable-5": "@ai-sdk/anthropic",
+  "claude-fable-5-1": "@ai-sdk/anthropic",
+  "claude-haiku-4-5": "@ai-sdk/anthropic",
+  "claude-opus-4-5": "@ai-sdk/anthropic",
+  "claude-opus-4-6": "@ai-sdk/anthropic",
+  "claude-opus-4-7": "@ai-sdk/anthropic",
+  "claude-opus-4-8": "@ai-sdk/anthropic",
+  "claude-opus-5": "@ai-sdk/anthropic",
+  "claude-opus-5-5": "@ai-sdk/anthropic",
+  "claude-sonnet-4": "@ai-sdk/anthropic",
+  "claude-sonnet-4-5": "@ai-sdk/anthropic",
+  "claude-sonnet-4-6": "@ai-sdk/anthropic",
+  "claude-sonnet-5": "@ai-sdk/anthropic",
+  "deepseek-v4-flash": "",
+  "deepseek-v4-flash-vision-exp": "",
+  "deepseek-v4-pro": "",
+  "deepseek-v4.1-flash": "",
+  "gemini-3-flash": "@ai-sdk/google",
+  "gemini-3.1-pro": "@ai-sdk/google",
+  "gemini-3.5-flash": "@ai-sdk/google",
+  "gemini-3.5-flash-lite": "@ai-sdk/google",
+  "gemini-3.6-flash": "@ai-sdk/google",
+  "gemini-3.7-flash": "@ai-sdk/google",
+  "gemini-3.8-flash": "@ai-sdk/google",
+  "glm-5": "",
+  "glm-5.1": "",
+  "glm-5.2": "",
+  "glm-5.3": "",
+  "glm-5.3-flash": "",
+  "gpt-5": "@ai-sdk/openai",
+  "gpt-5-codex": "@ai-sdk/openai",
+  "gpt-5-nano": "@ai-sdk/openai",
+  "gpt-5.1": "@ai-sdk/openai",
+  "gpt-5.1-codex": "@ai-sdk/openai",
+  "gpt-5.1-codex-max": "@ai-sdk/openai",
+  "gpt-5.1-codex-mini": "@ai-sdk/openai",
+  "gpt-5.2": "@ai-sdk/openai",
+  "gpt-5.2-codex": "@ai-sdk/openai",
+  "gpt-5.3-codex": "@ai-sdk/openai",
+  "gpt-5.3-codex-spark": "@ai-sdk/openai",
+  "gpt-5.4": "@ai-sdk/openai",
+  "gpt-5.4-mini": "@ai-sdk/openai",
+  "gpt-5.4-nano": "@ai-sdk/openai",
+  "gpt-5.4-pro": "@ai-sdk/openai",
+  "gpt-5.5": "@ai-sdk/openai",
+  "gpt-5.5-pro": "@ai-sdk/openai",
+  "gpt-5.6-luna": "@ai-sdk/openai",
+  "gpt-5.6-sol": "@ai-sdk/openai",
+  "gpt-5.6-terra": "@ai-sdk/openai",
+  "gpt-6-astra": "@ai-sdk/openai",
+  "gpt-6-luna": "@ai-sdk/openai",
+  "gpt-6-sol": "@ai-sdk/openai",
+  "grok-4.5": "@ai-sdk/openai",
+  "grok-4.6": "@ai-sdk/openai",
+  "grok-4.7": "@ai-sdk/openai",
+  "grok-build-0.1": "@ai-sdk/openai",
+  "kimi-k2.5": "",
+  "kimi-k2.6": "",
+  "kimi-k2.7-code": "",
+  "kimi-k3": "",
+  "ling-3.0-flash-fin-free": "",
+  "longcat-2.5-preview-free": "",
+  "mimo-v2.6-flash-free": "",
+  "minimax-m2.5": "",
+  "minimax-m2.7": "",
+  "minimax-m3": "",
+  "muse-spark-1.2": "@ai-sdk/openai",
+  "muse-spark-1.3": "@ai-sdk/openai",
+  "muse-spark-1.3-contributor-free": "@ai-sdk/openai",
+  "nemotron-3-ultra-free": "",
+  "nemotron-3.5-lightning-free": "",
+  "qwen3.5-plus": "@ai-sdk/anthropic",
+  "qwen3.6-plus": "@ai-sdk/anthropic",
+  "qwen3.8-flash": "@ai-sdk/anthropic",
+  "qwen3.8-max": "",
+  "space-bunny-free": ""
+ },
+ "opencode-go": {
+  "deepseek-v4-flash": "",
+  "deepseek-v4-flash-vision-exp": "",
+  "deepseek-v4-pro": "",
+  "deepseek-v4.1-flash": "",
+  "glm-5.1": "",
+  "glm-5.2": "",
+  "glm-5.3": "",
+  "glm-5.3-flash": "",
+  "gpt-5.6-luna": "@ai-sdk/openai",
+  "gpt-6-luna": "@ai-sdk/openai",
+  "grok-4.6": "@ai-sdk/openai",
+  "grok-4.7": "@ai-sdk/openai",
+  "hy3": "",
+  "hy4-preview": "",
+  "kimi-k2.6": "",
+  "kimi-k2.7-code": "",
+  "kimi-k3": "",
+  "longcat-2.0": "",
+  "longcat-2.5-preview-free": "",
+  "mimo-v2.5": "",
+  "mimo-v2.5-pro": "",
+  "mimo-v2.6-flash": "",
+  "mimo-v2.6-pro": "",
+  "minimax-m2.7": "@ai-sdk/anthropic",
+  "minimax-m3": "@ai-sdk/anthropic",
+  "muse-spark-1.2-contributor": "@ai-sdk/openai",
+  "muse-spark-1.3-contributor": "@ai-sdk/openai",
+  "qwen3.6-plus": "",
+  "qwen3.7-max": "",
+  "qwen3.7-plus": "",
+  "qwen3.8-flash": "@ai-sdk/anthropic",
+  "qwen3.8-max": "",
+  "space-bunny-free": ""
+ }
+}
```

**Fix** (`gw1-fix.diff`, 440 lines):

```diff
diff --git a/llmprovider/discovery_ranking_test.go b/llmprovider/discovery_ranking_test.go
--- a/llmprovider/discovery_ranking_test.go
+++ b/llmprovider/discovery_ranking_test.go
@@ -283,7 +283,8 @@
 				"google/gemini-3.6-flash", "meta/muse-spark-1.2", "thinkingmachines/inkling"},
 			[]string{"openai/gpt-6-astra", "anthropic/claude-fable-5.1", "openai/gpt-5.6-sol",
 				"deepseek/deepseek-v4.1-flash", "google/gemini-3.8-flash", "x-ai/grok-4.6"}},
-		{ProviderOpencodeZen, "zen.json", 80,
+		// 78: the listing's jev-1.13 and jev-1.13-free are not usable (MADR 0012 §3.1).
+		{ProviderOpencodeZen, "zen.json", 78,
 			[]string{opencodeDeepSeekV41Flash, "qwen3.8-flash", "glm-5.3-flash", opencodeDeepSeekV4Flash,
 				"gemini-3.5-flash-lite", "gemini-3.8-flash"},
 			[]string{"claude-opus-5-5", "gpt-6-sol", "gpt-6-luna", "grok-4.7", "gpt-6-astra", "muse-spark-1.3"}},
diff --git a/llmprovider/model_metadata.go b/llmprovider/model_metadata.go
--- a/llmprovider/model_metadata.go
+++ b/llmprovider/model_metadata.go
@@ -57,6 +57,12 @@
 	// Interleaved is {"field": name} when the provider expects prior reasoning
 	// replayed on assistant messages under that field, or true/absent.
 	Interleaved json.RawMessage `json:"interleaved"`
+	// Provider.NPM names the AI SDK package OpenCode's client uses for the
+	// model, which fixes its route on the Zen and Go gateways (MADR 0012
+	// §3.1). Unset means the section's openai-compatible package.
+	Provider struct {
+		NPM string `json:"npm"`
+	} `json:"provider"`
 }
 
 // modelReasoningOption is one models.dev reasoning_options entry.
@@ -95,6 +101,16 @@
 		return ""
 	}
 	return declared.Field
+}
+
+// opencodeRoute returns the route a Zen or Go model's provider.npm selects,
+// and false when the document does not list the model (MADR 0012 §3.1).
+func (d modelMetadataDoc) opencodeRoute(gateway, model string) (OpencodeRoute, bool) {
+	m, ok := d[modelMetadataKey(gateway)][model]
+	if !ok {
+		return "", false
+	}
+	return opencodeRouteForNPM(m.Provider.NPM), true
 }
 
 // modelMetadataKey returns the document key for a provider, or "".
diff --git a/llmprovider/models_catalog.go b/llmprovider/models_catalog.go
--- a/llmprovider/models_catalog.go
+++ b/llmprovider/models_catalog.go
@@ -203,7 +203,9 @@
 // deliberately aggregate many vendors under bare IDs.
 func isUsableOpencodeModel(id string) bool {
 	sm := strings.ToLower(strings.TrimSpace(id))
-	if sm == "" {
+	// jev-* models use OpenCode's "systemone" route, which this package has
+	// no encoder for (MADR 0012 §3.1).
+	if sm == "" || strings.HasPrefix(sm, "jev-") {
 		return false
 	}
 	for _, deny := range opencodeDenySubstrings {
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -14,10 +14,11 @@
 // OpencodeProvider implements Provider against the OpenCode Zen and OpenCode Go
 // AI gateways. Both are multi-protocol: the gateway dispatches each model to one
 // of four upstream wire formats and does NOT normalize them, so this provider
-// selects the request shape, path and decoder per model via resolveOpencodeRoute.
+// selects the request shape, path and decoder per request via requestRoute:
+// the model's provider.npm in the model metadata, else the built-in table.
 // Sending a model to the wrong route fails with an opaque HTTP 500, which
 // classifies as the retryable ErrProviderUnavailable — use WithOpencodeRoute to
-// override the table when the gateway adds a model.
+// pin a route.
 //
 // OpencodeProvider does NOT implement Continuer. The gateway rejects
 // previous_response_id with HTTP 400 "referenced response not found or expired"
@@ -37,6 +38,8 @@
 	modelProfile    ModelProfile
 	// identity names the client on every request (MADR 0012 §1.4).
 	identity clientIdentity
+	// routePinned is set by WithOpencodeRoute: route is used as given.
+	routePinned bool
 }
 
 // opencodeSessionHeader carries a stable conversation id. OpenCode Go rejects
@@ -50,7 +53,8 @@
 
 // NewOpencode creates an OpenCode gateway provider. gateway must be
 // ProviderOpencodeZen or ProviderOpencodeGo. The wire format is resolved once,
-// here, so a misroute is a construction-time fact rather than a per-call surprise.
+// here from WithOpencodeRoute or the table; each request then prefers the
+// model metadata's route unless WithOpencodeRoute pinned one (MADR 0012 §3.1).
 // An empty apiKey uses the gateway's public token.
 func NewOpencode(gateway, apiKey, model string, opts ...ProviderOption) (*OpencodeProvider, error) {
 	defaultBase, err := opencodeBaseURL(gateway)
@@ -79,6 +83,7 @@
 		thinkingBudget:  cfg.ThinkingBudget,
 		reasoningEffort: cfg.ReasoningEffort,
 		route:           route,
+		routePinned:     cfg.OpencodeRoute != "",
 		metadataURL:     cfg.ModelMetadataURL,
 		modelProfile:    cfg.ModelProfile,
 		identity:        identityOf(cfg),
@@ -88,7 +93,10 @@
 // Name returns the gateway identifier this provider was constructed for.
 func (p *OpencodeProvider) Name() string { return p.gateway }
 
-// Route reports the wire format resolved for this provider's model.
+// Route reports the wire format resolved at construction: the
+// WithOpencodeRoute override, else the built-in table, else the prefix
+// heuristic. Without an override, a request uses the model's provider.npm
+// route from the model metadata when that is available (MADR 0012 §3.1).
 func (p *OpencodeProvider) Route() OpencodeRoute { return p.route }
 
 // Generate sends a prompt to the gateway and returns the generated text.
@@ -289,9 +297,29 @@
 	return false
 }
 
+// requestRoute returns one request's wire format: the pinned route, else the
+// model's provider.npm route in the metadata, else the construction-time
+// route (MADR 0012 §3.1). The lookup waits at most metadataLookupTimeout.
+func (p *OpencodeProvider) requestRoute(ctx context.Context) OpencodeRoute {
+	if p.routePinned {
+		return p.route
+	}
+	ctx, cancel := context.WithTimeout(ctx, metadataLookupTimeout)
+	defer cancel()
+	doc, err := loadModelMetadata(ctx, ProviderConfig{HTTPClient: p.client, ModelMetadataURL: p.metadataURL})
+	if err != nil {
+		return p.route
+	}
+	if r, ok := doc.opencodeRoute(p.gateway, p.model); ok {
+		return r
+	}
+	return p.route
+}
+
 func (p *OpencodeProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
+	route := p.requestRoute(ctx)
 	var body map[string]any
-	switch p.route {
+	switch route {
 	case OpencodeRouteResponses:
 		body = p.responsesBody(input, tool, thinking)
 	case OpencodeRouteMessages:
@@ -309,7 +337,7 @@
 		return nil, fmt.Errorf("opencode: marshal request: %w", err)
 	}
 
-	url := p.baseURL + p.route.path(p.model)
+	url := p.baseURL + route.path(p.model)
 	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
 	if err != nil {
 		return nil, err
@@ -318,7 +346,7 @@
 	p.identity.setUserAgent(req)
 	// Each route reads the key from its vendor's header (MADR 0009 §1c); the
 	// key stays in a header, never the URL.
-	name, value := opencodeKeyHeader(p.route, p.apiKey)
+	name, value := opencodeKeyHeader(route, p.apiKey)
 	req.Header.Set(name, value)
 	// x-opencode-session is fixed for the provider's lifetime (MADR 0012 §1.4).
 	req.Header.Set(opencodeSessionHeader, p.identity.session)
@@ -333,11 +361,11 @@
 	// response bodies are also bounded.
 	limitedBody := io.LimitReader(resp.Body, 1<<20)
 
-	if err := classifyHTTPError(p.gateway+"/"+string(p.route), resp); err != nil {
+	if err := classifyHTTPError(p.gateway+"/"+string(route), resp); err != nil {
 		return nil, err
 	}
 
-	switch p.route {
+	switch route {
 	case OpencodeRouteResponses:
 		return decodeResponsesAPIOutput(limitedBody)
 	case OpencodeRouteMessages:
diff --git a/llmprovider/opencode_route.go b/llmprovider/opencode_route.go
--- a/llmprovider/opencode_route.go
+++ b/llmprovider/opencode_route.go
@@ -23,7 +23,7 @@
 // nothing warns when it goes stale — unlike a local engine, which can report a
 // version on boot (see magic-cli-remote/internal/provider/opencode/version.go).
 // Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
-const wireShapesProbedOnOpencode = "2026-08-28"
+const wireShapesProbedOnOpencode = "2026-09-27"
 
 // OpencodeRoute identifies which wire format the OpenCode gateway expects for a
 // given model. OpenCode Zen and Go are multi-protocol gateways: they do not
@@ -79,94 +79,144 @@
 	}
 }
 
-// opencodeRouteTable maps gateway -> model id -> wire format, transcribed from the
-// published endpoint tables at https://opencode.ai/docs/zen/ and
-// https://opencode.ai/docs/go/ (retrieved 2026-08-28).
+// opencodeRouteTable maps gateway -> model id -> wire format. It is the
+// fallback for a request whose metadata is unavailable (MADR 0012 §3.1), and
+// it is generated from the same document: every model in the "opencode" and
+// "opencode-go" sections of models.opencode.ai/api.json (retrieved
+// 2026-09-27) whose status is not "deprecated", routed by its provider.npm
+// (opencodeRouteForNPM). TestOpencodeRouteTable_MatchesMetadataSnapshot pins
+// it to testdata/opencode-routes.json.
 //
 // Routing is per-gateway, not per-model: the minimax-* family takes
-// chat_completions on Zen and messages on Go. Reconcile this table against both
-// docs pages when either gateway announces new models.
+// chat_completions on Zen and messages on Go.
 //
 //nolint:goconst // model IDs are intentionally repeated across gateways and tests
 var opencodeRouteTable = map[string]map[string]OpencodeRoute{
 	ProviderOpencodeZen: {
 		// responses (@ai-sdk/openai)
-		"gpt-5.6-sol": OpencodeRouteResponses, "gpt-5.6-terra": OpencodeRouteResponses,
-		"gpt-5.6-luna": OpencodeRouteResponses, "gpt-5.5": OpencodeRouteResponses,
-		"gpt-5.5-pro": OpencodeRouteResponses, "gpt-5.4": OpencodeRouteResponses,
-		"gpt-5.4-pro": OpencodeRouteResponses, "gpt-5.4-mini": OpencodeRouteResponses,
-		"gpt-5.4-nano": OpencodeRouteResponses, "gpt-5.3-codex": OpencodeRouteResponses,
-		"gpt-5.3-codex-spark": OpencodeRouteResponses, "gpt-5.2": OpencodeRouteResponses,
-		"gpt-5.2-codex": OpencodeRouteResponses, "gpt-5.1": OpencodeRouteResponses,
-		"gpt-5.1-codex": OpencodeRouteResponses, "gpt-5.1-codex-max": OpencodeRouteResponses,
-		"gpt-5.1-codex-mini": OpencodeRouteResponses, "gpt-5": OpencodeRouteResponses,
-		"gpt-5-codex": OpencodeRouteResponses, "gpt-5-nano": OpencodeRouteResponses,
-		"grok-4.6": OpencodeRouteResponses, "grok-4.5": OpencodeRouteResponses,
-		"grok-build-0.1": OpencodeRouteResponses, "muse-spark-1.2": OpencodeRouteResponses,
-		"muse-spark-1.2-contributor-free": OpencodeRouteResponses,
+		"gpt-5":                           OpencodeRouteResponses,
+		"gpt-5-codex":                     OpencodeRouteResponses,
+		"gpt-5-nano":                      OpencodeRouteResponses,
+		"gpt-5.1":                         OpencodeRouteResponses,
+		"gpt-5.1-codex":                   OpencodeRouteResponses,
+		"gpt-5.1-codex-max":               OpencodeRouteResponses,
+		"gpt-5.1-codex-mini":              OpencodeRouteResponses,
+		"gpt-5.2":                         OpencodeRouteResponses,
+		"gpt-5.2-codex":                   OpencodeRouteResponses,
+		"gpt-5.3-codex":                   OpencodeRouteResponses,
+		"gpt-5.3-codex-spark":             OpencodeRouteResponses,
+		"gpt-5.4":                         OpencodeRouteResponses,
+		"gpt-5.4-mini":                    OpencodeRouteResponses,
+		"gpt-5.4-nano":                    OpencodeRouteResponses,
+		"gpt-5.4-pro":                     OpencodeRouteResponses,
+		"gpt-5.5":                         OpencodeRouteResponses,
+		"gpt-5.5-pro":                     OpencodeRouteResponses,
+		"gpt-5.6-luna":                    OpencodeRouteResponses,
+		"gpt-5.6-sol":                     OpencodeRouteResponses,
+		"gpt-5.6-terra":                   OpencodeRouteResponses,
+		"gpt-6-astra":                     OpencodeRouteResponses,
+		"gpt-6-luna":                      OpencodeRouteResponses,
+		"gpt-6-sol":                       OpencodeRouteResponses,
+		"grok-4.5":                        OpencodeRouteResponses,
+		"grok-4.6":                        OpencodeRouteResponses,
+		"grok-4.7":                        OpencodeRouteResponses,
+		"grok-build-0.1":                  OpencodeRouteResponses,
+		"muse-spark-1.2":                  OpencodeRouteResponses,
+		"muse-spark-1.3":                  OpencodeRouteResponses,
+		"muse-spark-1.3-contributor-free": OpencodeRouteResponses,
 
 		// messages (@ai-sdk/anthropic)
-		"claude-fable-5": OpencodeRouteMessages, "claude-opus-5": OpencodeRouteMessages,
-		"claude-opus-4-8": OpencodeRouteMessages, "claude-opus-4-7": OpencodeRouteMessages,
-		"claude-opus-4-6": OpencodeRouteMessages, "claude-opus-4-5": OpencodeRouteMessages,
-		"claude-sonnet-5": OpencodeRouteMessages, "claude-sonnet-4-6": OpencodeRouteMessages,
-		"claude-sonnet-4-5": OpencodeRouteMessages, "claude-haiku-4-5": OpencodeRouteMessages,
-		"qwen3.7-max": OpencodeRouteMessages, "qwen3.7-plus": OpencodeRouteMessages,
-		"qwen3.6-plus": OpencodeRouteMessages, "qwen3.5-plus": OpencodeRouteMessages,
+		"claude-fable-5":    OpencodeRouteMessages,
+		"claude-fable-5-1":  OpencodeRouteMessages,
+		"claude-haiku-4-5":  OpencodeRouteMessages,
+		"claude-opus-4-5":   OpencodeRouteMessages,
+		"claude-opus-4-6":   OpencodeRouteMessages,
+		"claude-opus-4-7":   OpencodeRouteMessages,
+		"claude-opus-4-8":   OpencodeRouteMessages,
+		"claude-opus-5":     OpencodeRouteMessages,
+		"claude-opus-5-5":   OpencodeRouteMessages,
+		"claude-sonnet-4":   OpencodeRouteMessages,
+		"claude-sonnet-4-5": OpencodeRouteMessages,
+		"claude-sonnet-4-6": OpencodeRouteMessages,
+		"claude-sonnet-5":   OpencodeRouteMessages,
+		"qwen3.5-plus":      OpencodeRouteMessages,
+		"qwen3.6-plus":      OpencodeRouteMessages,
+		"qwen3.8-flash":     OpencodeRouteMessages,
 
 		// google (@ai-sdk/google)
-		"gemini-3.7-flash": OpencodeRouteGoogle, "gemini-3.6-flash": OpencodeRouteGoogle,
-		"gemini-3.5-flash": OpencodeRouteGoogle, "gemini-3.5-flash-lite": OpencodeRouteGoogle,
-		"gemini-3.1-pro": OpencodeRouteGoogle, "gemini-3-flash": OpencodeRouteGoogle,
-
-		// chat_completions (@ai-sdk/openai-compatible)
-		"deepseek-v4-pro":   OpencodeRouteChatCompletions,
-		"deepseek-v4-flash": OpencodeRouteChatCompletions,
-		"minimax-m3":        OpencodeRouteChatCompletions,
-		"minimax-m2.7":      OpencodeRouteChatCompletions,
-		"minimax-m2.5":      OpencodeRouteChatCompletions,
-		"glm-5.2":           OpencodeRouteChatCompletions,
-		"glm-5.1":           OpencodeRouteChatCompletions,
-		"glm-5":             OpencodeRouteChatCompletions,
-		"kimi-k2.5":         OpencodeRouteChatCompletions,
-		"kimi-k2.6":         OpencodeRouteChatCompletions,
-		"kimi-k2.7-code":    OpencodeRouteChatCompletions,
-		"kimi-k3":           OpencodeRouteChatCompletions,
-		"big-pickle":        OpencodeRouteChatCompletions,
-		"mimo-v2.5-free":    OpencodeRouteChatCompletions,
-		"hy3-free":          OpencodeRouteChatCompletions,
-
-		"ling-3.0-flash-fin-free":     OpencodeRouteChatCompletions,
-		"nemotron-3-ultra-free":       OpencodeRouteChatCompletions,
-		"nemotron-3.5-lightning-free": OpencodeRouteChatCompletions,
+		"gemini-3-flash":        OpencodeRouteGoogle,
+		"gemini-3.1-pro":        OpencodeRouteGoogle,
+		"gemini-3.5-flash":      OpencodeRouteGoogle,
+		"gemini-3.5-flash-lite": OpencodeRouteGoogle,
+		"gemini-3.6-flash":      OpencodeRouteGoogle,
+		"gemini-3.7-flash":      OpencodeRouteGoogle,
+		"gemini-3.8-flash":      OpencodeRouteGoogle,
+
+		// chat_completions (any other npm, or unset)
+		"big-pickle":                   OpencodeRouteChatCompletions,
+		"deepseek-v4-flash":            OpencodeRouteChatCompletions,
+		"deepseek-v4-flash-vision-exp": OpencodeRouteChatCompletions,
+		"deepseek-v4-pro":              OpencodeRouteChatCompletions,
+		"deepseek-v4.1-flash":          OpencodeRouteChatCompletions,
+		"glm-5":                        OpencodeRouteChatCompletions,
+		"glm-5.1":                      OpencodeRouteChatCompletions,
+		"glm-5.2":                      OpencodeRouteChatCompletions,
+		"glm-5.3":                      OpencodeRouteChatCompletions,
+		"glm-5.3-flash":                OpencodeRouteChatCompletions,
+		"kimi-k2.5":                    OpencodeRouteChatCompletions,
+		"kimi-k2.6":                    OpencodeRouteChatCompletions,
+		"kimi-k2.7-code":               OpencodeRouteChatCompletions,
+		"kimi-k3":                      OpencodeRouteChatCompletions,
+		"ling-3.0-flash-fin-free":      OpencodeRouteChatCompletions,
+		"longcat-2.5-preview-free":     OpencodeRouteChatCompletions,
+		"mimo-v2.6-flash-free":         OpencodeRouteChatCompletions,
+		"minimax-m2.5":                 OpencodeRouteChatCompletions,
+		"minimax-m2.7":                 OpencodeRouteChatCompletions,
+		"minimax-m3":                   OpencodeRouteChatCompletions,
+		"nemotron-3-ultra-free":        OpencodeRouteChatCompletions,
+		"nemotron-3.5-lightning-free":  OpencodeRouteChatCompletions,
+		"qwen3.8-max":                  OpencodeRouteChatCompletions,
+		"space-bunny-free":             OpencodeRouteChatCompletions,
 	},
 	ProviderOpencodeGo: {
 		// responses (@ai-sdk/openai)
-		"grok-4.6": OpencodeRouteResponses, "gpt-5.6-luna": OpencodeRouteResponses,
+		"gpt-5.6-luna":               OpencodeRouteResponses,
+		"gpt-6-luna":                 OpencodeRouteResponses,
+		"grok-4.6":                   OpencodeRouteResponses,
+		"grok-4.7":                   OpencodeRouteResponses,
 		"muse-spark-1.2-contributor": OpencodeRouteResponses,
-
-		// messages (@ai-sdk/anthropic) — note minimax-* differs from Zen
-		"minimax-m3": OpencodeRouteMessages, "minimax-m2.7": OpencodeRouteMessages,
-		"minimax-m2.5": OpencodeRouteMessages, "qwen3.8-max": OpencodeRouteMessages,
-		"qwen3.8-flash": OpencodeRouteMessages, "qwen3.7-max": OpencodeRouteMessages,
-		"qwen3.7-plus": OpencodeRouteMessages, "qwen3.6-plus": OpencodeRouteMessages,
-
-		// chat_completions (@ai-sdk/openai-compatible)
-		"glm-5.3-flash":                OpencodeRouteChatCompletions,
-		"glm-5.3":                      OpencodeRouteChatCompletions,
-		"glm-5.2":                      OpencodeRouteChatCompletions,
-		"glm-5.1":                      OpencodeRouteChatCompletions,
-		"kimi-k3":                      OpencodeRouteChatCompletions,
-		"kimi-k2.7-code":               OpencodeRouteChatCompletions,
-		"kimi-k2.6":                    OpencodeRouteChatCompletions,
-		"longcat-2.0":                  OpencodeRouteChatCompletions,
-		"deepseek-v4-pro":              OpencodeRouteChatCompletions,
+		"muse-spark-1.3-contributor": OpencodeRouteResponses,
+
+		// messages (@ai-sdk/anthropic)
+		"minimax-m2.7":  OpencodeRouteMessages,
+		"minimax-m3":    OpencodeRouteMessages,
+		"qwen3.8-flash": OpencodeRouteMessages,
+
+		// chat_completions (any other npm, or unset)
 		"deepseek-v4-flash":            OpencodeRouteChatCompletions,
 		"deepseek-v4-flash-vision-exp": OpencodeRouteChatCompletions,
+		"deepseek-v4-pro":              OpencodeRouteChatCompletions,
+		"deepseek-v4.1-flash":          OpencodeRouteChatCompletions,
+		"glm-5.1":                      OpencodeRouteChatCompletions,
+		"glm-5.2":                      OpencodeRouteChatCompletions,
+		"glm-5.3":                      OpencodeRouteChatCompletions,
+		"glm-5.3-flash":                OpencodeRouteChatCompletions,
+		"hy3":                          OpencodeRouteChatCompletions,
+		"hy4-preview":                  OpencodeRouteChatCompletions,
+		"kimi-k2.6":                    OpencodeRouteChatCompletions,
+		"kimi-k2.7-code":               OpencodeRouteChatCompletions,
+		"kimi-k3":                      OpencodeRouteChatCompletions,
+		"longcat-2.0":                  OpencodeRouteChatCompletions,
+		"longcat-2.5-preview-free":     OpencodeRouteChatCompletions,
 		"mimo-v2.5":                    OpencodeRouteChatCompletions,
 		"mimo-v2.5-pro":                OpencodeRouteChatCompletions,
-		"hy4-preview":                  OpencodeRouteChatCompletions,
-		"hy3":                          OpencodeRouteChatCompletions,
+		"mimo-v2.6-flash":              OpencodeRouteChatCompletions,
+		"mimo-v2.6-pro":                OpencodeRouteChatCompletions,
+		"qwen3.6-plus":                 OpencodeRouteChatCompletions,
+		"qwen3.7-max":                  OpencodeRouteChatCompletions,
+		"qwen3.7-plus":                 OpencodeRouteChatCompletions,
+		"qwen3.8-max":                  OpencodeRouteChatCompletions,
+		"space-bunny-free":             OpencodeRouteChatCompletions,
 	},
 }
 
@@ -198,6 +248,22 @@
 	}
 
 	return OpencodeRouteChatCompletions
+}
+
+// opencodeRouteForNPM maps a model's provider.npm to its route, as OpenCode's
+// client picks an AI SDK package (provider.ts:1274-1278); any other package,
+// or none, is the openai-compatible chat route.
+func opencodeRouteForNPM(npm string) OpencodeRoute {
+	switch npm {
+	case "@ai-sdk/openai":
+		return OpencodeRouteResponses
+	case "@ai-sdk/anthropic":
+		return OpencodeRouteMessages
+	case "@ai-sdk/google":
+		return OpencodeRouteGoogle
+	default:
+		return OpencodeRouteChatCompletions
+	}
 }
 
 // resolveOpencodeRoute picks the wire format for (gateway, model), honouring an
diff --git a/llmprovider/opencode_route_test.go b/llmprovider/opencode_route_test.go
--- a/llmprovider/opencode_route_test.go
+++ b/llmprovider/opencode_route_test.go
@@ -56,7 +56,7 @@
 		model   string
 		want    OpencodeRoute
 	}{
-		{ProviderOpencodeZen, "claude-sonnet-4", OpencodeRouteMessages},
+		{ProviderOpencodeZen, "claude-sonnet-9", OpencodeRouteMessages},
 		{ProviderOpencodeZen, "deepseek-v4-flash-free", OpencodeRouteChatCompletions},
 		{ProviderOpencodeZen, "laguna-s-2.1-free", OpencodeRouteChatCompletions},
 		{ProviderOpencodeGo, "kimi-k2.5", OpencodeRouteChatCompletions},
```

### B.G2 Phase G2 — OpenCode Responses store and MiniMax-M3 thinking (§3.2)

**Tests** (`gw2-tests.diff`, 168 lines):

```diff
diff --git a/llmprovider/live_opencode_conventions_test.go b/llmprovider/live_opencode_conventions_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_opencode_conventions_test.go
@@ -0,0 +1,83 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"bytes"
+	"io"
+	"net/http"
+	"strings"
+	"testing"
+)
+
+// recordingClient records the last POST body sent to an opencode.ai host.
+func recordingClient(sent *[]byte) *http.Client {
+	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		if strings.Contains(r.URL.Host, "opencode.ai") && r.Method == http.MethodPost && r.Body != nil {
+			b, err := io.ReadAll(r.Body)
+			if err != nil {
+				return nil, err
+			}
+			*sent = b
+			r.Body = io.NopCloser(bytes.NewReader(b))
+		}
+		return http.DefaultTransport.RoundTrip(r)
+	})}
+}
+
+// TestLive_OpencodeResponsesStoreFalse: OpenCode Go's Responses route accepts
+// store:false (measured 2026-09-27 on gpt-6-luna, with and without it).
+func TestLive_OpencodeResponsesStoreFalse(t *testing.T) {
+	var sent []byte
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "gpt-6-luna", WithHTTPClient(recordingClient(&sent)))
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("Generate: %v", err)
+	}
+	if !bytes.Contains(sent, []byte(`"store":false`)) {
+		t.Fatalf("request did not send store:false: %s", sent)
+	}
+	if strings.TrimSpace(out) == "" {
+		t.Error("empty output")
+	}
+}
+
+// TestLive_OpencodeMinimaxM3Adaptive: minimax-m3 on OpenCode Go's Messages
+// route thinks under thinking.type "adaptive" (measured 2026-09-27: budget,
+// adaptive and none are all accepted, so this proves acceptance and that
+// reasoning is returned, not necessity).
+func TestLive_OpencodeMinimaxM3Adaptive(t *testing.T) {
+	var sent []byte
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "minimax-m3", WithHTTPClient(recordingClient(&sent)))
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	res, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: "What is 17 * 23? Reply with the number."})
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("GenerateItemsThinking: %v", err)
+	}
+	if !bytes.Contains(sent, []byte(`"thinking":{"type":"adaptive"}`)) {
+		t.Fatalf("request did not think adaptively: %s", sent)
+	}
+	var reasoned bool
+	for _, it := range res.Output {
+		if r, ok := it.(ReasoningItem); ok && strings.TrimSpace(r.Text) != "" {
+			reasoned = true
+		}
+	}
+	if !reasoned {
+		t.Errorf("no ReasoningItem in %#v", res.Output)
+	}
+	if !strings.Contains(res.OutputText(), "391") {
+		t.Errorf("reply %q, want 391", res.OutputText())
+	}
+}
diff --git a/llmprovider/opencode_conventions_test.go b/llmprovider/opencode_conventions_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/opencode_conventions_test.go
@@ -0,0 +1,75 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"net/http"
+	"net/http/httptest"
+	"strings"
+	"testing"
+)
+
+// conventionsBody runs one call against a stub gateway and returns the
+// request body it sent.
+func conventionsBody(t *testing.T, model string, thinking bool, opts ...ProviderOption) map[string]any {
+	t.Helper()
+	var body map[string]any
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
+			t.Errorf("decode: %v", err)
+		}
+		switch {
+		case strings.HasSuffix(r.URL.Path, "/messages"):
+			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
+		default:
+			_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
+		}
+	}))
+	t.Cleanup(srv.Close)
+	p, err := NewOpencode(ProviderOpencodeGo, "k", model, append([]ProviderOption{WithBaseURL(srv.URL)}, opts...)...)
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	if thinking {
+		_, err = p.GenerateThinking(context.Background(), "hi")
+	} else {
+		_, err = p.Generate(context.Background(), "hi")
+	}
+	if err != nil {
+		t.Fatalf("generate: %v", err)
+	}
+	return body
+}
+
+// TestOpencodeResponses_SendsStoreFalse: OpenCode's client sets store false
+// for every @ai-sdk/openai model (transform.ts:1235-1243).
+func TestOpencodeResponses_SendsStoreFalse(t *testing.T) {
+	body := conventionsBody(t, "gpt-6-luna", false)
+	if v, ok := body["store"]; !ok || v != false {
+		t.Fatalf("store = %v (present %t), want false", v, ok)
+	}
+}
+
+// TestOpencodeMessages_MinimaxM3ThinksAdaptive: MiniMax's Anthropic interface
+// takes thinking.type "adaptive" (transform.ts:1293-1296), with no budget and
+// no effort, whatever effort is configured.
+func TestOpencodeMessages_MinimaxM3ThinksAdaptive(t *testing.T) {
+	body := conventionsBody(t, "minimax-m3", true, WithReasoningEffort(effortHigh))
+	thinking, _ := body[jsonKeyThinking].(map[string]any)
+	if len(thinking) != 1 || thinking[jsonKeyType] != "adaptive" {
+		t.Fatalf("thinking = %v, want exactly {type: adaptive}", body[jsonKeyThinking])
+	}
+	if _, ok := body["output_config"]; ok {
+		t.Errorf("output_config = %v, want absent", body["output_config"])
+	}
+}
+
+// TestOpencodeMessages_OtherModelsKeepBudget: a non-Claude, non-MiniMax-M3
+// model on the Messages route keeps the enabled budget.
+func TestOpencodeMessages_OtherModelsKeepBudget(t *testing.T) {
+	body := conventionsBody(t, "qwen3.8-flash", true)
+	thinking, _ := body[jsonKeyThinking].(map[string]any)
+	if thinking[jsonKeyType] != jsonKeyEnabled || thinking["budget_tokens"] == nil {
+		t.Fatalf("thinking = %v, want an enabled budget", body[jsonKeyThinking])
+	}
+}
```

**Fix** (`gw2-fix.diff`, 41 lines):

```diff
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -161,6 +161,9 @@
 		jsonKeyModel:           p.model,
 		jsonKeyInput:           itemsToInput(input),
 		jsonKeyMaxOutputTokens: p.maxTokens,
+		// OpenCode's client stores nothing for @ai-sdk/openai models
+		// (transform.ts:1235-1243, MADR 0012 §3.2); items are replayed.
+		"store": false,
 	}
 	if tool != nil {
 		body[jsonKeyTools] = []map[string]any{{
diff --git a/llmprovider/thinking_wire.go b/llmprovider/thinking_wire.go
--- a/llmprovider/thinking_wire.go
+++ b/llmprovider/thinking_wire.go
@@ -62,6 +62,13 @@
 	return major > 4 || (major == 4 && minor >= 7)
 }
 
+// minimaxAdaptiveThinking reports a MiniMax-M3 id. MiniMax's Anthropic
+// interface takes thinking.type "adaptive" with no budget or effort, as
+// OpenCode's client sends it (transform.ts:1293-1296, MADR 0012 §3.2).
+func minimaxAdaptiveThinking(model string) bool {
+	return strings.Contains(strings.ToLower(model), "minimax-m3")
+}
+
 // addMessagesThinking adds the thinking fields of an Anthropic Messages
 // request to body and returns its max_tokens. Adaptive-only models get
 // thinking.type "adaptive", plus output_config.effort when an effort is set; a
@@ -70,6 +77,10 @@
 // defaultClaudeThinkingBudget, with max_tokens raised above it when needed
 // (Anthropic requires max_tokens > budget_tokens).
 func addMessagesThinking(body map[string]any, model, effort string, budget, maxTokens int) int {
+	if minimaxAdaptiveThinking(model) {
+		body[jsonKeyThinking] = map[string]any{jsonKeyType: "adaptive"}
+		return maxTokens
+	}
 	if claudeAdaptiveOnly(model) {
 		fields := map[string]any{jsonKeyThinking: map[string]any{jsonKeyType: "adaptive"}}
 		if effort != "" {
```

### B.G3 Phase G3 — Kilo data collection (§3.3, G-K)

**Tests** (`gw3-tests.diff`, 108 lines):

```diff
diff --git a/llmprovider/kilo_data_collection_test.go b/llmprovider/kilo_data_collection_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/kilo_data_collection_test.go
@@ -0,0 +1,70 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"io"
+	"net/http"
+	"net/http/httptest"
+	"strings"
+	"testing"
+)
+
+// kiloBody runs one Generate against a stub gateway and returns the request
+// body it sent.
+func kiloBody(t *testing.T, opts ...ProviderOption) map[string]any {
+	t.Helper()
+	var body map[string]any
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
+			t.Errorf("decode: %v", err)
+		}
+		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
+	}))
+	t.Cleanup(srv.Close)
+	p, err := NewKilo("k", "deepseek/deepseek-v4.1-flash", append([]ProviderOption{WithBaseURL(srv.URL)}, opts...)...)
+	if err != nil {
+		t.Fatalf("NewKilo: %v", err)
+	}
+	if _, err := p.Generate(context.Background(), "hi"); err != nil {
+		t.Fatalf("Generate: %v", err)
+	}
+	return body
+}
+
+// TestKilo_DeniesDataCollectionByDefault: Kilo's opt-out from upstreams that
+// train on prompts is sent by default, matching the listing's policy.
+func TestKilo_DeniesDataCollectionByDefault(t *testing.T) {
+	provider, _ := kiloBody(t)["provider"].(map[string]any)
+	if provider["data_collection"] != "deny" {
+		t.Fatalf("provider = %v, want data_collection deny", provider)
+	}
+}
+
+// kiloDataCollectionRequired is gate G-K's response to deny on a model that
+// requires collection (kilo-auto/free, 2026-09-27).
+const kiloDataCollectionRequired = `{"error":"Data collection is required for this model. Please enable data collection to use this model or choose another model.","error_type":"data_collection_required","message":"Data collection is required for this model. Please enable data collection to use this model or choose another model."}`
+
+// TestClassify_KiloDataCollectionRequired: the refusal is a terminal
+// ErrNotPermitted carrying Kilo's error_type and message.
+func TestClassify_KiloDataCollectionRequired(t *testing.T) {
+	err := classifyHTTPError(ProviderKilo, &http.Response{
+		StatusCode: http.StatusBadRequest,
+		Header:     http.Header{},
+		Body:       io.NopCloser(strings.NewReader(kiloDataCollectionRequired)),
+	})
+	var apiErr *APIError
+	if !errors.As(err, &apiErr) {
+		t.Fatalf("err = %T %v, want *APIError", err, err)
+	}
+	if !errors.Is(err, ErrNotPermitted) || !apiErr.Terminal {
+		t.Errorf("err = %v (terminal %t), want terminal ErrNotPermitted", err, apiErr.Terminal)
+	}
+	if apiErr.Type != "data_collection_required" || !strings.HasPrefix(apiErr.Message, "Data collection is required") {
+		t.Errorf("Type = %q, Message = %q", apiErr.Type, apiErr.Message)
+	}
+	if !errors.Is(err, ErrInvalidRequest) {
+		t.Errorf("err = %v, want it to still match the 400 status sentinel ErrInvalidRequest", err)
+	}
+}
diff --git a/llmprovider/live_kilo_data_collection_test.go b/llmprovider/live_kilo_data_collection_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_kilo_data_collection_test.go
@@ -0,0 +1,28 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"errors"
+	"testing"
+)
+
+// TestLive_KiloDataCollectionDenied is gate G-K's assertion: with the default
+// deny, a free model that requires data collection (kilo-auto/free) is refused
+// with a terminal ErrNotPermitted carrying data_collection_required.
+func TestLive_KiloDataCollectionDenied(t *testing.T) {
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewKilo(kiloKey(t), "kilo-auto/free")
+	if err != nil {
+		t.Fatalf("NewKilo: %v", err)
+	}
+	_, err = p.Generate(ctx, "Reply with only the word ALPHA")
+	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProviderUnavailable) {
+		t.Skipf("transient: %v", err)
+	}
+	var apiErr *APIError
+	if !errors.Is(err, ErrNotPermitted) || !errors.As(err, &apiErr) || apiErr.Type != "data_collection_required" {
+		t.Fatalf("err = %v, want ErrNotPermitted with type data_collection_required", err)
+	}
+}
```

**Fix** (`gw3-fix.diff`, 150 lines):

```diff
diff --git a/llmprovider/api_error.go b/llmprovider/api_error.go
--- a/llmprovider/api_error.go
+++ b/llmprovider/api_error.go
@@ -159,7 +159,7 @@
 		return true, ErrQuotaExhausted
 	case service == serviceOpencode && has(opencodeForbiddenTypes...),
 		service == ProviderOpenAI && has("usage_not_included"),
-		service == ProviderKilo && status == http.StatusForbidden:
+		service == ProviderKilo && (status == http.StatusForbidden || has("data_collection_required")):
 		return true, ErrNotPermitted
 	case service == serviceOpencode && has("ModelError"),
 		service == ProviderKilo && has("PAID_MODEL_AUTH_REQUIRED"):
@@ -179,7 +179,8 @@
 
 // apiErrorEnvelope is the union of the error bodies MADR 0012 §1.1 lists:
 // OpenCode and Claude {type:"error",error:{type,message}}, Kilo
-// {error:{code,message}} or {code}, OpenAI/Codex {error:{type,code,message}},
+// {error:{code,message}}, {code} or {error,error_type,message},
+// OpenAI/Codex {error:{type,code,message}},
 // xAI nested or flat {code,error}, Gemini {error:{code,message,status}}.
 type apiErrorEnvelope struct {
 	types []string // candidate classifications, most specific first
@@ -199,10 +200,11 @@
 
 func parseAPIErrorBody(body []byte) apiErrorEnvelope {
 	var top struct {
-		Type    string          `json:"type"`
-		Code    json.RawMessage `json:"code"`
-		Message string          `json:"message"`
-		Error   json.RawMessage `json:"error"`
+		Type      string          `json:"type"`
+		Code      json.RawMessage `json:"code"`
+		ErrorType string          `json:"error_type"`
+		Message   string          `json:"message"`
+		Error     json.RawMessage `json:"error"`
 	}
 	if json.Unmarshal(body, &top) != nil {
 		return apiErrorEnvelope{msg: strings.TrimSpace(string(body))}
@@ -229,7 +231,7 @@
 	case json.Unmarshal(top.Error, &text) == nil:
 		env.msg = text
 	}
-	add(jsonString(top.Code), top.Type)
+	add(top.ErrorType, jsonString(top.Code), top.Type)
 	if env.msg == "" {
 		env.msg = top.Message
 	}
diff --git a/llmprovider/kilo.go b/llmprovider/kilo.go
--- a/llmprovider/kilo.go
+++ b/llmprovider/kilo.go
@@ -49,6 +49,8 @@
 	// nil means "unknown" — send the standard request rather than guessing a
 	// model lacks a capability.
 	caps map[string]struct{}
+	// allowDataCollection omits the data_collection "deny" preference.
+	allowDataCollection bool
 	// identity names the client on every request (MADR 0012 §1.4).
 	identity clientIdentity
 }
@@ -85,6 +87,8 @@
 		reasoningEffort: cfg.ReasoningEffort,
 		modelProfile:    cfg.ModelProfile,
 		caps:            caps,
+
+		allowDataCollection: cfg.KiloDataCollection,
 	}, nil
 }
 
@@ -191,6 +195,11 @@
 		ReasoningEffort: effort,
 		Reasoning:       reasoning,
 	})
+	if !p.allowDataCollection {
+		// Kilo's opt-out from upstreams that train on prompts, which its client
+		// sends with hide_prompt_training_models (MADR 0012 §3.3).
+		body["provider"] = map[string]any{"data_collection": "deny"}
+	}
 
 	reqBody, err := json.Marshal(body)
 	if err != nil {
diff --git a/llmprovider/kilo_data_collection_allow_test.go b/llmprovider/kilo_data_collection_allow_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/kilo_data_collection_allow_test.go
@@ -0,0 +1,11 @@
+package llmprovider
+
+import "testing"
+
+// TestKilo_DataCollectionAllowed: WithKiloDataCollection(true) sends no
+// provider preference, as Kilo's client does without its privacy setting.
+func TestKilo_DataCollectionAllowed(t *testing.T) {
+	if provider, ok := kiloBody(t, WithKiloDataCollection(true))["provider"]; ok {
+		t.Fatalf("provider = %v, want absent", provider)
+	}
+}
diff --git a/llmprovider/live_gateways_test.go b/llmprovider/live_gateways_test.go
--- a/llmprovider/live_gateways_test.go
+++ b/llmprovider/live_gateways_test.go
@@ -189,7 +189,7 @@
 func TestLive_KiloChatCompletions(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewKilo(kiloKey(t), "kilo-auto/free")
+	p, err := NewKilo(kiloKey(t), "kilo-auto/free", WithKiloDataCollection(true))
 	if err != nil {
 		t.Fatalf("NewKilo: %v", err)
 	}
@@ -209,7 +209,7 @@
 func TestLive_KiloToolCall(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewKilo(kiloKey(t), "kilo-auto/free", WithMaxTokens(400))
+	p, err := NewKilo(kiloKey(t), "kilo-auto/free", WithMaxTokens(400), WithKiloDataCollection(true))
 	if err != nil {
 		t.Fatalf("NewKilo: %v", err)
 	}
diff --git a/llmprovider/options.go b/llmprovider/options.go
--- a/llmprovider/options.go
+++ b/llmprovider/options.go
@@ -50,6 +50,10 @@
 	// accepts (its supported_parameters). Empty means "unknown — send
 	// everything". Ignored by all other providers.
 	KiloCapabilities []string
+	// KiloDataCollection allows Kilo upstreams that may train on prompts.
+	// false (the default) sends provider.data_collection "deny"; see
+	// WithKiloDataCollection. Ignored by all other providers.
+	KiloDataCollection bool
 	// ModelProfile selects how the recommended models of the open catalogs
 	// (Kilo, OpenCode Zen and Go, Hugging Face) are ranked. The zero value is
 	// ProfileUtility. Those providers' DiscoverModels ranks with it too.
@@ -134,6 +138,18 @@
 	}
 }
 
+// WithKiloDataCollection lets Kilo route to upstreams that may train on
+// prompts when allow is true. By default every Kilo request sends
+// provider.data_collection "deny", matching the listing's exclusion of such
+// models (MADR 0012 §3.3). A model that requires collection is then refused
+// with ErrNotPermitted: kilo-auto/free was, and on 2026-09-27 every free text
+// model in Kilo's listing was flagged mayTrainOnYourPrompts.
+func WithKiloDataCollection(allow bool) ProviderOption {
+	return func(cfg *ProviderConfig) {
+		cfg.KiloDataCollection = allow
+	}
+}
+
 // WithModelProfile selects how ListAvailableModels, ListModelCatalog and the
 // open catalogs' DiscoverModels rank the recommended models (MADR 0010 §1,
 // MADR 0013 A4).
```

### B.G4 Phase G4 — Kilo URL-prefixed tokens and organizations (§3.3, K4)

**Tests** (`gw4-tests.diff`, 106 lines):

```diff
diff --git a/llmprovider/kilo_endpoints_test.go b/llmprovider/kilo_endpoints_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/kilo_endpoints_test.go
@@ -0,0 +1,101 @@
+package llmprovider
+
+import (
+	"context"
+	"io"
+	"net/http"
+	"slices"
+	"strings"
+	"sync"
+	"testing"
+)
+
+// kiloCall is one request a Kilo provider made.
+type kiloCall struct {
+	method, url, auth, org string
+}
+
+// absentHeader marks a header the request did not carry.
+const absentHeader = "<absent>"
+
+// kiloCalls runs one Generate and one DiscoverModels through a recording
+// transport that answers without a network, and returns the requests made.
+func kiloCalls(t *testing.T, token string, opts ...ProviderOption) []kiloCall {
+	t.Helper()
+	var (
+		mu    sync.Mutex
+		calls []kiloCall
+	)
+	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		org := absentHeader
+		if v, ok := r.Header[http.CanonicalHeaderKey("X-KILOCODE-ORGANIZATIONID")]; ok {
+			org = strings.Join(v, ",")
+		}
+		mu.Lock()
+		calls = append(calls, kiloCall{r.Method, r.URL.String(), r.Header.Get("Authorization"), org})
+		mu.Unlock()
+		body := `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`
+		if r.Method == http.MethodGet {
+			body = `{"data":[{"id":"deepseek/deepseek-v4.1-flash","architecture":{"input_modalities":["text"],` +
+				`"output_modalities":["text"]},"supported_parameters":["tools"],"pricing":{"completion":"0.1"}}]}`
+		}
+		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
+			Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
+	})}
+	p, err := NewKilo(token, "deepseek/deepseek-v4.1-flash", append([]ProviderOption{WithHTTPClient(client)}, opts...)...)
+	if err != nil {
+		t.Fatalf("NewKilo: %v", err)
+	}
+	if _, err := p.Generate(context.Background(), "hi"); err != nil {
+		t.Fatalf("Generate: %v", err)
+	}
+	if _, err := p.DiscoverModels(context.Background()); err != nil {
+		t.Fatalf("DiscoverModels: %v", err)
+	}
+	mu.Lock()
+	defer mu.Unlock()
+	return slices.Clone(calls)
+}
+
+func wantKiloCalls(t *testing.T, got []kiloCall, want ...kiloCall) {
+	t.Helper()
+	if !slices.Equal(got, want) {
+		t.Fatalf("requests:\n got  %+v\n want %+v", got, want)
+	}
+}
+
+// TestKilo_URLTokenSelectsBase: a token "{url}:{secret}" sends generation and
+// the listing to {origin}{prefix}/api/gateway (url.ts route()), with the whole
+// token as the bearer.
+func TestKilo_URLTokenSelectsBase(t *testing.T) {
+	for _, tc := range []struct{ token, base string }{
+		{"https://kilo.example.test/tenant:secret", "https://kilo.example.test/tenant/api/gateway"},
+		{"http://127.0.0.1:8080:secret", "http://127.0.0.1:8080/api/gateway"},
+		{"https://kilo.example.test/tenant/api/openrouter/:secret", "https://kilo.example.test/tenant/api/gateway"},
+	} {
+		t.Run(tc.token, func(t *testing.T) {
+			wantKiloCalls(t, kiloCalls(t, tc.token),
+				kiloCall{http.MethodPost, tc.base + "/chat/completions", "Bearer " + tc.token, absentHeader},
+				kiloCall{http.MethodGet, tc.base + "/models", "Bearer " + tc.token, absentHeader})
+		})
+	}
+}
+
+// TestKilo_URLTokenOrganization: an /api/organizations/{id} token path scopes
+// both requests to that organization and lists the organization's models
+// (models.ts:219-229).
+func TestKilo_URLTokenOrganization(t *testing.T) {
+	const token = "https://kilo.example.test/api/organizations/org-9:secret"
+	wantKiloCalls(t, kiloCalls(t, token),
+		kiloCall{http.MethodPost, "https://kilo.example.test/api/gateway/chat/completions", "Bearer " + token, "org-9"},
+		kiloCall{http.MethodGet, "https://kilo.example.test/api/organizations/org-9/models", "Bearer " + token, "org-9"})
+}
+
+// TestKilo_PlainTokenUsesDefaults: an ordinary key keeps the default gateway
+// and sends no organization, even when a URL appears after its start.
+func TestKilo_PlainTokenUsesDefaults(t *testing.T) {
+	const token = "sk-plain:https://elsewhere.test/p:q"
+	wantKiloCalls(t, kiloCalls(t, token),
+		kiloCall{http.MethodPost, kiloBaseURL + "/chat/completions", "Bearer " + token, absentHeader},
+		kiloCall{http.MethodGet, kiloBaseURL + "/models", "Bearer " + token, absentHeader})
+}
```

**Fix** (`gw4-fix.diff`, 239 lines):

```diff
diff --git a/llmprovider/discovery.go b/llmprovider/discovery.go
--- a/llmprovider/discovery.go
+++ b/llmprovider/discovery.go
@@ -826,20 +826,21 @@
 	return v
 }
 
-// fetchKiloCatalog performs the shared GET {base}/models. The endpoint is PUBLIC
-// (200 with no credential, verified 2026-08-29), so apiKey may be empty.
+// fetchKiloCatalog performs the shared GET {base}/models, or an organization's
+// listing (resolveKiloEndpoints). The public endpoint answers with no
+// credential (verified 2026-08-29), so apiKey may be empty.
 func fetchKiloCatalog(ctx context.Context, apiKey string, cfg ProviderConfig) ([]kiloCatalogEntry, error) {
-	baseURL := kiloBaseURL
-	if cfg.BaseURL != "" {
-		baseURL = strings.TrimRight(cfg.BaseURL, "/")
-	}
-	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
+	endpoints := resolveKiloEndpoints(cfg.BaseURL, apiKey, cfg.KiloOrganization)
+	req, err := http.NewRequestWithContext(ctx, "GET", endpoints.models, http.NoBody)
 	if err != nil {
 		return nil, err
 	}
 	identityOf(cfg).setUserAgent(req)
 	if apiKey != "" {
 		req.Header.Set("Authorization", "Bearer "+apiKey)
+	}
+	if endpoints.org != "" {
+		req.Header.Set(kiloOrganizationHeader, endpoints.org)
 	}
 	resp, err := cfg.HTTPClient.Do(req)
 	if err != nil {
diff --git a/llmprovider/kilo.go b/llmprovider/kilo.go
--- a/llmprovider/kilo.go
+++ b/llmprovider/kilo.go
@@ -7,6 +7,8 @@
 	"fmt"
 	"io"
 	"net/http"
+	"net/url"
+	"regexp"
 	"strings"
 )
 
@@ -51,8 +53,83 @@
 	caps map[string]struct{}
 	// allowDataCollection omits the data_collection "deny" preference.
 	allowDataCollection bool
+	// org is the organization every request is scoped to, or "".
+	org string
 	// identity names the client on every request (MADR 0012 §1.4).
 	identity clientIdentity
+}
+
+// kiloTokenURLRE matches Kilo's URL-prefixed token, "{backend URL}:{secret}"
+// (kilocode auth/token.ts:9). The whole token is still the bearer.
+var kiloTokenURLRE = regexp.MustCompile(`^(https?://[^:]+(?::\d+)?(?:/[^:]*)?):`)
+
+// kiloOrganizationHeader scopes a request to a Kilo organization.
+const kiloOrganizationHeader = "X-KILOCODE-ORGANIZATIONID"
+
+// kiloEndpoints are the URLs and organization one Kilo credential uses
+// (MADR 0012 §3.3).
+type kiloEndpoints struct {
+	gateway string // generation base, {origin}{prefix}/api/gateway by default
+	models  string // listing URL
+	org     string // organization id, or ""
+}
+
+// resolveKiloEndpoints derives the endpoints from the configured base (else
+// kiloBaseURL), the token and an explicit organization. A URL-prefixed token
+// replaces the base with its origin and path prefix, as Kilo's client does
+// (api/url.ts:9-30), and an /api/organizations/{id} path in it names the
+// organization when none is given. An organization lists
+// {origin}{prefix}/api/organizations/{id}/models (api/models.ts:219).
+func resolveKiloEndpoints(baseURL, token, org string) kiloEndpoints {
+	base := kiloBaseURL
+	if baseURL != "" {
+		base = strings.TrimRight(baseURL, "/")
+	}
+	if m := kiloTokenURLRE.FindStringSubmatch(token); m != nil {
+		if u, err := url.Parse(m[1]); err == nil && u.Host != "" {
+			base = kiloRoute(u, "gateway")
+			if org == "" {
+				org = kiloPathOrganization(u)
+			}
+		}
+	}
+	e := kiloEndpoints{gateway: base, models: base + "/models", org: org}
+	if u, err := url.Parse(base); err == nil && org != "" {
+		e.models = kiloRoute(u, "organizations/"+url.PathEscape(org)) + "/models"
+	}
+	return e
+}
+
+// kiloPathSegments splits a URL path into its non-empty segments and returns
+// them with the index of the last "api" segment, or -1.
+func kiloPathSegments(u *url.URL) (parts []string, api int) {
+	parts = strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
+	for api = len(parts) - 1; api >= 0; api-- {
+		if parts[api] == "api" {
+			break
+		}
+	}
+	return parts, api
+}
+
+// kiloRoute is Kilo's route() (api/url.ts:9-18): the path before the last
+// "api" segment, then /api/{name}, without query, fragment or trailing slash.
+func kiloRoute(u *url.URL, name string) string {
+	parts, api := kiloPathSegments(u)
+	if api >= 0 {
+		parts = parts[:api]
+	}
+	return u.Scheme + "://" + u.Host + "/" + strings.Join(append(parts, "api", name), "/")
+}
+
+// kiloPathOrganization returns {id} from a path ending .../api/organizations/{id},
+// or "".
+func kiloPathOrganization(u *url.URL) string {
+	parts, api := kiloPathSegments(u)
+	if api >= 0 && len(parts) == api+3 && parts[api+1] == "organizations" {
+		return parts[api+2]
+	}
+	return ""
 }
 
 // kiloAnonymousToken is the bearer Kilo's own client sends without a login;
@@ -60,16 +137,14 @@
 const kiloAnonymousToken = "anonymous"
 
 // NewKilo creates a Kilo Gateway provider. An empty apiKey uses Kilo's
-// anonymous token.
+// anonymous token. A URL-prefixed token ("https://host/prefix:secret") selects
+// that backend (MADR 0012 §3.3).
 func NewKilo(apiKey, model string, opts ...ProviderOption) (*KiloProvider, error) {
 	if apiKey == "" {
 		apiKey = kiloAnonymousToken
 	}
 	cfg := ApplyOptions(opts)
-	baseURL := kiloBaseURL
-	if cfg.BaseURL != "" {
-		baseURL = strings.TrimRight(cfg.BaseURL, "/")
-	}
+	endpoints := resolveKiloEndpoints(cfg.BaseURL, apiKey, cfg.KiloOrganization)
 	var caps map[string]struct{}
 	if len(cfg.KiloCapabilities) > 0 {
 		caps = make(map[string]struct{}, len(cfg.KiloCapabilities))
@@ -80,7 +155,7 @@
 	return &KiloProvider{
 		apiKey:          apiKey,
 		model:           model,
-		baseURL:         baseURL,
+		baseURL:         endpoints.gateway,
 		client:          cfg.HTTPClient,
 		identity:        identityOf(cfg),
 		maxTokens:       cfg.MaxTokens,
@@ -89,6 +164,7 @@
 		caps:            caps,
 
 		allowDataCollection: cfg.KiloDataCollection,
+		org:                 endpoints.org,
 	}, nil
 }
 
@@ -215,6 +291,9 @@
 	req.Header.Set(kiloEditorHeader, p.identity.name)
 	req.Header.Set(kiloTaskHeader, p.identity.session)
 	req.Header.Set("Authorization", "Bearer "+p.apiKey)
+	if p.org != "" {
+		req.Header.Set(kiloOrganizationHeader, p.org)
+	}
 
 	resp, err := p.client.Do(req)
 	if err != nil {
@@ -236,9 +315,10 @@
 // static catalog. It spends no generation on probes (MADR 0012 §1.6).
 func (p *KiloProvider) DiscoverModels(ctx context.Context) ([]string, error) {
 	listed, err := listKiloModels(ctx, p.apiKey, p.identity.apply(ProviderConfig{
-		HTTPClient:   p.client,
-		BaseURL:      p.baseURL,
-		ModelProfile: p.modelProfile,
+		HTTPClient:       p.client,
+		BaseURL:          p.baseURL,
+		ModelProfile:     p.modelProfile,
+		KiloOrganization: p.org,
 	}))
 	if err != nil || len(listed) == 0 {
 		listed = StaticModels(ProviderKilo)
diff --git a/llmprovider/kilo_organization_test.go b/llmprovider/kilo_organization_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/kilo_organization_test.go
@@ -0,0 +1,14 @@
+package llmprovider
+
+import (
+	"net/http"
+	"testing"
+)
+
+// TestKilo_OrganizationOption: WithKiloOrganization scopes generation with
+// X-KILOCODE-ORGANIZATIONID and lists /api/organizations/{id}/models.
+func TestKilo_OrganizationOption(t *testing.T) {
+	wantKiloCalls(t, kiloCalls(t, "sk-plain", WithKiloOrganization("org-1")),
+		kiloCall{http.MethodPost, kiloBaseURL + "/chat/completions", "Bearer sk-plain", "org-1"},
+		kiloCall{http.MethodGet, "https://api.kilo.ai/api/organizations/org-1/models", "Bearer sk-plain", "org-1"})
+}
diff --git a/llmprovider/options.go b/llmprovider/options.go
--- a/llmprovider/options.go
+++ b/llmprovider/options.go
@@ -54,6 +54,9 @@
 	// false (the default) sends provider.data_collection "deny"; see
 	// WithKiloDataCollection. Ignored by all other providers.
 	KiloDataCollection bool
+	// KiloOrganization scopes Kilo requests to an organization; see
+	// WithKiloOrganization. Ignored by all other providers.
+	KiloOrganization string
 	// ModelProfile selects how the recommended models of the open catalogs
 	// (Kilo, OpenCode Zen and Go, Hugging Face) are ranked. The zero value is
 	// ProfileUtility. Those providers' DiscoverModels ranks with it too.
@@ -150,6 +153,17 @@
 	}
 }
 
+// WithKiloOrganization scopes Kilo generation and listing to an organization:
+// requests carry X-KILOCODE-ORGANIZATIONID and the listing is the
+// organization's /api/organizations/{id}/models, as Kilo's client does
+// (MADR 0012 §3.3). A URL-prefixed token whose path is
+// .../api/organizations/{id} names the organization without this option.
+func WithKiloOrganization(id string) ProviderOption {
+	return func(cfg *ProviderConfig) {
+		cfg.KiloOrganization = id
+	}
+}
+
 // WithModelProfile selects how ListAvailableModels, ListModelCatalog and the
 // open catalogs' DiscoverModels rank the recommended models (MADR 0010 §1,
 // MADR 0013 A4).
```

### B.G5 Phase G5 — The live suite picks active models at run time (§3.4)

**Tests** (`gw5-tests.diff`, 430 lines):

```diff
diff --git a/llmprovider/live_gateways_test.go b/llmprovider/live_gateways_test.go
--- a/llmprovider/live_gateways_test.go
+++ b/llmprovider/live_gateways_test.go
@@ -115,7 +115,8 @@
 func TestLive_OpencodeChatCompletions(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "hy3")
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t),
+		liveModel(t, ProviderOpencodeGo, "hy3", "glm-5.3-flash", "kimi-k2.6"))
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
@@ -132,7 +133,8 @@
 func TestLive_OpencodeResponses(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "gpt-6-luna")
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t),
+		liveModel(t, ProviderOpencodeGo, "gpt-6-luna", "grok-4.6"))
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
@@ -159,7 +161,7 @@
 // route table rests on: routes are NOT interchangeable. If this fails, OpenCode
 // has become a translating gateway and the table is no longer necessary.
 func TestLive_OpencodeRouteStillEnforced(t *testing.T) {
-	const model = "gpt-6-luna"
+	model := liveModel(t, ProviderOpencodeGo, "gpt-6-luna", "grok-4.6")
 	ctx, cancel := liveCtx(t)
 	defer cancel()
 
@@ -189,7 +191,7 @@
 func TestLive_KiloChatCompletions(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewKilo(kiloKey(t), "kilo-auto/free", WithKiloDataCollection(true))
+	p, err := NewKilo(kiloKey(t), liveModel(t, ProviderKilo, kiloFreeCollecting...), WithKiloDataCollection(true))
 	if err != nil {
 		t.Fatalf("NewKilo: %v", err)
 	}
@@ -209,7 +211,8 @@
 func TestLive_KiloToolCall(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewKilo(kiloKey(t), "kilo-auto/free", WithMaxTokens(400), WithKiloDataCollection(true))
+	p, err := NewKilo(kiloKey(t), liveModel(t, ProviderKilo, kiloFreeCollecting...), WithMaxTokens(400),
+		WithKiloDataCollection(true))
 	if err != nil {
 		t.Fatalf("NewKilo: %v", err)
 	}
@@ -239,7 +242,7 @@
 func TestLive_KiloReasoningSpelling(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	body := chatCompletionsBody("kilo-auto/free", 400,
+	body := chatCompletionsBody(liveModel(t, ProviderKilo, kiloFreeCollecting...), 400,
 		[]Item{MessageItem{Role: jsonRoleUser, Text: "Say ALPHA only"}}, chatCompletionsOpts{})
 	raw, err := json.Marshal(body)
 	if err != nil {
@@ -490,8 +493,9 @@
 func TestLive_OpencodeChatReasoningEffort(t *testing.T) {
 	key := opencodeKey(t)
 	enableModelMetadata(t)
-	for _, model := range []string{"glm-5.3-flash", "hy3"} {
-		t.Run(model, func(t *testing.T) {
+	for _, candidate := range []string{"glm-5.3-flash", "hy3"} {
+		t.Run(candidate, func(t *testing.T) {
+			model := liveModel(t, ProviderOpencodeGo, candidate)
 			ctx, cancel := liveCtx(t)
 			defer cancel()
 			doc, err := loadModelMetadata(ctx, ApplyOptions(nil))
@@ -534,7 +538,8 @@
 		t.Run(tc.name, func(t *testing.T) {
 			ctx, cancel := liveCtx(t)
 			defer cancel()
-			p, err := NewKilo(key, "deepseek/deepseek-v4.1-flash", WithMaxTokens(400), WithReasoningEffort(tc.effort))
+			p, err := NewKilo(key, liveModel(t, ProviderKilo, kiloNonTraining...), WithMaxTokens(400),
+				WithReasoningEffort(tc.effort))
 			if err != nil {
 				t.Fatalf("NewKilo: %v", err)
 			}
diff --git a/llmprovider/live_items_test.go b/llmprovider/live_items_test.go
--- a/llmprovider/live_items_test.go
+++ b/llmprovider/live_items_test.go
@@ -22,24 +22,32 @@
 	}
 	for _, c := range []struct {
 		name, env string
-		build     func(key string) (ItemProvider, error)
+		build     func(t *testing.T, key string) (ItemProvider, error)
 	}{
-		{"claude", "ANTHROPIC_API_KEY", func(k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
-		{"gemini", "GEMINI_API_KEY", func(k string) (ItemProvider, error) {
+		{"claude", "ANTHROPIC_API_KEY", func(_ *testing.T, k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
+		{"gemini", "GEMINI_API_KEY", func(_ *testing.T, k string) (ItemProvider, error) {
 			return NewGemini(context.Background(), k, "gemini-3.7-flash")
 		}},
-		{"grok", "XAI_API_KEY", func(k string) (ItemProvider, error) { return NewGrok(k, "grok-4.5") }},
-		{"kilo", "KILO_API_KEY", func(k string) (ItemProvider, error) { return NewKilo(k, "deepseek/deepseek-v4.1-flash") }},
-		{"go-chat", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "glm-5.3-flash") }},
-		{"go-messages", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "qwen3.8-flash") }},
-		{"go-responses", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "gpt-6-luna") }},
+		{"grok", "XAI_API_KEY", func(_ *testing.T, k string) (ItemProvider, error) { return NewGrok(k, "grok-4.5") }},
+		{"kilo", "KILO_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
+			return NewKilo(k, liveModel(t, ProviderKilo, kiloNonTraining...))
+		}},
+		{"go-chat", "OPENCODE_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
+			return NewOpencode(ProviderOpencodeGo, k, liveModel(t, ProviderOpencodeGo, "glm-5.3-flash", "glm-5.3", "kimi-k2.6"))
+		}},
+		{"go-messages", "OPENCODE_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
+			return NewOpencode(ProviderOpencodeGo, k, liveModel(t, ProviderOpencodeGo, "qwen3.8-flash", "minimax-m3"))
+		}},
+		{"go-responses", "OPENCODE_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
+			return NewOpencode(ProviderOpencodeGo, k, liveModel(t, ProviderOpencodeGo, "gpt-6-luna", "grok-4.6"))
+		}},
 	} {
 		t.Run(c.name, func(t *testing.T) {
 			key := os.Getenv(c.env)
 			if key == "" {
 				t.Skipf("%s unset", c.env)
 			}
-			p, err := c.build(key)
+			p, err := c.build(t, key)
 			if err != nil {
 				t.Fatalf("construct: %v", err)
 			}
diff --git a/llmprovider/live_kilo_data_collection_test.go b/llmprovider/live_kilo_data_collection_test.go
--- a/llmprovider/live_kilo_data_collection_test.go
+++ b/llmprovider/live_kilo_data_collection_test.go
@@ -13,7 +13,7 @@
 func TestLive_KiloDataCollectionDenied(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewKilo(kiloKey(t), "kilo-auto/free")
+	p, err := NewKilo(kiloKey(t), liveModel(t, ProviderKilo, kiloFreeCollecting...))
 	if err != nil {
 		t.Fatalf("NewKilo: %v", err)
 	}
diff --git a/llmprovider/live_model_picker_test.go b/llmprovider/live_model_picker_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_model_picker_test.go
@@ -0,0 +1,86 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"context"
+	"fmt"
+	"net/http"
+	"sync"
+	"testing"
+	"time"
+)
+
+// Candidate groups that share what the Kilo live tests need.
+var (
+	// kiloFreeCollecting are free models that require data collection
+	// (every free text model did on 2026-09-27, gate G-K).
+	kiloFreeCollecting = []string{"kilo-auto/free", "nvidia/nemotron-3.5-lightning:free", "poolside/laguna-s-2.1:free"}
+	// kiloNonTraining are paid models that do not train on prompts, so they
+	// answer with the default data_collection "deny".
+	kiloNonTraining = []string{"deepseek/deepseek-v4.1-flash", "z-ai/glm-5.3-flash"}
+)
+
+// liveMetadata is models.opencode.ai's document, fetched once per run for the
+// picker. It is read directly, so the providers under test still run without
+// metadata (TestMain turns theirs off).
+var liveMetadata = sync.OnceValues(func() (pickerDoc, error) {
+	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
+	defer cancel()
+	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultModelMetadataURL, http.NoBody)
+	if err != nil {
+		return nil, err
+	}
+	resp, err := http.DefaultClient.Do(req)
+	if err != nil {
+		return nil, err
+	}
+	defer closeResponseBody(resp)
+	if resp.StatusCode != http.StatusOK {
+		return nil, fmt.Errorf("model picker: %s returned HTTP %d", defaultModelMetadataURL, resp.StatusCode)
+	}
+	return decodePickerDoc(resp.Body)
+})
+
+// liveModel picks, at run time, the first candidate the document lists as
+// active for provider: OpenCode Zen, OpenCode Go or Kilo (MADR 0012 §3.4).
+// Candidates share what the test needs (a route, a thinking shape, a
+// data-collection policy). An unreachable document keeps the first
+// candidate; no active candidate skips the test.
+func liveModel(t *testing.T, provider string, candidates ...string) string {
+	t.Helper()
+	doc, err := liveMetadata()
+	if err != nil {
+		t.Logf("model metadata unreachable (%v); using %s", err, candidates[0])
+		return candidates[0]
+	}
+	model, ok := firstActiveModel(doc, provider, candidates)
+	if !ok {
+		t.Skipf("no active %s model among %v (MADR 0012 §3.4)", provider, candidates)
+	}
+	t.Logf("picked %s from %v", model, candidates)
+	return model
+}
+
+// TestLive_ModelPickerSkipsDeprecated: Go's glm-5 is deprecated in the
+// 2026-09-27 document, so the picker takes the next candidate.
+func TestLive_ModelPickerSkipsDeprecated(t *testing.T) {
+	if got := liveModel(t, ProviderOpencodeGo, "glm-5", "glm-5.3-flash"); got != "glm-5.3-flash" {
+		t.Fatalf("picked %q, want glm-5.3-flash", got)
+	}
+}
+
+// TestLive_ModelPickerKilo: the document's kilo section drives Kilo's picks,
+// and a model it does not list is passed over. It fails, rather than skips,
+// when the section is missing.
+func TestLive_ModelPickerKilo(t *testing.T) {
+	doc, err := liveMetadata()
+	if err != nil {
+		t.Skipf("model metadata unreachable: %v", err)
+	}
+	want := kiloNonTraining[0]
+	got, ok := firstActiveModel(doc, ProviderKilo, []string{"kilo-test/no-such-model", want})
+	if !ok || got != want {
+		t.Fatalf("picked %q (%t) from %d kilo models, want %s", got, ok, len(doc[ProviderKilo]), want)
+	}
+}
diff --git a/llmprovider/live_opencode_conventions_test.go b/llmprovider/live_opencode_conventions_test.go
--- a/llmprovider/live_opencode_conventions_test.go
+++ b/llmprovider/live_opencode_conventions_test.go
@@ -31,7 +31,8 @@
 	var sent []byte
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "gpt-6-luna", WithHTTPClient(recordingClient(&sent)))
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), liveModel(t, ProviderOpencodeGo, "gpt-6-luna", "grok-4.6"),
+		WithHTTPClient(recordingClient(&sent)))
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
@@ -56,7 +57,8 @@
 	var sent []byte
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "minimax-m3", WithHTTPClient(recordingClient(&sent)))
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), liveModel(t, ProviderOpencodeGo, "minimax-m3"),
+		WithHTTPClient(recordingClient(&sent)))
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
diff --git a/llmprovider/live_opencode_route_test.go b/llmprovider/live_opencode_route_test.go
--- a/llmprovider/live_opencode_route_test.go
+++ b/llmprovider/live_opencode_route_test.go
@@ -29,7 +29,8 @@
 	})}
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "qwen3.8-max", WithHTTPClient(client))
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), liveModel(t, ProviderOpencodeGo, "qwen3.8-max"),
+		WithHTTPClient(client))
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
diff --git a/llmprovider/live_reasoning_replay_test.go b/llmprovider/live_reasoning_replay_test.go
--- a/llmprovider/live_reasoning_replay_test.go
+++ b/llmprovider/live_reasoning_replay_test.go
@@ -35,7 +35,7 @@
 		}
 		return http.DefaultTransport.RoundTrip(r)
 	})}
-	p, err := NewOpencode(ProviderOpencodeGo, key, "kimi-k2.6", WithHTTPClient(client))
+	p, err := NewOpencode(ProviderOpencodeGo, key, liveModel(t, ProviderOpencodeGo, "kimi-k2.6"), WithHTTPClient(client))
 	if err != nil {
 		t.Fatalf("NewOpencode: %v", err)
 	}
diff --git a/llmprovider/live_system_test.go b/llmprovider/live_system_test.go
--- a/llmprovider/live_system_test.go
+++ b/llmprovider/live_system_test.go
@@ -19,17 +19,19 @@
 	}
 	for _, c := range []struct {
 		name, env string
-		build     func(key string) (ItemProvider, error)
+		build     func(t *testing.T, key string) (ItemProvider, error)
 	}{
-		{"claude", "ANTHROPIC_API_KEY", func(k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
-		{"go-messages", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "qwen3.8-flash") }},
+		{"claude", "ANTHROPIC_API_KEY", func(_ *testing.T, k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
+		{"go-messages", "OPENCODE_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
+			return NewOpencode(ProviderOpencodeGo, k, liveModel(t, ProviderOpencodeGo, "qwen3.8-flash", "minimax-m3"))
+		}},
 	} {
 		t.Run(c.name, func(t *testing.T) {
 			key := os.Getenv(c.env)
 			if key == "" {
 				t.Skipf("%s unset", c.env)
 			}
-			p, err := c.build(key)
+			p, err := c.build(t, key)
 			if err != nil {
 				t.Fatalf("construct: %v", err)
 			}
diff --git a/llmprovider/live_thinking_test.go b/llmprovider/live_thinking_test.go
--- a/llmprovider/live_thinking_test.go
+++ b/llmprovider/live_thinking_test.go
@@ -85,7 +85,8 @@
 func TestLive_OpencodeMessagesThinking(t *testing.T) {
 	ctx, cancel := liveCtx(t)
 	defer cancel()
-	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "qwen3.8-flash", WithReasoningEffort(effortLow))
+	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), liveModel(t, ProviderOpencodeGo, "qwen3.8-flash"),
+		WithReasoningEffort(effortLow))
 	if err != nil {
 		t.Fatal(err)
 	}
diff --git a/llmprovider/model_picker_test.go b/llmprovider/model_picker_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/model_picker_test.go
@@ -0,0 +1,107 @@
+package llmprovider
+
+import (
+	"encoding/json"
+	"fmt"
+	"io"
+	"strings"
+	"testing"
+)
+
+// pickerSections maps models.opencode.ai's sections to the providers the live
+// suite picks models for (MADR 0012 §3.4). mcplib's own decoder keeps only
+// the sections it ranks with, so the picker reads the document itself.
+var pickerSections = map[string]string{
+	metadataKeyZen: ProviderOpencodeZen,
+	metadataKeyGo:  ProviderOpencodeGo,
+	"kilo":         ProviderKilo,
+}
+
+// pickerDoc is provider -> model id -> status, from the document.
+type pickerDoc map[string]map[string]string
+
+// decodePickerDoc reads the picker's sections of a models.dev-format document.
+func decodePickerDoc(r io.Reader) (pickerDoc, error) {
+	var raw map[string]struct {
+		Models map[string]struct {
+			Status string `json:"status"`
+		} `json:"models"`
+	}
+	if err := json.NewDecoder(r).Decode(&raw); err != nil {
+		return nil, fmt.Errorf("model picker: decode: %w", err)
+	}
+	doc := pickerDoc{}
+	for key, provider := range pickerSections {
+		section, ok := raw[key]
+		if !ok {
+			continue
+		}
+		doc[provider] = make(map[string]string, len(section.Models))
+		for id, m := range section.Models {
+			doc[provider][id] = m.Status
+		}
+	}
+	return doc, nil
+}
+
+// firstActiveModel returns the first candidate the document lists for
+// provider with a status other than "deprecated", and false when none is.
+func firstActiveModel(doc pickerDoc, provider string, candidates []string) (string, bool) {
+	models := doc[provider]
+	for _, id := range candidates {
+		if status, ok := models[id]; ok && status != "deprecated" {
+			return id, true
+		}
+	}
+	return "", false
+}
+
+// TestFirstActiveModel pins the picker the live suite uses.
+func TestFirstActiveModel(t *testing.T) {
+	doc := pickerDoc{
+		ProviderOpencodeGo: {"old": "deprecated", "live1": "", "live2": "beta"},
+		ProviderKilo:       {"vendor/model": ""},
+	}
+	for _, tc := range []struct {
+		name, provider string
+		candidates     []string
+		want           string
+		ok             bool
+	}{
+		{"skips deprecated", ProviderOpencodeGo, []string{"old", "live1"}, "live1", true},
+		{"skips absent", ProviderOpencodeGo, []string{"gone", "live2", "live1"}, "live2", true},
+		{"keeps order", ProviderOpencodeGo, []string{"live1", "live2"}, "live1", true},
+		{"kilo section", ProviderKilo, []string{"gone/model", "vendor/model"}, "vendor/model", true},
+		{"other provider only", ProviderOpencodeGo, []string{"vendor/model"}, "", false},
+		{"none active", ProviderOpencodeGo, []string{"old", "gone"}, "", false},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			got, ok := firstActiveModel(doc, tc.provider, tc.candidates)
+			if got != tc.want || ok != tc.ok {
+				t.Fatalf("firstActiveModel(%s, %v) = %q, %t; want %q, %t", tc.provider, tc.candidates, got, ok, tc.want, tc.ok)
+			}
+		})
+	}
+}
+
+// TestDecodePickerDoc: the Zen, Go and Kilo sections are kept with each
+// model's status; other sections are not.
+func TestDecodePickerDoc(t *testing.T) {
+	doc, err := decodePickerDoc(strings.NewReader(`{
+  "opencode": {"models": {"zen-model": {"status": "beta"}}},
+  "opencode-go": {"models": {"go-model": {"status": "deprecated"}}},
+  "kilo": {"models": {"vendor/kilo-model": {}}},
+  "huggingface": {"models": {"hf/model": {}}}
+}`))
+	if err != nil {
+		t.Fatal(err)
+	}
+	want := pickerDoc{
+		ProviderOpencodeZen: {"zen-model": "beta"},
+		ProviderOpencodeGo:  {"go-model": "deprecated"},
+		ProviderKilo:        {"vendor/kilo-model": ""},
+	}
+	if fmt.Sprint(doc) != fmt.Sprint(want) {
+		t.Fatalf("decodePickerDoc = %v, want %v", doc, want)
+	}
+}
```

**Fix:** no change in this part.

