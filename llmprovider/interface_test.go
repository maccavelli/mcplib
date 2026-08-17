package llmprovider

import "testing"

// TestProviderInterfaceSatisfaction verifies the full interface satisfaction
// matrix across all four concrete provider implementations in llmprovider.
func TestProviderInterfaceSatisfaction(t *testing.T) {
	// Base Provider interface (all 4 satisfy)
	var _ Provider = (*OpenAIProvider)(nil)
	var _ Provider = (*ClaudeProvider)(nil)
	var _ Provider = (*GeminiProvider)(nil)
	var _ Provider = (*GrokProvider)(nil)

	// ToolProvider interface (all 4 satisfy)
	var _ ToolProvider = (*OpenAIProvider)(nil)
	var _ ToolProvider = (*ClaudeProvider)(nil)
	var _ ToolProvider = (*GeminiProvider)(nil)
	var _ ToolProvider = (*GrokProvider)(nil)

	// ThinkingProvider interface (all 4 satisfy)
	var _ ThinkingProvider = (*OpenAIProvider)(nil)
	var _ ThinkingProvider = (*ClaudeProvider)(nil)
	var _ ThinkingProvider = (*GeminiProvider)(nil)
	var _ ThinkingProvider = (*GrokProvider)(nil)

	// ThinkingToolProvider interface (all 4 satisfy)
	var _ ThinkingToolProvider = (*OpenAIProvider)(nil)
	var _ ThinkingToolProvider = (*ClaudeProvider)(nil)
	var _ ThinkingToolProvider = (*GeminiProvider)(nil)
	var _ ThinkingToolProvider = (*GrokProvider)(nil)

	// ItemProvider interface (all 4 satisfy)
	var _ ItemProvider = (*OpenAIProvider)(nil)
	var _ ItemProvider = (*ClaudeProvider)(nil)
	var _ ItemProvider = (*GeminiProvider)(nil)
	var _ ItemProvider = (*GrokProvider)(nil)

	// ItemToolProvider interface (all 4 satisfy)
	var _ ItemToolProvider = (*OpenAIProvider)(nil)
	var _ ItemToolProvider = (*ClaudeProvider)(nil)
	var _ ItemToolProvider = (*GeminiProvider)(nil)
	var _ ItemToolProvider = (*GrokProvider)(nil)

	// ItemThinkingProvider interface (all 4 satisfy)
	var _ ItemThinkingProvider = (*OpenAIProvider)(nil)
	var _ ItemThinkingProvider = (*ClaudeProvider)(nil)
	var _ ItemThinkingProvider = (*GeminiProvider)(nil)
	var _ ItemThinkingProvider = (*GrokProvider)(nil)

	// ItemThinkingToolProvider interface (all 4 satisfy)
	var _ ItemThinkingToolProvider = (*OpenAIProvider)(nil)
	var _ ItemThinkingToolProvider = (*ClaudeProvider)(nil)
	var _ ItemThinkingToolProvider = (*GeminiProvider)(nil)
	var _ ItemThinkingToolProvider = (*GrokProvider)(nil)

	// Continuer optional interface (OpenAI, Gemini, Grok satisfy; Claude is stateless)
	var _ Continuer = (*OpenAIProvider)(nil)
	var _ Continuer = (*GeminiProvider)(nil)
	var _ Continuer = (*GrokProvider)(nil)

	// ModelDiscoverer interface (all 4 satisfy)
	var _ ModelDiscoverer = (*OpenAIProvider)(nil)
	var _ ModelDiscoverer = (*ClaudeProvider)(nil)
	var _ ModelDiscoverer = (*GeminiProvider)(nil)
	var _ ModelDiscoverer = (*GrokProvider)(nil)
}
