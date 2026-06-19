package diff

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

type GeminiModelsSummary struct {
	hash  string
	count int
}

type ClaudeModelsSummary struct {
	hash  string
	count int
}

type CodexModelsSummary struct {
	hash  string
	count int
}

// SummarizeGeminiModels hashes Gemini model aliases for change detection.
func SummarizeGeminiModels(models []config.GeminiModel) GeminiModelsSummary {
	if len(models) == 0 {
		return GeminiModelsSummary{}
	}
	keys := normalizeModelPairs(func(out func(key string)) {
		for _, model := range models {
			name := strings.TrimSpace(model.Name)
			alias := strings.TrimSpace(model.Alias)
			if name == "" && alias == "" {
				continue
			}
			out(strings.ToLower(name) + "|" + strings.ToLower(alias))
		}
	})
	return GeminiModelsSummary{
		hash:  hashJoined(keys),
		count: len(keys),
	}
}

// SummarizeClaudeModels hashes Claude model aliases for change detection.
func SummarizeClaudeModels(models []config.ClaudeModel) ClaudeModelsSummary {
	if len(models) == 0 {
		return ClaudeModelsSummary{}
	}
	keys := normalizeModelPairs(func(out func(key string)) {
		for _, model := range models {
			name := strings.TrimSpace(model.Name)
			alias := strings.TrimSpace(model.Alias)
			if name == "" && alias == "" {
				continue
			}
			out(strings.ToLower(name) + "|" + strings.ToLower(alias))
		}
	})
	return ClaudeModelsSummary{
		hash:  hashJoined(keys),
		count: len(keys),
	}
}

// SummarizeCodexModels hashes Codex model aliases for change detection.
func SummarizeCodexModels(models []config.CodexModel) CodexModelsSummary {
	if len(models) == 0 {
		return CodexModelsSummary{}
	}
	keys := normalizeModelPairs(func(out func(key string)) {
		for _, model := range models {
			name := strings.TrimSpace(model.Name)
			alias := strings.TrimSpace(model.Alias)
			if name == "" && alias == "" {
				continue
			}
			out(strings.ToLower(name) + "|" + strings.ToLower(alias))
		}
	})
	return CodexModelsSummary{
		hash:  hashJoined(keys),
		count: len(keys),
	}
}
