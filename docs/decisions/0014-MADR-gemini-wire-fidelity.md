---
status: accepted
date: 2026-09-27
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---
# Complete Gemini's Move to the Interactions API, and Fix the `generateContent` Wire That Stays

## Context and Problem Statement

`GeminiProvider` calls Gemini's `generateContent` endpoint
(`llmprovider/gemini.go:231`), but three parts of it follow a different
Gemini API. `0012-MADR-conform-providers-to-reference-clients.md` revision 3
recorded two of them as out of scope. Investigating them for this record
found the third, and the cause common to all three.

**Where it came from.**
[0001-MADR-add-grok-xai-llm-provider.md](../0001-MADR-add-grok-xai-llm-provider.md)
decided to move Gemini onto the **Interactions API**: typed `steps[]`,
server-side state, and `previous_interaction_id`
([0001-PLAN-add-grok-xai-llm-provider.md](../0001-PLAN-add-grok-xai-llm-provider.md),
Phase 3). Commit `0578a6c` implemented that phase only in part:
* The endpoint stayed `generateContent`.
* `Continue` sends `previous_interaction_id`, an Interactions field
  (`gemini.go:222-224`).
* The decoder reads `id` and `interaction_id`, which `generateContent` never
  returns (`gemini.go:260-261`).
* The tests serve `thought` as a string, an Interactions-style fixture
  (`gemini_items_test.go:90-99`), and pin `previous_interaction_id`
  (`gemini_items_test.go:122-138`).

**The three defects.** Each was observed on 2026-09-27, against
`gemini-3.7-flash`, with probes in scratch copies of `1c3d79f`:

1. **Thought summaries cannot be read, and are never asked for.**
   * With `thinkingConfig.includeThoughts: true`, `generateContent` returns
     `{"thought": true, "text": "…summary…"}` parts. The decoder types
     `thought` as a string (`gemini.go:266`), so such a response fails to
     decode entirely:
     `json: cannot unmarshal bool into Go struct field .candidates.content.parts.thought of type string`.
   * The defect is latent only because `mcplib` never sets
     `includeThoughts` (`thinking_wire.go:109-119`). As a result, a Gemini
     thinking call never surfaces reasoning.
   * OpenCode's client sets `includeThoughts: true` for every Gemini model
     with reasoning
     (`opencode` `696f41bc8e`, `packages/opencode/src/provider/transform.ts:1280-1288`).
2. **A `system` message becomes a `model` turn.**
   * `geminiItemsToContents` maps every non-user role, `system` included, to
     `model` (`gemini.go:157-163`).
   * `generateContent` has a dedicated `systemInstruction` field.
   * Live, both forms answered "Bonjour" 3 of 3 times to "You only ever
     reply in French." On this model and prompt the effect is structural,
     not a visible failure.
3. **`Continue` always fails.** `previous_interaction_id` on
   `generateContent` answers **400** "Unknown name "previous_interaction_id":
   Cannot find field."

**The Interactions API works.** It was characterised live on 2026-09-27
(evidence in More Information):
* `POST /v1beta/interactions` answers every model in `StaticGemini`;
* it replays a stateless history;
* it chains with `previous_interaction_id`.

**Two Gemini wires remain after this decision.** OpenCode's `google` route
reaches Gemini through its gateways' `generateContent` path
(`opencode_route.go`, `OpencodeRouteGoogle.path`), and it shares
`geminiItemsToContents` and `decodeGeminiResponse` with `GeminiProvider`
today (`opencode.go:224`, `:379`).

The question is how `GeminiProvider` should speak to Gemini, and what
becomes of the `generateContent` code that OpenCode's route still needs.

## Decision Drivers

* **Correct against the endpoint called.** A request must never carry a
  field the endpoint rejects, and a response it returns must never fail to
  decode.
* **Server-side continuation works where it is offered.** `Continuer` is
  part of `GeminiProvider`'s API today (`interface_test.go:92`), and 0001
  intended it to work.
* **Parity of thinking output.** Every other thinking path returns a
  `ReasoningItem`.
