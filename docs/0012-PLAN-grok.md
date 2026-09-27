---
status: complete
date: 2026-09-27
associated-madr: "0012-MADR-conform-providers-to-reference-clients.md"
decision-makers: mcplib maintainers
---

# Implement 0012 §6 — Grok

Associated MADR: [0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
(accepted 2026-09-27, revision 3). This is the last of that MADR's six plans
(§8).

This plan executes MADR §6, and nothing else. If a fact contradicts the MADR
or this plan, **stop and prompt**. Add a dated entry to §9 of this plan,
amend the MADR when a decision or an asserted fact changes, and only then
continue.

## How this plan was proven

Every phase below was executed on 2026-09-27, in scratch copies of
`git archive 7ea0ad4` with the §2–§5 plans applied:
* **Red.** Each phase's tests diff was applied, and each named test was seen
  to fail on the unfixed code, including live tests against xAI.
* **Green.** The fix diff was applied, and the full gate (§0.2) passed.
* **Mutants.** A guard that holds today, or a test of new API, was seen to
  fail on a named mutant.
* **Diffs.** Appendix B's diffs were generated mechanically from that proof.
  Applying all five plans' diffs in §8's order to a fresh `7ea0ad4` archive
  reproduces the proven tree byte for byte: 308 files, 0 mismatches.

The reference is the Grok CLI at `grok-build` `f0e3be11`. xAI accepted every
effort on `grok-4.5`, `grok-4.6` and `grok-3-mini` on 2026-09-27, so the
menus are conformance to the CLI, not a failure avoided (MADR revision 3).

## Goal

* **Effort menus.** `reasoning_effort` follows the CLI's per-model menus:
  * `grok-4.6`: `xhigh`, `high`, `medium`, `low`;
  * `grok-4.5`: `high`, `medium`, `low`;
  * `grok-4.6-build` and `grok-3-mini*`: `low`, `high`;
  * nothing else.
* **Catalog.** Media models are not usable, and `StaticGrok` leads with the
  CLI's defaults.
* **Tool description.** Grok's tool definition sends it.
* **`WithStore(bool)`** sets `store` on the Responses providers.

## Scope

**In scope:** MADR §6: K1–K3 below.

**Out of scope:** `grok-4.7`, which xAI now lists but the CLI catalog at
`f0e3be11` does not. It gets no effort, per the menu rule.

## 0. Preconditions and conventions

### 0.1 Baseline

* `0012-PLAN-oauth-hygiene.md` complete on `main`, O6 aside. The diffs were
  generated after O6. K1–K3 touch no line O6 changes, so they apply either
  way; a rebase is a §9 deviation.
* `go test -count=1 ./...` passes before K1.

### 0.2 Gate (every phase)

1. `gofmt -l` prints nothing, and `golint -set_exit_status` passes, on each
   `.go` file the phase touched.
2. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
3. `make lint`.
4. `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

### 0.3 Red first

As in `0012-PLAN-item-fidelity.md` §0.3.

### 0.4 Credentials for live steps

`XAI_API_KEY`. `TestLive_ResponsesStoreFalse` also reads `OPENAI_API_KEY`,
and skips its OpenAI case without credit.

### 0.5 Commits

One `git commit --no-edit` per phase, after the gate. No push and no tag.

## Phase K0 — Start

1. Confirm §0.1.
2. Set this plan to `status: in-progress`.
3. Commit the documents only.

## Phase K1 — Exact effort menus (§6)

**Today.** `grokReasoningSupport` (`grok_reasoning.go`) grants the full
range to any `grok-4.5*` or `grok-4.6*` prefix, and clamps `grok-3-mini`'s
`medium` up to `high`.

**Changes** (Appendix B.K1):
* **Menus.** `grokEffortMenu` returns the CLI's exact menus
  (`xai-grok-models/default_models.json`). `grok-4.6-build` takes `low` and
  `high`, as the CLI's config tests pin
  (`xai-grok-shell/src/agent/config_tests.rs:7958`).
* **Clamping.** `grokClampReasoningEffort` handles three cases:
  * no effort sends `high`;
  * an effort off the menu clamps to the nearest lower one;
  * failing that, the menu's highest.
* **Removed:** the tier type.
* **Test updates.** The tier tests are replaced, and the pinned
  `grok-3-mini` `medium` → `high` expectation becomes `low`.

**Verification.**
* Red: `TestGrokEffortMenus_ClampToCLIMenu`, and live
  `TestLive_GrokEffortMenu`, which sent `xhigh` for `grok-4.5`.
* The menu-value guard is proven by two mutants.

## Phase K2 — Media models out, CLI defaults first (§6)

**Today.**
* `isUsableGrokModel` admits `grok-imagine-video` and
  `grok-imagine-video-1.5`, which xAI lists.
* `StaticGrok` leads with `grok-3-mini-fast`.

**Changes** (Appendix B.K2):
* The deny list gains `imagine`, `video`, `voice`, `stt` and `tts`.
* `StaticGrok` starts `grok-4.6`, `grok-4.5`.

**Verification.**
* Red: two unit tests, and live `TestLive_GrokListingTextOnly`.
* The text-model guard, every text model xAI listed, is proven by a mutant.

## Phase K3 — Tool description and `WithStore` (§6)

**Today.**
* Grok's tool definition omits `description`.
* No option sets `store`.

**Changes** (Appendix B.K3):
* Grok's tool definition sends `description`.
* **`WithStore(bool)`:**
  * `ProviderConfig.Store` holds it;
  * OpenAI API-key mode and Grok send it when set;
  * a ChatGPT session always sends `false`.

**Verification.**
* Red: `TestGrok_SendsToolDescription`, and live
  `TestLive_GrokToolDescription`.
* `TestWithStore_ResponsesProviders` is proven by three mutants.
* Live, `WithStore(false)` works on xAI. The OpenAI API-key case skipped:
  the platform key had no credit (429 `credit_balance_exhausted`, typed as
  `ErrQuotaExhausted`).

## Phase K4 — Records and close-out

1. Record each phase's result in §10.
2. Set this plan to `status: complete` once §7 holds.
3. Commit the documents only.

## 7. Acceptance criteria

* Every Appendix A red test fails before its fix and passes after it.
* Every mutant is killed.
* The gate passes after each phase.
* The Grok live tests pass.
* The MADR's §6 confirmation holds: `grok-4.5` with `xhigh` sends `high`, and
  `grok-imagine-video-1.5` is not usable.

## 8. Rollout and rollback

* **Behaviour changes:**
  * `grok-3-mini` with `medium` now sends `low`;
  * `grok-4.5` with `xhigh` sends `high`;
  * media models leave Grok menus.
* **Additive API:** `WithStore`.
* **Rollback:** revert the phase commit. The phases are independent.

## 9. Deviation log

None yet.

## 10. Execution record

Executed on `main`, 2026-09-27, after `0012-PLAN-circuit-breaker-test.md`
(`05a1fcf`). Each phase applied Appendix B with `git apply`, taken from this
document, and checked equal to the proven diff.

* **K1**, commit `ddae325`: 2 red tests failed as required; 1 base guards passed; gate passed; 2/2 mutants killed.
* **K2**, commit `1362920`: 3 red tests failed as required; 1 base guards passed; gate passed; 1/1 mutants killed.
* **K3**, commit `8db54ac`: 2 red tests failed as required; 0 base guards passed; gate passed; 3/3 mutants killed.

**Live** (`-tags live_gateways`, this plan's tests, on the executed tree): 8 passed, 1 skipped, 0 failed.
* Skip: `TestLive_ResponsesStoreFalse/openai`: the OpenAI platform key has no credit (429 `credit_balance_exhausted`, typed as `ErrQuotaExhausted`).

## Appendix A — Proof record (2026-09-27)

### A.K1 Phase K1 — Exact effort menus (§6)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestGrokEffortMenus_ClampToCLIMenu` | unit | grokClampReasoningEffort("grok-4.5", "xhigh") = "xhigh", want "high" |
| `TestLive_GrokEffortMenu` | live | request did not clamp xhigh to high: {"input":[{"content":"Reply with only the word ALPHA","role":"user"}],"max_output_tokens":8192,"model":"grok-4.5","reasoning":{"effort":"xhigh"}} |

**Guards before the fix:**

* `TestGrokEffortMenus_KeepMenuValues`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 5 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gk1-46-no-xhigh` | `TestGrokEffortMenus_KeepMenuValues` | killed | grokClampReasoningEffort("grok-4.6", "xhigh") = "high", want "xhigh" |
| `gk1-empty-is-highest` | `TestGrokEffortMenus_KeepMenuValues` | killed | grokClampReasoningEffort("grok-4.6", "") = "xhigh", want "high" |

### A.K2 Phase K2 — Media models out, CLI defaults first (§6)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestIsUsableGrokModel_RejectsMedia` | unit | isUsableGrokModel("grok-imagine-video") = true, want false |
| `TestStaticGrok_LeadsWithCLIDefaults` | unit | StaticGrok = [grok-3-mini-fast grok-3-mini grok-4 grok-4.5 grok-4.6 grok-4-fast-reasoning], want it to lead with grok-4.6, grok-4.5 |
| `TestLive_GrokListingTextOnly` | live | usable "grok-imagine-video" is a media model |

**Guards before the fix:**

* `TestIsUsableGrokModel_KeepsTextModels`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 3 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gk2-deny-too-broad` | `TestIsUsableGrokModel_KeepsTextModels` | killed | isUsableGrokModel("grok-4.20-0309-non-reasoning") = false, want true |

### A.K3 Phase K3 — Tool description and `WithStore` (§6)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestGrok_SendsToolDescription` | unit | tools = [map[name:get_weather parameters:map[type:object] type:function]], want the description |
| `TestLive_GrokToolDescription` | live | request did not send the tool description: {"input":[{"content":"What is the weather in Paris?","role":"user"}],"max_output_tokens":8192,"model":"grok-4.6","tool_choice":{"name":"get_weather","type":"function"},"tools":[ |

**Guards before the fix:**

* `TestWithStore_ResponsesProviders`: added with the fix.
* `TestLive_ResponsesStoreFalse`: added with the fix.

**Gate** (fix applied): all passed — per-file `golint` on 8 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `gk3-grok-ignores-store` | `TestWithStore_ResponsesProviders` | killed | store = <nil> (present false), want false (present true) |
| `gk3-openai-ignores-store` | `TestWithStore_ResponsesProviders` | killed | store = <nil> (present false), want false (present true) |
| `gk3-chatgpt-honours-store` | `TestWithStore_ResponsesProviders` | killed | store = true, want false |

### A.L Live runs on this plan's final tree (`-tags live_gateways`, 2026-09-27)

| Test | Result |
|---|---|
| `TestLive_GrokListingTextOnly` | PASS |
| `TestLive_GrokEffortMenu` | PASS |
| `TestLive_GrokToolDescription` | PASS |
| `TestLive_ToolRoundTrip` | PASS |
| `TestLive_ToolRoundTrip/grok` | PASS |
| `TestLive_GrokDiscoveryPublishesRevocation` | PASS |
| `TestLive_ResponsesStoreFalse` | PASS |
| `TestLive_ResponsesStoreFalse/openai` | SKIP |
| `TestLive_ResponsesStoreFalse/grok` | PASS |

* Skip reason: gateway transient (rate limit / outage / quota / not permitted): llm: quota exhausted: openai HTTP 429 credit_balance_exhausted: You have no credits remaining. Add credits to continue using the API at https://platform.openai.com/settings/organization/billing/.

Totals: 8 passed, 1 skipped, 0 failed.

### A.S The whole live suite on the final tree (all five plans)

| Test | Result |
|---|---|
| `TestListModelCatalog_OllamaEmptyIsLive` | PASS |
| `TestListModelCatalog_LiveFlag` | PASS |
| `TestListModelCatalog_LiveFlag/gemini` | PASS |
| `TestListModelCatalog_LiveFlag/openai` | PASS |
| `TestListModelCatalog_LiveFlag/claude` | PASS |
| `TestListModelCatalog_LiveFlag/grok` | PASS |
| `TestListModelCatalog_LiveFlag/opencode-zen` | PASS |
| `TestListModelCatalog_LiveFlag/opencode-go` | PASS |
| `TestListModelCatalog_LiveFlag/huggingface` | PASS |
| `TestListModelCatalog_LiveFlag/kilo` | PASS |
| `TestLive_ChatGPTErrorDetail` | PASS |
| `TestLive_ChatGPTListingVersion` | PASS |
| `TestLive_ChatGPTGenerate` | PASS |
| `TestLive_OpencodeChatCompletions` | PASS |
| `TestLive_OpencodeResponses` | PASS |
| `TestLive_OpencodeRouteStillEnforced` | PASS |
| `TestLive_KiloChatCompletions` | PASS |
| `TestLive_KiloToolCall` | SKIP |
| `TestLive_KiloReasoningSpelling` | SKIP |
| `TestLive_KiloSupportedParameters` | PASS |
| `TestLive_HuggingFaceMetadataFields` | PASS |
| `TestLive_HuggingFaceChatCompletions` | PASS |
| `TestLive_ListingsNeedNoCredential` | PASS |
| `TestLive_ListingsNeedNoCredential/opencode-zen` | PASS |
| `TestLive_ListingsNeedNoCredential/opencode-go` | PASS |
| `TestLive_ListingsNeedNoCredential/huggingface` | PASS |
| `TestLive_ListingsNeedNoCredential/kilo` | PASS |
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
| `TestLive_VendorCLISession/openai` | PASS |
| `TestLive_VendorCLISession/grok` | PASS |

* Skip reason: gateway transient (rate limit / outage / quota / not permitted): llm: rate limited: kilo HTTP 429 (retry-after 0s): Provider returned error
* Skip reason: gateway returned HTTP 429 (free tier limits)
* Skip reason: MCPLIB_LIVE_BROWSER_LOGIN unset: this needs a person to sign in in a browser
* Skip reason: gateway transient (rate limit / outage / quota / not permitted): llm: quota exhausted: openai HTTP 429 credit_balance_exhausted: You have no credits remaining. Add credits to continue using the API at https://platform.openai.com/settings/organization/billing/.

Totals: 82 passed, 4 skipped, 0 failed.


## Appendix B — Diffs

Generated from the proof. Apply each phase's **Tests** diff, then its **Fix**
diff, with `git apply`, in order, on the §0.1 baseline.

### B.K1 Phase K1 — Exact effort menus (§6)

**Tests** (`gk1-tests.diff`, 108 lines):

```diff
diff --git a/llmprovider/grok_effort_menu_test.go b/llmprovider/grok_effort_menu_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/grok_effort_menu_test.go
@@ -0,0 +1,43 @@
+package llmprovider
+
+import "testing"
+
+type grokEffortCase struct{ model, effort, want string }
+
+func checkGrokEfforts(t *testing.T, cases []grokEffortCase) {
+	t.Helper()
+	for _, tc := range cases {
+		if got := grokClampReasoningEffort(tc.model, tc.effort); got != tc.want {
+			t.Errorf("grokClampReasoningEffort(%q, %q) = %q, want %q", tc.model, tc.effort, got, tc.want)
+		}
+	}
+}
+
+// TestGrokEffortMenus_ClampToCLIMenu: the Grok CLI offers grok-4.5 only
+// high/medium/low and grok-4.6-build and grok-3-mini only low/high
+// (grok-build f0e3be11: xai-grok-models/default_models.json;
+// xai-grok-shell/src/agent/config_tests.rs:7958).
+func TestGrokEffortMenus_ClampToCLIMenu(t *testing.T) {
+	checkGrokEfforts(t, []grokEffortCase{
+		{"grok-4.5", "xhigh", "high"},
+		{"grok-4.6-build", "xhigh", "high"},
+		{"grok-4.6-build", "medium", "low"},
+		{"grok-3-mini", "medium", "low"},
+		{"grok-3-mini-fast", "medium", "low"},
+	})
+}
+
+// TestGrokEffortMenus_KeepMenuValues: an effort on the menu is sent as is,
+// no effort sends high, and a model without a menu sends none.
+func TestGrokEffortMenus_KeepMenuValues(t *testing.T) {
+	checkGrokEfforts(t, []grokEffortCase{
+		{"grok-4.6", "xhigh", "xhigh"},
+		{"grok-4.6", "medium", "medium"},
+		{"grok-4.5", "low", "low"},
+		{"grok-4.5", "", "high"},
+		{"grok-4.6", "", "high"},
+		{"grok-3-mini", "high", "high"},
+		{"grok-4", "high", ""},
+		{"grok-4.7", "high", ""},
+	})
+}
diff --git a/llmprovider/live_grok_effort_test.go b/llmprovider/live_grok_effort_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_grok_effort_test.go
@@ -0,0 +1,55 @@
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
+// xaiRecordingClient records the last POST body sent to api.x.ai.
+func xaiRecordingClient(sent *[]byte) *http.Client {
+	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		if r.Method == http.MethodPost && r.Body != nil {
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
+// TestLive_GrokEffortMenu: grok-4.5 asked for xhigh sends the CLI menu's high
+// and answers. xAI accepted every effort on grok-4.5 on 2026-09-27, so this
+// proves conformance to the CLI, not a rejection avoided.
+func TestLive_GrokEffortMenu(t *testing.T) {
+	key := os.Getenv("XAI_API_KEY")
+	if key == "" {
+		t.Skip("XAI_API_KEY unset")
+	}
+	var sent []byte
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewGrok(key, "grok-4.5", WithReasoningEffort(effortXHigh), WithHTTPClient(xaiRecordingClient(&sent)))
+	if err != nil {
+		t.Fatal(err)
+	}
+	out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("GenerateThinking: %v", err)
+	}
+	if !bytes.Contains(sent, []byte(`"reasoning":{"effort":"high"}`)) {
+		t.Fatalf("request did not clamp xhigh to high: %s", sent)
+	}
+	if !strings.Contains(strings.ToUpper(out), "ALPHA") {
+		t.Errorf("reply %q", out)
+	}
+}
```

**Fix** (`gk1-fix.diff`, 197 lines):

```diff
diff --git a/llmprovider/grok_reasoning.go b/llmprovider/grok_reasoning.go
--- a/llmprovider/grok_reasoning.go
+++ b/llmprovider/grok_reasoning.go
@@ -1,63 +1,61 @@
 package llmprovider
 
-import "strings"
-
-// reasoningSupport enumerates gating tiers for Grok reasoning_effort.
-type reasoningSupport int
-
-const (
-	// reasoningUnsupported: model reasons automatically, sending reasoning_effort
-	// causes a 400 Bad Request. Applies to: grok-3, grok-4, grok-4-fast-reasoning,
-	// grok-code-fast-1.
-	reasoningUnsupported reasoningSupport = iota
-
-	// reasoningLowHigh: model accepts reasoning_effort with only "low" or "high".
-	// Applies to: grok-3-mini, grok-3-mini-fast.
-	reasoningLowHigh
-
-	// reasoningFull: model accepts reasoning_effort with "low"/"medium"/"high"/"xhigh".
-	// Reasoning cannot be disabled. Applies to: grok-4.5, grok-4.6.
-	reasoningFull
+import (
+	"slices"
+	"strings"
 )
 
-// grokReasoningSupport returns the reasoning-effort gating tier for a Grok model.
-// Unknown models default to reasoningUnsupported (safest: omit the parameter).
-func grokReasoningSupport(model string) reasoningSupport {
+// The Grok CLI catalog's models (MADR 0012 §6).
+const (
+	grokModel46 = "grok-4.6"
+	grokModel45 = "grok-4.5"
+)
+
+// grokEffortOrder ranks the efforts, lowest first.
+var grokEffortOrder = []string{effortLow, effortMedium, effortHigh, effortXHigh}
+
+// grokEffortMenu returns the reasoning efforts the Grok CLI offers a model,
+// lowest first, or nil when reasoning_effort must be omitted (MADR 0012 §6).
+// grok-4.6 and grok-4.5 are the CLI catalog's entries (grok-build f0e3be11:
+// xai-grok-models/default_models.json); grok-4.6-build takes low and high
+// (xai-grok-shell/src/agent/config_tests.rs:7958), as does grok-3-mini. Other
+// models reason automatically and may reject the parameter.
+func grokEffortMenu(model string) []string {
 	sm := strings.ToLower(model)
 	switch {
-	// grok-3-mini family: low/high only
-	case strings.HasPrefix(sm, "grok-3-mini"):
-		return reasoningLowHigh
-	// grok-4.5 / grok-4.6 family: full range, always-on reasoning
-	case strings.HasPrefix(sm, "grok-4.5"), strings.HasPrefix(sm, "grok-4.6"):
-		return reasoningFull
-	// grok-3, grok-4, grok-4-fast-reasoning, grok-code-fast-1: unsupported
+	case sm == grokModel46:
+		return []string{effortLow, effortMedium, effortHigh, effortXHigh}
+	case sm == grokModel45:
+		return []string{effortLow, effortMedium, effortHigh}
+	case sm == "grok-4.6-build", strings.HasPrefix(sm, "grok-3-mini"):
+		return []string{effortLow, effortHigh}
 	default:
-		return reasoningUnsupported
+		return nil
 	}
 }
 
 // grokClampReasoningEffort returns the reasoning_effort value to include in the
-// request body, or "" if the parameter must be omitted entirely.
+// request body, or "" if the parameter must be omitted entirely. No effort
+// sends high, every menu's CLI default. An effort off the menu clamps to the
+// nearest lower one on it, else the menu's highest.
 func grokClampReasoningEffort(model, effort string) string {
-	tier := grokReasoningSupport(model)
-	switch tier {
-	case reasoningUnsupported:
-		return "" // must not send
-	case reasoningLowHigh:
-		switch strings.ToLower(effort) {
-		case effortLow:
-			return effortLow
-		default:
-			return effortHigh // clamp everything else to "high"
-		}
-	case reasoningFull:
-		switch strings.ToLower(effort) {
-		case effortLow, effortMedium, effortHigh, effortXHigh:
-			return strings.ToLower(effort)
-		default:
-			return effortHigh // default for full-range models
+	menu := grokEffortMenu(model)
+	if len(menu) == 0 {
+		return ""
+	}
+	want := strings.ToLower(effort)
+	if want == "" {
+		return effortHigh
+	}
+	rank := slices.Index(grokEffortOrder, want)
+	clamped := ""
+	for _, e := range menu {
+		if slices.Index(grokEffortOrder, e) <= rank {
+			clamped = e
 		}
 	}
-	return ""
+	if clamped == "" {
+		return menu[len(menu)-1]
+	}
+	return clamped
 }
diff --git a/llmprovider/grok_reasoning_test.go b/llmprovider/grok_reasoning_test.go
--- a/llmprovider/grok_reasoning_test.go
+++ b/llmprovider/grok_reasoning_test.go
@@ -2,50 +2,15 @@
 
 import "testing"
 
-func TestGrokReasoningSupport(t *testing.T) {
-	tests := []struct {
-		model string
-		want  reasoningSupport
-	}{
-		{"grok-3-mini", reasoningLowHigh},
-		{"grok-3-mini-fast", reasoningLowHigh},
-		{"grok-4.5", reasoningFull},
-		{"grok-4.6", reasoningFull},
-		{"grok-3", reasoningUnsupported},
-		{"grok-4", reasoningUnsupported},
-		{"grok-4-fast-reasoning", reasoningUnsupported},
-		{"grok-code-fast-1", reasoningUnsupported},
-		{"unknown-model", reasoningUnsupported},
-	}
-	for _, tc := range tests {
-		if got := grokReasoningSupport(tc.model); got != tc.want {
-			t.Errorf("grokReasoningSupport(%q) = %d, want %d", tc.model, got, tc.want)
-		}
-	}
-}
-
+// TestGrokClampReasoningEffort pins models without a CLI menu: they send no
+// reasoning_effort (MADR 0012 §6). The menus are pinned by
+// grok_effort_menu_test.go.
 func TestGrokClampReasoningEffort(t *testing.T) {
-	tests := []struct {
-		model, effort, want string
-	}{
-		// unsupported: must not send
+	checkGrokEfforts(t, []grokEffortCase{
 		{"grok-3", "high", ""},
 		{"grok-4", "medium", ""},
 		{"grok-4-fast-reasoning", "low", ""},
-		// low/high only (grok-3-mini)
-		{"grok-3-mini", "medium", "high"},
-		{"grok-3-mini", "low", "low"},
-		{"grok-3-mini", "high", "high"},
-		{"grok-3-mini-fast", "xhigh", "high"},
-		// full range (grok-4.5, grok-4.6)
-		{"grok-4.5", "xhigh", "xhigh"},
-		{"grok-4.5", "", "high"},
-		{"grok-4.6", "medium", "medium"},
-		{"grok-4.6", "low", "low"},
-	}
-	for _, tc := range tests {
-		if got := grokClampReasoningEffort(tc.model, tc.effort); got != tc.want {
-			t.Errorf("grokClampReasoningEffort(%q, %q) = %q, want %q", tc.model, tc.effort, got, tc.want)
-		}
-	}
+		{"grok-code-fast-1", "high", ""},
+		{"unknown-model", "high", ""},
+	})
 }
diff --git a/llmprovider/grok_test.go b/llmprovider/grok_test.go
--- a/llmprovider/grok_test.go
+++ b/llmprovider/grok_test.go
@@ -138,7 +138,7 @@
 }
 
 // TestGrok_GenerateThinking_ReasoningEffortClamped verifies grok-3-mini clamps
-// "medium" to "high" (only low/high supported).
+// "medium" to the nearest lower effort on its low/high menu (MADR 0012 §6).
 func TestGrok_GenerateThinking_ReasoningEffortClamped(t *testing.T) {
 	var body map[string]any
 	srv := captureServer(t, &body, `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)
@@ -148,8 +148,8 @@
 		t.Fatal(err)
 	}
 	reasoning := body["reasoning"].(map[string]any)
-	if reasoning["effort"] != "high" {
-		t.Errorf("reasoning.effort = %v, want high (clamped from medium)", reasoning["effort"])
+	if reasoning["effort"] != "low" {
+		t.Errorf("reasoning.effort = %v, want low (clamped from medium)", reasoning["effort"])
 	}
 }
 
```

### B.K2 Phase K2 — Media models out, CLI defaults first (§6)

**Tests** (`gk2-tests.diff`, 78 lines):

```diff
diff --git a/llmprovider/grok_catalog_test.go b/llmprovider/grok_catalog_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/grok_catalog_test.go
@@ -0,0 +1,37 @@
+package llmprovider
+
+import (
+	"slices"
+	"testing"
+)
+
+// TestIsUsableGrokModel_RejectsMedia: xAI lists grok-imagine-video and
+// grok-imagine-video-1.5 (2026-09-27), which "image" does not match.
+func TestIsUsableGrokModel_RejectsMedia(t *testing.T) {
+	for _, id := range []string{"grok-imagine-video", "grok-imagine-video-1.5", "grok-imagine-image",
+		"grok-voice-1", "grok-stt-1", "grok-tts-1"} {
+		if isUsableGrokModel(id) {
+			t.Errorf("isUsableGrokModel(%q) = true, want false", id)
+		}
+	}
+}
+
+// TestIsUsableGrokModel_KeepsTextModels: every text model xAI listed on
+// 2026-09-27 stays usable.
+func TestIsUsableGrokModel_KeepsTextModels(t *testing.T) {
+	for _, id := range []string{"grok-4.20-0309-non-reasoning", "grok-4.20-0309-reasoning", "grok-4.20-multi-agent-0309",
+		"grok-4.3", "grok-4.5", "grok-4.6", "grok-4.7", "grok-build-0.1", "grok-3-mini"} {
+		if !isUsableGrokModel(id) {
+			t.Errorf("isUsableGrokModel(%q) = false, want true", id)
+		}
+	}
+}
+
+// TestStaticGrok_LeadsWithCLIDefaults: the Grok CLI's default model is
+// grok-4.6, then grok-4.5 (grok-build f0e3be11:
+// xai-grok-models/default_models.json).
+func TestStaticGrok_LeadsWithCLIDefaults(t *testing.T) {
+	if len(StaticGrok) < 2 || !slices.Equal(StaticGrok[:2], []string{"grok-4.6", "grok-4.5"}) {
+		t.Fatalf("StaticGrok = %v, want it to lead with grok-4.6, grok-4.5", StaticGrok)
+	}
+}
diff --git a/llmprovider/live_grok_catalog_test.go b/llmprovider/live_grok_catalog_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_grok_catalog_test.go
@@ -0,0 +1,31 @@
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
+// TestLive_GrokListingTextOnly: xAI's live listing yields no media model.
+func TestLive_GrokListingTextOnly(t *testing.T) {
+	key := os.Getenv("XAI_API_KEY")
+	if key == "" {
+		t.Skip("XAI_API_KEY unset")
+	}
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	cat, err := ListModelCatalog(ctx, ProviderGrok, key)
+	if err != nil || !cat.Live {
+		t.Skipf("listing unavailable: live=%t err=%v", cat.Live, err)
+	}
+	for _, id := range cat.Usable {
+		for _, media := range []string{"imagine", "video", "image", "voice", "tts", "stt"} {
+			if strings.Contains(id, media) {
+				t.Errorf("usable %q is a media model", id)
+			}
+		}
+	}
+	t.Logf("usable: %v", cat.Usable)
+}
```

**Fix** (`gk2-fix.diff`, 33 lines):

```diff
diff --git a/llmprovider/models_catalog.go b/llmprovider/models_catalog.go
--- a/llmprovider/models_catalog.go
+++ b/llmprovider/models_catalog.go
@@ -100,13 +100,14 @@
 		"thinkingmachines/inkling",
 	}
 
