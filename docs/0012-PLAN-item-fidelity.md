---
status: complete
date: 2026-09-27
associated-madr: "0012-MADR-conform-providers-to-reference-clients.md"
decision-makers: mcplib maintainers
---

# Implement 0012 §2 — Canonical Item Fidelity

Associated MADR: [0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
(accepted 2026-09-27, revision 3). This is the second of that MADR's six
plans (§8).

This plan executes MADR §2 as amended in revision 3, and nothing else. If a
fact contradicts the MADR or this plan, **stop and prompt**. Add a dated
entry to §9 of this plan, amend the MADR when a decision or an asserted fact
changes, and only then continue.

## How this plan was proven

Every phase below was executed on 2026-09-27, in scratch copies of the tree
made from `git archive 7ea0ad4`, outside the repository:
* **Red.** Each phase's tests diff was applied, and each named test was seen
  to fail on the unfixed code. Appendix A quotes each failure.
* **Green.** The fix diff was applied, and the full gate (§0.2) passed.
* **Mutants.** A test that cannot fail before the fix was seen to fail on a
  named mutant instead. This covers a test naming new API and a guard that
  holds today.
* **Live.** Each live test ran against the real service.
* **Diffs.** Appendix B's diffs were generated mechanically from that proof.
  The five 0012 plans were proven stacked in §8's order. Applying all their
  diffs in that order to a fresh `7ea0ad4` archive with `git apply`
  reproduces the proven tree byte for byte: 308 files, 0 mismatches.

Executing this plan repeats a proven sequence. It does not design anything.

## Goal

Every canonical item reaches every wire in the form that service expects, so
a tool round trip replayed through `GenerateItems` works on every provider:
* A `FunctionCallItem` is sent as the assistant's native call, immediately
  before its result:
  * Chat Completions: `tool_calls`;
  * Responses: `function_call`;
  * Anthropic: `tool_use`;
  * Gemini: `functionCall`.

  Today Chat, Anthropic and Gemini drop the call and send only the result,
  which each service rejects or misreads.
* Gemini gets the `thoughtSignature` it issued back on the replayed call.
* A `system` item reaches Anthropic's top-level `system` field, on Claude and
  on OpenCode's `messages` route, instead of being sent as a user turn.
* On OpenCode's chat route, a model whose metadata declares an interleaved
  reasoning field gets the preceding reasoning on each assistant message.
  This is OpenCode's own client behaviour (MADR §2, O5).

## Scope

**In scope:** MADR §2: F1–F3 below.

**Out of scope:**
* Streaming, and every other section of MADR 0012.
* Two latent Gemini defects, noted in the MADR's revision 3:
  * the decoder types `thought` as a string;
  * a `system` message becomes a model turn.

## 0. Preconditions and conventions

### 0.1 Baseline

* `main` at `7ea0ad4` (0012-PLAN-shared-transport.md complete), or a
  descendant with no change to the files Appendix B touches.
* `go test -count=1 ./...` passes before F1.

### 0.2 Gate (every phase)

1. `gofmt -l` prints nothing, and `golint -set_exit_status` passes, on each
   `.go` file the phase touched.
2. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
3. `make lint`.
4. `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

### 0.3 Red first

* Apply the phase's **Tests** diff (Appendix B), then run each test Appendix A
  lists as red. Each must fail with the recorded message.
* Apply the **Fix** diff, then run the gate.
* Re-run each mutant on a scratch copy, never on the working tree.

### 0.4 Credentials for live steps

The live tests read `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `XAI_API_KEY`,
`KILO_API_KEY` and `OPENCODE_API_KEY`, and skip without them.

### 0.5 Commits

* One `git commit --no-edit` per phase, after the gate.
* No push and no tag.

## Phase F0 — Start

1. Confirm §0.1.
2. Set this plan to `status: in-progress`.
3. Commit the documents only.

## Phase F1 — Tool calls reach every wire

**Today.**
* `itemsToChatMessages` (`chatcompletions.go`), `claudeItemsToMessages`
  (`claude.go`) and `geminiItemsToContents` (`gemini.go`) have no case for
  `FunctionCallItem`. They send the tool result with no call before it.
* `decodeGeminiResponse` drops the call's `thoughtSignature`.

**Changes** (Appendix B.F1):
* **Chat Completions:**
  * a `FunctionCallItem` becomes
    `{"id","type":"function","function":{name,arguments}}` in the
    assistant's `tool_calls`;
  * consecutive calls merge into one assistant message, which opens with
    `content: ""` when no assistant text precedes it.
* **Responses:** the call becomes
  `{type: function_call, call_id, name, arguments}`.
* **Anthropic:**
  * the call becomes a `tool_use` block `{id, name, input}` in the
    assistant turn, where `input` is the parsed arguments (new
    `item_convert.go`, `toolArguments`);
  * consecutive `tool_result` blocks share one user message.
* **Gemini:**
  * the call becomes a `functionCall {name, args}` part in the model turn;
  * the response is sent with role `user`, named after its call;
  * consecutive calls and responses are grouped;
  * the part carries the call's `thoughtSignature`, or the placeholder
    `skip_thought_signature_validator` when the call has none.
* **New field:** `FunctionCallItem.Signature` holds the opaque token a
  provider requires back. The Gemini decoder fills it, and turns a call
  without `args` into `{}`.
* **Updated test:** `TestItemsToChatMessages` pinned the defect (three
  messages, no call). It now expects four.

**Verification.** The eight unit tests in Appendix A.F1 are red before the
fix. `TestItemFidelity_GeminiReplaysSignature` names the new field, so its
two mutants prove it. Live:
* `TestLive_ToolRoundTrip` replays `[user, call, result]` on seven wires;
* `TestLive_GeminiReplaysRealCall` replays a signature Gemini itself issued.

## Phase F2 — System messages are top-level on Anthropic wires

**Today.** `claudeItemsToMessages` sends a `system` item as a `user` message,
on Claude and on OpenCode's `messages` route alike.

**Changes** (Appendix B.F2):
* `claudeSystemPrompt(items)` joins the `system` items with a blank line.
* `claudeItemsToMessages` skips `system` items.
* `body["system"]` is set in `claude.go` and in `opencode.go`'s
  `messagesBody`.

**Verification.** Two unit tests are red. `TestLive_SystemMessage`, on Claude
and on Go's `messages` route, holds before the fix because models often obey
a user-turn instruction. Mutant `fid2-system-dropped` proves it instead: with
the system prompt dropped, the reply ignores the instruction.

## Phase F3 — Interleaved reasoning replay on OpenCode's chat route

**Today.** The chat route never sends a reasoning field on assistant
messages, although the metadata document declares
`interleaved: {field: …}` for such models (O5).

**Changes** (Appendix B.F3):
* `modelMetadata.Interleaved` is decoded, and `interleavedField` reads it.
* `itemsToChatMessagesReplaying(items, field)`:
  * puts the reasoning that preceded each assistant message into `field`;
  * sets the field, possibly to `""`, on every assistant message, as
    OpenCode's client does (`transform.ts:321-349`).
* OpenCode's chat route looks the field up only when the input holds an
  assistant turn. The lookup is bounded by `metadataLookupTimeout`.

**Verification.**
* `TestOpencodeChat_ReplaysInterleavedReasoning` is red.
* The guard `TestOpencodeChat_NoReplayWithoutInterleaved` holds today, and
  mutant `fid3-replay-always` proves it.
* Live, `TestLive_InterleavedReasoningReplay` records the request on
  `kimi-k2.6` and asserts the field was sent. It is red before the fix.
  `kimi-k2.6` accepts the field without requiring it (MADR revision 3).

## Phase F4 — Records and close-out

1. Record each phase's result in §10.
2. Set this plan to `status: complete` once §7 holds.
3. Commit the documents only.

## 7. Acceptance criteria

* Every Appendix A red test fails before its fix and passes after it.
* Every mutant is killed.
* The gate passes after each phase.
* The live tests pass, or skip only for a missing credential or a transient
  error.
* The MADR's confirmation for §2 holds: a `[user, FunctionCallItem,
  FunctionCallOutputItem]` round trip through each of the four converters
  produces the native call immediately before its result, and a `system`
  item reaches Claude's top-level `system`.

## 8. Rollout and rollback

* **Additive API:** `FunctionCallItem.Signature` only.
* **Behaviour change:** requests replaying tool calls now carry them. A
  caller that worked around the missing call by other means may now send it
  twice. No known consumer does.
* **Rollback:** revert the phase commit. Each phase is independent of later
  ones within this plan.

## 9. Deviation log

None yet.

## 10. Execution record

Executed on `main`, 2026-09-27, after `0012-PLAN-circuit-breaker-test.md`
(`05a1fcf`). Each phase applied Appendix B with `git apply`, taken from this
document, and checked equal to the proven diff.

* **F1**, commit `a346457`: 10 red tests failed as required; 0 base guards passed; gate passed; 2/2 mutants killed.
* **F2**, commit `d5429e6`: 2 red tests failed as required; 1 base guard passed; gate passed; 1/1 mutants killed.
* **F3**, commit `db2d0d6`: 2 red tests failed as required; 1 base guard passed; gate passed; 1/1 mutants killed.

**Live** (`-tags live_gateways`, this plan's tests, on the executed tree): 13 passed, 0 skipped, 0 failed.

## Appendix A — Proof record (2026-09-27, scratch copies of `7ea0ad4`)

### A.F1 Phase F1 — Tool calls reach every wire

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestItemFidelity_ChatToolCall` | unit | got  [{"content":"weather?","role":"user"},{"content":"{\"forecast\":\"sunny\"}","role":"tool","tool_call_id":"call_1"}] |
| `TestItemFidelity_ChatGroupsCalls` | unit | got  [{"content":"weather in two cities?","role":"user"},{"content":"Checking both.","role":"assistant"},{"content":"{\"forecast\":\"sunny\"}","role":"tool","tool_call_id":"call_1"},{"content":"{\"forecast\":\"rain\"}"," |
| `TestItemFidelity_ResponsesToolCall` | unit | got  [{"content":"weather?","role":"user"},{"call_id":"call_1","output":"{\"forecast\":\"sunny\"}","type":"function_call_output"}] |
| `TestItemFidelity_AnthropicToolCall` | unit | got  [{"content":"weather?","role":"user"},{"content":[{"content":"{\"forecast\":\"sunny\"}","tool_use_id":"call_1","type":"tool_result"}],"role":"user"}] |
| `TestItemFidelity_AnthropicGroupsCallsAndResults` | unit | got  [{"content":"weather in two cities?","role":"user"},{"content":"Checking both.","role":"assistant"},{"content":[{"content":"{\"forecast\":\"sunny\"}","tool_use_id":"call_1","type":"tool_result"}],"role":"user"},{"co |
| `TestItemFidelity_GeminiToolCall` | unit | got  [{"parts":[{"text":"weather?"}],"role":"user"},{"parts":[{"functionResponse":{"name":"call_1","response":{"output":"{\"forecast\":\"sunny\"}"}}}],"role":"function"}] |
| `TestItemFidelity_GeminiGroupsCallsAndResults` | unit | got  [{"parts":[{"text":"weather in two cities?"}],"role":"user"},{"parts":[{"text":"Checking both."}],"role":"model"},{"parts":[{"functionResponse":{"name":"call_1","response":{"output":"{\"forecast\":\"sunny\"}"}}}],"r |
| `TestItemFidelity_GeminiDecodesCallWithoutArgs` | unit | decode = &{ID: Output:[] FinishReason:}/<nil>, want one function call |
| `TestLive_ToolRoundTrip` | live | GenerateItems: llm: invalid request: claude HTTP 400 invalid_request_error: messages.0.content.1: unexpected `tool_use_id` found in `tool_result` blocks: call_rt_1. Each `tool_result` block must have a corresponding `too |
| `TestLive_GeminiReplaysRealCall` | live | replay: llm: invalid request: gemini HTTP 400 INVALID_ARGUMENT: Role 'function' is not supported. Please use a valid role: SYSTEM, SYSTEM_1, USER, ASSISTANT, DEVELOPER, CONTEXT, USER_CONTEXT, MODEL, USER. |

**Gate** (fix applied): all passed — per-file `golint` on 11 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `fid1-signature-not-decoded` | `TestItemFidelity_GeminiReplaysSignature` | killed | item = llmprovider.FunctionCallItem{CallID:"get_weather", Name:"get_weather", Arguments:"{\"city\":\"Paris\"}", Signature:""}, want Signature sig-abc |
| `fid1-signature-not-replayed` | `TestItemFidelity_GeminiReplaysSignature` | killed | got  {"parts":[{"functionCall":{"args":{"city":"Paris"},"name":"get_weather"},"thoughtSignature":"skip_thought_signature_validator"}],"role":"model"} |

### A.F2 Phase F2 — System messages are top-level on Anthropic wires

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestClaude_SystemMessageIsTopLevel` | unit | system = <nil>, want the two system items joined |
| `TestOpencodeMessages_SystemMessageIsTopLevel` | unit | system = <nil>, want the two system items joined |

**Guards before the fix:**

* `TestLive_SystemMessage`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 6 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `fid2-system-dropped` | `TestLive_SystemMessage` | killed | reply "Hello" does not follow the system instruction |

### A.F3 Phase F3 — Interleaved reasoning replay on OpenCode's chat route

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestOpencodeChat_ReplaysInterleavedReasoning` | unit | assistant reasoning_content = []interface {}{interface {}(nil), interface {}(nil)}, want ["Call the tool." ""] |
| `TestLive_InterleavedReasoningReplay` | live | request did not replay the reasoning: {"max_tokens":8192,"messages":[{"content":"What is the weather in Paris? Use the tool.","role":"user"},{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{\"cit |

**Guards before the fix:**

* `TestOpencodeChat_NoReplayWithoutInterleaved`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 5 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `fid3-replay-always` | `TestOpencodeChat_NoReplayWithoutInterleaved` | killed | message map[content: reasoning_content:Call the tool. role:assistant tool_calls:[map[function:map[arguments:{"city":"Paris"} name:get_weather] id:call_1 type:function]]] carries reasoning_content for a non-interleaved mo |

### A.L Live runs on this plan's final tree (`-tags live_gateways`, 2026-09-27)

| Test | Result |
|---|---|
| `TestLive_ToolRoundTrip` | PASS |
| `TestLive_ToolRoundTrip/claude` | PASS |
| `TestLive_ToolRoundTrip/gemini` | PASS |
| `TestLive_ToolRoundTrip/grok` | PASS |
| `TestLive_ToolRoundTrip/kilo` | PASS |
| `TestLive_ToolRoundTrip/go-chat` | PASS |
| `TestLive_ToolRoundTrip/go-messages` | PASS |
| `TestLive_ToolRoundTrip/go-responses` | PASS |
| `TestLive_GeminiReplaysRealCall` | PASS |
| `TestLive_InterleavedReasoningReplay` | PASS |
| `TestLive_SystemMessage` | PASS |
| `TestLive_SystemMessage/claude` | PASS |
| `TestLive_SystemMessage/go-messages` | PASS |


Totals: 13 passed, 0 skipped, 0 failed.


## Appendix B — Diffs

Generated from the proof. Apply each phase's **Tests** diff, then its **Fix**
diff, with `git apply`, in order, on the §0.1 baseline.

### B.F1 Phase F1 — Tool calls reach every wire

**Tests** (`fid1-tests.diff`, 239 lines):

```diff
diff --git a/llmprovider/item_fidelity_test.go b/llmprovider/item_fidelity_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/item_fidelity_test.go
@@ -0,0 +1,130 @@
+package llmprovider
+
+import (
+	"encoding/json"
+	"reflect"
+	"strings"
+	"testing"
+)
+
+// roundTrip is the canonical tool round trip MADR 0012 §2 must survive.
+func roundTrip() []Item {
+	return []Item{
+		MessageItem{Role: jsonRoleUser, Text: "weather?"},
+		FunctionCallItem{CallID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
+		FunctionCallOutputItem{CallID: "call_1", Output: `{"forecast":"sunny"}`},
+	}
+}
+
+// twoCalls is a turn in which the model says something, then calls two tools.
+func twoCalls() []Item {
+	return []Item{
+		MessageItem{Role: jsonRoleUser, Text: "weather in two cities?"},
+		MessageItem{Role: jsonRoleAssistant, Text: "Checking both."},
+		FunctionCallItem{CallID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
+		FunctionCallItem{CallID: "call_2", Name: "get_weather", Arguments: `{"city":"Rome"}`},
+		FunctionCallOutputItem{CallID: "call_1", Output: `{"forecast":"sunny"}`},
+		FunctionCallOutputItem{CallID: "call_2", Output: `{"forecast":"rain"}`},
+	}
+}
+
+// asJSON normalises a request fragment through JSON, as the wire sees it.
+func asJSON(t *testing.T, v any) any {
+	t.Helper()
+	raw, err := json.Marshal(v)
+	if err != nil {
+		t.Fatalf("marshal: %v", err)
+	}
+	var out any
+	if err := json.Unmarshal(raw, &out); err != nil {
+		t.Fatalf("unmarshal: %v", err)
+	}
+	return out
+}
+
+func mustEqualJSON(t *testing.T, got any, want string) {
+	t.Helper()
+	var w any
+	if err := json.Unmarshal([]byte(want), &w); err != nil {
+		t.Fatalf("bad want: %v", err)
+	}
+	if g := asJSON(t, got); !reflect.DeepEqual(g, w) {
+		gj, _ := json.Marshal(g)
+		t.Fatalf("got  %s\nwant %s", gj, want)
+	}
+}
+
+func TestItemFidelity_ChatToolCall(t *testing.T) {
+	mustEqualJSON(t, itemsToChatMessages(roundTrip()), `[
+		{"role":"user","content":"weather?"},
+		{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},
+		{"role":"tool","tool_call_id":"call_1","content":"{\"forecast\":\"sunny\"}"}]`)
+}
+
+func TestItemFidelity_ChatGroupsCalls(t *testing.T) {
+	mustEqualJSON(t, itemsToChatMessages(twoCalls()), `[
+		{"role":"user","content":"weather in two cities?"},
+		{"role":"assistant","content":"Checking both.","tool_calls":[
+			{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}},
+			{"id":"call_2","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Rome\"}"}}]},
+		{"role":"tool","tool_call_id":"call_1","content":"{\"forecast\":\"sunny\"}"},
+		{"role":"tool","tool_call_id":"call_2","content":"{\"forecast\":\"rain\"}"}]`)
+}
+
+func TestItemFidelity_ResponsesToolCall(t *testing.T) {
+	mustEqualJSON(t, itemsToInput(roundTrip()), `[
+		{"role":"user","content":"weather?"},
+		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"Paris\"}"},
+		{"type":"function_call_output","call_id":"call_1","output":"{\"forecast\":\"sunny\"}"}]`)
+}
+
+func TestItemFidelity_AnthropicToolCall(t *testing.T) {
+	mustEqualJSON(t, claudeItemsToMessages(roundTrip()), `[
+		{"role":"user","content":"weather?"},
+		{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"get_weather","input":{"city":"Paris"}}]},
+		{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"{\"forecast\":\"sunny\"}"}]}]`)
+}
+
+func TestItemFidelity_AnthropicGroupsCallsAndResults(t *testing.T) {
+	mustEqualJSON(t, claudeItemsToMessages(twoCalls()), `[
+		{"role":"user","content":"weather in two cities?"},
+		{"role":"assistant","content":[
+			{"type":"text","text":"Checking both."},
+			{"type":"tool_use","id":"call_1","name":"get_weather","input":{"city":"Paris"}},
+			{"type":"tool_use","id":"call_2","name":"get_weather","input":{"city":"Rome"}}]},
+		{"role":"user","content":[
+			{"type":"tool_result","tool_use_id":"call_1","content":"{\"forecast\":\"sunny\"}"},
+			{"type":"tool_result","tool_use_id":"call_2","content":"{\"forecast\":\"rain\"}"}]}]`)
+}
+
+func TestItemFidelity_GeminiToolCall(t *testing.T) {
+	mustEqualJSON(t, geminiItemsToContents(roundTrip()), `[
+		{"role":"user","parts":[{"text":"weather?"}]},
+		{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"skip_thought_signature_validator"}]},
+		{"role":"user","parts":[{"functionResponse":{"name":"get_weather","response":{"output":"{\"forecast\":\"sunny\"}"}}}]}]`)
+}
+
+// TestItemFidelity_GeminiDecodesCallWithoutArgs: a call to a tool with no
+// parameters has no args; it is still a call.
+func TestItemFidelity_GeminiDecodesCallWithoutArgs(t *testing.T) {
+	res, err := decodeGeminiResponse(strings.NewReader(
+		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"now"}}]}}]}`))
+	if err != nil || len(res.Output) != 1 {
+		t.Fatalf("decode = %+v/%v, want one function call", res, err)
+	}
+	if call, ok := res.Output[0].(FunctionCallItem); !ok || call.Name != "now" || call.Arguments != "{}" {
+		t.Fatalf("item = %#v, want FunctionCallItem now with {}", res.Output[0])
+	}
+}
+
+func TestItemFidelity_GeminiGroupsCallsAndResults(t *testing.T) {
+	mustEqualJSON(t, geminiItemsToContents(twoCalls()), `[
+		{"role":"user","parts":[{"text":"weather in two cities?"}]},
+		{"role":"model","parts":[
+			{"text":"Checking both."},
+			{"functionCall":{"name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"skip_thought_signature_validator"},
+			{"functionCall":{"name":"get_weather","args":{"city":"Rome"}},"thoughtSignature":"skip_thought_signature_validator"}]},
+		{"role":"user","parts":[
+			{"functionResponse":{"name":"get_weather","response":{"output":"{\"forecast\":\"sunny\"}"}}},
+			{"functionResponse":{"name":"get_weather","response":{"output":"{\"forecast\":\"rain\"}"}}}]}]`)
+}
diff --git a/llmprovider/live_items_test.go b/llmprovider/live_items_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_items_test.go
@@ -0,0 +1,99 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"context"
+	"os"
+	"strings"
+	"testing"
+	"time"
+)
+
+// TestLive_ToolRoundTrip sends a completed tool call and its result on every
+// wire, and requires the model to answer from the result (MADR 0012 §2).
+// Before the fix the call was dropped: Anthropic, Gemini and the Responses
+// route answered 400, and the chat routes answered without the tool.
+func TestLive_ToolRoundTrip(t *testing.T) {
+	convo := []Item{
+		MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris? Use the tool."},
+		FunctionCallItem{CallID: "call_rt_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
+		FunctionCallOutputItem{CallID: "call_rt_1", Output: `{"forecast":"sunny, 21C"}`},
+	}
+	for _, c := range []struct {
+		name, env string
+		build     func(key string) (ItemProvider, error)
+	}{
+		{"claude", "ANTHROPIC_API_KEY", func(k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
+		{"gemini", "GEMINI_API_KEY", func(k string) (ItemProvider, error) {
+			return NewGemini(context.Background(), k, "gemini-3.7-flash")
+		}},
+		{"grok", "XAI_API_KEY", func(k string) (ItemProvider, error) { return NewGrok(k, "grok-4.5") }},
+		{"kilo", "KILO_API_KEY", func(k string) (ItemProvider, error) { return NewKilo(k, "deepseek/deepseek-v4.1-flash") }},
+		{"go-chat", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "glm-5.3-flash") }},
+		{"go-messages", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "qwen3.8-flash") }},
+		{"go-responses", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "gpt-6-luna") }},
+	} {
+		t.Run(c.name, func(t *testing.T) {
+			key := os.Getenv(c.env)
+			if key == "" {
+				t.Skipf("%s unset", c.env)
+			}
+			p, err := c.build(key)
+			if err != nil {
+				t.Fatalf("construct: %v", err)
+			}
+			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
+			defer cancel()
+			res, err := p.GenerateItems(ctx, convo...)
+			skipIfTransient(t, err)
+			if err != nil {
+				t.Fatalf("GenerateItems: %v", err)
+			}
+			if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
+				t.Fatalf("reply %q does not use the tool result", res.OutputText())
+			}
+		})
+	}
+}
+
+// TestLive_GeminiReplaysRealCall: a call Gemini issued goes back with the
+// thoughtSignature it came with, and Gemini answers from the result.
+func TestLive_GeminiReplaysRealCall(t *testing.T) {
+	key := os.Getenv("GEMINI_API_KEY")
+	if key == "" {
+		t.Skip("GEMINI_API_KEY unset")
+	}
+	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
+	defer cancel()
+	p, err := NewGemini(ctx, key, "gemini-3.7-flash")
+	if err != nil {
+		t.Fatalf("NewGemini: %v", err)
+	}
+	ask := MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris? Use the tool."}
+	tool := Tool{Name: "get_weather", Description: "Current weather for a city",
+		Schema: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}},
+			"required": []string{"city"}}}
+	first, err := p.GenerateItemsWithTool(ctx, tool, ask)
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("GenerateItemsWithTool: %v", err)
+	}
+	var call FunctionCallItem
+	for _, item := range first.Output {
+		if c, ok := item.(FunctionCallItem); ok {
+			call = c
+		}
+	}
+	if call.Name != "get_weather" {
+		t.Fatalf("no get_weather call in %+v", first.Output)
+	}
+	res, err := p.GenerateItems(ctx, ask, call, FunctionCallOutputItem{CallID: call.CallID, Output: `{"forecast":"sunny, 21C"}`})
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("replay: %v", err)
+	}
+	if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
+		t.Fatalf("reply %q does not use the tool result", res.OutputText())
+	}
+}
```

**Fix** (`fid1-fix.diff`, 345 lines):

```diff
diff --git a/llmprovider/chatcompletions.go b/llmprovider/chatcompletions.go
--- a/llmprovider/chatcompletions.go
+++ b/llmprovider/chatcompletions.go
@@ -25,9 +25,9 @@
 }
 
 // itemsToChatMessages converts canonical items to OpenAI Chat Completions
-// messages. Tool results become role:"tool" messages keyed by tool_call_id,
-// which is the Chat Completions equivalent of the Responses API's
-// function_call_output item.
+// messages. A function call becomes the assistant turn's tool_calls entry,
+// and its result a role:"tool" message keyed by tool_call_id, the Chat
+// Completions equivalent of the Responses API's function_call_output item.
 func itemsToChatMessages(items []Item) []map[string]any {
 	var messages []map[string]any
 	for _, item := range items {
@@ -40,6 +40,30 @@
 			messages = append(messages, map[string]any{
 				jsonKeyRole:    role,
 				jsonKeyContent: v.Text,
+			})
+		case FunctionCallItem:
+			call := map[string]any{
+				"id":        v.CallID,
+				jsonKeyType: jsonKeyFunction,
+				jsonKeyFunction: map[string]any{
+					jsonKeyName:      v.Name,
+					jsonKeyArguments: v.Arguments,
+				},
+			}
+			// A call joins the assistant turn it follows (its text, or the
+			// calls before it); otherwise it opens one (MADR 0012 §2).
+			if n := len(messages); n > 0 && messages[n-1][jsonKeyRole] == jsonRoleAssistant {
+				calls, ok := messages[n-1][jsonKeyToolCalls].([]map[string]any)
+				if !ok {
+					calls = nil
+				}
+				messages[n-1][jsonKeyToolCalls] = append(calls, call)
+				continue
+			}
+			messages = append(messages, map[string]any{
+				jsonKeyRole:      jsonRoleAssistant,
+				jsonKeyContent:   "",
+				jsonKeyToolCalls: []map[string]any{call},
 			})
 		case FunctionCallOutputItem:
 			messages = append(messages, map[string]any{
diff --git a/llmprovider/chatcompletions_test.go b/llmprovider/chatcompletions_test.go
--- a/llmprovider/chatcompletions_test.go
+++ b/llmprovider/chatcompletions_test.go
@@ -146,10 +146,13 @@
 		MessageItem{Text: "no role"},
 		MessageItem{Role: jsonRoleAssistant, Text: "assistant text"},
 		FunctionCallOutputItem{CallID: "call_1", Output: `{"ok":true}`},
-		FunctionCallItem{CallID: "c", Name: "n", Arguments: "{}"}, // unhandled: skipped
-	})
-	if len(msgs) != 3 {
-		t.Fatalf("expected 3 messages, got %d", len(msgs))
+		FunctionCallItem{CallID: "c", Name: "n", Arguments: "{}"}, // its own assistant turn (MADR 0012 §2)
+	})
+	if len(msgs) != 4 {
+		t.Fatalf("expected 4 messages, got %d", len(msgs))
+	}
+	if calls, ok := msgs[3][jsonKeyToolCalls].([]map[string]any); !ok || len(calls) != 1 || msgs[3][jsonKeyRole] != jsonRoleAssistant {
+		t.Errorf("function call message = %v, want an assistant turn with one tool call", msgs[3])
 	}
 	if msgs[0][jsonKeyRole] != jsonRoleUser {
 		t.Errorf("empty role should default to user, got %v", msgs[0][jsonKeyRole])
diff --git a/llmprovider/claude.go b/llmprovider/claude.go
--- a/llmprovider/claude.go
+++ b/llmprovider/claude.go
@@ -128,6 +128,25 @@
 
 func claudeItemsToMessages(items []Item) []map[string]any {
 	var messages []map[string]any
+	// appendBlock adds a content block to the previous message when it has the
+	// same role and already holds blocks (or, for the assistant, text), so a
+	// turn's calls and a turn's results each stay in one message (MADR 0012
+	// §2); otherwise it opens a message.
+	appendBlock := func(role string, block map[string]any) {
+		if n := len(messages); n > 0 && messages[n-1][jsonKeyRole] == role {
+			switch content := messages[n-1][jsonKeyContent].(type) {
+			case []map[string]any:
+				messages[n-1][jsonKeyContent] = append(content, block)
+				return
+			case string:
+				if role == jsonRoleAssistant {
+					messages[n-1][jsonKeyContent] = []map[string]any{{jsonKeyType: jsonKeyText, jsonKeyText: content}, block}
+					return
+				}
+			}
+		}
+		messages = append(messages, map[string]any{jsonKeyRole: role, jsonKeyContent: []map[string]any{block}})
+	}
 	for _, item := range items {
 		switch v := item.(type) {
 		case MessageItem:
@@ -141,16 +160,18 @@
 				jsonKeyRole:    role,
 				jsonKeyContent: v.Text,
 			})
+		case FunctionCallItem:
+			appendBlock(jsonRoleAssistant, map[string]any{
+				jsonKeyType:  "tool_use",
+				"id":         v.CallID,
+				jsonKeyName:  v.Name,
+				jsonKeyInput: toolArguments(v.Arguments),
+			})
 		case FunctionCallOutputItem:
-			messages = append(messages, map[string]any{
-				jsonKeyRole: jsonRoleUser,
-				jsonKeyContent: []map[string]any{
-					{
-						jsonKeyType:    "tool_result",
-						"tool_use_id":  v.CallID,
-						jsonKeyContent: v.Output,
-					},
-				},
+			appendBlock(jsonRoleUser, map[string]any{
+				jsonKeyType:    "tool_result",
+				"tool_use_id":  v.CallID,
+				jsonKeyContent: v.Output,
 			})
 		}
 	}
diff --git a/llmprovider/constants.go b/llmprovider/constants.go
--- a/llmprovider/constants.go
+++ b/llmprovider/constants.go
@@ -56,6 +56,13 @@
 	jsonKeyMaxOutputTokens = "max_output_tokens"
 	jsonKeyEffort          = "effort"
 	jsonRoleTool           = "tool"
+	// jsonKeyToolCalls is the Chat Completions assistant message's call list.
+	jsonKeyToolCalls = "tool_calls"
+	// geminiRoleModel is Gemini's assistant role.
+	geminiRoleModel = "model"
+	// geminiSkipThoughtSignature is Gemini's documented placeholder for a
+	// replayed call it did not issue (accepted live, 2026-09-27).
+	geminiSkipThoughtSignature = "skip_thought_signature_validator"
 )
 
 // Reasoning effort level values shared across providers.
diff --git a/llmprovider/gemini.go b/llmprovider/gemini.go
--- a/llmprovider/gemini.go
+++ b/llmprovider/gemini.go
@@ -132,7 +132,25 @@
 }
 
 func geminiItemsToContents(items []Item) []map[string]any {
+	// Gemini pairs a functionResponse with its functionCall by name, so a
+	// result takes the name of the call it answers (MADR 0012 §2).
+	names := map[string]string{}
+	for _, item := range items {
+		if call, ok := item.(FunctionCallItem); ok {
+			names[call.CallID] = call.Name
+		}
+	}
 	var contents []map[string]any
+	lastIsResponses := false
+	appendPart := func(role string, part map[string]any, merge bool) {
+		if n := len(contents); merge && n > 0 && contents[n-1][jsonKeyRole] == role {
+			if parts, ok := contents[n-1]["parts"].([]map[string]any); ok {
+				contents[n-1]["parts"] = append(parts, part)
+				return
+			}
+		}
+		contents = append(contents, map[string]any{jsonKeyRole: role, "parts": []map[string]any{part}})
+	}
 	for _, item := range items {
 		switch v := item.(type) {
 		case MessageItem:
@@ -140,28 +158,36 @@
 			if role == "" || role == jsonRoleUser {
 				role = jsonRoleUser
 			} else {
-				role = "model"
-			}
-			contents = append(contents, map[string]any{
-				jsonKeyRole: role,
-				"parts": []map[string]string{
-					{jsonKeyText: v.Text},
+				role = geminiRoleModel
+			}
+			appendPart(role, map[string]any{jsonKeyText: v.Text}, false)
+			lastIsResponses = false
+		case FunctionCallItem:
+			// Gemini refuses a replayed call without its thoughtSignature. A call
+			// Gemini did not issue (synthetic, or from another provider) carries
+			// the documented skip value instead.
+			signature := v.Signature
+			if signature == "" {
+				signature = geminiSkipThoughtSignature
+			}
+			appendPart(geminiRoleModel, map[string]any{
+				"functionCall":     map[string]any{jsonKeyName: v.Name, "args": toolArguments(v.Arguments)},
+				"thoughtSignature": signature,
+			}, true)
+			lastIsResponses = false
+		case FunctionCallOutputItem:
+			name := names[v.CallID]
+			if name == "" {
+				name = v.CallID
+			}
+			// A turn's results share one user turn, as its calls share one model turn.
+			appendPart(jsonRoleUser, map[string]any{
+				"functionResponse": map[string]any{
+					jsonKeyName: name,
+					"response":  map[string]any{jsonKeyOutput: v.Output},
 				},
-			})
-		case FunctionCallOutputItem:
-			contents = append(contents, map[string]any{
-				jsonKeyRole: "function",
-				"parts": []map[string]any{
-					{
-						"functionResponse": map[string]any{
-							jsonKeyName: v.CallID,
-							"response": map[string]any{
-								jsonKeyOutput: v.Output,
-							},
-						},
-					},
-				},
-			})
+			}, lastIsResponses)
+			lastIsResponses = true
 		}
 	}
 	return contents
@@ -242,6 +268,7 @@
 						Name string         `json:"name"`
 						Args map[string]any `json:"args"`
 					} `json:"functionCall"`
+					ThoughtSignature string `json:"thoughtSignature"`
 				} `json:"parts"`
 			} `json:"content"`
 		} `json:"candidates"`
@@ -267,8 +294,12 @@
 		if part.Text != "" {
 			result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: part.Text})
 		}
-		if part.FunctionCall != nil && part.FunctionCall.Args != nil {
-			argsBytes, err := json.Marshal(part.FunctionCall.Args)
+		if part.FunctionCall != nil {
+			args := part.FunctionCall.Args
+			if args == nil {
+				args = map[string]any{} // a tool without parameters
+			}
+			argsBytes, err := json.Marshal(args)
 			if err != nil {
 				return nil, fmt.Errorf("failed to marshal gemini function args: %w", err)
 			}
@@ -276,6 +307,7 @@
 				CallID:    part.FunctionCall.Name,
 				Name:      part.FunctionCall.Name,
 				Arguments: string(argsBytes),
+				Signature: part.ThoughtSignature,
 			})
 		}
 	}
diff --git a/llmprovider/grok.go b/llmprovider/grok.go
--- a/llmprovider/grok.go
+++ b/llmprovider/grok.go
@@ -140,6 +140,13 @@
 			input = append(input, map[string]any{
 				jsonKeyRole:    v.Role,
 				jsonKeyContent: v.Text,
+			})
+		case FunctionCallItem:
+			input = append(input, map[string]any{
+				jsonKeyType:      itemTypeFunctionCall,
+				jsonKeyCallID:    v.CallID,
+				jsonKeyName:      v.Name,
+				jsonKeyArguments: v.Arguments,
 			})
 		case FunctionCallOutputItem:
 			input = append(input, map[string]any{
diff --git a/llmprovider/item.go b/llmprovider/item.go
--- a/llmprovider/item.go
+++ b/llmprovider/item.go
@@ -29,6 +29,10 @@
 	CallID    string // provider-issued call identifier
 	Name      string // function name
 	Arguments string // JSON-encoded arguments
+	// Signature is an opaque token the provider issued with the call and
+	// requires back when the call is replayed (Gemini's thoughtSignature).
+	// Empty when the provider issues none.
+	Signature string
 }
 
 func (FunctionCallItem) itemKind() string { return itemTypeFunctionCall }
diff --git a/llmprovider/item_convert.go b/llmprovider/item_convert.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/item_convert.go
@@ -0,0 +1,21 @@
+package llmprovider
+
+import (
+	"encoding/json"
+	"strings"
+)
+
+// toolArguments decodes a call's JSON arguments into the object Anthropic's
+// tool_use.input and Gemini's functionCall.args require. Empty arguments are
+// an empty object; arguments that are not a JSON object are kept under
+// "arguments" rather than dropped.
+func toolArguments(arguments string) map[string]any {
+	if strings.TrimSpace(arguments) == "" {
+		return map[string]any{}
+	}
+	var args map[string]any
+	if err := json.Unmarshal([]byte(arguments), &args); err == nil && args != nil {
+		return args
+	}
+	return map[string]any{jsonKeyArguments: arguments}
+}
diff --git a/llmprovider/item_signature_test.go b/llmprovider/item_signature_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/item_signature_test.go
@@ -0,0 +1,23 @@
+package llmprovider
+
+import (
+	"strings"
+	"testing"
+)
+
+// TestItemFidelity_GeminiReplaysSignature: the thoughtSignature Gemini issues
+// with a call is kept on the item and sent back with it.
+func TestItemFidelity_GeminiReplaysSignature(t *testing.T) {
+	res, err := decodeGeminiResponse(strings.NewReader(
+		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"sig-abc"}]}}]}`))
+	if err != nil {
+		t.Fatalf("decode: %v", err)
+	}
+	call, ok := res.Output[0].(FunctionCallItem)
+	if !ok || call.Signature != "sig-abc" {
+		t.Fatalf("item = %#v, want Signature sig-abc", res.Output[0])
+	}
+	contents := geminiItemsToContents([]Item{MessageItem{Role: jsonRoleUser, Text: "weather?"}, call,
+		FunctionCallOutputItem{CallID: call.CallID, Output: "sunny"}})
+	mustEqualJSON(t, contents[1], `{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"sig-abc"}]}`)
+}
```

### B.F2 Phase F2 — System messages are top-level on Anthropic wires

**Tests** (`fid2-tests.diff`, 112 lines):

```diff
diff --git a/llmprovider/claude_system_test.go b/llmprovider/claude_system_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/claude_system_test.go
@@ -0,0 +1,53 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"net/http"
+	"net/http/httptest"
+	"testing"
+)
+
+func systemConversation() []Item {
+	return []Item{
+		MessageItem{Role: "system", Text: "Always answer in French."},
+		MessageItem{Role: "system", Text: "Be brief."},
+		MessageItem{Role: jsonRoleUser, Text: "Say hello."},
+	}
+}
+
+// TestClaude_SystemMessageIsTopLevel: system items are joined into the
+// request's system field and never sent as an assistant turn (MADR 0012 §2).
+func TestClaude_SystemMessageIsTopLevel(t *testing.T) {
+	var body map[string]any
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
+			t.Errorf("decode: %v", err)
+		}
+		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Bonjour"}]}`))
+	}))
+	t.Cleanup(srv.Close)
+	p, err := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
+	if err != nil {
+		t.Fatalf("NewClaude: %v", err)
+	}
+	if _, err := p.GenerateItems(context.Background(), systemConversation()...); err != nil {
+		t.Fatalf("GenerateItems: %v", err)
+	}
+	if body["system"] != "Always answer in French.\n\nBe brief." {
+		t.Errorf("system = %#v, want the two system items joined", body["system"])
+	}
+	mustEqualJSON(t, body[jsonKeyMessages], `[{"role":"user","content":"Say hello."}]`)
+}
+
+func TestOpencodeMessages_SystemMessageIsTopLevel(t *testing.T) {
+	p, err := NewOpencode(ProviderOpencodeGo, "k", "qwen3.8-flash", WithOpencodeRoute(OpencodeRouteMessages))
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	body := p.messagesBody(systemConversation(), nil, false)
+	if body["system"] != "Always answer in French.\n\nBe brief." {
+		t.Errorf("system = %#v, want the two system items joined", body["system"])
+	}
+	mustEqualJSON(t, body[jsonKeyMessages], `[{"role":"user","content":"Say hello."}]`)
+}
diff --git a/llmprovider/live_system_test.go b/llmprovider/live_system_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_system_test.go
@@ -0,0 +1,49 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"context"
+	"os"
+	"strings"
+	"testing"
+	"time"
+)
+
+// TestLive_SystemMessage: a system instruction reaches the model on Anthropic
+// and on OpenCode's messages route, and the model follows it.
+func TestLive_SystemMessage(t *testing.T) {
+	items := []Item{
+		MessageItem{Role: "system", Text: "You only ever reply in French."},
+		MessageItem{Role: jsonRoleUser, Text: "Say hello in one word, nothing else."},
+	}
+	for _, c := range []struct {
+		name, env string
+		build     func(key string) (ItemProvider, error)
+	}{
+		{"claude", "ANTHROPIC_API_KEY", func(k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
+		{"go-messages", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "qwen3.8-flash") }},
+	} {
+		t.Run(c.name, func(t *testing.T) {
+			key := os.Getenv(c.env)
+			if key == "" {
+				t.Skipf("%s unset", c.env)
+			}
+			p, err := c.build(key)
+			if err != nil {
+				t.Fatalf("construct: %v", err)
+			}
+			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
+			defer cancel()
+			res, err := p.GenerateItems(ctx, items...)
+			skipIfTransient(t, err)
+			if err != nil {
+				t.Fatalf("GenerateItems: %v", err)
+			}
+			if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "bonjour") && !strings.Contains(text, "salut") &&
+				!strings.Contains(text, "coucou") {
+				t.Fatalf("reply %q does not follow the system instruction", res.OutputText())
+			}
+		})
+	}
+}
```

**Fix** (`fid2-fix.diff`, 69 lines):

```diff
diff --git a/llmprovider/claude.go b/llmprovider/claude.go
--- a/llmprovider/claude.go
+++ b/llmprovider/claude.go
@@ -151,6 +151,9 @@
 		switch v := item.(type) {
 		case MessageItem:
 			role := v.Role
+			if role == jsonRoleSystem {
+				continue // top-level system field; see claudeSystemPrompt
+			}
 			if role == "" || role == jsonRoleUser {
 				role = jsonRoleUser
 			} else {
@@ -185,6 +188,9 @@
 		"max_tokens":    maxTokens,
 		jsonKeyMessages: claudeItemsToMessages(input),
 	}
+	if system := claudeSystemPrompt(input); system != "" {
+		body[jsonKeySystem] = system
+	}
 
 	if thinking {
 		body["max_tokens"] = addMessagesThinking(body, p.model, p.reasoningEffort, p.thinkingBudget, maxTokens)
diff --git a/llmprovider/constants.go b/llmprovider/constants.go
--- a/llmprovider/constants.go
+++ b/llmprovider/constants.go
@@ -34,8 +34,10 @@
 	jsonKeyEnabled     = "enabled"
 	jsonKeyType        = "type"
 	jsonKeyRole        = "role"
+	jsonKeySystem      = "system"
 	jsonRoleUser       = "user"
 	jsonRoleAssistant  = "assistant"
+	jsonRoleSystem     = "system"
 	jsonKeyParameters  = "parameters"
 	jsonKeyInput       = "input"
 	jsonKeyOutput      = "output"
diff --git a/llmprovider/item_convert.go b/llmprovider/item_convert.go
--- a/llmprovider/item_convert.go
+++ b/llmprovider/item_convert.go
@@ -19,3 +19,15 @@
 	}
 	return map[string]any{jsonKeyArguments: arguments}
 }