* **Retain nothing new by default.** The Interactions API stores every
  request unless told not to. The docs give retention of 55 days on paid
  tiers and 1 day on free tiers. `generateContent`, which `mcplib` calls
  today, has no such storage.
* **Keep callers compiling.** No exported signature may change.
* **Small and provable.** The change must be provable red, then green, with
  mutants and live tests, like the 0012 plans.

## Considered Options

* Complete 0001's migration of `GeminiProvider` to the Interactions API, and fix the `generateContent` wire OpenCode keeps
* Conform `GeminiProvider` to `generateContent`: fix all three defects and withdraw `Continue`
* Keep `generateContent` for generation and use the Interactions API for `Continue` only
* Fix only the decoder's `thought` type

## Decision Outcome

Chosen option (**accepted** 2026-09-27: the owner chose the option, then
approved the design below with its plan): **"Complete 0001's migration
of `GeminiProvider` to the Interactions API, and fix the `generateContent`
wire OpenCode keeps"**. It makes `Continue` work, as 0001 intended. It
finishes a migration left half done. It fixes every defect on both Gemini
wires.

The owner chose this over the recorded recommendation, which was to conform
to `generateContent` and withdraw `Continue`. The consequences below state
the cost.

### 1. `GeminiProvider` speaks the Interactions API

**Endpoint and authentication.** `POST {base}/interactions`, where `base`
is the existing v1beta base URL, with the key in `x-goog-api-key` as today.
Listing (`DiscoverModels`) is unchanged.

**Request mapping.** All measured live on 2026-09-27:

| `mcplib` | Interactions request |
|---|---|
| `MessageItem` `user` | step `{"type":"user_input","content":[{"type":"text","text":…}]}` |
| `MessageItem` `assistant` | step `{"type":"model_output","content":[{"type":"text","text":…}]}` |
| `MessageItem` `system` | top-level `system_instruction` (string); several are joined with a blank line |
| `FunctionCallItem` | step `{"type":"thought","signature":S}` then `{"type":"function_call","id","name","arguments":{…}}`. `S` is the item's `Signature`, else the placeholder `skip_thought_signature_validator` |
| `FunctionCallOutputItem` | step `{"type":"function_result","call_id","name","result":<string>}`, named after its call |
| `ReasoningItem` | not sent |
| `Tool` | `tools: [{"type":"function","name","description","parameters"}]` |
| forced tool | `generation_config.tool_choice: {"allowed_tools":{"mode":"any","tools":[name]}}` |
| `WithMaxTokens` | `generation_config.max_output_tokens` |

* **Why every call is preceded by a thought step.** A replayed call without
  a thought step, or with no signature, is refused with 400. The
  placeholder is accepted before a synthetic call, and before each of
  several parallel calls.
* **`tool_choice` goes in `generation_config`.** A top-level `tool_choice`
  is rejected with "Unknown parameter", although the API reference lists
  it.

**Thinking.**
* A thinking call sends `generation_config.thinking_summaries: "auto"`.
  Without it, a thought step carries only a signature.
* **Effort.** A configured effort is sent as `thinking_level`: `low`,
  `medium` or `high`, with `xhigh` sent as `high`. With no effort, the
  model's default applies.
* **Per-model menus.** `minimal` is refused by `gemini-3.7-flash`.
  `gemini-2.5-flash-lite` refuses `medium` and fails on `low` (the server
  maps `low` to a 256-token budget the model rejects), so for that model any
  effort other than `high` is omitted.
* **No thinking budget.** The API has no budget field: `thinking_budget` is
  "Unknown parameter". `WithThinkingBudget` therefore does not apply to
  `GeminiProvider`, and its doc says so. It still applies to Claude and to
  OpenCode's `google` route.

**Response mapping.**

| Interactions response | `mcplib` |
|---|---|
| `id` | `Response.ID` |
| step `thought` with `summary: [{"type":"text","text"}]` | `ReasoningItem` with the summary texts joined |
| a `thought` step's `signature` | the `Signature` of each `function_call` that follows it, up to the next `thought` step |
| step `model_output` text content | `MessageItem` (assistant) |
| step `function_call` | `FunctionCallItem{CallID: id, Name, Arguments: <arguments as compact JSON>, Signature}`; compact, as the `generateContent` decoder returns them, because callers compare the strings |
| `status` `completed` or `requires_action` | success |
| `status` `incomplete` (as at `max_output_tokens`) | `*IncompleteError`, as in 0012 §1.5 |
| `status` `failed` or `cancelled` | `ErrProviderUnavailable` |