-	// StaticGrok: fast/flagship models first.
+	// StaticGrok leads with the Grok CLI's defaults, grok-4.6 then grok-4.5
+	// (MADR 0012 §6), then the fast models.
 	StaticGrok = []string{
+		grokModel46,
+		grokModel45,
 		"grok-3-mini-fast",
 		"grok-3-mini",
 		"grok-4",
-		"grok-4.5",
-		"grok-4.6",
 		"grok-4-fast-reasoning",
 	}
 )
@@ -490,8 +491,9 @@
 	if sm == "" || !strings.HasPrefix(sm, "grok") {
 		return false
 	}
-	// Skip non-text specialties if they appear.
-	for _, deny := range []string{"vision", "image", "embed"} {
+	// Skip non-text specialties: xAI lists grok-imagine-video, which "image"
+	// does not match (MADR 0012 §6).
+	for _, deny := range []string{"vision", denyImage, "embed", "imagine", "video", "voice", "stt", "tts"} {
 		if strings.Contains(sm, deny) {
 			return false
 		}
```

### B.K3 Phase K3 — Tool description and `WithStore` (§6)

**Tests** (`gk3-tests.diff`, 73 lines):

```diff
diff --git a/llmprovider/grok_tool_description_test.go b/llmprovider/grok_tool_description_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/grok_tool_description_test.go
@@ -0,0 +1,25 @@
+package llmprovider
+
+import (
+	"context"
+	"testing"
+)
+
+// TestGrok_SendsToolDescription: the tool's description reaches xAI, as the
+// OpenAI and OpenCode Responses paths already send it.
+func TestGrok_SendsToolDescription(t *testing.T) {
+	var body map[string]any
+	srv := captureServer(t, &body, `{"id":"r","output":[{"type":"function_call","call_id":"c","name":"get_weather","arguments":"{}"}]}`)
+	p, err := NewGrok("k", "grok-4.6", WithBaseURL(srv.URL))
+	if err != nil {
+		t.Fatal(err)
+	}
+	tool := Tool{Name: "get_weather", Description: "Get the weather for a city", Schema: map[string]any{"type": "object"}}
+	if _, err := p.GenerateWithTool(context.Background(), "weather?", tool); err != nil {
+		t.Fatal(err)
+	}
+	tools, _ := body[jsonKeyTools].([]any)
+	if len(tools) != 1 || tools[0].(map[string]any)[jsonKeyDescription] != tool.Description {
+		t.Fatalf("tools = %v, want the description", body[jsonKeyTools])
+	}
+}
diff --git a/llmprovider/live_grok_tool_test.go b/llmprovider/live_grok_tool_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_grok_tool_test.go
@@ -0,0 +1,38 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"bytes"
+	"os"
+	"testing"
+)
+
+// TestLive_GrokToolDescription: xAI takes a tool with its description and
+// calls it.
+func TestLive_GrokToolDescription(t *testing.T) {
+	key := os.Getenv("XAI_API_KEY")
+	if key == "" {
+		t.Skip("XAI_API_KEY unset")
+	}
+	var sent []byte
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewGrok(key, "grok-4.6", WithHTTPClient(xaiRecordingClient(&sent)))
+	if err != nil {
+		t.Fatal(err)
+	}
+	tool := Tool{Name: "get_weather", Description: "Get the weather for a city", Schema: map[string]any{
+		"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}, "required": []string{"city"}}}
+	args, err := p.GenerateWithTool(ctx, "What is the weather in Paris?", tool)
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("GenerateWithTool: %v", err)
+	}
+	if !bytes.Contains(sent, []byte(`"description":"Get the weather for a city"`)) {
+		t.Fatalf("request did not send the tool description: %s", sent)
+	}
+	if !bytes.Contains(bytes.ToLower([]byte(args)), []byte("paris")) {
+		t.Errorf("arguments %q", args)
+	}
+}
```

**Fix** (`gk3-fix.diff`, 238 lines):

```diff
diff --git a/llmprovider/grok.go b/llmprovider/grok.go
--- a/llmprovider/grok.go
+++ b/llmprovider/grok.go
@@ -21,6 +21,8 @@
 	reasoningEffort string // reasoning effort for the GenerateThinking path
 	// identity names the client on every request (MADR 0012 §1.4).
 	identity clientIdentity
+	// store is WithStore's value, or nil for the service default.
+	store *bool
 }
 
 // NewGrok creates a new Grok provider instance.
@@ -52,6 +54,7 @@
 		identity:        identityOf(cfg),
 		maxTokens:       cfg.MaxTokens,
 		reasoningEffort: cfg.ReasoningEffort,
+		store:           cfg.Store,
 	}, nil
 }
 
