---
status: accepted
date: 2026-09-12
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---

# Support Browser, API-Key, and Headless Subscription Authentication for OpenAI and xAI Grok in `mcplib`

> **Revision notes (revision 2, 2026-09-12, applied in place to this same
> `proposed` document — same convention as `0001-MADR` and `0004-MADR`).**
>
> **Revision 1** researched four first-party providers (`openai`, `grok`,
> `claude`, `gemini`) and recommended native OAuth only where it is both legal
> and feasible (OpenAI Codex and xAI Grok), with Anthropic and Google remaining
> API-key in `mcplib`.
>
> **Revision 2** is a maintainer scope cut, not a reversal of that
> recommendation: **this MADR implements subscription auth for `openai` and
> `grok` only.** Claude, Gemini, Vertex ADC, and any CLI-wrap of `claude` /
> `gemini` / Antigravity are out of this decision. The revision-1 research
> that established *why* those two are excluded is kept below, struck through
> where it described them as in-scope, so the record shows what was surveyed
> and what this MADR will actually change.
>
> **Revision 3 (2026-09-12).** Maintainer answered the five remaining OpenAI/Grok
> questions, including a live host probe. See "Decisions Resolved (revision 3)".
> The Grok assumption that a session bearer might only work on the CLI chat
> proxy is **falsified**: `POST https://api.x.ai/v1/responses` returned HTTP 200
> with the OIDC session from `~/.grok/auth.json`. The CLI proxy is the wrong
> host for `mcplib` (426 version gate on `/responses`; API keys 401).
>
> **Revision 4 (2026-09-12).** Maintainer **accepted** this MADR. Implementation
> is specified in [0008-PLAN-subscription-auth-for-llm-providers.md](0008-PLAN-subscription-auth-for-llm-providers.md).
> No source changes accompany this status change.
>
> **Revision 5 (2026-09-12).** Maintainer named **prepare-commit-msg** as the
> first consumer to adopt this API. That does **not** unify fleet config
> storage (MADR 0004 out of scope stands). The hook keeps its own
> `config.json` (already mode `0600`) and a sidecar `FileTokenStore` for
> rotating OAuth sessions. MagicDev and MagicTools remain later consumers.

## Context and Problem Statement

`mcplib/llmprovider` authenticates every remote provider with a static API key string.
`NewProvider(name, apiKey, model, opts...)` (`llmprovider/provider.go`) constructs an
HTTP client that sends that string as `Authorization: Bearer`, `x-api-key`, or
`x-goog-api-key`. `wizard.ConfigureLLM` (`wizard/configure.go`) collects that key
(environment → existing → `Prompter.Secret`) and returns
`Result{Provider, APIKey, Model, BaseURL, Fallbacks}`. `ProviderDescriptor.RequiresAPIKey`
(`llmprovider/descriptor.go`) is a boolean: a credential is either required or not
(Ollama). There is no token refresh, no OAuth, no device-code flow, no credential store,
and no notion that the same provider name might talk to a different backend depending
on how the user signed in.

That design matches the library's original contract — SDK-free HTTP, one factory, one
wizard — and it is now the bottleneck. Users of the fleet already pay for **consumer
subscriptions** ~~(ChatGPT Plus/Pro, SuperGrok / xAI plans, Claude Pro/Max, Google AI
Pro/Ultra)~~ **(ChatGPT Plus/Pro and SuperGrok / xAI plans)** whose official coding
CLIs authenticate with a **browser**, a **device code**, or a **piped token**, and only
fall back to a platform API key for CI. Forcing those users onto pay-per-token keys
is the opposite of "maximum value and flexibility regarding provider configuration."

The question this MADR answers is not "can we paste a different string into `apiKey`?"
It is: **can `mcplib` grow first-class browser / API-key / headless authentication so
standalone consumers can spend a ChatGPT or Grok subscription, without abandoning the
library's SDK-free, renderer-agnostic, descriptor-driven design?**

~~The four first-party providers in scope are `openai`, `grok`, `claude`, and `gemini`.~~
**In scope (revision 2): `openai` and `grok` only.** Gateway providers
(`opencode-zen`, `opencode-go`, `huggingface`, `kilo`), local Ollama, `claude`, and
`gemini` stay API-key / no-key as they are today. Claude and Gemini were surveyed in
revision 1; they are excluded because native third-party OAuth is not legal there
(§3, §4), not because the work was forgotten.

### What this library actually does today (verified in-tree)

Facts, not design intent:

* `NewOpenAI` defaults to `https://api.openai.com/v1` and stores `apiKey` on the struct
  (`llmprovider/openai.go`). Requests use the platform Responses API, not ChatGPT.
* `NewGrok` **requires a non-empty API key** and defaults to `https://api.x.ai/v1`
  (`llmprovider/grok.go`); the header is `Authorization: Bearer` + that key.
* `NewClaude` **requires a non-empty API key** and defaults to
  `https://api.anthropic.com/v1` (`llmprovider/claude.go`); the header is `x-api-key`.
* `NewGemini` defaults to `https://generativelanguage.googleapis.com/v1beta`
  (`llmprovider/gemini.go`); the header is `x-goog-api-key` (asserted by
  `gemini_test.go`).
* `ProviderEnvVars` maps those four to `OPENAI_API_KEY`, `XAI_API_KEY`,
  `CLAUDE_API_KEY`, `GEMINI_API_KEY` (`llmprovider/provider.go`).
* `Descriptors()` derives `RequiresAPIKey` / `EnvVar` from that map
  (`llmprovider/descriptor.go`). A configuration UI cannot offer "Sign in with
  ChatGPT" because the descriptor has no auth-method list.
* `wizard.ConfigureLLM` never writes config and never logs a key (MADR 0004,
  accepted). Persistence of `Result` is each consumer's problem. Refreshable
  OAuth tokens **do not fit that shape**: a refresh token that is not stored
  cannot be refreshed, and a consumer that only persists `APIKey string` cannot
  represent an expiry or a rotated pair.
* Orchestrated processes use `NewBackplaneClient` (`backplane.go`) when
  `MCP_LLM_ENABLED=true`. That path already shares an LLM pool; it does not
  collect per-user provider credentials. `IsOrchestratorOwned()` is the only
  supported detector (`orchestrator.go`).
* The module has no OAuth dependency. Direct requires are the MCP Go SDK,
  `invopop/jsonschema`, `golang.org/x/mod`, `golang.org/x/sys`, and
  `golang.org/x/term` (`go.mod`). `llmprovider` speaks HTTP itself — "no vendor
  SDKs" is a stated design invariant (README).
* MADR 0002 accepted shared XDG-aware user paths. This checkout has no
  `userpath` package yet; any credential file still needs an owner and a
  location decision.

### Why "just send the OAuth token as the API key" is not a design

Subscription login is not a different value for the existing `apiKey` field.
Official CLIs change **at least one** of: the authorization server, the grant
type, the inference host, the request headers, the billed product, and the
model catalog.

