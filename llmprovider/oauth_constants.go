package llmprovider

const (
	// DefaultOpenAIIssuer is the issuer used for ChatGPT OAuth sessions.
	DefaultOpenAIIssuer = "https://auth.openai.com"
	// DefaultOpenAIClientID is the public OAuth client used by Codex-compatible flows.
	DefaultOpenAIClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// DefaultOpenAIChatGPTBaseURL is the ChatGPT subscription API endpoint.
	DefaultOpenAIChatGPTBaseURL = "https://chatgpt.com/backend-api/codex"
	// DefaultOpenAIPlatformBaseURL is the OpenAI API-key endpoint.
	DefaultOpenAIPlatformBaseURL = "https://api.openai.com/v1"
	// DefaultGrokOAuthIssuer is the production xAI OAuth issuer.
	DefaultGrokOAuthIssuer = "https://auth.x.ai"
	// DefaultGrokOAuthClientID is the public OAuth client used by Grok-compatible flows.
	DefaultGrokOAuthClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	// DefaultGrokBaseURL is the xAI API endpoint used by API keys and OAuth sessions.
	DefaultGrokBaseURL = "https://api.x.ai/v1"

	defaultGrokOAuthRefreshURL = "https://auth.x.ai/oauth2/token"
	defaultGrokOAuthDeviceURL  = "https://auth.x.ai/oauth2/device/code"
)
