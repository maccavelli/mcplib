package llmprovider

// Canonical provider identifiers.
const (
	ProviderGemini = "gemini"
	ProviderOpenAI = "openai"
	ProviderClaude = "claude"
	ProviderGrok   = "grok"
	// ProviderOpencodeZen is the OpenCode Zen gateway (pay-as-you-go).
	ProviderOpencodeZen = "opencode-zen"
	// ProviderOpencodeGo is the OpenCode Go gateway (subscription).
	ProviderOpencodeGo = "opencode-go"
	// ProviderHuggingFace is the Hugging Face Inference Providers router.
	ProviderHuggingFace = "huggingface"
	// ProviderKilo is the Kilo Gateway (the API behind the Kilo Code agent).
	// models.dev registers this gateway as "kilo"; this package follows that
	// registry key. See docs/0003-MADR-add-gateway-llm-providers.md revision 4.
	ProviderKilo = "kilo"
)

// Common LLM API JSON field names.
const (
	jsonKeyModel       = "model"
	jsonKeyMessages    = "messages"
	jsonKeyContent     = "content"
	jsonKeyText        = "text"
	jsonKeyTools       = "tools"
	jsonKeyName        = "name"
	jsonKeyDescription = "description"
	jsonKeyFunction    = "function"
	jsonKeyEnabled     = "enabled"
	jsonKeyType        = "type"
	jsonKeyRole        = "role"
	jsonRoleUser       = "user"
	jsonRoleAssistant  = "assistant"
	jsonKeyParameters  = "parameters"
	jsonKeyInput       = "input"
	jsonKeyOutput      = "output"
	jsonKeyCallID      = "call_id"
	jsonKeyArguments   = "arguments"

	// Chat Completions field names, shared by every gateway provider that
	// speaks that format (OpenCode's chat route, Hugging Face, Kilo).
	jsonKeyMaxTokens       = "max_tokens"
	jsonKeyToolChoice      = "tool_choice"
	jsonKeyReasoningEffort = "reasoning_effort"
	jsonRoleTool           = "tool"
)

// Reasoning effort level values shared across providers.
const (
	effortLow    = "low"
	effortMedium = "medium"
	effortHigh   = "high"
	effortXHigh  = "xhigh"
)

// Item envelope types for Responses API and canonical item models.
const (
	itemTypeMessage            = "message"
	itemTypeFunctionCall       = "function_call"
	itemTypeFunctionCallOutput = "function_call_output"
	itemTypeReasoning          = "reasoning"
)