| Provider | API-key inference | Subscription inference | Auth grant |
|---|---|---|---|
| OpenAI (**in scope**) | `https://api.openai.com/v1` Responses, Platform billing | `https://chatgpt.com/backend-api/codex/responses`, ChatGPT plan credits | OAuth PKCE at `https://auth.openai.com`, client id published in Codex OSS |
| xAI (**in scope**) | `https://api.x.ai/v1`, console.x.ai key | Session bearer with scopes `grok-cli:access` and `api:access`; CLI default host is the chat proxy | OAuth2 / OIDC at `https://auth.x.ai`, RFC 8628 device code, or `XAI_API_KEY` |
| ~~Anthropic~~ (**out of scope**, revision 2) | `https://api.anthropic.com/v1` Messages, `x-api-key` | Claude Code / claude.ai OAuth; `CLAUDE_CODE_OAUTH_TOKEN` from `claude setup-token` | Browser login in the **unmodified Claude Code binary**; third-party reuse forbidden |
| ~~Google~~ (**out of scope**, revision 2) | Gemini Developer API, `x-goog-api-key` / `GEMINI_API_KEY` | Gemini Code Assist via Google account (Pro/Ultra quotas) | Gemini CLI Google login; **third-party use of that OAuth is a stated ToS violation**. Headless Gemini CLI is API key or Vertex |

A library that keeps `NewProvider("openai", oauthAccessToken, "gpt-4.1-mini")` pointed
at `api.openai.com` will either 401 or bill the **Platform** product, not the ChatGPT
plan the user thought they were using. That is a billing defect, not a missing header.

## Research evidence

Evidence is grouped by source. Claims that are inferences from the evidence are
labelled as such.

### 1. OpenAI / Codex — official docs and local CLI source