+
+// claudeSystemPrompt joins the system items, in order, for the Messages API's
+// top-level system field; claudeItemsToMessages leaves them out (MADR 0012 §2).
+func claudeSystemPrompt(items []Item) string {
+	var parts []string
+	for _, item := range items {
+		if m, ok := item.(MessageItem); ok && m.Role == jsonRoleSystem && m.Text != "" {
+			parts = append(parts, m.Text)
+		}
+	}
+	return strings.Join(parts, "\n\n")
+}
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -180,6 +180,9 @@
 		jsonKeyModel:    p.model,
 		jsonKeyMessages: claudeItemsToMessages(input),
 	}
+	if system := claudeSystemPrompt(input); system != "" {
+		body[jsonKeySystem] = system
+	}
 	if thinking {
 		maxTokens = addMessagesThinking(body, p.model, p.reasoningEffort, p.thinkingBudget, maxTokens)
 	}
```

### B.F3 Phase F3 — Interleaved reasoning replay on OpenCode's chat route

**Tests** (`fid3-tests.diff`, 153 lines):

```diff
diff --git a/llmprovider/live_reasoning_replay_test.go b/llmprovider/live_reasoning_replay_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_reasoning_replay_test.go
@@ -0,0 +1,59 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"bytes"
+	"context"
+	"io"
+	"net/http"
+	"os"
+	"strings"
+	"testing"
+	"time"
+)
+
+// TestLive_InterleavedReasoningReplay: an interleaved Go chat model accepts the
+// replayed reasoning on the assistant tool-call turn and answers from the
+// result. Measured 2026-09-27: kimi-k2.6 accepts the turn with or without the
+// field, so this proves acceptance, not necessity.
+func TestLive_InterleavedReasoningReplay(t *testing.T) {
+	key := os.Getenv("OPENCODE_API_KEY")
+	if key == "" {
+		t.Skip("OPENCODE_API_KEY unset")
+	}
+	t.Setenv(envDisableModelMetadata, "0") // the real models.opencode.ai document decides
+	var sent []byte
+	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		if strings.HasSuffix(r.URL.Path, "/chat/completions") && r.Body != nil {
+			b, err := io.ReadAll(r.Body)
+			if err != nil {
+				return nil, err
+			}
+			sent = b
+			r.Body = io.NopCloser(bytes.NewReader(b))
+		}
+		return http.DefaultTransport.RoundTrip(r)
+	})}
+	p, err := NewOpencode(ProviderOpencodeGo, key, "kimi-k2.6", WithHTTPClient(client))
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
+	defer cancel()
+	res, err := p.GenerateItems(ctx,
+		MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris? Use the tool."},
+		ReasoningItem{Text: "The user wants Paris weather; call get_weather."},
+		FunctionCallItem{CallID: "call_rp_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
+		FunctionCallOutputItem{CallID: "call_rp_1", Output: `{"forecast":"sunny, 21C"}`})
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("GenerateItems: %v", err)
+	}
+	if !bytes.Contains(sent, []byte(`"reasoning_content":"The user wants Paris weather; call get_weather."`)) {
+		t.Fatalf("request did not replay the reasoning: %s", sent)
+	}
+	if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
+		t.Fatalf("reply %q does not use the tool result", res.OutputText())
+	}
+}
diff --git a/llmprovider/reasoning_replay_test.go b/llmprovider/reasoning_replay_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/reasoning_replay_test.go
@@ -0,0 +1,84 @@
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
+// replayServer serves a metadata document declaring interleaved for one Go
+// model, and records the chat request body.
+func replayServer(t *testing.T, interleaved string) (*httptest.Server, *map[string]any) {
+	t.Helper()
+	t.Setenv(envDisableModelMetadata, "0") // TestMain turns the fetch off
+	var body map[string]any
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		if strings.HasSuffix(r.URL.Path, "/api.json") {
+			_, _ = w.Write([]byte(`{"opencode-go":{"models":{"kimi-k2.6":{"id":"kimi-k2.6"` + interleaved + `}}}}`))
+			return
+		}
+		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
+			t.Errorf("decode: %v", err)
+		}
+		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"sunny"}}]}`))
+	}))
+	t.Cleanup(srv.Close)
+	return srv, &body
+}
+
+func replayConversation() []Item {
+	return []Item{
+		MessageItem{Role: jsonRoleUser, Text: "weather?"},
+		ReasoningItem{Text: "Call the tool."},
+		FunctionCallItem{CallID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
+		FunctionCallOutputItem{CallID: "call_1", Output: "sunny"},
+		MessageItem{Role: jsonRoleAssistant, Text: "It is sunny."},
+		MessageItem{Role: jsonRoleUser, Text: "thanks"},
+	}
+}
+
+// TestOpencodeChat_ReplaysInterleavedReasoning: the declared field is set on
+// every assistant message, with the reasoning that preceded it or "" (as
+// OpenCode's client does, transform.ts:321-349).
+func TestOpencodeChat_ReplaysInterleavedReasoning(t *testing.T) {
+	srv, body := replayServer(t, `,"interleaved":{"field":"reasoning_content"}`)
+	p, err := NewOpencode(ProviderOpencodeGo, "k", "kimi-k2.6", WithBaseURL(srv.URL),
+		WithModelMetadataURL(srv.URL+"/api.json"), WithOpencodeRoute(OpencodeRouteChatCompletions))
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	if _, err := p.GenerateItems(context.Background(), replayConversation()...); err != nil {
+		t.Fatalf("GenerateItems: %v", err)
+	}
+	var assistants []any
+	for _, m := range (*body)[jsonKeyMessages].([]any) {
+		if msg := m.(map[string]any); msg[jsonKeyRole] == jsonRoleAssistant {
+			assistants = append(assistants, msg["reasoning_content"])
+		}
+	}
+	if len(assistants) != 2 || assistants[0] != "Call the tool." || assistants[1] != "" {
+		t.Fatalf("assistant reasoning_content = %#v, want [\"Call the tool.\" \"\"]", assistants)
+	}
+}
+
+// TestOpencodeChat_NoReplayWithoutInterleaved: a model the metadata does not
+// mark interleaved gets no reasoning field.
+func TestOpencodeChat_NoReplayWithoutInterleaved(t *testing.T) {
+	srv, body := replayServer(t, "")
+	p, err := NewOpencode(ProviderOpencodeGo, "k", "kimi-k2.6", WithBaseURL(srv.URL),
+		WithModelMetadataURL(srv.URL+"/api.json"), WithOpencodeRoute(OpencodeRouteChatCompletions))
+	if err != nil {
+		t.Fatalf("NewOpencode: %v", err)
+	}
+	if _, err := p.GenerateItems(context.Background(), replayConversation()...); err != nil {
+		t.Fatalf("GenerateItems: %v", err)
+	}
+	for _, m := range (*body)[jsonKeyMessages].([]any) {
+		if _, ok := m.(map[string]any)["reasoning_content"]; ok {
+			t.Fatalf("message %v carries reasoning_content for a non-interleaved model", m)
+		}
+	}
+}
```

