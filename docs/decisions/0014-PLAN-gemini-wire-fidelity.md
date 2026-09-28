---
status: in-progress
date: 2026-09-27
associated-madr: "0014-MADR-gemini-wire-fidelity.md"
decision-makers: mcplib maintainers
---

# Implement 0014 — Gemini on the Interactions API, and the `generateContent` Fixes

Associated MADR: [0014-MADR-gemini-wire-fidelity.md](0014-MADR-gemini-wire-fidelity.md)
(`status: accepted`). The owner approved the MADR and this plan together on
2026-09-27.

This plan executes the MADR's §1–§3, and nothing else. If a fact contradicts
the MADR or this plan, **stop and prompt**. Add a dated entry to §9 of this
plan, amend the MADR when a decision or an asserted fact changes, and only
then continue.

## How this plan was proven

Every phase below was executed on 2026-09-27, in scratch copies of
`git archive 1c3d79f`:
* **Red.** Each phase's tests diff was applied, and each named test was seen
  to fail on the unfixed code, including live tests against Gemini.
* **Base guards.** A guard that holds today was seen to pass on the unfixed
  code, and then to fail on a named mutant of the fixed code.
* **Green.** The fix diff was applied, and the full gate (§0.2) passed.
* **Live.** The Gemini live tests passed on the final tree, and so did the
  whole live suite (Appendix A.L, A.S).
* **Diffs.** Appendix B's diffs were generated mechanically from that proof.
  Applying them in order to a fresh `1c3d79f` archive reproduces the proven
  tree byte for byte: 319 files, 0 mismatches.

## Goal

* **`generateContent`, the wire OpenCode's `google` route keeps** (MADR §3):
  * a `thought: true` part decodes, as a `ReasoningItem`;
  * a thinking call asks for thoughts (`includeThoughts: true`);
  * `system` items go to `systemInstruction`, not a `model` turn.
* **`GeminiProvider` on the Interactions API** (MADR §1–§2):
  * requests go to `POST {base}/interactions`, with the mappings of MADR §1;
  * every request sends `store: false` unless `WithStore(true)`;
  * `Continue` works with `WithStore(true)`, and without it fails before
    any request;
  * a thinking call returns its thought summary, and its effort is sent as
    `thinking_level`.

## Scope

**In scope:** MADR §1–§3, as phases M1 and M2.

**Out of scope:**
* **A live test of OpenCode's `google` route with thoughts.** M1's live test
  proves the fixed `generateContent` decoder against Gemini itself, through
  `GeminiProvider`, before M2 moves that provider to the Interactions API.
  The OpenCode route shares the decoder, and unit tests pin its requests.
* **Streaming.** `mcplib` does not stream from Gemini today.
* **`DiscoverModels`.** It keeps the models listing, unchanged.
* **Consumers.** A consumer that sets `WithThinkingBudget` on a Gemini
  provider should move to `WithReasoningEffort` (§8). This plan changes no
  consumer.

## 0. Preconditions and conventions

### 0.1 Baseline

* `main` at `1c3d79f`, or a descendant that changes no file in Appendix B.
  Anything else means a rebase, which is a §9 deviation.
* `go test -count=1 ./...` passes before M1.

### 0.2 Gate (every phase)

1. `gofmt -l` prints nothing, and `golint -set_exit_status` passes, on each
   `.go` file the phase touched.
2. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
3. `make lint`.
4. `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

### 0.3 Red first

* Each phase applies its **Tests** diff first. Every red test named in
  Appendix A must fail, with the message recorded there, before the **Fix**
  diff is applied.
* Each base guard must pass before the fix.
* A mutant is applied only to a scratch copy, never to the working tree.

### 0.4 Credentials for live steps

* `GEMINI_API_KEY`, which is never printed.
* `TestLive_ToolRoundTrip` also runs its other providers' cases, each with
  its own credentials.

### 0.5 Commits

One `git commit --no-edit` per phase, after the gate. No push and no tag.

## Phase M0 — Start

1. Confirm §0.1.
2. Set the MADR to `status: accepted`, and this plan to `status: in-progress`.
3. Commit the documents only.

## Phase M1 — Fix the `generateContent` wire (MADR §3)

**Today**, at `1c3d79f`:
* The decoder types a part's `thought` as a string (`gemini.go:266`). Gemini
  sends a boolean, so a response with a thought summary fails to decode.
* `geminiThinkingConfig` never sets `includeThoughts`
  (`thinking_wire.go:109-119`), so no summary is ever returned.
* `geminiItemsToContents` sends a `system` item as a `model` turn
  (`gemini.go:157-163`).

`GeminiProvider` and OpenCode's `google` route share all three.

**Changes** (Appendix B.M1):
* **Shared system prompt.** `claudeSystemPrompt` becomes `systemPrompt`
  (`item_convert.go`), used by the Messages wires and by both Gemini wires.
* **System instruction.** `geminiSystemInstruction` builds
  `systemInstruction: {parts: [{text}]}`. `GeminiProvider` and OpenCode's
  `googleBody` send it, and `geminiItemsToContents` leaves system items out.
* **Thoughts asked for.** `geminiThinkingConfig` always includes
  `"includeThoughts": true`, as OpenCode's client does.
* **Thoughts decoded.** `Thought` is a `bool`. A part with `thought: true`
  becomes a `ReasoningItem` carrying its `text`.
* **Test updates:**
  * the thinking wire tables (`thinking_wire_test.go`) pin
    `includeThoughts: true`;
  * the interleaved fixture in `gemini_items_test.go` served `thought` as a
    string, which Gemini never sends. It now serves the boolean shape.

**Verification.**
* Red: three unit tests, and live `TestLive_GeminiThoughtSummary`.
* Guards: `TestGemini_NoSystemInstructionWithoutSystem` and
  `TestGeminiThinking_NotAskedWithoutThinking`, each killed by a mutant.

## Phase M2 — `GeminiProvider` on the Interactions API (MADR §1–§2)

**Today**, at `1c3d79f`:
* `GeminiProvider` posts to `generateContent` (`gemini.go:231`).
* `Continue` adds `previous_interaction_id` (`gemini.go:222-224`), which
  `generateContent` refuses with 400.
* The decoder reads `id` and `interaction_id` (`gemini.go:260-261`), which
  `generateContent` never returns, so `Response.ID` is always empty.

**Changes** (Appendix B.M2):
* **New `gemini_interactions.go`:**
  * `interactionsInput` maps items to steps. Each call is preceded by a
    `thought` step carrying the call's signature, or the placeholder.
  * `geminiThinkingLevel` maps an effort to `thinking_level`: `xhigh` is
    sent as `high`, and `gemini-2.5-flash-lite` gets `high` or nothing.
  * `interactionsBody` builds the request: `store`, `system_instruction`,
    `tools`, and `generation_config` with `tool_choice`, `thinking_summaries`
    and `thinking_level`.
  * `decodeInteraction` handles the statuses and maps the steps. It compacts
    `arguments` (see "Established while proving" below).
* **`gemini.go`:**
  * `doGenerateItems` posts to `{base}/interactions`.
  * The `thinkingBudget` field and `genConfig` are removed, and a `store`
    field is added.
  * `Continue` returns `ErrInvalidRequest` unless `WithStore(true)`.
  * `decodeGeminiResponse` loses `id` / `interaction_id`. It now serves only
    OpenCode's `google` route.
* **`http_helpers.go`.** A `statusIncomplete` constant, shared by the
  Responses and Interactions decoders. `goconst` requires it once the
  literal appears three times.
* **`options.go`, `item.go`.** Doc comments: `ThinkingBudget` and
  `WithThinkingBudget` no longer apply to `GeminiProvider`; `Store` and
  `WithStore` now cover Gemini; `Response.ID` is a Gemini interaction id.
* **Test migrations.** These live in the fix diff, because the fix breaks
  them: each pinned `GeminiProvider` to `generateContent`.
  * **To Interactions fixtures and field names:** `gemini_items_test.go`,
    `gemini_test.go`, `TestGemini_WithMaxTokens`
    (`provider_correctness_test.go`) and `TestGeminiThinkingTool`
    (`thinking_test.go`). `TestGemini_Continue` now uses `WithStore(true)`,
    and checks `store: true`.
  * **To OpenCode's `google` route**, because they pin the `generateContent`
    wire itself, including its thinking budget, which the Interactions API
    does not have:
    * `TestGeminiThinking_RequestBody` and
      `TestGeminiThinking_DynamicBudgetDefault`, renamed
      `TestGoogleRouteThinking_*`;
    * `TestThinkingWire_Gemini`;
    * M1's `geminiWireBodies` helper, which drops `GeminiProvider`.

**Verification.**
* Red: seven unit tests, and live `TestLive_GeminiInteractions`.
* Guards:
  * `TestGeminiInteractions_PlainCallSendsNoExtras`, killed by two mutants;
  * live `TestLive_StaticGeminiServed`, killed by a mutant that adds a model
    id Gemini does not serve.

**Established while proving.** The Interactions API returns `arguments` as
a JSON object, and decoding it as raw JSON keeps whatever spacing the server
sent. The `generateContent` decoder re-marshals arguments compactly, and
callers compare the strings. The existing
`TestGemini_GenerateItems_FunctionCallPart` caught this: its spaced fixture
came through as `{"query": "weather"}`. So `decodeInteraction` compacts
`arguments`, and `TestGeminiInteractions_Response` pins that with a spaced
fixture. The MADR's response table says so.

## Phase M3 — Records and close-out

1. Record each phase's result in §10.
2. Set this plan to `status: complete` once §7 holds.
3. Commit the documents only.

## 7. Acceptance criteria

* Every Appendix A red test fails before its fix, and passes after it.
* Every mutant is killed.
* The gate passes after each phase.
* The Gemini live tests of Appendix A.L pass.
* Each item of the MADR's Confirmation holds, pinned as below:

| MADR Confirmation | Pinned by |
|---|---|
| Interactions request | `TestGeminiInteractions_Request`, `TestGeminiInteractions_SyntheticCallCarriesPlaceholder` |
| Interactions response | `TestGeminiInteractions_Response`, `TestGeminiInteractions_Status` |
| Thinking | `TestGeminiInteractions_Thinking` |
| `Continue` | `TestGemini_ContinueNeedsStore`, `TestGemini_ContinueChainsWhenStored` |
| `generateContent` (OpenCode `google` route) | `TestGeminiDecode_ThoughtSummaryPart`, `TestGeminiThinking_AsksForThoughts`, `TestGemini_SystemInstruction` |
| Live: text, system instruction obeyed, `store: false`, `Continue` recall | `TestLive_GeminiInteractions` |
| Live: forced tool call and round trip | `TestLive_ToolRoundTrip/gemini` |
| Live: real-signature replay | `TestLive_GeminiReplaysRealCall` |
| Live: thought summary | `TestLive_GeminiThoughtSummary` |
| Live: every `StaticGemini` model | `TestLive_StaticGeminiServed` |

## 8. Rollout and rollback

* **Behaviour changes for `GeminiProvider`:**
  * it calls `/v1beta/interactions`, with `store: false` unless
    `WithStore(true)`;
  * `WithThinkingBudget` no longer affects it. An effort is sent as
    `thinking_level`, and with no effort the model's default applies;
  * `gemini-2.5-flash-lite` drops efforts other than `high`;
  * `Response.ID` is the interaction id; it was always empty before;
  * `Continue` returns `ErrInvalidRequest` without a request unless
    `WithStore(true)`; it used to fail with HTTP 400 every time.
* **Behaviour changes on both Gemini wires:**
  * a thinking call returns a thought summary as a `ReasoningItem`. Summaries
    are billed as output tokens;
  * `system` items reach the system field, and no longer form a `model`
    turn.
* **Exported API.** No signature changes. `WithStore` extends to Gemini.
* **Rollback.**
  * Reverting M2 alone returns `GeminiProvider` to `generateContent`, keeping
    M1's fixes.
  * M2 depends on M1 (`systemPrompt`), so revert M2 before M1.
* **Release.** This plan tags nothing. A release is the owner's call.

## 9. Deviation log

None yet.

## 10. Execution record

Not executed yet.

## Appendix A — Proof record (2026-09-27)

### A.M1 Phase M1 — Fix the `generateContent` wire (MADR §3)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestGeminiDecode_ThoughtSummaryPart` | unit | decode: json: cannot unmarshal bool into Go struct field .candidates.content.parts.thought of type string |
| `TestGeminiThinking_AsksForThoughts` | unit | opencode-google: thinkingConfig = map[thinkingBudget:-1], want includeThoughts true |
| `TestGemini_SystemInstruction` | unit | opencode-google: systemInstruction = <nil> |
| `TestLive_GeminiThoughtSummary` | live | reasoning "", answer "391" |