@@ -174,13 +177,17 @@
 		jsonKeyInput:        itemsToInput(input),
 		"max_output_tokens": p.maxTokens,
 	}
+	if p.store != nil {
+		body["store"] = *p.store
+	}
 
 	if tool != nil {
 		body[jsonKeyTools] = []map[string]any{
 			{
-				jsonKeyType:       jsonKeyFunction,
-				jsonKeyName:       tool.Name,
-				jsonKeyParameters: tool.Schema,
+				jsonKeyType:        jsonKeyFunction,
+				jsonKeyName:        tool.Name,
+				jsonKeyDescription: tool.Description,
+				jsonKeyParameters:  tool.Schema,
 			},
 		}
 		body["tool_choice"] = map[string]any{
diff --git a/llmprovider/live_responses_store_test.go b/llmprovider/live_responses_store_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_responses_store_test.go
@@ -0,0 +1,54 @@
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
+// TestLive_ResponsesStoreFalse: OpenAI (API key) and xAI accept store:false
+// from WithStore(false).
+func TestLive_ResponsesStoreFalse(t *testing.T) {
+	for _, tc := range []struct{ name, env, model string }{
+		{ProviderOpenAI, "OPENAI_API_KEY", "gpt-6-luna"},
+		{ProviderGrok, "XAI_API_KEY", "grok-4.6"},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			key := os.Getenv(tc.env)
+			if key == "" {
+				t.Skipf("%s unset", tc.env)
+			}
+			var sent []byte
+			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+				if r.Method == http.MethodPost && r.Body != nil {
+					b, err := io.ReadAll(r.Body)
+					if err != nil {
+						return nil, err
+					}
+					sent = b
+					r.Body = io.NopCloser(bytes.NewReader(b))
+				}
+				return http.DefaultTransport.RoundTrip(r)
+			})}
+			ctx, cancel := liveCtx(t)
+			defer cancel()
+			p, err := NewProvider(tc.name, key, tc.model, WithStore(false), WithHTTPClient(client))
+			if err != nil {
+				t.Fatal(err)
+			}
+			out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+			skipIfTransient(t, err)
+			if err != nil {
+				t.Fatalf("Generate: %v", err)
+			}
+			if !bytes.Contains(sent, []byte(`"store":false`)) || !strings.Contains(strings.ToUpper(out), "ALPHA") {
+				t.Fatalf("out = %q, sent %s", out, sent)
+			}
+		})
+	}
+}
diff --git a/llmprovider/openai.go b/llmprovider/openai.go
--- a/llmprovider/openai.go
+++ b/llmprovider/openai.go
@@ -21,6 +21,9 @@
 	reasoningEffort string // reasoning effort for the GenerateThinking path
 	// identity names the client on every request (MADR 0012 §1.4).
 	identity clientIdentity