**Fix** (`fid3-fix.diff`, 151 lines):

```diff
diff --git a/llmprovider/chatcompletions.go b/llmprovider/chatcompletions.go
--- a/llmprovider/chatcompletions.go
+++ b/llmprovider/chatcompletions.go
@@ -22,6 +22,9 @@
 	// Reasoning, when non-nil, is sent as the OpenRouter-style reasoning
 	// object Kilo reads: {"effort": …} or {"enabled": true}.
 	Reasoning map[string]any
+	// ReplayReasoningField, when non-empty, replays prior reasoning on every
+	// assistant message under this field (OpenCode interleaved models).
+	ReplayReasoningField string
 }
 
 // itemsToChatMessages converts canonical items to OpenAI Chat Completions
@@ -29,9 +32,20 @@
 // and its result a role:"tool" message keyed by tool_call_id, the Chat
 // Completions equivalent of the Responses API's function_call_output item.
 func itemsToChatMessages(items []Item) []map[string]any {
+	return itemsToChatMessagesReplaying(items, "")
+}
+
+// itemsToChatMessagesReplaying is itemsToChatMessages that, when field is set,
+// puts the reasoning preceding each assistant message into that field, and
+// sets it (possibly "") on every assistant message, as OpenCode's client does
+// for interleaved models (MADR 0012 §2, O5).
+func itemsToChatMessagesReplaying(items []Item, field string) []map[string]any {
 	var messages []map[string]any
+	var pending strings.Builder
 	for _, item := range items {
 		switch v := item.(type) {
+		case ReasoningItem:
+			pending.WriteString(v.Text)
 		case MessageItem:
 			role := v.Role
 			if role == "" {
@@ -72,6 +86,17 @@
 				jsonKeyContent: v.Output,
 			})
 		}
+		if field == "" {
+			continue
+		}
+		// The reasoning belongs to the assistant turn it precedes.
+		if n := len(messages); n > 0 && messages[n-1][jsonKeyRole] == jsonRoleAssistant {
+			if _, isReasoning := item.(ReasoningItem); !isReasoning {
+				prior, _ := messages[n-1][field].(string) //nolint:errcheck // absent is ""
+				messages[n-1][field] = prior + pending.String()
+				pending.Reset()
+			}
+		}
 	}
 	return messages
 }
@@ -81,7 +106,7 @@
 func chatCompletionsBody(model string, maxTokens int, input []Item, o chatCompletionsOpts) map[string]any {
 	body := map[string]any{
 		jsonKeyModel:     model,
-		jsonKeyMessages:  itemsToChatMessages(input),
+		jsonKeyMessages:  itemsToChatMessagesReplaying(input, o.ReplayReasoningField),
 		jsonKeyMaxTokens: maxTokens,
 	}
 	if o.Tool != nil {
diff --git a/llmprovider/model_metadata.go b/llmprovider/model_metadata.go
--- a/llmprovider/model_metadata.go
+++ b/llmprovider/model_metadata.go
@@ -54,6 +54,9 @@
 	ReleaseDate      string                 `json:"release_date"`
 	Status           string                 `json:"status"`
 	ReasoningOptions []modelReasoningOption `json:"reasoning_options"`
+	// Interleaved is {"field": name} when the provider expects prior reasoning
+	// replayed on assistant messages under that field, or true/absent.
+	Interleaved json.RawMessage `json:"interleaved"`
 }
 
 // modelReasoningOption is one models.dev reasoning_options entry.
@@ -79,6 +82,19 @@
 		}
 	}
 	return nil
+}
+
+// interleavedField returns the message field a model expects its prior
+// reasoning replayed under ("reasoning_content"), or "" when it declares none
+// (MADR 0012 §2, O5).
+func (d modelMetadataDoc) interleavedField(provider, model string) string {
+	var declared struct {
+		Field string `json:"field"`
+	}
+	if json.Unmarshal(d[modelMetadataKey(provider)][model].Interleaved, &declared) != nil {
+		return ""
+	}
+	return declared.Field
 }
 
 // modelMetadataKey returns the document key for a provider, or "".
diff --git a/llmprovider/opencode.go b/llmprovider/opencode.go
--- a/llmprovider/opencode.go
+++ b/llmprovider/opencode.go
@@ -253,12 +253,40 @@
 // the DeepSeek/GLM/Kimi/MiniMax families routed there share no other
 // portable reasoning parameter. Asserted by TestOpencode_Thinking_PerRoute
 // and TestOpencode_ChatReasoningEffort.
-func (p *OpencodeProvider) chatBody(input []Item, tool *Tool, effort string) map[string]any {
+func (p *OpencodeProvider) chatBody(input []Item, tool *Tool, effort, replayField string) map[string]any {
 	return chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
-		Tool:            tool,
-		ForceTool:       tool != nil,
-		ReasoningEffort: effort,
+		Tool:                 tool,
+		ForceTool:            tool != nil,
+		ReasoningEffort:      effort,
+		ReplayReasoningField: replayField,
 	})
+}
+
+// chatReplayField returns the interleaved reasoning field the model's
+// metadata declares, looked up only when input holds an assistant turn to
+// replay onto (MADR 0012 §2, O5). Unavailable metadata replays nothing.
+func (p *OpencodeProvider) chatReplayField(ctx context.Context, input []Item) string {
+	if !slices.ContainsFunc(input, isAssistantTurn) {
+		return ""
+	}
+	ctx, cancel := context.WithTimeout(ctx, metadataLookupTimeout)
+	defer cancel()
+	doc, err := loadModelMetadata(ctx, ProviderConfig{HTTPClient: p.client, ModelMetadataURL: p.metadataURL})
+	if err != nil {
+		return ""
+	}
+	return doc.interleavedField(p.gateway, p.model)
+}
+
+// isAssistantTurn reports whether an item is part of an assistant turn.
+func isAssistantTurn(item Item) bool {
+	switch v := item.(type) {
+	case FunctionCallItem:
+		return true
+	case MessageItem:
+		return v.Role == jsonRoleAssistant
+	}
+	return false
 }
 
 func (p *OpencodeProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
@@ -271,7 +299,7 @@
 	case OpencodeRouteGoogle:
 		body = p.googleBody(input, tool, thinking)
 	case OpencodeRouteChatCompletions:
-		body = p.chatBody(input, tool, p.chatReasoningEffort(ctx, thinking))
+		body = p.chatBody(input, tool, p.chatReasoningEffort(ctx, thinking), p.chatReplayField(ctx, input))
 	default:
 		return nil, fmt.Errorf("%w: unresolved opencode route for model %q", ErrInvalidRequest, p.model)
 	}
```