**Errors.** They arrive as `{"error":{"message","code"}}`, with `code` a
string such as `invalid_request` or `not_found`. The existing
`classifyHTTPError` reads that envelope.

### 2. Storage and `Continue`

* **Default.** Every `GeminiProvider` request sends `store: false`, so no
  interaction is stored, as none is today.
* **Opting in.** `WithStore(true)` sends `store: true`. It already exists for
  the Responses providers (`0012-PLAN-grok.md`, K3) and now applies to
  Gemini too.
* **`Continue`** sends `previous_interaction_id` and only the new items.
  Gemini requires `store: true` on such a request ("store must be true when
  previous_interaction_id is set"). So on a provider without
  `WithStore(true)`, `Continue` returns `ErrInvalidRequest` before any
  request, and says to enable storage.

### 3. The `generateContent` wire OpenCode keeps

OpenCode's `google` route keeps `geminiItemsToContents` and
`decodeGeminiResponse`, which then serve only that route. They gain three
fixes:
* **Thoughts.** A thinking call sends `thinkingConfig.includeThoughts:
  true`, as OpenCode's client does. A part with `thought: true` (a boolean)
  decodes to a `ReasoningItem` carrying its `text`.
* **System messages.** `system` items become
  `systemInstruction: {parts: [{text}]}` and leave `contents`.
* **Dead fields removed.** The `previous_interaction_id` parameter and the
  `id` / `interaction_id` fields go.

### Consequences

* Good, because `Continue` works for a caller that opts into storage. The
  probe chained two calls and recalled the first call's content.
* Good, because a Gemini thinking call returns its thought summary as a
  `ReasoningItem`, on both wires.
* Good, because system prompts reach Gemini through the fields built for
  them, on both wires.
* Good, because the half-finished migration of 0001 is completed rather than
  left misleading.
* Neutral, because `store: false` by default keeps retention as today. A
  caller who wants `Continue` accepts Google's retention by choosing
  `WithStore(true)`.
* Neutral, because thought summaries are billed as output tokens.
* Bad, because there are two Gemini encoders and decoders: Interactions for
  `GeminiProvider`, and `generateContent` for OpenCode's `google` route.
* Bad, because `WithThinkingBudget` no longer affects `GeminiProvider`. A
  consumer that set it now gets the model's default thinking, or the level
  its effort selects.
* Bad, because the tool-call, signature and grouping behaviour
  `0012-PLAN-item-fidelity.md` proved for `generateContent` must be
  re-established and re-proven on the new wire. The live tool round trip and
  real-signature replay tests cover it.
* Bad, because the endpoint is under `/v1beta/`, like `generateContent`
  today. Google's docs describe the Interactions API as generally available.

### Confirmation

Each check below must first be seen to fail on the unfixed code, or on a
named mutant:
* **Interactions request.** A recorded request for `[system, user,
  FunctionCallItem, FunctionCallOutputItem]` has:
  * `system_instruction`;
  * `user_input`, then `thought` (the signature, or the placeholder), then
    `function_call`, then `function_result`, keyed by `call_id`;
  * `store: false`.
* **Interactions response.** A recorded response yields a `ReasoningItem`
  from the summary, a `FunctionCallItem` carrying the preceding thought's
  signature, and `Response.ID`. An `incomplete` status is an
  `*IncompleteError`.
* **Thinking.** A thinking call sends `thinking_summaries: "auto"` and the
  effort's `thinking_level`, and `gemini-2.5-flash-lite` drops efforts other
  than `high`.
* **`Continue`.** Without `WithStore(true)` it returns `ErrInvalidRequest`
  with zero requests. With it, it sends `previous_interaction_id` and
  `store: true`.
* **`generateContent` (OpenCode `google` route).** A boolean `thought` part
  decodes. A thinking call sends `includeThoughts`. A `system` item reaches
  `systemInstruction`.
* **Live, pinned by live-tagged tests:**
  * text, a forced tool call, a tool round trip and a real-signature replay
    on `GeminiProvider`;
  * a thought summary;
  * a system instruction obeyed;
  * `Continue` recalling an earlier turn with `WithStore(true)`;
  * every `StaticGemini` model answering.

## Pros and Cons of the Options

### Complete 0001's migration of `GeminiProvider` to the Interactions API, and fix the `generateContent` wire OpenCode keeps

* Good, because `Continue` works.
* Good, because it finishes what 0001 decided.
* Good, because both wires lose all three defects.
* Bad, because it needs a second encoder and decoder, and re-proof of the
  item-fidelity behaviour.
* Bad, because `WithThinkingBudget` stops applying to `GeminiProvider`.

### Conform `GeminiProvider` to `generateContent`: fix all three defects and withdraw `Continue`

* Good, because it is the smallest change, with one encoder for both wires.
* Good, because it matches the reference client's use of `generateContent`.
* Bad, because Gemini would have no server-side continuation, contrary to
  0001.

### Keep `generateContent` for generation and use the Interactions API for `Continue` only

* Bad, because an ID from `generateContent` cannot be continued by the
  Interactions API, so `Continue` would fail for every response
  `GeminiProvider` returns.

### Fix only the decoder's `thought` type

* Good, because it is the smallest change.
* Bad, because thoughts would stay unrequested, system messages would stay
  model turns, and `Continue` would still fail with 400.

## More Information

**Relationship to earlier records.**
* **Completes**
  [0001-MADR-add-grok-xai-llm-provider.md](../0001-MADR-add-grok-xai-llm-provider.md)'s
  Gemini migration. It adds what 0001 did not decide: `store: false` by
  default, and `Continue` requiring `WithStore(true)`.
* **Completes**
  [0012-MADR-conform-providers-to-reference-clients.md](../0012-MADR-conform-providers-to-reference-clients.md)
  revision 3's two "out of scope" Gemini notes, and adds the third defect.
* **Extends** `WithStore`, from `0012-PLAN-grok.md` K3, to Gemini.

**Evidence.** Scratch probes, 2026-09-27, `GEMINI_API_KEY` never printed:

| Probe | Result |
|---|---|
| `generateContent`, `includeThoughts: true` | parts `{thought: true, text}`, `{text, thoughtSignature}`; `mcplib` decode fails |
| `generateContent` with `previous_interaction_id` | 400, unknown field |
| system as a `model` turn / as `systemInstruction`, ×3 each | "Bonjour" every time |
| Interactions, `thinking_summaries: "auto"` | `thought {signature, summary:[{type:text,text}]}`, then `model_output` |
| Interactions, no summaries | `thought {signature}` only |
| Interactions, `system_instruction` | obeyed |
| Interactions, top-level `tool_choice` | 400 "Unknown parameter 'tool_choice'" |
| Interactions, `generation_config.tool_choice` `"any"` / `allowed_tools` | `requires_action`; `function_call {id, name, arguments}` |
| chained `function_result` keyed by `call_id` (with `store: true`) | completed |
| chained with `store: false` | 400 "store must be true when previous_interaction_id is set" |
| stateless replay `user_input, thought, function_call, function_result` | completed |
| the same without the thought step, or without a signature | 400 |
| synthetic call after a placeholder thought signature | completed |
| two parallel calls, replayed as returned, or with a thought before each | completed |
| `max_output_tokens: 16` | status `incomplete` |
| `thinking_level` `minimal` on `gemini-3.7-flash` | 400; allowed: low, medium, high |
| `gemini-2.5-flash-lite` `low` / `medium` / `high` | 400 (budget 256) / 400 (not allowed) / 200 |
| `thinking_budget` in `generation_config` | 400 "Unknown parameter" |
| all six `StaticGemini` models, plain | 200 |
| unknown model | 404 `{"error":{"message","code":"not_found"}}` |

**Plan.** [0014-PLAN-gemini-wire-fidelity.md](0014-PLAN-gemini-wire-fidelity.md).