+	// store is WithStore's value, or nil for the service default. A ChatGPT
+	// session ignores it.
+	store *bool
 }
 
 // defaultOpenAIReasoningEffort is used by GenerateThinking when none is configured.
@@ -137,6 +140,9 @@
 		body["prompt_cache_key"] = p.identity.session
 	} else {
 		body["max_output_tokens"] = p.maxTokens
+		if p.store != nil {
+			body["store"] = *p.store
+		}
 	}
 
 	if tool != nil {
diff --git a/llmprovider/openai_chatgpt.go b/llmprovider/openai_chatgpt.go
--- a/llmprovider/openai_chatgpt.go
+++ b/llmprovider/openai_chatgpt.go
@@ -44,6 +44,7 @@
 		identity:        identityOf(cfg),
 		maxTokens:       cfg.MaxTokens,
 		reasoningEffort: cfg.ReasoningEffort,
+		store:           cfg.Store,
 	}, nil
 }
 
diff --git a/llmprovider/options.go b/llmprovider/options.go
--- a/llmprovider/options.go
+++ b/llmprovider/options.go
@@ -57,6 +57,9 @@
 	// KiloOrganization scopes Kilo requests to an organization; see
 	// WithKiloOrganization. Ignored by all other providers.
 	KiloOrganization string
