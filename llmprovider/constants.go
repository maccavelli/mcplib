package llmprovider

// Canonical provider identifiers.
const (
	ProviderGemini = "gemini"
	ProviderOpenAI = "openai"
	ProviderClaude = "claude"
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
)
