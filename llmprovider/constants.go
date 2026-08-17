package llmprovider

// Canonical provider identifiers.
const (
	ProviderGemini = "gemini"
	ProviderOpenAI = "openai"
	ProviderClaude = "claude"
	ProviderGrok   = "grok"
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