+	// Store sets the Responses API store field for OpenAI API-key mode and
+	// Grok; nil leaves the service default. See WithStore.
+	Store *bool
 	// ModelProfile selects how the recommended models of the open catalogs
 	// (Kilo, OpenCode Zen and Go, Hugging Face) are ranked. The zero value is
 	// ProfileUtility. Those providers' DiscoverModels ranks with it too.
@@ -164,6 +167,17 @@
 	}
 }
 
+// WithStore sets whether OpenAI (API-key mode) and Grok store responses
+// (MADR 0012 §6). Without it the service default applies, which keeps
+// Continue working; callers under zero-data-retention pass false, after
+// which Continue has nothing to chain from. A ChatGPT session always sends
+// false. Ignored by all other providers.
+func WithStore(store bool) ProviderOption {
+	return func(cfg *ProviderConfig) {
+		cfg.Store = &store
+	}
+}
+
 // WithModelProfile selects how ListAvailableModels, ListModelCatalog and the
 // open catalogs' DiscoverModels rank the recommended models (MADR 0010 §1,
 // MADR 0013 A4).
diff --git a/llmprovider/responses_store_test.go b/llmprovider/responses_store_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/responses_store_test.go
@@ -0,0 +1,68 @@
+package llmprovider
+
+import (
+	"context"
+	"testing"
+)
+
+// TestWithStore_ResponsesProviders: store is absent unless WithStore sets it,
+// on OpenAI API-key mode and Grok; a ChatGPT session always sends false.
+func TestWithStore_ResponsesProviders(t *testing.T) {
+	const reply = `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`
+	store := func(t *testing.T, build func(url string) (Provider, error)) (any, bool) {
+		t.Helper()
+		var body map[string]any
+		srv := captureServer(t, &body, reply)
+		p, err := build(srv.URL)
+		if err != nil {
+			t.Fatal(err)
+		}
+		if _, err := p.Generate(context.Background(), "hi"); err != nil {
+			t.Fatal(err)
+		}
+		v, ok := body["store"]
+		return v, ok
+	}
+	for _, tc := range []struct {
+		name  string
+		opts  []ProviderOption
+		want  any
+		isSet bool
+	}{
+		{"default", nil, nil, false},
+		{"false", []ProviderOption{WithStore(false)}, false, true},
+		{"true", []ProviderOption{WithStore(true)}, true, true},
+	} {
+		t.Run("openai/"+tc.name, func(t *testing.T) {
+			v, ok := store(t, func(url string) (Provider, error) {
+				return NewOpenAI("sk-test", "gpt-6-luna", append([]ProviderOption{WithBaseURL(url)}, tc.opts...)...)
+			})
+			if ok != tc.isSet || v != tc.want {
+				t.Errorf("store = %v (present %t), want %v (present %t)", v, ok, tc.want, tc.isSet)
+			}
+		})
+		t.Run("grok/"+tc.name, func(t *testing.T) {
+			v, ok := store(t, func(url string) (Provider, error) {
+				return NewGrok("k", "grok-4.6", append([]ProviderOption{WithBaseURL(url)}, tc.opts...)...)
+			})
+			if ok != tc.isSet || v != tc.want {
+				t.Errorf("store = %v (present %t), want %v (present %t)", v, ok, tc.want, tc.isSet)
+			}
+		})
+	}
+	t.Run("chatgpt ignores true", func(t *testing.T) {
+		client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
+		p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client), WithStore(true))
+		if err != nil {
+			t.Fatal(err)
+		}
+		if _, err := p.Generate(context.Background(), "hi"); err != nil {
+			t.Fatal(err)
+		}
+		c.mu.Lock()
+		defer c.mu.Unlock()
+		if c.body["store"] != false {
+			t.Errorf("store = %v, want false", c.body["store"])
+		}
+	})
+}
```