**Official documentation** ([Authentication – Codex](https://learn.chatgpt.com/docs/auth),
retrieved 2026-09-12; CLI reference at [developers.openai.com/codex/cli/reference](https://developers.openai.com/codex/cli/reference)):

* Two sign-in methods for OpenAI models: **Sign in with ChatGPT** (subscription) and
  **Sign in with an API key** (usage-based Platform billing).
* ChatGPT login is the default when no valid session exists. The CLI opens a browser;
  the browser returns credentials to the CLI.
* Headless / no-loopback: **device code** (`codex login --device-auth`), copy
  `~/.codex/auth.json` to the remote host, SSH-forward `localhost:1455`, or pipe
  `CODEX_ACCESS_TOKEN` / `OPENAI_API_KEY` on stdin (`--with-access-token`,
  `--with-api-key`). API keys must not appear in process listings; the legacy
  `--api-key` flag is rejected.
* Tokens refresh automatically during use. Storage is `~/.codex/auth.json` (mode
  treated as a password) or the OS keyring (`cli_auth_credentials_store`:
  `file` / `keyring` / `auto` / `ephemeral`).
* ChatGPT Enterprise can mint **Codex access tokens** for trusted non-interactive
  local workflows; workload identity federation is the other headless path.
* The Codex Python SDK exposes the same three logins: `login_chatgpt()`,
  `login_chatgpt_device_code()`, `login_api_key()` (local `codex/sdk/python`).

**Local Codex source** (`codex-rs/login`, inspected 2026-09-12):

* Public OAuth client id `app_EMoamEEZ73f0CkXaXp7hrann` (`login/src/auth/manager.rs`).
* Issuer `https://auth.openai.com`; loopback ports **1455** with fallback **1457**
  (`login/src/server.rs`) — "Keep in sync with the Codex CLI Hydra redirect URI
  allow-list."
* PKCE (`login/src/pkce.rs`) + tiny-http callback server + HTML success/error pages.
* Device-code module (`login/src/device_code_auth.rs`) prints a verification URL and
  user code, then polls; the server can refuse device-code ("not enabled for this
  Codex server").
* `AuthMode` distinguishes ChatGPT vs API key vs workload identity vs PAT; ChatGPT
  mode is what drives the Codex backend, not the Platform API.

**Open-source consumer that already does this:** OpenCode's Codex plugin
(`opencode/packages/opencode/src/plugin/openai/codex.ts`) copies that same client
id and issuer, binds port 1455, and sends completions to
`https://chatgpt.com/backend-api/codex/responses` — **not** `api.openai.com`. It
parses `chatgpt_account_id` out of the JWT. Model allow-lists are Codex models,
not the Platform catalog `llmprovider` currently ships in `StaticOpenAI`.

**Feasibility for `mcplib`: high**, if and only if the ChatGPT credential selects
the Codex backend (host, headers, model catalog) rather than being stuffed into
the existing Platform client. OpenAI documents this flow for the CLI, the IDE
extension, the desktop app, and the Codex SDK. Reimplementing the published OSS
login in Go is technically straightforward; it is a new transport, not a new
header.

### 2. xAI / Grok — official CLI docs and local Grok Build source

**Official Grok Build user guide** (`grok-build` crate
`xai-grok-pager/docs/user-guide/02-authentication.md`, inspected 2026-09-12)
and CLI reference at [x.ai/docs/build/cli/reference](https://x.ai/docs/build/cli/reference):

* **Browser login is the default.** `grok` / `grok login` opens a browser against
  SpaceXAI OAuth at `auth.x.ai`. Credentials land in `~/.grok/auth.json` (Unix
  `0600`). Access tokens refresh in the background; credentials without a
  server expiry fall back to a 30-day lifetime.
* **`--device-auth`** (alias `--device-code`) is the headless path: print URL +
  user code, poll until approved. Also `GROK_LOGIN_DEVICE_FLOW`.
* **`XAI_API_KEY`** is the CI/automation fallback. Documented precedence:
  per-model key > **active session token** > `XAI_API_KEY`. Interactive session
  wins over the env key until `grok logout`.
* **Enterprise OIDC** (customer IdP, PKCE, loopback `http://127.0.0.1/callback`
  with an ephemeral port per RFC 8252 §7.3).
* **External auth provider**: a helper binary whose stdout is the token and
  whose stderr is user-facing (login URL). `GROK_AUTH_EXPIRED=1` means
  headless refresh — do not prompt. This is the pattern `mcplib` would need
  for "headless that is not device-code."
* Scopes for first-party OAuth2 include `grok-cli:access` and `api:access`
  (`xai-grok-login/src/config.rs`). Issuer constant `https://auth.x.ai`.

**Local Grok Build login crate** (`crates/codegen/xai-grok-login`):

* `LoginTransportOverride`: `ForceLoopback` (`--oauth`), `ForceDevice`
  (`--device-auth`), or resolve from env / config / remote flag; loopback is
  the default (`flow.rs`).
* Device-code is RFC 8628, two-phase (`device_code.rs`): `request_device_code`
  then `complete_device_code_login`. Surfaces are classified `Ui` / `Cli` /
  `Headless` so headless automation is not counted as a completable login.
* `AuthMode`: `Oidc` (OAuth2/OIDC session), `ApiKey`, `External`, plus a
  deprecated `WebLogin`.
* Client id is **not** a hard-coded public SDK constant in the crate; it is
  configured via `GROK_OAUTH2_CLIENT_ID` / remote settings
  (`oauth2_client_id`). OpenCode's xAI plugin **does** hard-code
  `b1a00492-073a-47ea-816f-4c329264a828` and talks to
  `https://auth.x.ai/oauth2/device/code` and `/oauth2/token`
  (`opencode/.../plugin/xai.ts`). That id matching the current first-party
  CLI is an **assumption** until xAI publishes it as a stable public client.

**Feasibility for `mcplib`: high.** ~~medium if we must also prove that a
Grok-CLI session bearer is accepted by `https://api.x.ai/v1`~~ **Revision 3
live probe (2026-09-12):** an OIDC session bearer from `~/.grok/auth.json`
(`auth_mode=oidc`, issuer `https://auth.x.ai`, client id
`b1a00492-073a-47ea-816f-4c329264a828`) returned **HTTP 200** on both
`GET https://api.x.ai/v1/models` and `POST https://api.x.ai/v1/responses`
with `Authorization: Bearer` only. The extra `X-XAI-Token-Auth: xai-grok-cli`
header the official CLI sends is **not required** on that host. The CLI chat
proxy (`https://cli-chat-proxy.grok.com/v1`) accepted the session on
`GET /models` but **426**'d `POST /responses` ("Grok CLI version (none) is
outdated") and **401**'d a console API key. `mcplib` therefore keeps
`https://api.x.ai/v1` for both API-key and session credentials; Grok does
**not** need an OpenAI-style host branch.

Client id: the official CLI's `GrokComConfig::default()` hard-codes that same
UUID (via `obfstr` in `xai-grok-login/src/config.rs`) and allows
`GROK_OAUTH2_CLIENT_ID` to override it. That UUID is a **public PKCE client**,
not a secret. `mcplib` ships it as a documented default constant, overridable
the same way. See Decision Outcome §xAI client id.

### 3. Anthropic / Claude — official docs and policy (surveyed; **out of scope**, revision 2)

**Authentication** ([code.claude.com/docs/en/authentication](https://code.claude.com/docs/en/authentication),
retrieved 2026-09-12):

* First launch opens a **browser**. WSL2 / SSH / containers that cannot reach
  the local callback paste a code into the terminal ("Paste code here if
  prompted").
* Account types: Claude Pro/Max (claude.ai), Teams/Enterprise, Claude Console
  (with or without creating an API key), Bedrock / Vertex / Foundry, Claude
  apps gateway.
* Headless CI: `claude setup-token` mints a one-year OAuth token printed to
  the terminal; the user sets `CLAUDE_CODE_OAUTH_TOKEN`. It "can only make
  model requests." Bare mode does not read it.
* Credential store: macOS Keychain, else `~/.claude/.credentials.json` mode
  `0600`.
* Precedence (abbreviated): cloud provider env → `ANTHROPIC_AUTH_TOKEN`
  (Bearer, for gateways) → `ANTHROPIC_API_KEY` (`X-Api-Key`) → `apiKeyHelper`
  script → `CLAUDE_CODE_OAUTH_TOKEN` → Anthropic profile / WIF → **subscription
  OAuth from `/login`**. An approved `ANTHROPIC_API_KEY` silently overrides a
  Pro/Max login.

**Legal and compliance** ([code.claude.com/docs/en/legal-and-compliance](https://code.claude.com/docs/en/legal-and-compliance),
retrieved 2026-09-12) — this is the constraint that decides the option, not a
footnote:

* OAuth "is intended exclusively for purchasers of Claude Free, Pro, Max, Team,
  and Enterprise subscription plans and is designed to support ordinary use of
  **Claude Code and other native Anthropic applications**."
* "Developers building products or services that interact with Claude's
  capabilities, **including those using the Agent SDK**, should use API key
  authentication through Claude Console or a supported cloud provider."
* "Anthropic **does not permit third-party developers to offer Claude.ai login
  into their own applications**, or to route requests through Free, Pro, or Max
  plan credentials on behalf of their users."
* "Developers may not collect, store, or intermediate Claude.ai credentials or
  session tokens — sign-in to a Claude account must complete through Anthropic's
  own flow."
* Hosting the **unmodified Claude Code binary**, with each end user signing in
  themselves, is the permitted product-embedding path.

`mcplib` is a third-party library consumed by twelve repositories, including
headless MCP servers and MagicTools. Implementing Claude.ai / Claude Code OAuth
inside `llmprovider` and storing the tokens would be exactly the pattern that
document forbids. Copying Claude Code's client id (as several GitHub projects
do) does not make it first-party.

**Feasibility for native OAuth in `mcplib`: low — blocked on policy, not on
HTTP.** Legal paths that remain: the existing `CLAUDE_API_KEY` / Console key;
`ANTHROPIC_AUTH_TOKEN` against a gateway the user runs; Bedrock/Vertex/Foundry
(out of scope unless a later MADR adds those transports); wrapping the
**unmodified** `claude` binary as a subprocess so Anthropic's own flow runs.
Reimplementing Claude Code OAuth in this library is not a considered-viable
option; it is listed under "rejected on evidence" below.

### 4. Google / Gemini — official docs and policy (surveyed; **out of scope**, revision 2)

**Gemini CLI authentication** ([geminicli.com/docs/get-started/authentication](https://geminicli.com/docs/get-started/authentication/),
retrieved 2026-09-12):

* Recommended for individuals, including **Google AI Pro and Ultra**: Sign in
  with Google (browser). Credentials cached locally.
* API key from AI Studio (`GEMINI_API_KEY`) — what `mcplib` does today.
* Vertex AI: Application Default Credentials via `gcloud auth application-default login`,
  service-account JSON, or a Google Cloud API key. Requires a Cloud project.
* **Headless mode** "will use your existing authentication method, if an existing
  authentication credential is cached." If not already signed in, headless must
  use **Gemini API key or Vertex AI**. There is no documented device-code grant
  for Gemini Code Assist analogous to Codex / Grok.
* As of 18 June 2026, unpaid-tier and Google One users were directed from Gemini
  CLI to **Antigravity CLI**. That churn is a stability risk for any design that
  wraps "the Google coding CLI" as a long-lived subprocess contract.

**Gemini API OAuth quickstart** ([ai.google.dev/gemini-api/docs/oauth](https://ai.google.dev/gemini-api/docs/oauth),
retrieved 2026-09-12) is a **different product**: a Google Cloud project's OAuth
client, Generative Language API, `gcloud auth application-default login`, Bearer
access token + `x-goog-user-project`. It is stricter Cloud IAM, **not** "bill
this to my Google AI Ultra subscription." Shipping Cloud-project OAuth would not
satisfy the user request for Pro/Ultra subscription use.

**Terms** ([gemini-cli `docs/resources/tos-privacy.md`](https://github.com/google-gemini/gemini-cli/blob/main/docs/resources/tos-privacy.md),
retrieved 2026-09-12):

> Directly accessing the services powering Gemini CLI (for example, the Gemini
> Code Assist service) using third-party software, tools, or services (for
> example, using OpenClaw with Gemini CLI OAuth) is a violation of applicable
> terms and policies. Such actions may be grounds for suspension or termination
> of your account.

That sentence names the exact design "reuse Gemini CLI OAuth tokens from another
process." `mcplib` sending Code Assist OAuth as `Authorization: Bearer` to a
Code Assist endpoint would be that violation. Using a Gemini **API key** against
`generativelanguage.googleapis.com` (current code) is the documented supported
method for this library's current transport.

**Feasibility for native Code Assist OAuth in `mcplib`: low — blocked on
policy.** Legal paths that remain: existing `GEMINI_API_KEY`; Vertex ADC (real
Google browser login, Cloud billing, not Ultra); wrapping the unmodified Gemini
or Antigravity CLI. There is no official device-code subscription path to copy.

### 5. How other open-source projects handle this

Inspected locally and on the public web, 2026-09-12. Three recurring patterns:

**A. Reimplement the vendor CLI's OAuth (copy client id, PKCE, loopback or device
code, rewrite the inference host).**

* OpenCode: first-party plugins for Codex (`chatgpt.com/backend-api/codex/responses`,
  client `app_EMoamEEZ73f0CkXaXp7hrann`) and xAI (device-code at `auth.x.ai`,
  client `b1a00492-…`, label "SuperGrok Subscription"). Auth methods are
  `oauth` | `api` (`packages/opencode/src/provider/auth.ts`). Token refresh is
  in the fetch wrapper, with a documented cross-process stale-disk limitation
  when refresh-token rotation races.
* CortexKit `openai-auth`, clodex, ChatCLI, dsh-plugin-oauth-subs: same Codex
  client id, same three methods (browser / headless device / API key).

This is the pattern that **works technically** for OpenAI and xAI. It is also
the pattern Anthropic and Google have written ToS language to kill.

**B. Wrap or import the official CLI's credential files / subprocess.**

* `subllm`, `unified_cli`, CLIProxyAPI, `coding_agent_account_manager`: the
  user runs `claude login` / `codex login` / `gemini` / `grok login` once; the
  tool either shells out to that binary or copies `~/.codex/auth.json`,
  `~/.grok/auth.json`, `~/.claude/.credentials.json`, `~/.gemini/`.
* Anthropic's own docs bless "unmodified Claude Code binary, user signs in
  themselves." Google's docs do **not** bless reading Gemini CLI's OAuth cache
  from a third-party process.

**C. Give up on subscription for some vendors and say so.**

* `ai-sub-auth` (2026 README): only OpenAI Codex OAuth is treated as a supported
  third-party subscription bridge; Claude and Gemini are API-key-only after
  Anthropic's ban and Google's Code Assist ToS. That matches the official
  documents cited above, even if the project's other claims are not independently
  verified here.

No inspected Go library of `mcplib`'s shape (SDK-free multi-provider HTTP
adapters) implements all four vendors' subscription OAuth as a first-class,
ToS-clean feature. The honest OSS lesson is: **OpenAI and xAI published the
flows; Anthropic and Google published the prohibition.**

### 6. What "browser / key / headless" maps to in the vendors that allow it

This is the vocabulary the user asked for, grounded in the two CLIs we have
source for. Anthropic and Google columns are the revision-1 survey; they are
**not** methods this MADR will implement.

| User-facing method | OpenAI Codex (in scope) | xAI Grok (in scope) | ~~Anthropic~~ (out of scope) | ~~Google Gemini CLI~~ (out of scope) |
|---|---|---|---|---|
| **Browser** | OAuth authorization code + PKCE, loopback `:1455`/`:1457`, default | OAuth2/OIDC PKCE, loopback, default | Claude Code opens browser; paste-code fallback when loopback unreachable | Google account login, loopback to the CLI host |
| **API key** | stdin `--with-api-key`; Platform billing | `XAI_API_KEY`; fallback when no session | `ANTHROPIC_API_KEY` / Console; **the supported third-party path** | `GEMINI_API_KEY`; **the supported third-party path** |
| **Headless** | Device code; stdin access token; copy `auth.json`; SSH-forward 1455; workload identity | Device code; external helper binary; copy `auth.json`; env key | `claude setup-token` → `CLAUDE_CODE_OAUTH_TOKEN` (Claude Code / Agent SDK surfaces, not third-party apps); cloud provider env | Cached prior login, else API key or Vertex ADC. No device-code for Code Assist |

"Headless" is therefore **not one protocol**. It is device-code where the vendor
ships RFC 8628, token-on-stdin where they ship that, credential-file copy,
loopback tunnelling, or a helper binary. For OpenAI and Grok, device-code plus
stdin/import covers the cases the official CLIs document. Vertex and Claude
Console headless paths are irrelevant to this MADR after the revision-2 cut.

## Decision Drivers

* **Users with a ChatGPT or Grok subscription should be able to spend it** in
  standalone consumers (MagicDev, prepare-commit-msg, and any future wizard).
* **Do not ship a ToS trap.** Revision 1 established that Claude.ai and Gemini
  Code Assist OAuth from a third-party library is such a trap; revision 2
  therefore keeps those providers off this MADR entirely.
* **Auth method is data the descriptor must own**, the same way MADR 0004 made
  provider identity and catalogs data the descriptor owns. Adding a grant type
  must not require three wizard edits.
* **A credential is not a string.** Refresh, expiry, rotation, and "this token
  is only valid on host X" have to be in the type system or they will be lost
  in `Result.APIKey`.
* **Transport follows credential.** ChatGPT login must not silently hit
  `api.openai.com`. Grok session tokens must not silently hit a host that
  rejects them.
* **`mcplib` stays SDK-free.** No OpenAI or xAI official Go SDK. OAuth is HTTP
  (authorize, token, refresh, device code) plus a loopback listener. That is
  the same dependency posture as today's providers.
* **Renderer-agnostic.** Browser-open, "visit this URL and enter this code",
  and "paste the code" are `Prompter` operations (or a small extension of it),
  not a TUI toolkit import. Nine headless MCP servers must not grow a browser
  dependency.
* **Two runtime modes, already in the library.** Orchestrated sub-servers use
  the MagicTools LLM backplane (`NewBackplaneClient`). Standalone processes
  configure their own `TokenSource`. Subscription OAuth is never injected into
  sub-servers via `MCP_LLM_*`.
* **Do not regress API keys.** CI and users who prefer `OPENAI_API_KEY` /
  `XAI_API_KEY` keep working. `NewProvider(name, apiKey, model)` remains valid.
  `claude` and `gemini` constructors are not part of this change.
* **Testability without a vendor account.** PKCE, device-code polling, token
  refresh, and "wrong host for this credential" must be driveable against
  `httptest` and a scripted `Prompter`.

## Considered Options

* **1. Credential-source abstraction + three user-facing methods; native OAuth
  for OpenAI Codex and xAI Grok only** (chosen, revision 2)
* **2. Native OAuth for all four first-party providers**, copying each official
  CLI's client id and talking to each subscription backend (rejected: Anthropic
  and Google policy; maintainer then cut those vendors from scope)
* **3. Do not implement OAuth; wrap official CLIs as subprocess backends**
  (`codex`, `grok`, and in revision 1 also `claude` / `gemini`)
* **4. Import-only: read `~/.codex/auth.json` and `~/.grok/auth.json` after the
  user logs in with the vendor CLI; no login UX in `mcplib`**
* **5. Status quo: API keys only**
* **6. Add official vendor SDKs** (rejected on the existing library invariant)

Option 2 is listed because it is what the original request sounded like and what
OpenCode does for OpenAI and xAI. It is rejected for Anthropic and Google on
**documented policy**, not taste. Revision 2 does not reopen it.

## Decision Outcome

Chosen option: **"1. Credential-source abstraction + three user-facing methods;
native OAuth for OpenAI Codex and xAI Grok only"**, because those are the two
providers for which subscription login is both **legal** (vendor-documented
browser / device-code / key flows) and **feasible** (HTTP transports this
library can speak), and because the maintainer (revision 2) limited the work to
that pair rather than leaving Claude and Gemini as in-scope API-key leftovers.

~~If the maintainer later wants Claude Pro/Max or Google AI Ultra inside the
fleet, the supported extension is **option 3 for those two vendors only**
(unmodified vendor binary, user completes that vendor's own login) — not option
2. That extension is out of this MADR's implementation scope and would be its
own decision.~~ **Revision 2:** that follow-up is not part of this decision
at all. A later MADR may consider it; this one will not.

### What "option 1" means in this codebase

Three layers, same split as MADR 0004: **mcplib owns data and flow; consumers
own rendering and app-config persistence.**

#### 1. A credential is a typed source, not a string

Replace the implicit `apiKey string` at the provider boundary with a small
exported set of types (names indicative, not an API freeze):

```go
// TokenSource yields a bearer or key for a single request and may refresh.
type TokenSource interface {
    Token(ctx context.Context) (Token, error)
}

type Token struct {
    Value     string
    Type      TokenType // APIKey, Bearer, HeaderAPIKey
    Expiry    time.Time // zero = no known expiry
    Header    string    // "Authorization" for OpenAI/Grok; other providers unchanged
}

type StaticKey struct{ Key, Header string } // today's behaviour

type OAuthSession struct {
    Access, Refresh string
    Expiry          time.Time
    Issuer          string
    ClientID        string
    // RefreshPOST is owned by the provider adapter, not the wizard.
}

type ExternalHelper struct {
    Command string // stdout = token; stderr = user-facing; cf. Grok's contract
}
```

`NewProvider(name, apiKey, model, opts...)` stays: a non-empty `apiKey` becomes
a `StaticKey` internally. A new constructor (name indicative)
`NewProviderWithSource(name, src TokenSource, model, opts...)` is how OAuth
sessions enter, and it is **only valid for `openai` and `grok`**. Other
providers keep today's `apiKey string` field and do not grow a `TokenSource`.
OpenAI and Grok **must not** cache a token string across process lifetime
without going through `TokenSource.Token`.

`wizard.Result.APIKey` gains a sibling (or is replaced by) a
`Credential` value the consumer persists in **its** schema. MADR 0004's "we do
not unify YAML" still holds. What 0004 did not anticipate is that some
credentials **rotate**. That requires a `TokenStore` **seam** in `mcplib`:

* Interface: load / save / delete a named session (provider id + user-chosen
  label).
* One shipped implementation (revision 3): a `0600` file under the
  consumer-supplied directory (or, once MADR 0002's path helper exists, the
  XDG data directory). No OS keyring in v1.
* Refresh writes the rotated refresh token **before** the next request uses it.
  OpenCode's xAI plugin documents the failure mode if disk update is
  best-effort: the next process 4xxs and forces re-login. `mcplib` must not
  ship that bug.

`logging.Redact` remains the log path; `logging.MaskSecret` remains the UI
fingerprint. OAuth access and refresh tokens go through both, never through
`slog` raw.

#### 2. Descriptors grow an auth-method list

`ProviderDescriptor` today has `RequiresAPIKey bool`. That field stays (it is
true iff API key is a valid method) and is joined by:

```go
type AuthMethodID string // "api_key" | "browser_oauth" | "device_code" | "token_stdin" | "import_vendor_cli"

type AuthMethod struct {
    ID          AuthMethodID
    Label       string // "Sign in with ChatGPT", "xAI API key", …
    Detail      string // billing / when-to-use
    Interactive bool   // wizard may offer it
    HeadlessOK  bool   // device-code, stdin, import
}
```

Menu order is part of the descriptor, same as provider order today.
`TestDescriptors_CoverEveryRegisteredProvider` grows a per-provider assertion
of **which methods exist**, so a new grant cannot ship without a wizard path.

Default methods this MADR commits to:

| Provider | `api_key` | `browser_oauth` | `device_code` | `token_stdin` | `import_vendor_cli` |
|---|---|---|---|---|---|
| `openai` | yes (Platform) | yes (ChatGPT / Codex) | yes | yes (`CODEX_ACCESS_TOKEN` / `OPENAI_API_KEY`) | yes (`~/.codex/auth.json`) |
| `grok` | yes | yes (default public client + override) | yes | yes (`XAI_API_KEY`) | yes (`~/.grok/auth.json`) |
| `claude`, `gemini`, gateways, Ollama | unchanged (API key / none) | **no** | **no** | unchanged | **no** |

~~`claude` / `gemini` rows with explicit **no** on OAuth, and Vertex ADC as a
later Gemini auth method, were revision-1 leftovers.~~ **Revision 2:** those
providers are not in this MADR. Their descriptors do not grow `AuthMethod`
lists here. A test still asserts they do not acquire `browser_oauth` /
`device_code`, so a later change cannot sneak them in under this decision.

#### 3. Transport is selected by credential type

This is the load-bearing technical change.

* `openai` + `StaticKey` → today's `https://api.openai.com/v1` Responses client
  and `StaticOpenAI` catalog.
* `openai` + ChatGPT `OAuthSession` → Codex backend
  `https://chatgpt.com/backend-api/codex/responses` (override via option),
  ChatGPT account headers as required by that backend, and a **Codex** model
  catalog — not `gpt-4.1-mini` unless live listing says the plan includes it.
  `ListAvailableModels` must branch on credential type.
* `grok` + `StaticKey` → today's `https://api.x.ai/v1`.
* ~~`grok` + session `OAuthSession` → host confirmed by a live probe~~
  **Revision 3:** `grok` + session `OAuthSession` → **the same**
  `https://api.x.ai/v1`. No second host. Do **not** send
  `X-XAI-Token-Auth: xai-grok-cli` (that marker identifies the official CLI;
  the probe showed it is unnecessary on this host). Do **not** call
  `cli-chat-proxy.grok.com` (426 version gate; not an API-key surface).

`claude`, `gemini`, and the gateways do not grow a second host or a
`TokenSource` in this MADR.

#### 4. `wizard.ConfigureLLM` grows an auth-method step

After provider select, before model select:

1. Offer `AuthMethod`s from the descriptor (filterable via `Options`, same as
   `Providers []string` today).
2. **API key:** existing `resolveAPIKey` (env → existing → `Secret`).
3. **Browser OAuth:** generate PKCE, bind loopback `127.0.0.1` on a vendor-known
   port (OpenAI: 1455 then 1457; xAI: ephemeral port, RFC 8252), `Notify` the
   URL, attempt to open a browser (failure is not fatal — print the URL), wait
   for the callback or a `Prompter.Input` paste-code fallback (OpenAI and Grok
   document the same class of WSL/SSH loopback failure). Exchange the
   code, persist via `TokenStore`.
4. **Device code:** start RFC 8628, `Notify` verification URL + user code,
   poll with `authorization_pending` / `slow_down` handling (Grok and OpenCode
   already do this), persist.
5. **Token stdin / env:** existing `AllowEnv` plus an explicit "paste a token"
   path that does not echo (`Secret`).
6. **Import vendor CLI:** read the documented file (`~/.codex/auth.json`,
   `~/.grok/auth.json`), never log it, confirm with `MaskSecret`.

`Prompter` stays toolkit-free. Browser-open is a function option the consumer
injects (`OpenURL func(string) error`); `mcplib` does not import a GUI stack.
A default that shells out to `open` / `xdg-open` / `cmd /c start` is acceptable
behind that function, matching how the official CLIs do it.

Device-code and paste-code are **headless** in the sense the user asked: the
machine running `mcplib` does not need a local browser, only that the human has
a browser somewhere.

#### 5. Orchestrated mode **and** standalone (revision 3: both)

The library already has two runtime modes. This MADR uses both; it does not
invent a third.

* **Orchestrated** (`IsOrchestratorOwned()` / `MCP_LLM_ENABLED=true`):
  sub-servers call `NewBackplaneClient` and do **not** run
  `wizard.ConfigureLLM` OAuth, do not hold ChatGPT/Grok refresh tokens, and
  do not grow `MCP_LLM_*` bindings. MagicTools is the LLM. No change to
  `backplane.go`.
* **Standalone**: the process owns a `TokenSource` (API key, browser OAuth,
  device-code, stdin token, or import of `~/.codex/auth.json` /
  `~/.grok/auth.json`) and calls `NewProvider` / `NewProviderWithSource`
  itself.

MagicTools, if it wants the *backplane itself* to spend an operator ChatGPT
or Grok subscription, logs in **once in the orchestrator process** and keeps
serving sub-servers over the existing backplane protocol. That is a MagicTools
change, not an `mcplib` env-var smuggle. `mcplib` only has to keep the two
modes distinct so a sub-server never becomes a second OAuth client.

#### 6. Dependencies

No vendor SDKs. PKCE, device-code, and refresh are `net/http` +
`crypto/sha256` + a loopback `http.Server`, the same way Codex and Grok
Build implemented them in Rust and OpenCode in TypeScript.

`golang.org/x/oauth2` is **not** required and is **not** added by this
decision. It would be a fourth direct module (after `x/term`) for a problem
the stdlib already covers, and it does not know Codex's 1455 allow-list or
xAI's device endpoint.

#### 7. xAI OAuth client id (revision 3: documented default + override)

The official Grok CLI does not ask the user for a client id. `GrokComConfig::default()`
sets `issuer` to `https://auth.x.ai` and `client_id` to
`b1a00492-073a-47ea-816f-4c329264a828` (`xai-grok-login/src/config.rs`, wrapped
in `obfstr` in the CLI binary). `GROK_OAUTH2_CLIENT_ID` /
`GROK_OAUTH2_ISSUER` override that pair. The live probe's `~/.grok/auth.json`
scope key was `https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828`.
OpenCode hard-codes the same UUID because it is the first-party public client.

The idiomatic `mcplib` shape matches **both** the official CLI and how this
MADR already treats Codex (`app_EMoamEEZ73f0CkXaXp7hrann`):

* Export `DefaultGrokOAuthClientID` and `DefaultGrokOAuthIssuer` as documented
  constants. They are public PKCE clients, not secrets; do not obfuscate them
  in Go (a library constant is source, not a shipped CLI binary).
* Use those defaults for browser and device-code login when the caller does
  not pass an override.
* Honour an option / env override (`GROK_OAUTH2_CLIENT_ID`, same name as the
  official CLI) for enterprise or future client rotations.
* Refresh and `import_vendor_cli` use the `oidc_client_id` stored on the
  session, not the compile-time default, so a file minted by `grok login`
  round-trips.

Rejected alternatives: configuration-only with no default (the official CLI
never requires this of users); silently citing OpenCode as the source of
truth (the source is grok-build's default); minting a distinct mcplib client
id (xAI has not published a third-party app registration for this library).

#### 8. `TokenStore` (revision 3: `0600` file in v1)

One shipped implementation: a file under the consumer-supplied directory,
created and kept at Unix mode `0600` (owner read/write only), matching
`~/.grok/auth.json` and Codex `auth.json` when stored as a file. No OS
keyring in v1. The interface still allows a keyring implementation later.

#### 9. Compatibility

* Existing API-key configs keep working without a wizard re-run.
* `RequiresAPIKey` remains true for `openai` and `grok` so older callers that
  only understand that flag still prompt for a key.
* `claude`, `gemini`, gateways, and Ollama are untouched.
* This MADR does not add Bedrock, Vertex, Foundry, or any Claude/Gemini auth
  method.

### Feasibility verdict (the question that was asked)

**Technically feasible as a credential-and-transport refactor of `openai` and
`grok`.** Revision 1's four-vendor table is struck to the in-scope pair:

| Vendor | Spend a consumer subscription from `mcplib` HTTP? | Feasible under this decision? | What actually has to change |
|---|---|---|---|
| OpenAI | Yes, via Codex backend + ChatGPT OAuth | **Yes** | New grant + **new host/catalog/headers**; keep Platform key path |
| xAI | Yes, via Grok CLI-equivalent session | **Yes (probed 2026-09-12)** | New grant on the **existing** `https://api.x.ai/v1` host; keep `XAI_API_KEY` |
| ~~Anthropic~~ | Not via third-party HTTP | **Out of scope** | No change in this MADR |
| ~~Google~~ | Not via Code Assist OAuth from a third-party | **Out of scope** | No change in this MADR |

### Consequences

* Good, because ChatGPT Plus/Pro and Grok subscription users can configure
  standalone consumers without buying a second, metered Platform key.
* Good, because browser / device-code / key become descriptor data on `openai`
  and `grok`, so a later vendor that publishes a legal OAuth path can follow
  the same seam without another three-wizard rewrite (MADR 0004's property).
* Good, because API-key CI and the orchestrated backplane are unchanged, and
  `claude` / `gemini` / gateways are not part of the refactor.
* Good, because keeping Claude.ai and Gemini Code Assist OAuth **out of this
  MADR** is an explicit, citable decision rather than an accident the next
  contributor "fixes" by pasting a client id from GitHub.
* Neutral, because `Result` / `NewProvider` grow types for OpenAI and Grok;
  consumers must persist a structured credential if they opt into OAuth.
  API-key-only consumers, including every Claude/Gemini caller, can ignore
  the new types.
* Neutral, because model catalogs for ChatGPT-mode OpenAI will diverge from
  `StaticOpenAI`. Live listing already exists; it must be credential-aware.
* Bad, because this is a real refactor of the OpenAI and Grok adapters: they
  must call `TokenSource` per request. Tests that inject keys keep working
  only if `StaticKey` is the default for those two.
* ~~Bad, because Claude Pro/Max and Google AI Ultra still cannot be spent
  inside `llmprovider` HTTP after this lands.~~ **Revision 2:** that is no
  longer a consequence of a half-measure; those providers are out of scope.
* Bad, because OAuth means secrets on disk that expire. `0600` files, refresh
  races, and "login expired, run the wizard" become operational surface the
  library does not have today.
* Bad, because a loopback listener and a browser-open helper are new failure
  domains (firewall, WSL, SSH, remote vs local). Device-code and paste-code
  exist specifically so those failures are not dead ends.

### Confirmation

Compliance with this decision is confirmed by tests that are required to have
been seen to fail on a deliberately wrong input (per project rule), not only
to pass:

* A ChatGPT-mode OpenAI client pointed at `api.openai.com` in a fixture **fails
  a test that asserts the Codex host**; the assertion is proven by temporarily
  aiming it at the Platform host and watching it fail.
* An API-key OpenAI client does **not** send requests to
  `chatgpt.com/backend-api`.
* `Descriptors()` for every provider other than `openai` and `grok` contain
  no `browser_oauth` / `device_code` method; a test that expects those
  methods on `claude` or `gemini` **fails**, proving the scope cut is
  enforced rather than documented only.
* Device-code polling honours `slow_down` and gives up at `expires_in`; a fake
  token endpoint that never approves must not hang the wizard past the
  deadline.
* Token refresh persists the new refresh token; a two-process fixture where
  process A refreshes and process B still holds the old refresh token is the
  negative case OpenCode already documented.
* `logging` tests: an access token in a log buffer is redacted; `MaskSecret`
  is used on wizard confirmations.
* `NewProvider("openai", "sk-…", model)` still constructs a Platform client
  (regression).
* A Grok session `TokenSource` pointed at `cli-chat-proxy.grok.com` in a
  fixture **fails** a test that asserts `api.x.ai`; the assertion is proven
  by aiming it at the proxy host and watching it fail (the live probe's 426
  is the production analogue).
* A Grok session request does **not** send `X-XAI-Token-Auth`.
* `TokenStore` file is created mode `0600`; a test that writes and then
  `stat`s the mode fails if the mode is wider.
* No new module appears in `go.mod` `require` except any that a later approved
  plan explicitly adds. This MADR adds none.

The Grok host probe is **done** (revision 3). An OpenAI ChatGPT-login live
probe against the Codex backend remains plan work: it needs an interactive
browser session and is not required to lock the transport (Codex source and
docs already name the host).

### Scope boundaries

**In scope for a later approved plan implementing this MADR:**

* `TokenSource` / `TokenStore` / `AuthMethod` on descriptors.
* Wizard auth-method step, PKCE loopback, RFC 8628, token-stdin, vendor-CLI
  import for OpenAI and xAI.
* OpenAI ChatGPT transport + catalog branch.
* Grok session transport once the live host probe has an answer.
* Tests listed under Confirmation.
* README notes that subscription login is offered for OpenAI (ChatGPT) and
  Grok only; other providers remain API-key / no-key.

**Out of scope, deliberately:**

* `claude` and `gemini` in any form: no native OAuth, no Vertex ADC, no
  wrapping of `claude` / `gemini` / Antigravity. Revision 1 surveyed them;
  revision 2 removed them from the decision.
* Wrapping `codex` or `grok` as subprocesses (option 3) — rejected in favour
  of native HTTP for those two.
* Unifying consumer YAML/config persistence (MADR 0004 out of scope stands).
* Teaching MagicTools to spend an operator ChatGPT/Grok login **on the
  backplane itself**. `mcplib` only keeps orchestrated sub-servers on
  `NewBackplaneClient` and standalone processes on `TokenSource`.
* Bedrock, Foundry, Vertex.
* Gateway providers' OAuth (OpenCode Zen/Go already use `OPENCODE_API_KEY`).
* Vendor SDKs; `golang.org/x/oauth2`.
* OS keyring (Codex's `keyring` mode). A `TokenStore` implementation can be
  added later without changing the interface.
* Multi-account rotation / "swap when you hit the cap" (the `caam` pattern).
* Enterprise OIDC against a customer IdP (Grok supports it; this MADR ships
  first-party `auth.x.ai` plus API key plus import). Customer IdP is a later
  option on the same `TokenSource` seam.

## Pros and Cons of the Options

### 1. Credential-source abstraction; native OAuth for OpenAI and xAI (chosen)

Implements browser, API-key, and headless (device-code / stdin / import) for
`openai` and `grok`. Other providers are unchanged.

* Good, because it matches published vendor policy instead of betting that
  enforcement is lax.
* Good, because it is the actual Codex and Grok CLI architecture, which we
  have in-tree as reference implementations.
* Good, because MADR 0004's "add it to the descriptor, every wizard updates"
  property extends to auth methods on the two providers that gain them.
* Neutral, because Claude and Gemini stay on API keys. That is the revision-2
  scope cut, not a deferred half-implementation.
* ~~Bad, because two of the four named providers do not gain subscription spend
  in this library.~~ **Revision 2:** those two are not named by this MADR.

### 2. Native OAuth for all four first-party providers

Copy each official CLI's client id; send subscription traffic from `mcplib`.

* Good, because it is what the user request describes and what OpenCode does
  for OpenAI and xAI.
* Bad, because Anthropic's legal-and-compliance page forbids third-party
  Claude.ai login, collecting/storing Claude.ai tokens, and routing Pro/Max
  credentials on behalf of users — including via the Agent SDK.
* Bad, because Google's Gemini CLI ToS forbids third-party software from
  accessing Gemini Code Assist using Gemini CLI OAuth, and names account
  suspension as the consequence.
* Bad, because even if those two calls "work" on day one, they are a
  time-bomb under a library twelve repos consume. A ban shows up in the
  user's Google or Anthropic account, not in our test suite.
* Neutral, because OpenAI and xAI parts of this option are absorbed into
  option 1.

### 3. Wrap official CLIs as subprocess backends

`llmprovider` shells out to `codex` / `grok` (revision 1 also considered
`claude` / `gemini`). Auth is entirely the vendor's problem.

* Good, because Codex and Grok are the official apps for those subscriptions.
* Good, because we do not reimplement PKCE or hold refresh tokens.
* Bad, because `mcplib` becomes a process supervisor: PATH, version skew,
  stdout JSON-vs-text, non-interactive flags, timeout, and a hard dependency
  on moving CLIs.
* Bad, because the library's "HTTP, no vendor SDK" invariant becomes "HTTP
  plus foreign binaries," which nine headless MCP servers may not have
  installed.
* Bad, because structured `GenerateItems` / tool-calling / thinking budgets
  have to be reverse-engineered from each CLI's non-interactive surface,
  which is a worse API than the HTTP we already speak.
* ~~Neutral, because it remains the **only** ToS-clean way to spend Claude
  Pro/Max or Gemini Code Assist from this fleet.~~ **Revision 2:** that
  follow-up is not this MADR. For OpenAI and Grok, native HTTP (option 1) is
  chosen over wrapping.

### 4. Import-only vendor credential files

No login UX in `mcplib`; read `~/.codex/auth.json` / `~/.grok/auth.json`
after the user uses the official CLI.

* Good, because it is small, matches how `caam` / CLIProxyAPI bootstrap, and
  reuses a login the user already completed.
* Good, because we do not run a loopback server.
* Bad, because it does not "support browser/key/headless auth" in *this*
  library — it supports "hope the user installed Codex." MagicDev and
  prepare-commit-msg would still send people to another product's wizard.
* Bad, because importing `~/.claude/.credentials.json` or Gemini CLI's OAuth
  cache is the "intermediate Claude.ai / Code Assist tokens" pattern both
  vendors forbid, even if we did not run the authorize URL ourselves.
* Neutral, because import of **OpenAI and xAI** files is included in option 1
  as one headless method, not as the whole design.

### 5. Status quo: API keys only

* Good, because it is already shipped, tested, and ToS-clean.
* Good, because CI and the backplane need nothing else.
* Bad, because it leaves the stated user need unmet for ChatGPT and Grok
  subscriptions, which *are* documented for exactly the browser / key /
  headless triad.
* Bad, because the fleet will keep growing one-off copies (the MADR 0004
  failure mode) as consumers paste OpenCode plugins or shell out to `codex
  login` themselves.

### 6. Official vendor SDKs

* Good, because OpenAI's Codex SDK already exists (Python/TS).
* Bad, because it contradicts the library's documented invariant (`llmprovider`
  speaks HTTP; no vendor SDKs) and would pull SDK dependency trees into
  twelve binaries, nine of which never draw a wizard.
* Bad, because there is no official Go Codex login SDK equivalent to the
  Python/TS SDK; we would still hand-roll OpenAI anyway.

## Decisions Resolved

Revision 2 (2026-09-12), maintainer direction: **limit scope to OpenAI and
Grok since it is legal and feasible to support them.**

| # | Question (revision 1) | Decision |
|---|---|---|
| 5 | Vertex ADC as a Gemini auth method? | **Out of this MADR.** Gemini is not in scope. |
| 6 | Follow-up CLI-wrap MADR for Claude and Gemini? | **Out of this MADR.** Not implied by accepting this record. |

Revision 3 (2026-09-12), maintainer answers plus live probe:

| # | Question | Decision |
|---|---|---|
| 1 | Orchestrated MagicTools vs standalone | **Both, already-existing modes.** Orchestrated sub-servers use the MagicTools backplane (`NewBackplaneClient`). Standalone processes own a `TokenSource`. Sub-servers never hold ChatGPT/Grok refresh tokens. If MagicTools itself spends a subscription, it logs in once in the orchestrator; that is a MagicTools change. |
| 2 | Grok inference host | **`https://api.x.ai/v1` for both API key and session.** Live probe 2026-09-12 (below). Do not use `cli-chat-proxy.grok.com`. Do not send `X-XAI-Token-Auth`. |
| 3 | OpenAI constructor shape | **Keep provider id `openai` and branch the transport** on credential type (Platform `api.openai.com` vs Codex `chatgpt.com/backend-api/codex/responses`). No `openai-codex` id. |
| 4 | xAI OAuth client id | **Documented default + override.** Default `b1a00492-073a-47ea-816f-4c329264a828` / issuer `https://auth.x.ai`, taken from official `GrokComConfig::default()`, not from OpenCode. Override via `GROK_OAUTH2_CLIENT_ID` (same env as the CLI). Refresh/import use the id stored on the session. |
| 5 | `TokenStore` | **`0600` file in v1.** Interface in `mcplib`; consumer supplies the directory. No OS keyring in v1. |

No OpenAI/Grok questions remain open. A plan may be written once this MADR is accepted.

## More Information

### Code in this repository (facts)

* Factory and env map: `llmprovider/provider.go` (`NewProvider`, `ProviderEnvVars`).
* Descriptors: `llmprovider/descriptor.go`.
* Options: `llmprovider/options.go` (`WithBaseURL`, no auth-method option).
* OpenAI / Grok constructors and default hosts (the adapters this MADR
  changes): `llmprovider/openai.go`, `grok.go`. Claude / Gemini constructors
  are cited only as unchanged neighbours.
* Wizard flow and `Result`: `wizard/configure.go`, `wizard/prompter.go`.
* Backplane / orchestrator: `backplane.go`, `orchestrator.go`.
* Secret handling: `logging/mask.go`, `logging/redact.go`.
* Prior decisions: MADR 0001 (Grok provider), 0003 (gateways), 0004 (wizard
  canonicalization; config persistence out of scope), 0002 (XDG paths,
  accepted; helper not present in this checkout).

### Local CLI sources inspected (2026-09-12)

* Codex: `codex-rs/login/` (server, PKCE, device-code, `CLIENT_ID`,
  `AuthMode`); `codex/docs/authentication.md`; `codex/sdk/python` login APIs.
* Grok Build: `crates/codegen/xai-grok-login/` (`flow.rs`, `device_code.rs`,
  `config.rs`, `model.rs`); user guide `02-authentication.md`.
* OpenCode: `packages/opencode/src/plugin/openai/codex.ts`,
  `packages/opencode/src/plugin/xai.ts`,
  `packages/opencode/src/provider/auth.ts`.

### Official documents retrieved (2026-09-12)

* OpenAI Codex authentication: <https://learn.chatgpt.com/docs/auth>
* OpenAI Codex CLI login flags: <https://developers.openai.com/codex/cli/reference>
* Claude Code authentication: <https://code.claude.com/docs/en/authentication>
* Claude Code legal and compliance (OAuth restriction):
  <https://code.claude.com/docs/en/legal-and-compliance>
* Gemini CLI authentication: <https://geminicli.com/docs/get-started/authentication/>
* Gemini API OAuth (Cloud, not Ultra): <https://ai.google.dev/gemini-api/docs/oauth>
* Gemini CLI ToS / third-party OAuth prohibition:
  <https://github.com/google-gemini/gemini-cli/blob/main/docs/resources/tos-privacy.md>
* xAI Grok CLI reference: <https://x.ai/docs/build/cli/reference>

### Related open-source projects (pattern evidence, not endorsements)

* OpenCode Codex and xAI OAuth plugins (local tree).
* CLIProxyAPI, subllm, unified_cli, coding_agent_account_manager, CortexKit
  openai-auth, clodex, ChatCLI — subscription-bridge or CLI-wrap approaches
  summarised in §5.

### Live Grok host probe (2026-09-12) — facts

Performed against a local `~/.grok/auth.json` OIDC session (`auth_mode=oidc`,
issuer `https://auth.x.ai`, client id matching `GrokComConfig::default()`)
and `XAI_API_KEY`. Tiny `POST /responses` body (`model=grok-4`,
`max_output_tokens=8`). Tokens and response bodies are not recorded here.

| Credential | Headers | `GET api.x.ai/v1/models` | `POST api.x.ai/v1/responses` | `GET cli-chat-proxy…/models` | `POST cli-chat-proxy…/responses` |
|---|---|---|---|---|---|
| OIDC session | `Authorization: Bearer` only | 200 | 200 | 200 | **426** CLI version gate |
| OIDC session | Bearer + `X-XAI-Token-Auth: xai-grok-cli` | 200 | 200 | 200 | **426** |
| `XAI_API_KEY` | Bearer only | 200 | 200 | **401** | **401** |
| `XAI_API_KEY` | Bearer + CLI marker | 200 | 200 | **401** | **401** |

A follow-up `POST api.x.ai/v1/responses` compared quota headers:

* Session: `x-ratelimit-limit-requests: 480`
* API key: `x-ratelimit-limit-requests: 1800`

Both returned `usage.cost_in_usd_ticks`. Distinct request quotas show they
are **not the same product class**. Which ledger is debited (SuperGrok
subscription vs console.x.ai credits) was **not** visible from the HTTP
response; that remains an assumption (below). `mcplib` still uses
`api.x.ai/v1` because that is the host that accepted the session on the
Responses path this library already speaks.

### Assumptions (not facts)

* ~~xAI session bearers are accepted by some HTTP inference host… **Must be
  probed**.~~ **Falsified / replaced by the probe table above.**
* OpenAI will not revoke the published Codex CLI client id for non-Codex
  binaries. Using it is what every inspected OSS client does; it is still a
  policy risk smaller than, and different from, Anthropic's and Google's
  written bans. The same class of risk applies to the Grok public client id.
* Distinct Grok request quotas (480 vs 1800) mean the session bearer spends
  a subscription-class entitlement rather than the console API-key wallet.
  HTTP 200 plus a different rate-limit class is the evidence; the invoice
  was not inspected.
* "Headless" in the user request includes device-code and stdin/import, not
  only "no TTY and no second device." If the requirement is fully unattended
  minting with no human anywhere, only API keys, Codex access tokens, Grok
  env keys, and helper binaries qualify — device-code still needs a human
  once.

### Implementation status

`status: accepted` (revision 5, 2026-09-12). No source, test, or dependency
changes accompany this status change. Execution is specified by
[0008-PLAN-subscription-auth-for-llm-providers.md](0008-PLAN-subscription-auth-for-llm-providers.md)
(`status: proposed` until the maintainer approves that plan). The plan covers
**OpenAI and Grok in mcplib**, then **prepare-commit-msg as the first
consumer**.