**Guards before the fix:**

* `TestGemini_NoSystemInstructionWithoutSystem`: passed on the unfixed code.
* `TestGeminiThinking_NotAskedWithoutThinking`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 9 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `m1-system-always` | `TestGemini_NoSystemInstructionWithoutSystem` | killed | gemini: systemInstruction = map[parts:[map[text:]]], want absent |
| `m1-thinking-always` | `TestGeminiThinking_NotAskedWithoutThinking` | killed | gemini: generationConfig = map[maxOutputTokens:8192 thinkingConfig:map[includeThoughts:true thinkingBudget:-1]], want no thinkingConfig |

### A.M2 Phase M2 — `GeminiProvider` on the Interactions API (MADR §1–§2)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestGeminiInteractions_Request` | unit | path = "/models/gemini-3.7-flash:generateContent", want /interactions |
| `TestGeminiInteractions_SyntheticCallCarriesPlaceholder` | unit | steps = [], want a placeholder thought before the call |
| `TestGeminiInteractions_Response` | unit | GenerateItemsWithTool: gemini returned no content |
| `TestGeminiInteractions_Thinking` | unit | gemini-3.7-flash/: generation_config = map[], want summaries auto and level <nil> |
| `TestGeminiInteractions_Status` | unit | incomplete: err = gemini returned no content, want *IncompleteError |
| `TestGemini_ContinueNeedsStore` | unit | err = gemini returned no content after request "/models/gemini-3.7-flash:generateContent", want ErrInvalidRequest and none |
| `TestGemini_ContinueChainsWhenStored` | unit | path "/models/gemini-3.7-flash:generateContent", body map[contents:[map[parts:[map[text:more]] role:user]] generationConfig:map[maxOutputTokens:8192] previous_interaction_id:v1_prev] |
| `TestLive_GeminiInteractions` | live | requests [/v1beta/models/gemini-3.7-flash:generateContent] |

**Guards before the fix:**

* `TestGeminiInteractions_PlainCallSendsNoExtras`: passed on the unfixed code.
* `TestLive_StaticGeminiServed` (live): passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 13 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `m2-summaries-always` | `TestGeminiInteractions_PlainCallSendsNoExtras` | killed | generation_config.thinking_summaries = auto, want absent |
| `m2-system-always` | `TestGeminiInteractions_PlainCallSendsNoExtras` | killed | system_instruction = , want absent |
| `m2-static-unserved-id` | `TestLive_StaticGeminiServed` | killed | gemini-no-such-model: Generate = "", llm: invalid request: gemini HTTP 404 not_found: Model 'gemini-no-such-model' not found. Did you mean 'gemini-flash-latest'? Please verify the model name against the supported list: h |

### A.L Gemini live tests on the final tree (`-tags live_gateways`, 2026-09-27)

