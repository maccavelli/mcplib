---
status: in-progress
date: 2026-09-27
associated-madr: "0012-MADR-conform-providers-to-reference-clients.md"
decision-makers: mcplib maintainers
---

# Implement 0012 §1 — Shared Transport

Associated MADR: [0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
(accepted 2026-09-27, revision 2). This is the first of that MADR's six
plans (§8); §9 orders it first because §1.1's typed errors carry the terminal
conditions of §3–§5.

This plan executes MADR §1 as amended on 2026-09-27, and nothing else. If a
fact contradicts the MADR or this plan, **stop and prompt**: add a dated
entry to §9 of this plan, amend the MADR when a decision changes, and only
then continue.

**How this plan was checked.** Every site and line below was read at
`1245496` (2026-09-27). Unlike PLAN 0013, the phases were **not** executed
in advance, so the red and green messages are recorded during execution.

## Goal

* A non-2xx response becomes a typed `*APIError` that carries the service's
  own classification and message. Quota, entitlement and policy failures
  are terminal.
* The retry helpers stop on terminal errors and on retry delays beyond
  their cap, and all three share one loop.
* Generation calls get the references' 300 s header budget.
* Every request identifies `mcplib` honestly, and OpenCode and Kilo get a
  stable session id.
* A truncated Responses or tool-call answer is an error, never an empty
  success.
* `DiscoverModels` spends no generations on metered gateways.
* Kilo and OpenCode can be used without a key, on their anonymous tokens.
* The live suite skips only transient and account-state failures.

## Scope

**In scope:** MADR §1.1–§1.7, and the 0013 items the 2026-09-27 amendment
routes to §1: B3, B4, B5, B6, B7, D5 and D6.

**Out of scope:**
* §2–§6 and their plans, and every gate (G-C, G-K, G-O).
* ChatGPT's `session-id` header and any change to `originator`: §1.4 makes
  both subject to G-C, so they belong to `0012-PLAN-chatgpt-backend.md`.
* Streaming.
* The `127.0.0.1` redirect and the `CODEX_ACCESS_TOKEN` withdrawal, which
  belong to `0012-PLAN-oauth-hygiene.md`.
* A Grok key-validation path; none exists (see the MADR amendment).

## 0. Preconditions and conventions

### 0.1 Baseline

* `main` at `1245496` or a descendant, with a clean tree.
* `go test -count=1 ./...` passes before T1.

### 0.2 Gate (every phase)

1. `gofmt -l` prints nothing, and `golint -set_exit_status` passes, on each
   `.go` file the phase touched.
2. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
3. `make lint`.
4. `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

### 0.3 Red first

* Write each phase's tests first, and run them on the unfixed code.
* Where the unfixed code can express the assertion, the test must fail at
  run time; record the message.
* Tests of new API (`APIError`, `IncompleteError`, `ErrQuotaExhausted`,
  `WithSessionID`, …) cannot compile before the fix, and a build failure
  proves nothing. Each is instead proven by a named mutant, applied to a
  scratch copy of the green working tree and never to the tree itself.
* Record every mutant and its result in §10.

### 0.4 Compatibility rules (MADR §7)

* Every existing `errors.Is` assertion in the repository must pass
  unmodified.
* A plain 429 stays a `*RateLimitError`, so `errors.As(err, &rl)` keeps
  working. Every other non-2xx becomes an `*APIError`.
* No exported signature changes. Additions only: `APIError`,
  `IncompleteError`, `ErrQuotaExhausted`, `ErrNotPermitted`,
  `WithClientInfo`, `WithSessionID`, and `Response.FinishReason`.

### 0.5 Commits

* One `git commit --no-edit` per phase, after the gate.
* No push and no tag.

## Phase T0 — Start

1. Confirm §0.1.
2. Set this plan to `status: in-progress`, and the MADR to
   `status: accepted` once the owner accepts it.
3. Commit the documents only.

## Phase T1 — Typed API errors (§1.1; 0013 B3, D1, D2)

**Today.**
* `classifyHTTPStatus` (`llmprovider/http_helpers.go:90-108`) maps on status
  alone and discards the body.
* Four providers repeat that switch inline: `openai.go:188`,
  `claude.go:211`, `gemini.go:223` and `grok.go:222`.
* Kilo, Hugging Face, OpenCode and Ollama call `classifyHTTPStatus`
  (`kilo.go:209`, `huggingface.go:169`, `opencode.go:300`, `ollama.go:185`).

**Changes.**
* **New `llmprovider/api_error.go`:**
  * `APIError` with the fields MADR §1.1 lists, and `Error()`/`Unwrap()`.
  * `ErrQuotaExhausted = fmt.Errorf("llm: quota exhausted: %w",
    ErrRateLimited)`, so `errors.Is(err, ErrRateLimited)` still holds for
    quota errors (MADR §1.1).
  * `ErrNotPermitted = errors.New("llm: not permitted")`.
* **`classifyHTTPError(provider string, resp *http.Response) error`:**
  * It reads at most 64 KiB of the body, then parses the service envelope
    (MADR §1.1 table). The service comes from the `provider` label:
    `opencode-*`, `kilo`, `openai`, `grok`, `claude`, `gemini`, and a
    generic `{error:{message}}` or `{error:"…"}` parser for Hugging Face and
    Ollama.
  * It classifies per the MADR §1.1 table plus the 2026-09-27 rows:
    OpenCode 403 `FreeTierError`, 402, and 408 (408 lands in T2).
  * `Message` is bounded to 512 bytes and passed through
    `logging.RedactString`.
  * A plain 429 returns today's `*RateLimitError`.
* **Call sites:** `classifyHTTPStatus` and the four inline switches are
  replaced by `classifyHTTPError`.

**Tests** (new `llmprovider/api_error_test.go`):
1. `TestClassifyHTTPError_Table`: one fixture per classification row,
   covering sentinel, `Terminal`, `Type`, a bounded `Message`, and
   `RetryAfter`. The API is new, so it is proven by mutants (§0.3): drop a
   row's `Terminal`; map `FreeTierError` to `ErrAuthFailure`; skip the
   512-byte bound.
2. `TestProviders_ErrorCarriesServiceMessage`: through each provider's
   public `Generate`, a 400 body `{"error":{"message":"model is
   unavailable"}}` yields an error containing that text. **Red at HEAD:**
   the body is discarded (B3).
3. `TestClassifyHTTPError_RedactsMessage`: a JWT-shaped message is not
   echoed. New API, so proven by a mutant that skips redaction.
4. **Compatibility:** the existing tests that assert `ErrAuthFailure`,
   `ErrInvalidRequest`, `ErrRateLimited` and `ErrProviderUnavailable` pass
   unmodified.

**Verification:** the gate, plus
`go test -count=1 -run 'TestClassifyHTTPError_|TestProviders_Error' ./llmprovider`.

## Phase T2 — Retry policy (§1.2; 0013 B4, B5, B6)

**Today.**
* `retryWithBackoff` (`provider.go:146`) stops only on `ErrAuthFailure` and
  `ErrInvalidRequest`. It caps a `Retry-After` at 30 s and then waits for it.
* `GenerateItemsWithRetry` (`provider.go:196`) has its own copy of the loop
  (B6).
* `parseRetryAfter` (`provider.go:96`) reads integer seconds or an HTTP date.
* A negative `retries` makes no call and returns
  `failed after 0 attempts: %!w(<nil>)` (B5).
* 408 is `ErrInvalidRequest`, and is therefore never retried (B4).

**Changes.**
* `retryWithBackoff` also stops at once on:
  * `errors.As(err, &*APIError)` with `Terminal`;
  * a `RetryAfter` greater than the 30 s cap, returning the error so the
    caller can reschedule.
* `classifyHTTPError` sets `Terminal` when the response carries
  `x-should-retry: false`.
* `parseRetryAfter` also accepts `retry-after-ms` and fractional seconds.
  Its signature takes the header set, as an internal change.
* 408 maps to `ErrProviderUnavailable`, and is not terminal.
* `GenerateItemsWithRetry` calls `retryWithBackoff`.
* A negative `retries` is treated as 0: one attempt.

**Tests** (`llmprovider/provider_test.go` or a new `retry_test.go`):
1. `TestRetry_TerminalMakesOneCall`: a fake provider returns a terminal
   `APIError`. Exactly 1 call. New API, proven by a mutant.
2. `TestRetry_RetryAfterAboveCapReturns`: a 429 with `Retry-After: 120`
   makes 1 call, and the error's `RetryAfter` is 120 s. **Red at HEAD:**
   today it sleeps 30 s and retries; the test's own 5 s context ends it.
3. `TestParseRetryAfter_MillisAndFractional`: `retry-after-ms: 1500` and
   `retry-after: 1.5` both give 1.5 s. **Red at HEAD.**
4. `TestRetry_408IsRetried`: a 408 then a 200 give 2 calls and success.
   **Red at HEAD** (B4).
5. `TestGenerateItemsWithRetry_UsesSharedLoop`: a terminal `APIError` gives
   1 call, and a `Retry-After` above the cap returns at once. **Red at
   HEAD** for the cap case (B6).
6. `TestRetry_NegativeRetriesMakesOneCall`: `retries = -1` gives 1 call, and
   the error has no `%!w`. **Red at HEAD** (B5).

## Phase T3 — Timeouts (§1.3; 0013 D6)

**Today:** `defaultHTTPClient` (`options.go:11-21`) sets
`ResponseHeaderTimeout` to 30 s and `Timeout` to 60 s.

**Changes.**
* `ResponseHeaderTimeout` becomes 300 s, and `Timeout` 330 s.
* Listing keeps its own 10 s context (`modelListingTimeout`, 0013 A5).
* The 5 s metadata lookup (0013 A6) is unchanged.

**Tests:** `TestDefaultHTTPClient_Timeouts` asserts both values. **Red at
HEAD.**

## Phase T4 — Identification without impersonation (§1.4; 0013 B7)

**Today.**
* Every request carries Go's default `User-Agent`.
* OpenCode generation requests carry `x-opencode-session`, a
  `rand.Text()` id fixed per provider (`opencode.go:39-47`, `:81`, `:288`).
* OpenCode listings and each health-probe provider carry none, or a new id
  per probe (B7).
* Kilo sends no client headers.

**Changes.**
* **Options.** `ProviderConfig` gains `ClientName`, `ClientVersion` and
  `SessionID`, with options `WithClientInfo(name, version)` and
  `WithSessionID(id)`.
* **Defaults:**
  * name `mcplib`;
  * version from `runtime/debug.ReadBuildInfo` (the main module's version;
    `(devel)` when unset);
  * session id random per provider instance.
* **User-Agent.** One helper, `setClientHeaders(req, cfg)`, sets
  `User-Agent: <name>/<version> (<GOOS>; <GOARCH>) mcplib/<mcplib version>`
  on every request site. That covers generation in every provider,
  listings in `discovery.go`, the metadata fetch in `model_metadata.go`,
  and the OAuth token, device and refresh requests (with the default
  identity).
* **Per service:**
  * OpenCode: `x-opencode-session: <id>` on generation (as today), on
    listings, and on every health probe. Probes and the listing reuse the
    provider's id through `WithSessionID`.
  * Kilo: `X-KILOCODE-EDITORNAME: <name>` and `X-KiloCode-TaskId: <id>`.
  * xAI: the `User-Agent` only.
  * ChatGPT: unchanged (out of scope).
* **Forbidden, anywhere:** `x-opencode-client`, `x-grok-*`,
  `X-XAI-Token-Auth`, and any reference client's `User-Agent` or
  `originator` value.

**Tests** (new `llmprovider/identification_test.go`):
1. `TestIdentification_UserAgent`: the capture on each provider's generate
   path and on a listing matches
   `^mcplib/\S+ \(\w+; \w+\) mcplib/\S+$`. **Red at HEAD** (Go's default
   agent).
2. `TestIdentification_OpencodeListingAndProbesShareSession`:
   `DiscoverModels` on OpenCode sends one `x-opencode-session` value on the
   listing and on every probe request. **Red at HEAD** (B7). If T6 removes
   the probes, the listing half remains.
3. `TestIdentification_KiloHeaders`: both Kilo headers are present. **Red
   at HEAD.**
4. `TestIdentification_ClientInfoAndSessionOptions`: `WithClientInfo("pcm",
   "1.2.3")` gives `pcm/1.2.3 …`, and `WithSessionID("s-1")` reaches
   OpenCode and Kilo. New API, proven by mutants.
5. `TestIdentification_NoForbiddenHeaders`: none of the forbidden headers
   appears on any capture. It passes at HEAD, and is proven by a mutant
   that adds `x-opencode-client`.

**Live** (owner credentials, `-tags live_gateways`): run the existing
OpenCode Go, Kilo and xAI live tests and require no new failure. The new
headers must not be rejected.

## Phase T5 — Truncation is an error (§1.5)

**Today.**
* `decodeResponsesAPIOutput` (`http_helpers.go:24`) ignores `status` and
  `incomplete_details`.
* `decodeChatCompletionsResponse` (`chatcompletions.go:104`) ignores
  `finish_reason`.

**Changes.**
* `IncompleteError{Reason string}` wraps `ErrInvalidRequest` and is terminal.
* The Responses decoder returns it on `status: "incomplete"`, with
  `incomplete_details.reason`.
* The Chat decoder returns it on `finish_reason: "length"` when the output
  is a tool call.
* A `length`-truncated text answer still returns its text, with
  `Response.FinishReason` set (new field, `item.go:54`).

**Tests:**
1. `TestDecodeResponses_IncompleteIsError`: the fixture returns an error
   satisfying `errors.Is(err, ErrInvalidRequest)`. **Red at HEAD** (success
   with empty text).
2. `TestDecodeChat_LengthToolCallIsError`. **Red at HEAD.**
3. `TestDecodeChat_LengthTextKeepsText`: text returned, and `FinishReason ==
   "length"`. New field, proven by a mutant.
4. `TestIncompleteError_Reason`: `errors.As` yields the `Reason`. New type,
   proven by a mutant.

## Phase T6 — Probes spend no generations on metered services (§1.6)

**Today:** `DiscoverModels` probes one generation per candidate on Kilo
(`kilo.go:219`), OpenCode (`opencode.go:348`), Hugging Face
(`huggingface.go:177`) and OpenAI, including ChatGPT (`openai.go:204`).

**Changes.**
* Kilo, OpenCode and Hugging Face return their curated listing without
  `probeGenerateHealth`.
* OpenAI skips the probe in ChatGPT mode only.
* Claude, Gemini, Grok, API-key OpenAI and Ollama are unchanged.

**Tests:** `TestDiscoverModels_MeteredServicesDoNotProbe`, a table over the
four cases. The listing server counts generation requests, and 0 are
required. **Red at HEAD.** A companion test asserts that API-key OpenAI
still probes, so the change is scoped.

## Phase T7 — Keyless free use (§1.7)

**Today:** `NewKilo` and `NewOpencode` reject an empty key (`kilo.go:56`,
`opencode.go:57`).

**Changes.**
* An empty key is accepted and replaced by the service's anonymous token:
  `anonymous` on Kilo and `public` on Zen/Go.
* The token goes in whichever key header the route uses:
  `Authorization: Bearer`, `x-api-key` or `x-goog-api-key`.
* Descriptors keep `RequiresAPIKey: true`, so the wizard is unchanged.

**Tests:** `TestKeyless_KiloAndOpencodeSendAnonymousToken`, which captures
the header per route. **Red at HEAD** (the constructor errors).

**Live** (network only, no credential):
* one Kilo free model with an empty key must answer;
* one Zen free model with an empty key must return either success or the
  typed `ErrNotPermitted` `FreeTierError` from T1, never an untyped error.

Record both results.

## Phase T8 — Live suite, records and close-out (0013 D5)

1. `skipIfTransient` (`live_gateways_test.go:46-54`) skips on
   `ErrRateLimited` without `Terminal`, and on `ErrProviderUnavailable`,
   `ErrQuotaExhausted` and `ErrNotPermitted`. It no longer skips on
   `ErrInvalidRequest`.
   * Test: a unit test of the classifier, with a mutant that re-adds
     `ErrInvalidRequest`.
2. Full live suite: `go test -count=1 -tags live_gateways -run Live -v
   ./llmprovider`.
   * No FAIL.
   * Skips only per step 1 or on an unset credential.
   * A previously skipped 400 that now fails is a finding, not a skip: stop
     and prompt.
3. README: document `APIError` and the two sentinels, `WithClientInfo`,
   `WithSessionID`, the 300 s timeout, `Response.FinishReason` and keyless
   Kilo/OpenCode.
4. MADR 0012 §8: mark `0012-PLAN-shared-transport.md` complete. Fill §10,
   and set this plan to `status: complete`.

## 7. Acceptance criteria

These map to the MADR's Confirmation section:
1. One fixture per §1.1 row, plus the three 2026-09-27 rows, asserts
   sentinel, `Terminal`, `Type` and a bounded `Message`.
2. A terminal error, and a `Retry-After` above the cap, each make exactly one
   call. `retry-after-ms: 1500` and `retry-after: 1.5` both parse to 1.5 s.
3. Existing `errors.Is` assertions pass unmodified.
4. The default client's timeouts are 300 s and 330 s.
5. Each service carries exactly the §1.4 headers, and never a forbidden one.
6. An `incomplete` Responses fixture, and a `length` tool-call Chat fixture,
   return `*IncompleteError`.
7. Metered `DiscoverModels` makes no generation request.
8. Every red test failed before its fix, and every mutant was killed.
9. The live suite has no FAIL.

## 8. Rollout and rollback

**Consumer-visible changes:**
* Errors now carry service messages and may be `*APIError`.
* Quota, entitlement and policy errors now stop retries.
* Generation calls may wait up to 330 s instead of 60 s.
* Requests carry a new `User-Agent`, plus the Kilo headers.
* `DiscoverModels` no longer probes Kilo, OpenCode, Hugging Face or ChatGPT.
* Kilo and OpenCode accept an empty key.

**Rollback:** each phase is one commit, so revert in reverse order. T2
depends on T1's `APIError`. There is no data migration.

**Risks:**
* **Longer waits.** A caller with no deadline can now wait 330 s on a hung
  call (MADR Consequences). Mitigation: document that callers should set a
  context deadline.
* **Misclassified errors.** A service envelope that `mcplib` misreads falls
  back to today's status mapping; it is never silently dropped.

## 9. Deviation log

* **2026-09-27 — T1 clarification: `APIError` unwraps to two sentinels.**
  * The conflict: MADR §1.1 maps a 402 to `ErrQuotaExhausted`. Today a 402
    is `ErrInvalidRequest`, and `TestClassifyHTTPStatus` pins that. MADR §7
    requires every existing sentinel to "still match the conditions it
    matched before".
  * The resolution satisfies both clauses: `APIError.Unwrap() []error`
    returns the classified sentinel and, when it differs, the sentinel the
    status alone gave before 0012. A Kilo 402 therefore matches
    `ErrQuotaExhausted`, `ErrRateLimited` (through `ErrQuotaExhausted`) and
    `ErrInvalidRequest`.
  * The retry loop decides from `APIError.Terminal`, not from the legacy
    sentinel. That is what lets a 408 retry (B4) while it still matches
    `ErrInvalidRequest`.
  * No decision changes. `TestClassifyHTTPStatus` keeps its assertions, and
    only its call is renamed to `classifyHTTPError`.
  * The per-provider `openAIAuthError` and `grokAuthError` are replaced by
    `*APIError`. The forced refresh after a 401 now checks
    `APIError.Status`.

## 10. Execution record

| Phase | Commit | Red (unfixed code) | Mutants | Gate |
|---|---|---|---|---|
| T0 | `46c5510` | n/a | n/a | baseline `go test ./...` passed |
| T1 | the T1 commit | `TestProviders_ErrorCarriesServiceMessage` ran on an archive of HEAD. All 11 providers and routes failed with the body discarded, e.g. `llm: invalid request: claude HTTP 400, want it to carry the service's message` | 10 killed: FreeTierError not special-cased; 525 retryable; no 408 rule; 402 not quota; no legacy unwrap (`TestClassifyHTTPStatus`: `want wrapping llm: invalid request`); Kilo body scan off; no message bound; no redaction; a plain 429 as `*APIError`; flat xAI message dropped | PASS; `make lint` needed fixes in the new code first (errorlint, goconst, revive, bodyclose, errcheck) |
| T2 | the T2 commit | All 7 failed on an archive of `31b4043`: the quota 429 was retried (`calls = 3`); the 120 s retry-after was slept on (`context deadline exceeded after 5.0s`); `retry-after-ms: 1500` and `Retry-After: 1.5` gave `retry-after 0s`; the 408 wasn't retried (`calls = 1`); `x-should-retry: false` was ignored and the items helper retried the quota error (`calls = 3`); `retries = -1` gave `failed after 0 attempts: %!w(<nil>)` | none needed: every test was red at run time | PASS. Plan vs. actual: `parseRetryAfter(string)` keeps its signature, so the existing tests are unmodified. It gains fractional seconds, and a new `retryAfterFrom(http.Header)` reads `retry-after-ms` |
| T3 | the T3 commit | `ResponseHeaderTimeout/Timeout = 30s/1m0s, want 5m0s/5m30s` | none needed | PASS. The existing `TestApplyOptions_DefaultTimeout` pinned the old 60 s, which §1.3 changes on purpose, so its expected value became 330 s. Its "has a timeout" purpose is kept |
| T4 | the T4 commit | On an archive of `d034d89`: every request sent `User-Agent: Go-http-client/1.1`; the OpenCode listing had no `x-opencode-session`, and `DiscoverModels used 3 session ids, want one` (B7 reproduced); Kilo's `editor/task = ""/""`. `TestIdentification_NoForbiddenHeaders` passed there, and is proven by a mutant | 6 killed: `x-opencode-client` added; `ClientName` ignored; `SessionID` ignored; no listing session; a new session per probe; no Kilo task id | PASS. Live: the full suite had 51 PASS, 0 FAIL, 1 SKIP (a Kilo 429, whose message now reads `…: Provider returned error`). The repository has no xAI live test, so a one-off scratch probe (not committed) ran one xAI listing (6 models) and one `grok-3-mini-fast` generation with `User-Agent: mcplib/(devel) (darwin; arm64) mcplib/(devel)`; both succeeded |
| T5 | the T5 commit | On an archive of `03033cf`: the incomplete Responses body decoded as `&{ID:r1 Output:[{Text:}]}/<nil>`, and the length-cut tool call as `{"subject":"fix: tru` with no error | 3 killed: `FinishReason` dropped; a length-cut text answer rejected; `Reason` dropped | PASS |