| Test | Result |
|---|---|
| `TestLive_GeminiInteractions` | PASS |
| `TestLive_StaticGeminiServed` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.7-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.6-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.5-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.5-flash-lite` | PASS |
| `TestLive_StaticGeminiServed/gemini-2.5-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-2.5-flash-lite` | PASS |
| `TestLive_GeminiThoughtSummary` | PASS |
| `TestLive_ToolRoundTrip` | PASS |
| `TestLive_ToolRoundTrip/claude` | PASS |
| `TestLive_ToolRoundTrip/gemini` | PASS |
| `TestLive_ToolRoundTrip/grok` | PASS |
| `TestLive_ToolRoundTrip/kilo` | PASS |
| `TestLive_ToolRoundTrip/go-chat` | PASS |
| `TestLive_ToolRoundTrip/go-messages` | PASS |
| `TestLive_ToolRoundTrip/go-responses` | PASS |
| `TestLive_GeminiReplaysRealCall` | PASS |
| `TestLive_GeminiThinkingShapes` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-2.5-flash/low` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-2.5-flash/` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-3.7-flash/low` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-3.7-flash/` | PASS |


Totals: 23 passed, 0 skipped, 0 failed.

### A.S The whole live suite on the final tree

| Test | Result |
|---|---|
| `TestLive_ChatGPTErrorDetail` | SKIP |
| `TestLive_ChatGPTListingVersion` | SKIP |
| `TestLive_ChatGPTGenerate` | SKIP |
| `TestLive_OpencodeChatCompletions` | PASS |
| `TestLive_OpencodeResponses` | PASS |
| `TestLive_OpencodeRouteStillEnforced` | PASS |
| `TestLive_KiloChatCompletions` | PASS |
| `TestLive_KiloToolCall` | PASS |
| `TestLive_KiloReasoningSpelling` | PASS |
| `TestLive_KiloSupportedParameters` | PASS |
| `TestLive_HuggingFaceMetadataFields` | PASS |
| `TestLive_HuggingFaceChatCompletions` | PASS |
| `TestLive_ListingsNeedNoCredential` | PASS |
| `TestLive_ListingsNeedNoCredential/opencode-go` | PASS |
| `TestLive_ListingsNeedNoCredential/huggingface` | PASS |
| `TestLive_ListingsNeedNoCredential/kilo` | PASS |
| `TestLive_ListingsNeedNoCredential/opencode-zen` | PASS |
| `TestLive_OpencodeKeyHeaderPerRoute` | PASS |
| `TestLive_OpencodeKeyHeaderPerRoute/messages` | PASS |
| `TestLive_OpencodeKeyHeaderPerRoute/google` | PASS |
| `TestLive_ModelMetadataDocument` | PASS |
| `TestLive_OpencodeChatReasoningEffort` | PASS |
| `TestLive_OpencodeChatReasoningEffort/glm-5.3-flash` | PASS |
| `TestLive_OpencodeChatReasoningEffort/hy3` | PASS |
| `TestLive_KiloReasoningShapes` | PASS |
| `TestLive_KiloReasoningShapes/enabled` | PASS |
| `TestLive_KiloReasoningShapes/effort_low` | PASS |
| `TestLive_GeminiInteractions` | PASS |
| `TestLive_StaticGeminiServed` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.7-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.6-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.5-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-3.5-flash-lite` | PASS |
| `TestLive_StaticGeminiServed/gemini-2.5-flash` | PASS |
| `TestLive_StaticGeminiServed/gemini-2.5-flash-lite` | PASS |
| `TestLive_GeminiThoughtSummary` | PASS |
| `TestLive_GrokListingTextOnly` | PASS |
| `TestLive_GrokEffortMenu` | PASS |
| `TestLive_GrokToolDescription` | PASS |
| `TestLive_ToolRoundTrip` | PASS |
| `TestLive_ToolRoundTrip/claude` | PASS |
| `TestLive_ToolRoundTrip/gemini` | PASS |
| `TestLive_ToolRoundTrip/grok` | PASS |
| `TestLive_ToolRoundTrip/kilo` | PASS |
| `TestLive_ToolRoundTrip/go-chat` | PASS |
| `TestLive_ToolRoundTrip/go-messages` | PASS |
| `TestLive_ToolRoundTrip/go-responses` | PASS |
| `TestLive_GeminiReplaysRealCall` | PASS |
| `TestLive_KiloDataCollectionDenied` | PASS |
| `TestLive_ModelPickerSkipsDeprecated` | PASS |
| `TestLive_ModelPickerKilo` | PASS |
| `TestLive_ChatGPTBrowserLogin` | SKIP |
| `TestLive_GrokDiscoveryPublishesRevocation` | PASS |
| `TestLive_OpencodeResponsesStoreFalse` | PASS |
| `TestLive_OpencodeMinimaxM3Adaptive` | PASS |
| `TestLive_OpencodeRoutesFromMetadata` | PASS |
| `TestLive_InterleavedReasoningReplay` | PASS |
| `TestLive_ResponsesStoreFalse` | PASS |
| `TestLive_ResponsesStoreFalse/openai` | SKIP |
| `TestLive_ResponsesStoreFalse/grok` | PASS |
| `TestLiveTransient_SkipsOnlyTransientClasses` | PASS |
| `TestLive_StaticClaudeServed` | PASS |
| `TestLive_StaticClaudeServed/claude-haiku-4-5` | PASS |
| `TestLive_StaticClaudeServed/claude-sonnet-5` | PASS |
| `TestLive_StaticClaudeServed/claude-sonnet-4-6` | PASS |
| `TestLive_StaticClaudeServed/claude-opus-4-8` | PASS |
| `TestLive_SystemMessage` | PASS |
| `TestLive_SystemMessage/claude` | PASS |
| `TestLive_SystemMessage/go-messages` | PASS |
| `TestLive_ClaudeThinkingShapes` | PASS |
| `TestLive_ClaudeThinkingShapes/claude-haiku-4-5/low` | PASS |
| `TestLive_ClaudeThinkingShapes/claude-haiku-4-5/` | PASS |
| `TestLive_ClaudeThinkingShapes/claude-sonnet-5/low` | PASS |
| `TestLive_ClaudeThinkingShapes/claude-sonnet-5/` | PASS |
| `TestLive_ClaudeThinkingShapes/claude-opus-4-8/low` | PASS |
| `TestLive_ClaudeThinkingShapes/claude-opus-4-8/` | PASS |
| `TestLive_GeminiThinkingShapes` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-2.5-flash/low` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-2.5-flash/` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-3.7-flash/low` | PASS |
| `TestLive_GeminiThinkingShapes/gemini-3.7-flash/` | PASS |
| `TestLive_OpencodeMessagesThinking` | PASS |
| `TestLive_VendorCLISession` | PASS |
| `TestLive_VendorCLISession/openai` | SKIP |
| `TestLive_VendorCLISession/grok` | SKIP |

* Skip, `TestLive_ChatGPTBrowserLogin`: MCPLIB_LIVE_BROWSER_LOGIN unset: this needs a person to sign in in a browser
* Skip, `TestLive_ChatGPTErrorDetail`: MCPLIB_LIVE_CHATGPT unset: live ChatGPT calls spend the subscription
* Skip, `TestLive_ChatGPTGenerate`: MCPLIB_LIVE_CHATGPT unset: live ChatGPT calls spend the subscription
* Skip, `TestLive_ChatGPTListingVersion`: MCPLIB_LIVE_CHATGPT unset: live ChatGPT calls spend the subscription
* Skip, `TestLive_ResponsesStoreFalse/openai`: gateway transient (rate limit / outage / quota / not permitted): llm: quota exhausted: openai HTTP 429 credit_balance_exhausted: You have no credits remaining. Add credits to continue using the API at https://platform.openai.com/settings/organization/billing/.
* Skip, `TestLive_VendorCLISession/grok`: MCPLIB_LIVE_GROK_CLI unset: this spends the CLI's subscription
* Skip, `TestLive_VendorCLISession/openai`: MCPLIB_LIVE_CHATGPT unset: this spends the CLI's subscription

Totals: 78 passed, 7 skipped, 0 failed.

## Appendix B — Diffs

Generated from the proof. Apply each phase's **Tests** diff, then its **Fix**
diff, with `git apply`, in order, on the §0.1 baseline.

### B.M1 Phase M1 — Fix the `generateContent` wire (MADR §3)

**Tests** (`m1-tests.diff`, 162 lines):

```diff
diff --git a/llmprovider/gemini_generatecontent_test.go b/llmprovider/gemini_generatecontent_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/gemini_generatecontent_test.go
@@ -0,0 +1,114 @@
+package llmprovider
+
+import (
+	"context"
+	"strings"
+	"testing"
+)
+
+// thoughtSummaryResponse is generateContent's shape with includeThoughts
+// (gemini-3.7-flash, 2026-09-27): the summary part carries thought: true, a
+// boolean, and its text in "text".
+const thoughtSummaryResponse = `{"candidates":[{"content":{"parts":[
+{"thought":true,"text":"Multiply 17 by 23."},
+{"text":"391","thoughtSignature":"sig-abc"}]}}]}`
+
+// TestGeminiDecode_ThoughtSummaryPart: the thought part is reasoning, the
+// answer is the only message, and the response decodes (MADR 0014 §3).
+func TestGeminiDecode_ThoughtSummaryPart(t *testing.T) {
+	res, err := decodeGeminiResponse(strings.NewReader(thoughtSummaryResponse))
+	if err != nil {
+		t.Fatalf("decode: %v", err)
+	}
+	if len(res.Output) != 2 {
+		t.Fatalf("output = %#v, want a ReasoningItem and a MessageItem", res.Output)
+	}
+	if r, ok := res.Output[0].(ReasoningItem); !ok || r.Text != "Multiply 17 by 23." {
+		t.Errorf("output[0] = %#v, want the thought summary", res.Output[0])
+	}
+	if res.OutputText() != "391" {
+		t.Errorf("OutputText = %q, want 391", res.OutputText())
+	}
+}
+
+// geminiWireBodies returns the request bodies GeminiProvider and OpenCode's
+// google route send for one call.
+func geminiWireBodies(t *testing.T, thinking bool, items ...Item) map[string]map[string]any {
+	t.Helper()
+	out := map[string]map[string]any{}
+	var body map[string]any
+	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
+	gp, err := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	if err != nil {
+		t.Fatal(err)
+	}
+	op, err := NewOpencode(ProviderOpencodeZen, "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	if err != nil {
+		t.Fatal(err)
+	}
+	for name, p := range map[string]ItemThinkingProvider{"gemini": gp, "opencode-google": op} {
+		body = nil
+		if thinking {
+			_, err = p.GenerateItemsThinking(context.Background(), items...)
+		} else {
+			_, err = p.(ItemProvider).GenerateItems(context.Background(), items...)
+		}
+		if err != nil {
+			t.Fatalf("%s: %v", name, err)
+		}
+		out[name] = body
+	}
+	return out
+}
+
+// TestGeminiThinking_AsksForThoughts: a thinking call sets includeThoughts,
+// as OpenCode's client does (transform.ts:1280-1288).
+func TestGeminiThinking_AsksForThoughts(t *testing.T) {
+	for name, body := range geminiWireBodies(t, true, MessageItem{Role: jsonRoleUser, Text: "hi"}) {
+		gen, _ := body["generationConfig"].(map[string]any)
+		tc, _ := gen["thinkingConfig"].(map[string]any)
+		if tc["includeThoughts"] != true {
+			t.Errorf("%s: thinkingConfig = %v, want includeThoughts true", name, tc)
+		}
+	}
+}
+
+// TestGeminiThinking_NotAskedWithoutThinking: a plain call sends no
+// thinkingConfig at all.
+func TestGeminiThinking_NotAskedWithoutThinking(t *testing.T) {
+	for name, body := range geminiWireBodies(t, false, MessageItem{Role: jsonRoleUser, Text: "hi"}) {
+		gen, _ := body["generationConfig"].(map[string]any)
+		if _, ok := gen["thinkingConfig"]; ok {
+			t.Errorf("%s: generationConfig = %v, want no thinkingConfig", name, gen)
+		}
+	}
+}
+
+// TestGemini_SystemInstruction: system items go to systemInstruction and
+// never become a model turn.
+func TestGemini_SystemInstruction(t *testing.T) {
+	bodies := geminiWireBodies(t, false,
+		MessageItem{Role: jsonRoleSystem, Text: "Reply in French."},
+		MessageItem{Role: jsonRoleSystem, Text: "Be brief."},
+		MessageItem{Role: jsonRoleUser, Text: "hello"})
+	for name, body := range bodies {
+		si, _ := body["systemInstruction"].(map[string]any)
+		parts, _ := si["parts"].([]any)
+		if len(parts) != 1 || parts[0].(map[string]any)["text"] != "Reply in French.\n\nBe brief." {
+			t.Errorf("%s: systemInstruction = %v", name, body["systemInstruction"])
+		}
+		contents, _ := body["contents"].([]any)
+		if len(contents) != 1 || contents[0].(map[string]any)["role"] != "user" {
+			t.Errorf("%s: contents = %v, want only the user turn", name, contents)
+		}
+	}
+}
+
+// TestGemini_NoSystemInstructionWithoutSystem: no system item, no field.
+func TestGemini_NoSystemInstructionWithoutSystem(t *testing.T) {
+	for name, body := range geminiWireBodies(t, false, MessageItem{Role: jsonRoleUser, Text: "hello"}) {
+		if v, ok := body["systemInstruction"]; ok {
+			t.Errorf("%s: systemInstruction = %v, want absent", name, v)
+		}
+	}
+}
diff --git a/llmprovider/live_gemini_wire_test.go b/llmprovider/live_gemini_wire_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_gemini_wire_test.go
@@ -0,0 +1,38 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"os"
+	"strings"
+	"testing"
+)
+
+// TestLive_GeminiThoughtSummary: a Gemini thinking call returns its thought
+// summary as a ReasoningItem (MADR 0014).
+func TestLive_GeminiThoughtSummary(t *testing.T) {
+	key := os.Getenv("GEMINI_API_KEY")
+	if key == "" {
+		t.Skip("GEMINI_API_KEY unset")
+	}
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewGemini(ctx, key, "gemini-3.7-flash")
+	if err != nil {
+		t.Fatal(err)
+	}
+	res, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: "What is 17 * 23? Reply with only the number."})
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("GenerateItemsThinking: %v", err)
+	}
+	var reasoning string
+	for _, it := range res.Output {
+		if r, ok := it.(ReasoningItem); ok {
+			reasoning += r.Text
+		}
+	}
+	if strings.TrimSpace(reasoning) == "" || !strings.Contains(res.OutputText(), "391") {
+		t.Fatalf("reasoning %q, answer %q; want a thought summary and 391", reasoning, res.OutputText())
+	}
+}
```

**Fix** (`m1-fix.diff`, 191 lines):

```diff
diff --git a/llmprovider/claude.go b/llmprovider/claude.go
--- a/llmprovider/claude.go
+++ b/llmprovider/claude.go
@@ -152,7 +152,7 @@
 		case MessageItem:
 			role := v.Role
 			if role == jsonRoleSystem {
-				continue // top-level system field; see claudeSystemPrompt
+				continue // top-level system field; see systemPrompt
 			}
 			if role == "" || role == jsonRoleUser {
 				role = jsonRoleUser
@@ -188,7 +188,7 @@
 		"max_tokens":    maxTokens,
 		jsonKeyMessages: claudeItemsToMessages(input),
 	}
-	if system := claudeSystemPrompt(input); system != "" {
+	if system := systemPrompt(input); system != "" {
 		body[jsonKeySystem] = system
 	}
 
diff --git a/llmprovider/gemini.go b/llmprovider/gemini.go
--- a/llmprovider/gemini.go
+++ b/llmprovider/gemini.go
@@ -131,6 +131,16 @@
 	return p.doGenerateItems(ctx, input, nil, false, previousInteractionID)
 }
 
+// geminiSystemInstruction is generateContent's systemInstruction for the
+// system items, or nil when there are none (MADR 0014 §3).
+func geminiSystemInstruction(items []Item) map[string]any {
+	system := systemPrompt(items)
+	if system == "" {
+		return nil
+	}
+	return map[string]any{"parts": []map[string]any{{jsonKeyText: system}}}
+}
+
 func geminiItemsToContents(items []Item) []map[string]any {
 	// Gemini pairs a functionResponse with its functionCall by name, so a
 	// result takes the name of the call it answers (MADR 0012 §2).
@@ -154,6 +164,9 @@
 	for _, item := range items {
 		switch v := item.(type) {
 		case MessageItem:
+			if v.Role == jsonRoleSystem {
+				continue // systemInstruction; see geminiSystemInstruction
+			}
 			role := v.Role
 			if role == "" || role == jsonRoleUser {
 				role = jsonRoleUser
@@ -197,6 +210,9 @@
 	body := map[string]any{
 		"contents":         geminiItemsToContents(input),
 		"generationConfig": p.genConfig(thinking),
+	}
+	if system := geminiSystemInstruction(input); system != nil {
+		body["systemInstruction"] = system
 	}
 
 	if tool != nil {
@@ -263,7 +279,7 @@
 			Content struct {
 				Parts []struct {
 					Text         string `json:"text"`
-					Thought      string `json:"thought"`
+					Thought      bool   `json:"thought"`
 					FunctionCall *struct {
 						Name string         `json:"name"`
 						Args map[string]any `json:"args"`
@@ -288,8 +304,12 @@
 
 	result := &Response{ID: id}
 	for _, part := range raw.Candidates[0].Content.Parts {
-		if part.Thought != "" {
-			result.Output = append(result.Output, ReasoningItem{Text: part.Thought})
+		// A thought summary is flagged thought: true, with its text in text.
+		if part.Thought {
+			if part.Text != "" {
+				result.Output = append(result.Output, ReasoningItem{Text: part.Text})
+			}
+			continue
 		}
 		if part.Text != "" {
 			result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: part.Text})
diff --git a/llmprovider/gemini_items_test.go b/llmprovider/gemini_items_test.go
--- a/llmprovider/gemini_items_test.go
+++ b/llmprovider/gemini_items_test.go
@@ -91,7 +91,7 @@
 			"candidates": [{
 				"content": {
 					"parts": [
-						{"thought": "pondering the question"},
+						{"thought": true, "text": "pondering the question"},
 						{"text": "the solution is 42"}
 					]
 				}
diff --git a/llmprovider/item_convert.go b/llmprovider/item_convert.go
--- a/llmprovider/item_convert.go
+++ b/llmprovider/item_convert.go
@@ -20,9 +20,10 @@
 	return map[string]any{jsonKeyArguments: arguments}
 }
 
-// claudeSystemPrompt joins the system items, in order, for the Messages API's
-// top-level system field; claudeItemsToMessages leaves them out (MADR 0012 §2).
-func claudeSystemPrompt(items []Item) string {
+// systemPrompt joins the system items, in order, for a wire's dedicated
+// system field: the Messages API's system (MADR 0012 §2) and Gemini's
+// system instruction (MADR 0014). The converters leave system items out.
+func systemPrompt(items []Item) string {
 	var parts []string
 	for _, item := range items {
 		if m, ok := item.(MessageItem); ok && m.Role == jsonRoleSystem && m.Text != "" {
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -191,7 +191,7 @@
 		jsonKeyModel:    p.model,
 		jsonKeyMessages: claudeItemsToMessages(input),
 	}
-	if system := claudeSystemPrompt(input); system != "" {
+	if system := systemPrompt(input); system != "" {
 		body[jsonKeySystem] = system
 	}
 	if thinking {
@@ -223,6 +223,9 @@
 	body := map[string]any{
 		"contents":         geminiItemsToContents(input),
 		"generationConfig": genCfg,
+	}
+	if system := geminiSystemInstruction(input); system != nil {
+		body["systemInstruction"] = system
 	}
 	if tool != nil {
 		body[jsonKeyTools] = []map[string]any{{
diff --git a/llmprovider/thinking_wire.go b/llmprovider/thinking_wire.go
--- a/llmprovider/thinking_wire.go
+++ b/llmprovider/thinking_wire.go
@@ -107,13 +107,18 @@
 // lowEffortThinkingBudget budget on 1.x and 2.x, and any other effort keeps
 // the dynamic budget.
 func geminiThinkingConfig(model, effort string, budget int) map[string]any {
+	// Thought summaries come back only when asked for, as OpenCode's client
+	// asks (transform.ts:1280-1288, MADR 0014 §3).
+	cfg := map[string]any{"includeThoughts": true}
 	switch {
 	case budget > 0:
-		return map[string]any{jsonKeyThinkingBudget: budget}
+		cfg[jsonKeyThinkingBudget] = budget
 	case effort != effortLow:
-		return map[string]any{jsonKeyThinkingBudget: dynamicGeminiThinkingBudget}
+		cfg[jsonKeyThinkingBudget] = dynamicGeminiThinkingBudget
 	case geminiLegacyRE.MatchString(model):
-		return map[string]any{jsonKeyThinkingBudget: lowEffortThinkingBudget}
+		cfg[jsonKeyThinkingBudget] = lowEffortThinkingBudget
+	default:
+		cfg["thinkingLevel"] = effortLow
 	}
-	return map[string]any{"thinkingLevel": effortLow}
+	return cfg
 }
diff --git a/llmprovider/thinking_wire_test.go b/llmprovider/thinking_wire_test.go
--- a/llmprovider/thinking_wire_test.go
+++ b/llmprovider/thinking_wire_test.go
@@ -68,11 +68,11 @@
 // is HTTP 400 there); other efforts keep the dynamic budget; a budget wins.
 func TestThinkingWire_Gemini(t *testing.T) {
 	for _, tc := range []thinkingCase{
-		{"gemini-3.7-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingLevel": "low"}}},
-		{"gemini-2.5-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(1024)}}},
-		{"gemini-3.7-flash", "", 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(-1)}}},
-		{"gemini-3.7-flash", effortHigh, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(-1)}}},
-		{"gemini-2.5-flash", effortLow, 512, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(512)}}},
+		{"gemini-3.7-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"includeThoughts": true, "thinkingLevel": "low"}}},
+		{"gemini-2.5-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"includeThoughts": true, "thinkingBudget": float64(1024)}}},
+		{"gemini-3.7-flash", "", 0, map[string]any{"thinkingConfig": map[string]any{"includeThoughts": true, "thinkingBudget": float64(-1)}}},
+		{"gemini-3.7-flash", effortHigh, 0, map[string]any{"thinkingConfig": map[string]any{"includeThoughts": true, "thinkingBudget": float64(-1)}}},
+		{"gemini-2.5-flash", effortLow, 512, map[string]any{"thinkingConfig": map[string]any{"includeThoughts": true, "thinkingBudget": float64(512)}}},
 	} {
 		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
 			var body map[string]any
@@ -104,7 +104,7 @@
 		{ProviderOpencodeGo, "qwen3.8-flash", fxOpencodeMessages, false, map[string]any{
 			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(1024)}}},
 		{ProviderOpencodeZen, "gemini-3.8-flash", fxOpencodeGoogle, true, map[string]any{
-			"thinkingConfig": map[string]any{"thinkingLevel": "low"}}},
+			"thinkingConfig": map[string]any{"includeThoughts": true, "thinkingLevel": "low"}}},
 	} {
 		t.Run(tc.gateway+"/"+tc.model, func(t *testing.T) {
 			var body map[string]any
```

### B.M2 Phase M2 — `GeminiProvider` on the Interactions API (MADR §1–§2)

**Tests** (`m2-tests.diff`, 330 lines):

```diff
diff --git a/llmprovider/gemini_interactions_test.go b/llmprovider/gemini_interactions_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/gemini_interactions_test.go
@@ -0,0 +1,229 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"net/http"
+	"net/http/httptest"
+	"sync"
+	"testing"
+)
+
+// interactionCapture records the requests a stub Interactions server received.
+type interactionCapture struct {
+	mu     sync.Mutex
+	paths  []string
+	bodies []map[string]any
+}
+
+func (c *interactionCapture) last() (string, map[string]any) {
+	c.mu.Lock()
+	defer c.mu.Unlock()
+	if len(c.paths) == 0 {
+		return "", nil
+	}
+	return c.paths[len(c.paths)-1], c.bodies[len(c.bodies)-1]
+}
+
+// interactionServer answers every request with reply and records it.
+func interactionServer(t *testing.T, reply string) (*httptest.Server, *interactionCapture) {
+	t.Helper()
+	c := &interactionCapture{}
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		var body map[string]any
+		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
+			t.Errorf("decode: %v", err)
+		}
+		c.mu.Lock()
+		c.paths, c.bodies = append(c.paths, r.URL.Path), append(c.bodies, body)
+		c.mu.Unlock()
+		_, _ = w.Write([]byte(reply))
+	}))
+	t.Cleanup(srv.Close)
+	return srv, c
+}
+
+const interactionText = `{"id":"v1_int_1","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"ok"}]}]}`
+
+// stepTypes lists the input steps' types.
+func stepTypes(body map[string]any) []string {
+	var out []string
+	steps, _ := body["input"].([]any)
+	for _, s := range steps {
+		out = append(out, fmt.Sprint(s.(map[string]any)["type"]))
+	}
+	return out
+}
+
+var weatherTool = Tool{Name: "get_weather", Description: "Weather for a city",
+	Schema: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}}
+
+// TestGeminiInteractions_Request: the request shape measured on 2026-09-27
+// (MADR 0014 §1): system_instruction, typed steps, a thought step with the
+// call's signature before it, a result keyed by call_id, the tool and its
+// forced choice in generation_config, and store false.
+func TestGeminiInteractions_Request(t *testing.T) {
+	srv, c := interactionServer(t, interactionText)
+	p, err := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	if err != nil {
+		t.Fatal(err)
+	}
+	_, genErr := p.GenerateItemsWithTool(context.Background(), weatherTool,
+		MessageItem{Role: jsonRoleSystem, Text: "Be brief."},
+		MessageItem{Role: jsonRoleUser, Text: "weather?"},
+		FunctionCallItem{CallID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`, Signature: "sig-1"},
+		FunctionCallOutputItem{CallID: "call_1", Output: "sunny"})
+	path, body := c.last()
+	if path != "/interactions" {
+		t.Fatalf("path = %q, want /interactions", path)
+	}
+	if got := fmt.Sprint(stepTypes(body)); got != "[user_input thought function_call function_result]" {
+		t.Errorf("steps = %s", got)
+	}
+	steps, _ := body["input"].([]any)
+	if len(steps) == 4 {
+		thought, call, result := steps[1].(map[string]any), steps[2].(map[string]any), steps[3].(map[string]any)
+		if thought["signature"] != "sig-1" || call["id"] != "call_1" || fmt.Sprint(call["arguments"]) != "map[city:Paris]" ||
+			result["call_id"] != "call_1" || result["name"] != "get_weather" || result["result"] != "sunny" {
+			t.Errorf("thought %v, call %v, result %v", thought, call, result)
+		}
+	}
+	gen, _ := body["generation_config"].(map[string]any)
+	if body["system_instruction"] != "Be brief." || body["store"] != false || body["model"] != "gemini-3.7-flash" {
+		t.Errorf("system_instruction %v, store %v, model %v", body["system_instruction"], body["store"], body["model"])
+	}
+	if fmt.Sprint(gen["tool_choice"]) != "map[allowed_tools:map[mode:any tools:[get_weather]]]" {
+		t.Errorf("tool_choice = %v", gen["tool_choice"])
+	}
+	tools, _ := body["tools"].([]any)
+	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "function" || tools[0].(map[string]any)["name"] != "get_weather" {
+		t.Errorf("tools = %v", body["tools"])
+	}
+	if genErr != nil {
+		t.Errorf("GenerateItemsWithTool: %v", genErr)
+	}
+}
+
+// TestGeminiInteractions_SyntheticCallCarriesPlaceholder: a call Gemini did not
+// issue replays after a thought step with the placeholder signature.
+func TestGeminiInteractions_SyntheticCallCarriesPlaceholder(t *testing.T) {
+	srv, c := interactionServer(t, interactionText)
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	_, _ = p.GenerateItems(context.Background(), MessageItem{Role: jsonRoleUser, Text: "q"},
+		FunctionCallItem{CallID: "c", Name: "f", Arguments: "{}"}, FunctionCallOutputItem{CallID: "c", Output: "r"})
+	_, body := c.last()
+	steps, _ := body["input"].([]any)
+	if len(steps) < 2 || steps[1].(map[string]any)["signature"] != geminiSkipThoughtSignature {
+		t.Fatalf("steps = %v, want a placeholder thought before the call", steps)
+	}
+}
+
+// TestGeminiInteractions_Response: a thought summary is reasoning, its
+// signature rides on the calls after it, arguments are compact JSON, and the
+// interaction id is the ID.
+func TestGeminiInteractions_Response(t *testing.T) {
+	srv, _ := interactionServer(t, `{"id":"v1_int_9","status":"requires_action","steps":[
+{"type":"thought","signature":"sig-9","summary":[{"type":"text","text":"Look up the weather."}]},
+{"type":"function_call","id":"call_7","name":"get_weather","arguments":{"city": "Paris"}},
+{"type":"function_call","id":"call_8","name":"get_weather","arguments":{"city":"Rome"}}]}`)
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	res, err := p.GenerateItemsWithTool(context.Background(), weatherTool, MessageItem{Role: jsonRoleUser, Text: "q"})
+	if err != nil {
+		t.Fatalf("GenerateItemsWithTool: %v", err)
+	}
+	want := []Item{
+		ReasoningItem{Text: "Look up the weather."},
+		FunctionCallItem{CallID: "call_7", Name: "get_weather", Arguments: `{"city":"Paris"}`, Signature: "sig-9"},
+		FunctionCallItem{CallID: "call_8", Name: "get_weather", Arguments: `{"city":"Rome"}`, Signature: "sig-9"},
+	}
+	if res.ID != "v1_int_9" || fmt.Sprint(res.Output) != fmt.Sprint(want) {
+		t.Fatalf("ID %q, output %+v; want v1_int_9, %+v", res.ID, res.Output, want)
+	}
+}
+
+// TestGeminiInteractions_Thinking: summaries on every thinking call; an effort
+// becomes thinking_level, xhigh as high; gemini-2.5-flash-lite takes only high.
+func TestGeminiInteractions_Thinking(t *testing.T) {
+	for _, tc := range []struct{ model, effort, want string }{
+		{"gemini-3.7-flash", "", "<nil>"},
+		{"gemini-3.7-flash", effortLow, "low"},
+		{"gemini-3.7-flash", effortXHigh, "high"},
+		{"gemini-2.5-flash-lite", effortLow, "<nil>"},
+		{"gemini-2.5-flash-lite", effortHigh, "high"},
+	} {
+		srv, c := interactionServer(t, interactionText)
+		p, _ := NewGemini(context.Background(), "k", tc.model, WithBaseURL(srv.URL), WithReasoningEffort(tc.effort))
+		_, _ = p.GenerateThinking(context.Background(), "hi")
+		_, body := c.last()
+		gen, _ := body["generation_config"].(map[string]any)
+		if gen["thinking_summaries"] != "auto" || fmt.Sprint(gen["thinking_level"]) != tc.want {
+			t.Errorf("%s/%s: generation_config = %v, want summaries auto and level %s", tc.model, tc.effort, gen, tc.want)
+		}
+	}
+}
+
+// TestGeminiInteractions_PlainCallSendsNoExtras: no thinking, system or tool
+// fields on a plain call.
+func TestGeminiInteractions_PlainCallSendsNoExtras(t *testing.T) {
+	srv, c := interactionServer(t, interactionText)
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	_, _ = p.Generate(context.Background(), "hi")
+	_, body := c.last()
+	gen, _ := body["generation_config"].(map[string]any)
+	for _, k := range []string{"thinking_summaries", "thinking_level", "tool_choice"} {
+		if v, ok := gen[k]; ok {
+			t.Errorf("generation_config.%s = %v, want absent", k, v)
+		}
+	}
+	for _, k := range []string{"system_instruction", "tools", "previous_interaction_id"} {
+		if v, ok := body[k]; ok {
+			t.Errorf("%s = %v, want absent", k, v)
+		}
+	}
+}
+
+// TestGeminiInteractions_Status: incomplete is an *IncompleteError (as at
+// max_output_tokens), failed is retryable.
+func TestGeminiInteractions_Status(t *testing.T) {
+	srv, _ := interactionServer(t, `{"id":"v1_x","status":"incomplete","steps":[{"type":"model_output","content":[{"type":"text","text":"1, 2, "}]}]}`)
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	_, err := p.Generate(context.Background(), "count")
+	var inc *IncompleteError
+	if !errors.As(err, &inc) {
+		t.Errorf("incomplete: err = %v, want *IncompleteError", err)
+	}
+	srv2, _ := interactionServer(t, `{"id":"v1_y","status":"failed","steps":[]}`)
+	p2, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv2.URL))
+	if _, err := p2.Generate(context.Background(), "hi"); !errors.Is(err, ErrProviderUnavailable) {
+		t.Errorf("failed: err = %v, want ErrProviderUnavailable", err)
+	}
+}
+
+// TestGemini_ContinueNeedsStore: without WithStore(true) nothing is stored, so
+// Continue fails before any request.
+func TestGemini_ContinueNeedsStore(t *testing.T) {
+	srv, c := interactionServer(t, interactionText)
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	_, err := p.Continue(context.Background(), "v1_prev", MessageItem{Role: jsonRoleUser, Text: "more"})
+	if path, _ := c.last(); !errors.Is(err, ErrInvalidRequest) || path != "" {
+		t.Fatalf("err = %v after request %q, want ErrInvalidRequest and none", err, path)
+	}
+}
+
+// TestGemini_ContinueChainsWhenStored: with WithStore(true), Continue sends
+// only the new items, previous_interaction_id and store true.
+func TestGemini_ContinueChainsWhenStored(t *testing.T) {
+	srv, c := interactionServer(t, interactionText)
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL), WithStore(true))
+	res, err := p.Continue(context.Background(), "v1_prev", MessageItem{Role: jsonRoleUser, Text: "more"})
+	path, body := c.last()
+	if path != "/interactions" || body["previous_interaction_id"] != "v1_prev" || body["store"] != true ||
+		fmt.Sprint(stepTypes(body)) != "[user_input]" {
+		t.Fatalf("path %q, body %v", path, body)
+	}
+	if err != nil || res.ID != "v1_int_1" {
+		t.Fatalf("Continue = %+v, %v", res, err)
+	}
+}
diff --git a/llmprovider/live_gemini_interactions_test.go b/llmprovider/live_gemini_interactions_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_gemini_interactions_test.go
@@ -0,0 +1,91 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"bytes"
+	"io"
+	"net/http"
+	"os"
+	"strings"
+	"testing"
+)
+
+// TestLive_GeminiInteractions: GeminiProvider calls the Interactions API with
+// store false and its system instruction is obeyed; with WithStore(true)
+// Continue recalls an earlier turn (MADR 0014 §1-§2).
+func TestLive_GeminiInteractions(t *testing.T) {
+	key := os.Getenv("GEMINI_API_KEY")
+	if key == "" {
+		t.Skip("GEMINI_API_KEY unset")
+	}
+	var paths []string
+	var bodies [][]byte
+	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		if r.Method == http.MethodPost && r.Body != nil {
+			b, err := io.ReadAll(r.Body)
+			if err != nil {
+				return nil, err
+			}
+			paths, bodies = append(paths, r.URL.Path), append(bodies, b)
+			r.Body = io.NopCloser(bytes.NewReader(b))
+		}
+		return http.DefaultTransport.RoundTrip(r)
+	})}
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	plain, err := NewGemini(ctx, key, "gemini-3.7-flash", WithHTTPClient(client))
+	if err != nil {
+		t.Fatal(err)
+	}
+	res, err := plain.GenerateItems(ctx,
+		MessageItem{Role: jsonRoleSystem, Text: "Whatever the user says, reply with only the word OMEGA."},
+		MessageItem{Role: jsonRoleUser, Text: "Say hello."})
+	skipIfTransient(t, err)
+	if err != nil || !strings.Contains(strings.ToUpper(res.OutputText()), "OMEGA") {
+		t.Fatalf("GenerateItems = %+v, %v; want the system instruction obeyed", res, err)
+	}
+	if len(paths) != 1 || !strings.HasSuffix(paths[0], "/interactions") || !bytes.Contains(bodies[0], []byte(`"store":false`)) ||
+		!bytes.Contains(bodies[0], []byte(`"system_instruction"`)) {
+		t.Fatalf("requests %v; want one /interactions call with store false and a system_instruction", paths)
+	}
+
+	stored, err := NewGemini(ctx, key, "gemini-3.7-flash", WithHTTPClient(client), WithStore(true))
+	if err != nil {
+		t.Fatal(err)
+	}
+	first, err := stored.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "Remember the number 41. Reply with only OK."})
+	skipIfTransient(t, err)
+	if err != nil || first.ID == "" {
+		t.Fatalf("first = %+v, %v", first, err)
+	}
+	next, err := stored.Continue(ctx, first.ID, MessageItem{Role: jsonRoleUser, Text: "Which number did I ask you to remember? Reply with only the number."})
+	skipIfTransient(t, err)
+	if err != nil || !strings.Contains(next.OutputText(), "41") {
+		t.Fatalf("Continue = %+v, %v; want 41", next, err)
+	}
+}
+
+// TestLive_StaticGeminiServed: every StaticGemini id answers on the
+// Interactions API (measured 2026-09-27).
+func TestLive_StaticGeminiServed(t *testing.T) {
+	key := os.Getenv("GEMINI_API_KEY")
+	if key == "" {
+		t.Skip("GEMINI_API_KEY unset")
+	}
+	for _, model := range StaticGemini {
+		t.Run(model, func(t *testing.T) {
+			ctx, cancel := liveCtx(t)
+			defer cancel()
+			p, err := NewGemini(ctx, key, model)
+			if err != nil {
+				t.Fatal(err)
+			}
+			out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+			skipIfTransient(t, err)
+			if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
+				t.Fatalf("%s: Generate = %q, %v", model, out, err)
+			}
+		})
+	}
+}
```

**Fix** (`m2-fix.diff`, 744 lines):

```diff
diff --git a/llmprovider/gemini.go b/llmprovider/gemini.go
--- a/llmprovider/gemini.go
+++ b/llmprovider/gemini.go
@@ -9,15 +9,17 @@
 	"net/http"
 )
 
-// GeminiProvider implements Provider using the Google Gemini API via standard http client.
+// GeminiProvider implements Provider over the Gemini Interactions API
+// (gemini_interactions.go, MADR 0014). It stores no interaction unless
+// WithStore(true), which Continue requires.
 type GeminiProvider struct {
 	apiKey          string
 	model           string
 	baseURL         string // For testing
 	client          *http.Client
 	maxTokens       int
-	thinkingBudget  int    // thinkingConfig budget for the GenerateThinking path
-	reasoningEffort string // effort for the GenerateThinking path (see geminiThinkingConfig)
+	reasoningEffort string // effort for the GenerateThinking path (see geminiThinkingLevel)
+	store           bool   // WithStore(true): interactions are stored and Continue works
 	// identity names the client on every request (MADR 0012 §1.4).
 	identity clientIdentity
 }
@@ -40,19 +42,9 @@
 		client:          cfg.HTTPClient,
 		identity:        identityOf(cfg),
 		maxTokens:       cfg.MaxTokens,
-		thinkingBudget:  cfg.ThinkingBudget,
 		reasoningEffort: cfg.ReasoningEffort,
+		store:           cfg.Store != nil && *cfg.Store,
 	}, nil
-}
-
-// genConfig builds the generationConfig map, adding a thinkingConfig when the thinking
-// path is requested (see geminiThinkingConfig).
-func (p *GeminiProvider) genConfig(thinking bool) map[string]any {
-	cfg := map[string]any{"maxOutputTokens": p.maxTokens}
-	if thinking {
-		cfg["thinkingConfig"] = geminiThinkingConfig(p.model, p.reasoningEffort, p.thinkingBudget)
-	}
-	return cfg
 }
 
 // Name returns the provider's unique identifier "gemini".
@@ -126,8 +118,14 @@
 	return p.doGenerateItems(ctx, input, &tool, true, "")
 }
 
-// Continue sends items to the Gemini API, chaining from a previous interaction.
+// Continue sends the new items, chaining from a stored interaction. Gemini
+// requires store true to chain, so a provider made without WithStore(true)
+// returns ErrInvalidRequest without a request (MADR 0014 §2).
 func (p *GeminiProvider) Continue(ctx context.Context, previousInteractionID string, input ...Item) (*Response, error) {
+	if !p.store {
+		return nil, fmt.Errorf("%w: gemini: Continue needs stored interactions; create the provider "+
+			"WithStore(true), or replay the items", ErrInvalidRequest)
+	}
 	return p.doGenerateItems(ctx, input, nil, false, previousInteractionID)
 }
 
@@ -141,6 +139,8 @@
 	return map[string]any{"parts": []map[string]any{{jsonKeyText: system}}}
 }
 
+// geminiItemsToContents is generateContent's contents, for OpenCode's google
+// route; GeminiProvider uses interactionsInput.
 func geminiItemsToContents(items []Item) []map[string]any {
 	// Gemini pairs a functionResponse with its functionCall by name, so a
 	// result takes the name of the call it answers (MADR 0012 §2).
@@ -207,45 +207,12 @@
 }
 
 func (p *GeminiProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool, prevInteractionID string) (*Response, error) {
-	body := map[string]any{
-		"contents":         geminiItemsToContents(input),
-		"generationConfig": p.genConfig(thinking),
-	}
-	if system := geminiSystemInstruction(input); system != nil {
-		body["systemInstruction"] = system
-	}
-
-	if tool != nil {
-		body[jsonKeyTools] = []map[string]any{
-			{
-				"functionDeclarations": []map[string]any{
-					{
-						jsonKeyName:        tool.Name,
-						jsonKeyDescription: tool.Description,
-						jsonKeyParameters:  tool.Schema,
-					},
-				},
-			},
-		}
-		body["toolConfig"] = map[string]any{
-			"functionCallingConfig": map[string]any{
-				"mode":                 "ANY",
-				"allowedFunctionNames": []string{tool.Name},
-			},
-		}
-	}
-
-	if prevInteractionID != "" {
-		body["previous_interaction_id"] = prevInteractionID
-	}
-
-	reqBody, err := json.Marshal(body)
+	reqBody, err := json.Marshal(p.interactionsBody(input, tool, thinking, prevInteractionID))
 	if err != nil {
 		return nil, fmt.Errorf("gemini: marshal request: %w", err)
 	}
 
-	url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, p.model)
-	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
+	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+interactionsPath, bytes.NewReader(reqBody))
 	if err != nil {
 		return nil, err
 	}
@@ -268,14 +235,12 @@
 		return nil, err
 	}
 
-	return decodeGeminiResponse(limitedBody)
+	return decodeInteraction(limitedBody)
 }
 
 func decodeGeminiResponse(body io.Reader) (*Response, error) {
 	var raw struct {
-		ID            string `json:"id"`
-		InteractionID string `json:"interaction_id"`
-		Candidates    []struct {
+		Candidates []struct {
 			Content struct {
 				Parts []struct {
 					Text         string `json:"text"`
@@ -297,12 +262,7 @@
 		return nil, fmt.Errorf("gemini returned no content")
 	}
 
-	id := raw.ID
-	if id == "" {
-		id = raw.InteractionID
-	}
-
-	result := &Response{ID: id}
+	result := &Response{}
 	for _, part := range raw.Candidates[0].Content.Parts {
 		// A thought summary is flagged thought: true, with its text in text.
 		if part.Thought {
diff --git a/llmprovider/gemini_generatecontent_test.go b/llmprovider/gemini_generatecontent_test.go
--- a/llmprovider/gemini_generatecontent_test.go
+++ b/llmprovider/gemini_generatecontent_test.go
@@ -31,22 +31,18 @@
 	}
 }
 
-// geminiWireBodies returns the request bodies GeminiProvider and OpenCode's
-// google route send for one call.
+// geminiWireBodies returns the request body OpenCode's google route sends for
+// one call; GeminiProvider speaks the Interactions API (MADR 0014 §1).
 func geminiWireBodies(t *testing.T, thinking bool, items ...Item) map[string]map[string]any {
 	t.Helper()
 	out := map[string]map[string]any{}
 	var body map[string]any
 	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
-	gp, err := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
-	if err != nil {
-		t.Fatal(err)
-	}
 	op, err := NewOpencode(ProviderOpencodeZen, "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
 	if err != nil {
 		t.Fatal(err)
 	}
-	for name, p := range map[string]ItemThinkingProvider{"gemini": gp, "opencode-google": op} {
+	for name, p := range map[string]ItemThinkingProvider{"opencode-google": op} {
 		body = nil
 		if thinking {
 			_, err = p.GenerateItemsThinking(context.Background(), items...)
diff --git a/llmprovider/gemini_interactions.go b/llmprovider/gemini_interactions.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/gemini_interactions.go
@@ -0,0 +1,208 @@
+package llmprovider
+
+import (
+	"bytes"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"io"
+	"strings"
+)
+
+// GeminiProvider's wire is the Interactions API, POST {base}/interactions
+// (MADR 0014 §1). Every shape below was measured against gemini-3.7-flash on
+// 2026-09-27; OpenCode's google route keeps generateContent (gemini.go).
+const (
+	interactionsPath              = "/interactions"
+	interactionStepThought        = "thought"
+	interactionStepModelOutput    = "model_output"
+	interactionStepUserInput      = "user_input"
+	interactionStepFunctionCall   = "function_call"
+	interactionStepFunctionResult = "function_result"
+	interactionSummariesAuto      = "auto"
+)
+
+// interactionTextStep is a user_input or model_output step holding one text.
+func interactionTextStep(stepType, text string) map[string]any {
+	return map[string]any{jsonKeyType: stepType, jsonKeyContent: []map[string]any{{jsonKeyType: jsonKeyText, jsonKeyText: text}}}
+}
+
+// interactionsInput converts items to Interactions steps. System items go to
+// system_instruction instead (systemPrompt). A function call is preceded by
+// the thought step whose signature Gemini requires to replay it (a replayed
+// call without one is refused with 400), carrying the item's Signature or,
+// for a call Gemini did not issue, the placeholder it accepts. A result is
+// keyed by call_id and named after its call.
+func interactionsInput(items []Item) []map[string]any {
+	names := map[string]string{}
+	for _, item := range items {
+		if call, ok := item.(FunctionCallItem); ok {
+			names[call.CallID] = call.Name
+		}
+	}
+	var steps []map[string]any
+	for _, item := range items {
+		switch v := item.(type) {
+		case MessageItem:
+			switch v.Role {
+			case jsonRoleSystem:
+				continue
+			case "", jsonRoleUser:
+				steps = append(steps, interactionTextStep(interactionStepUserInput, v.Text))
+			default:
+				steps = append(steps, interactionTextStep(interactionStepModelOutput, v.Text))
+			}
+		case FunctionCallItem:
+			signature := v.Signature
+			if signature == "" {
+				signature = geminiSkipThoughtSignature
+			}
+			steps = append(steps,
+				map[string]any{jsonKeyType: interactionStepThought, "signature": signature},
+				map[string]any{jsonKeyType: interactionStepFunctionCall, "id": v.CallID, jsonKeyName: v.Name,
+					jsonKeyArguments: toolArguments(v.Arguments)})
+		case FunctionCallOutputItem:
+			name := names[v.CallID]
+			if name == "" {
+				name = v.CallID
+			}
+			steps = append(steps, map[string]any{jsonKeyType: interactionStepFunctionResult, jsonKeyCallID: v.CallID,
+				jsonKeyName: name, "result": v.Output})
+		}
+	}
+	return steps
+}
+
+// geminiThinkingLevel maps an effort to thinking_level: low, medium or high,
+// with xhigh sent as high; anything else is omitted, leaving the model's
+// default. gemini-2.5-flash-lite refuses medium and fails on low (measured
+// 2026-09-27), so it gets high or nothing.
+func geminiThinkingLevel(model, effort string) string {
+	level := strings.ToLower(effort)
+	switch level {
+	case effortXHigh:
+		level = effortHigh
+	case effortLow, effortMedium, effortHigh:
+	default:
+		return ""
+	}
+	if strings.HasPrefix(strings.ToLower(model), "gemini-2.5-flash-lite") && level != effortHigh {
+		return ""
+	}
+	return level
+}
+
+// interactionsBody builds one Interactions request (MADR 0014 §1). The API has
+// no thinking budget, so WithThinkingBudget does not apply here.
+func (p *GeminiProvider) interactionsBody(input []Item, tool *Tool, thinking bool, prevInteractionID string) map[string]any {
+	gen := map[string]any{"max_output_tokens": p.maxTokens}
+	if thinking {
+		// Without it a thought step carries only its signature.
+		gen["thinking_summaries"] = interactionSummariesAuto
+		if level := geminiThinkingLevel(p.model, p.reasoningEffort); level != "" {
+			gen["thinking_level"] = level
+		}
+	}
+	body := map[string]any{
+		jsonKeyModel:        p.model,
+		jsonKeyInput:        interactionsInput(input),
+		"store":             p.store,
+		"generation_config": gen,
+	}
+	if system := systemPrompt(input); system != "" {
+		body["system_instruction"] = system
+	}
+	if tool != nil {
+		body[jsonKeyTools] = []map[string]any{{
+			jsonKeyType:        jsonKeyFunction,
+			jsonKeyName:        tool.Name,
+			jsonKeyDescription: tool.Description,
+			jsonKeyParameters:  tool.Schema,
+		}}
+		// tool_choice belongs in generation_config; top level is refused.
+		gen["tool_choice"] = map[string]any{"allowed_tools": map[string]any{"mode": "any", "tools": []string{tool.Name}}}
+	}
+	if prevInteractionID != "" {
+		body["previous_interaction_id"] = prevInteractionID
+	}
+	return body
+}
+
+// decodeInteraction maps an Interaction to a Response: a thought step's
+// summary becomes a ReasoningItem and its signature the Signature of the
+// calls after it; model_output text becomes a MessageItem; function_call a
+// FunctionCallItem. incomplete is an *IncompleteError (MADR 0012 §1.5);
+// failed and cancelled are retryable failures.
+func decodeInteraction(body io.Reader) (*Response, error) {
+	var raw struct {
+		ID     string `json:"id"`
+		Status string `json:"status"`
+		Steps  []struct {
+			Type      string `json:"type"`
+			Signature string `json:"signature"`
+			Summary   []struct {
+				Type string `json:"type"`
+				Text string `json:"text"`
+			} `json:"summary"`
+			Content []struct {
+				Type string `json:"type"`
+				Text string `json:"text"`
+			} `json:"content"`
+			ID        string          `json:"id"`
+			Name      string          `json:"name"`
+			Arguments json.RawMessage `json:"arguments"`
+		} `json:"steps"`
+	}
+	if err := json.NewDecoder(body).Decode(&raw); err != nil {
+		return nil, fmt.Errorf("gemini: decode interaction: %w", err)
+	}
+	switch raw.Status {
+	case "completed", "requires_action":
+	case statusIncomplete:
+		// An Interaction gives no reason (measured at max_output_tokens).
+		return nil, &IncompleteError{Reason: statusIncomplete}
+	default:
+		return nil, fmt.Errorf("%w: gemini interaction %s", ErrProviderUnavailable, raw.Status)
+	}
+	result := &Response{ID: raw.ID}
+	signature := ""
+	for _, step := range raw.Steps {
+		switch step.Type {
+		case interactionStepThought:
+			signature = step.Signature
+			var sb strings.Builder
+			for _, s := range step.Summary {
+				sb.WriteString(s.Text)
+			}
+			if sb.Len() > 0 {
+				result.Output = append(result.Output, ReasoningItem{Text: sb.String()})
+			}
+		case interactionStepModelOutput:
+			var sb strings.Builder
+			for _, c := range step.Content {
+				if c.Type == jsonKeyText {
+					sb.WriteString(c.Text)
+				}
+			}
+			if sb.Len() > 0 {
+				result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: sb.String()})
+			}
+		case interactionStepFunctionCall:
+			// Compact, as the generateContent decoder returns arguments.
+			args := "{}"
+			if a := bytes.TrimSpace(step.Arguments); len(a) > 0 && string(a) != "null" {
+				var buf bytes.Buffer
+				if err := json.Compact(&buf, a); err != nil {
+					return nil, fmt.Errorf("gemini: function_call arguments: %w", err)
+				}
+				args = buf.String()
+			}
+			result.Output = append(result.Output, FunctionCallItem{CallID: step.ID, Name: step.Name,
+				Arguments: args, Signature: signature})
+		}
+	}
+	if len(result.Output) == 0 {
+		return nil, errors.New("gemini returned no content")
+	}
+	return result, nil
+}
diff --git a/llmprovider/gemini_items_test.go b/llmprovider/gemini_items_test.go
--- a/llmprovider/gemini_items_test.go
+++ b/llmprovider/gemini_items_test.go
@@ -8,16 +8,13 @@
 	"testing"
 )
 
-// TestGemini_GenerateItems_TextParts verifies text candidate parts are decoded to MessageItem.
+// TestGemini_GenerateItems_TextParts verifies a model_output step is decoded to MessageItem.
 func TestGemini_GenerateItems_TextParts(t *testing.T) {
 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
 		_, _ = w.Write([]byte(`{
-			"id": "gemini_resp_1",
-			"candidates": [{
-				"content": {
-					"parts": [{"text": "hello from gemini"}]
-				}
-			}]
+			"id": "v1_int_text",
+			"status": "completed",
+			"steps": [{"type": "model_output", "content": [{"type": "text", "text": "hello from gemini"}]}]
 		}`))
 	}))
 	defer srv.Close()
@@ -37,25 +34,18 @@
 	if resp.OutputText() != "hello from gemini" {
 		t.Errorf("OutputText() = %q, want %q", resp.OutputText(), "hello from gemini")
 	}
-	if resp.ID != "gemini_resp_1" {
-		t.Errorf("Response.ID = %q, want gemini_resp_1", resp.ID)
+	if resp.ID != "v1_int_text" {
+		t.Errorf("Response.ID = %q, want v1_int_text", resp.ID)
 	}
 }
 
-// TestGemini_GenerateItems_FunctionCallPart verifies functionCall candidate parts are decoded.
+// TestGemini_GenerateItems_FunctionCallPart verifies a function_call step is decoded.
 func TestGemini_GenerateItems_FunctionCallPart(t *testing.T) {
 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
 		_, _ = w.Write([]byte(`{
-			"candidates": [{
-				"content": {
-					"parts": [{
-						"functionCall": {
-							"name": "lookup",
-							"args": {"query": "weather"}
-						}
-					}]
-				}
-			}]
+			"id": "v1_int_call",
+			"status": "requires_action",
+			"steps": [{"type": "function_call", "id": "call_1", "name": "lookup", "arguments": {"query": "weather"}}]
 		}`))
 	}))
 	defer srv.Close()
@@ -70,7 +60,7 @@
 		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
 	}
 	fc, ok := resp.Output[0].(FunctionCallItem)
-	if !ok || fc.Name != "lookup" || fc.Arguments != `{"query":"weather"}` {
+	if !ok || fc.CallID != "call_1" || fc.Name != "lookup" || fc.Arguments != `{"query":"weather"}` {
 		t.Errorf("FunctionCallItem = %+v", resp.Output[0])
 	}
 
@@ -84,18 +74,16 @@
 	}
 }
 
-// TestGemini_GenerateItems_InterleavedParts verifies thought and text parts are decoded.
+// TestGemini_GenerateItems_InterleavedParts verifies thought and model_output steps are decoded.
 func TestGemini_GenerateItems_InterleavedParts(t *testing.T) {
 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
 		_, _ = w.Write([]byte(`{
-			"candidates": [{
-				"content": {
-					"parts": [
-						{"thought": true, "text": "pondering the question"},
-						{"text": "the solution is 42"}
-					]
-				}
-			}]
+			"id": "v1_int_thought",
+			"status": "completed",
+			"steps": [
+				{"type": "thought", "signature": "sig", "summary": [{"type": "text", "text": "pondering the question"}]},
+				{"type": "model_output", "content": [{"type": "text", "text": "the solution is 42"}]}
+			]
 		}`))
 	}))
 	defer srv.Close()
@@ -119,18 +107,19 @@
 	}
 }
 
-// TestGemini_Continue verifies previous_interaction_id is sent in request body.
+// TestGemini_Continue verifies a stored provider sends previous_interaction_id.
 func TestGemini_Continue(t *testing.T) {
 	var body map[string]any
-	srv := captureServer(t, &body, `{"id":"interaction_2","candidates":[{"content":{"parts":[{"text":"more text"}]}}]}`)
+	srv := captureServer(t, &body, `{"id":"interaction_2","status":"completed","steps":[`+
+		`{"type":"model_output","content":[{"type":"text","text":"more text"}]}]}`)
 
-	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL), WithStore(true))
 	resp, err := p.Continue(context.Background(), "interaction_1", MessageItem{Role: "user", Text: "go on"})
 	if err != nil {
 		t.Fatal(err)
 	}
-	if body["previous_interaction_id"] != "interaction_1" {
-		t.Errorf("previous_interaction_id = %v, want interaction_1", body["previous_interaction_id"])
+	if body["previous_interaction_id"] != "interaction_1" || body["store"] != true {
+		t.Errorf("previous_interaction_id = %v, store = %v; want interaction_1, true", body["previous_interaction_id"], body["store"])
 	}
 	if resp.ID != "interaction_2" {
 		t.Errorf("Response.ID = %q, want interaction_2", resp.ID)
@@ -153,7 +142,8 @@
 
 func TestGemini_GenerateItems_FunctionCallOutput(t *testing.T) {
 	var body map[string]any
-	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"received output"}]}}]}`)
+	srv := captureServer(t, &body, `{"id":"v1_int_out","status":"completed","steps":[`+
+		`{"type":"model_output","content":[{"type":"text","text":"received output"}]}]}`)
 
 	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
 	items := []Item{
diff --git a/llmprovider/gemini_test.go b/llmprovider/gemini_test.go
--- a/llmprovider/gemini_test.go
+++ b/llmprovider/gemini_test.go
@@ -16,7 +16,7 @@
 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
 		gotKeyHeader = r.Header.Get("x-goog-api-key")
 		gotRawQuery = r.URL.RawQuery
-		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
+		_, _ = w.Write([]byte(interactionText))
 	}))
 	defer srv.Close()
 
diff --git a/llmprovider/http_helpers.go b/llmprovider/http_helpers.go
--- a/llmprovider/http_helpers.go
+++ b/llmprovider/http_helpers.go
@@ -20,6 +20,10 @@
 	}
 }
 
+// statusIncomplete is the status of a truncated Responses or Interactions
+// answer.
+const statusIncomplete = "incomplete"
+
 // decodeResponsesAPIOutput decodes a Responses API JSON body into a Response.
 // Shared by providers using the Responses API envelope (OpenAI, Grok).
 func decodeResponsesAPIOutput(body io.Reader) (*Response, error) {
@@ -35,7 +39,7 @@
 		return nil, err
 	}
 	// A truncated answer is an error, never an empty success (MADR 0012 §1.5).
-	if raw.Status == "incomplete" {
+	if raw.Status == statusIncomplete {
 		return nil, incompleteResponse(raw.IncompleteDetails.Reason)
 	}
 
diff --git a/llmprovider/item.go b/llmprovider/item.go
--- a/llmprovider/item.go
+++ b/llmprovider/item.go
@@ -57,7 +57,8 @@
 // Response is the canonical result of an item-based generation call.
 type Response struct {
 	// ID is the provider-issued response identifier, used for server-side
-	// conversation chaining (OpenAI/xAI: response ID, Gemini: interaction ID).
+	// conversation chaining (OpenAI/xAI: response ID, Gemini: interaction ID,
+	// which can be continued only when stored; see WithStore).
 	// Empty for providers that do not support server-side state (Claude).
 	ID string
 
diff --git a/llmprovider/options.go b/llmprovider/options.go
--- a/llmprovider/options.go
+++ b/llmprovider/options.go
@@ -29,9 +29,10 @@
 	MaxTokens  int
 	BaseURL    string // For Ollama URL and test injection
 	// ThinkingBudget is the token budget for extended thinking / reasoning, used
-	// by the GenerateThinking paths of providers that reason via a token budget
-	// (Claude "thinking", Gemini "thinkingConfig"). Zero leaves the per-provider
-	// default in effect.
+	// by the GenerateThinking paths of wires that reason via a token budget
+	// (Claude "thinking", OpenCode's google route "thinkingConfig"). The Gemini
+	// provider's Interactions API has no budget (MADR 0014). Zero leaves the
+	// per-provider default in effect.
 	ThinkingBudget int
 	// ReasoningEffort selects the reasoning effort ("low"|"medium"|"high") for
 	// the GenerateThinking path of every provider. Effort APIs send it as is;
@@ -57,8 +58,8 @@
 	// KiloOrganization scopes Kilo requests to an organization; see
 	// WithKiloOrganization. Ignored by all other providers.
 	KiloOrganization string
-	// Store sets the Responses API store field for OpenAI API-key mode and
-	// Grok; nil leaves the service default. See WithStore.
+	// Store sets whether OpenAI API-key mode, Grok and Gemini store responses;
+	// nil leaves each provider's default. See WithStore.
 	Store *bool
 	// ModelProfile selects how the recommended models of the open catalogs
 	// (Kilo, OpenCode Zen and Go, Hugging Face) are ranked. The zero value is
@@ -103,8 +104,10 @@
 }
 
 // WithThinkingBudget sets the extended-thinking/reasoning token budget used by the
-// provider's GenerateThinking path (Claude, Gemini). A non-positive value leaves the
-// per-provider default in effect.
+// provider's GenerateThinking path (Claude, OpenCode's google route). The Gemini
+// provider ignores it: the Interactions API has no budget; use
+// WithReasoningEffort (MADR 0014). A non-positive value leaves the per-provider
+// default in effect.
 func WithThinkingBudget(n int) ProviderOption {
 	return func(cfg *ProviderConfig) {
 		cfg.ThinkingBudget = n
@@ -167,11 +170,12 @@
 	}
 }
 
-// WithStore sets whether OpenAI (API-key mode) and Grok store responses
-// (MADR 0012 §6). Without it the service default applies, which keeps
-// Continue working; callers under zero-data-retention pass false, after
-// which Continue has nothing to chain from. A ChatGPT session always sends
-// false. Ignored by all other providers.
+// WithStore sets whether OpenAI (API-key mode), Grok and Gemini store
+// responses (MADR 0012 §6, MADR 0014 §2). For OpenAI and Grok, without it
+// the service default applies, which keeps Continue working; callers under
+// zero-data-retention pass false, after which Continue has nothing to chain
+// from. Gemini stores nothing unless given true, and its Continue needs
+// true. A ChatGPT session always sends false. Ignored by all other providers.
 func WithStore(store bool) ProviderOption {
 	return func(cfg *ProviderConfig) {
 		cfg.Store = &store
diff --git a/llmprovider/provider_correctness_test.go b/llmprovider/provider_correctness_test.go
--- a/llmprovider/provider_correctness_test.go
+++ b/llmprovider/provider_correctness_test.go
@@ -59,18 +59,18 @@
 
 // TestGemini_WithMaxTokens is the regression for Gemini's generationConfig.
 func TestGemini_WithMaxTokens(t *testing.T) {
-	srv, body := bodyCapture(t, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
+	srv, body := bodyCapture(t, interactionText)
 	defer srv.Close()
 	p, _ := NewGemini(context.Background(), "k", "gemini-x", WithBaseURL(srv.URL), WithMaxTokens(123))
 	if _, err := p.Generate(context.Background(), "hi"); err != nil {
 		t.Fatalf("Generate: %v", err)
 	}
-	gc, ok := (*body)["generationConfig"].(map[string]any)
+	gc, ok := (*body)["generation_config"].(map[string]any)
 	if !ok {
-		t.Fatalf("generationConfig missing: %v", *body)
+		t.Fatalf("generation_config missing: %v", *body)
 	}
-	if mt, ok := gc["maxOutputTokens"].(float64); !ok || int(mt) != 123 {
-		t.Errorf("maxOutputTokens not sent: %v", gc["maxOutputTokens"])
+	if mt, ok := gc["max_output_tokens"].(float64); !ok || int(mt) != 123 {
+		t.Errorf("max_output_tokens not sent: %v", gc["max_output_tokens"])
 	}
 }
 
diff --git a/llmprovider/thinking_test.go b/llmprovider/thinking_test.go
--- a/llmprovider/thinking_test.go
+++ b/llmprovider/thinking_test.go
@@ -137,13 +137,14 @@
 	}
 }
 
-// TestGeminiThinking_RequestBody verifies a thinkingConfig is nested in generationConfig
-// with the configured budget, and that the default path omits it.
-func TestGeminiThinking_RequestBody(t *testing.T) {
+// TestGoogleRouteThinking_RequestBody verifies a thinkingConfig is nested in
+// generationConfig with the configured budget on OpenCode's google route, and
+// that the default path omits it. GeminiProvider has no budget (MADR 0014).
+func TestGoogleRouteThinking_RequestBody(t *testing.T) {
 	var body map[string]any
 	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
 
-	p, _ := NewGemini(context.Background(), "k", "gemini-x", WithBaseURL(srv.URL), WithThinkingBudget(1234))
+	p, _ := NewOpencode(ProviderOpencodeZen, "k", "gemini-x", WithBaseURL(srv.URL), WithThinkingBudget(1234))
 	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
 		t.Fatal(err)
 	}
@@ -170,12 +171,13 @@
 	}
 }
 
-// TestGeminiThinking_DynamicBudgetDefault verifies an unset budget maps to -1 (dynamic).
-func TestGeminiThinking_DynamicBudgetDefault(t *testing.T) {
+// TestGoogleRouteThinking_DynamicBudgetDefault verifies an unset budget maps to
+// -1 (dynamic) on OpenCode's google route.
+func TestGoogleRouteThinking_DynamicBudgetDefault(t *testing.T) {
 	var body map[string]any
 	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
 
-	p, _ := NewGemini(context.Background(), "k", "gemini-x", WithBaseURL(srv.URL))
+	p, _ := NewOpencode(ProviderOpencodeZen, "k", "gemini-x", WithBaseURL(srv.URL))
 	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
 		t.Fatal(err)
 	}
@@ -207,9 +209,10 @@
 
 func TestGeminiThinkingTool(t *testing.T) {
 	var body map[string]any
-	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"q":"test"}}}]}}]}`)
-
-	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL), WithThinkingBudget(1000))
+	srv := captureServer(t, &body, `{"id":"v1_t","status":"requires_action","steps":[{"type":"thought","signature":"s"},`+
+		`{"type":"function_call","id":"c1","name":"lookup","arguments":{"q":"test"}}]}`)
+
+	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL), WithReasoningEffort(effortHigh))
 	tool := Tool{Name: "lookup", Description: "lookup", Schema: map[string]any{"type": "object"}}
 	args, err := p.GenerateWithToolThinking(context.Background(), "hi", tool)
 	if err != nil {
@@ -218,10 +221,9 @@
 	if args != `{"q":"test"}` {
 		t.Errorf("expected '{\"q\":\"test\"}', got %q", args)
 	}
-	gc := body["generationConfig"].(map[string]any)
-	tc := gc["thinkingConfig"].(map[string]any)
-	if b := tc["thinkingBudget"].(float64); int(b) != 1000 {
-		t.Errorf("thinkingBudget = %v, want 1000", b)
+	gc, _ := body["generation_config"].(map[string]any)
+	if gc["thinking_level"] != effortHigh || gc["tool_choice"] == nil {
+		t.Errorf("generation_config = %v, want thinking_level high and a tool_choice", gc)
 	}
 }
 
diff --git a/llmprovider/thinking_wire_test.go b/llmprovider/thinking_wire_test.go
--- a/llmprovider/thinking_wire_test.go
+++ b/llmprovider/thinking_wire_test.go
@@ -63,9 +63,10 @@
 	}
 }
 
-// TestThinkingWire_Gemini pins MADR 0013 Q1 on the Gemini wire: "low" is
-// thinkingLevel on Gemini 3 and later and a 1024 budget on 2.x (thinkingLevel
-// is HTTP 400 there); other efforts keep the dynamic budget; a budget wins.
+// TestThinkingWire_Gemini pins MADR 0013 Q1 on the generateContent wire, which
+// only OpenCode's google route speaks since MADR 0014: "low" is thinkingLevel
+// on Gemini 3 and later and a 1024 budget on 2.x (thinkingLevel is HTTP 400
+// there); other efforts keep the dynamic budget; a budget wins.
 func TestThinkingWire_Gemini(t *testing.T) {
 	for _, tc := range []thinkingCase{
 		{"gemini-3.7-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"includeThoughts": true, "thinkingLevel": "low"}}},
@@ -77,7 +78,7 @@
 		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
 			var body map[string]any
 			srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
-			p, err := NewGemini(context.Background(), "k", tc.model, WithBaseURL(srv.URL),
+			p, err := NewOpencode(ProviderOpencodeZen, "k", tc.model, WithBaseURL(srv.URL),
 				WithReasoningEffort(tc.effort), WithThinkingBudget(tc.budget))
 			if err != nil {
 				t.Fatal(err)
```
