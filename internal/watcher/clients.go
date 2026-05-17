// clients.go implements watcher client lifecycle logic and persistence helpers.
package watcher

import (
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
)

func (w *Watcher) reloadClients(rescanAuth bool, affectedOAuthProviders []string, forceAuthRefresh bool) {
	log.Debugf("starting full client load process")

	w.clientsMutex.RLock()
	cfg := w.config
	w.clientsMutex.RUnlock()

	if cfg == nil {
		log.Error("config is nil, cannot reload clients")
		return
	}

	geminiAPIKeyCount, vertexCompatAPIKeyCount, claudeAPIKeyCount, codexAPIKeyCount, bedrockAPIKeyCount, openCodeGoAPIKeyCount, openAICompatCount := BuildAPIKeyClients(cfg)
	totalAPIKeyClients := geminiAPIKeyCount + vertexCompatAPIKeyCount + claudeAPIKeyCount + codexAPIKeyCount + bedrockAPIKeyCount + openCodeGoAPIKeyCount + openAICompatCount
	log.Debugf("loaded %d API key clients", totalAPIKeyClients)

	if w.reloadCallback != nil {
		log.Debugf("triggering server update callback before auth refresh")
		w.reloadCallback(cfg)
	}

	w.refreshAuthState(forceAuthRefresh)

	log.Infof("full client load complete - %d clients (%d Gemini API keys + %d Vertex API keys + %d Claude API keys + %d Codex keys + %d Bedrock keys + %d OpenCode Go keys + %d OpenAI-compat)",
		geminiAPIKeyCount+vertexCompatAPIKeyCount+claudeAPIKeyCount+codexAPIKeyCount+bedrockAPIKeyCount+openCodeGoAPIKeyCount+openAICompatCount,
		geminiAPIKeyCount,
		vertexCompatAPIKeyCount,
		claudeAPIKeyCount,
		codexAPIKeyCount,
		bedrockAPIKeyCount,
		openCodeGoAPIKeyCount,
		openAICompatCount,
	)
}

func BuildAPIKeyClients(cfg *config.Config) (int, int, int, int, int, int, int) {
	geminiAPIKeyCount := 0
	vertexCompatAPIKeyCount := 0
	claudeAPIKeyCount := 0
	codexAPIKeyCount := 0
	bedrockAPIKeyCount := 0
	openCodeGoAPIKeyCount := 0
	openAICompatCount := 0

	if cfg == nil {
		return 0, 0, 0, 0, 0, 0, 0
	}
	if len(cfg.GeminiKey) > 0 {
		geminiAPIKeyCount += len(cfg.GeminiKey)
	}
	if len(cfg.VertexCompatAPIKey) > 0 {
		vertexCompatAPIKeyCount += len(cfg.VertexCompatAPIKey)
	}
	if len(cfg.ClaudeKey) > 0 {
		claudeAPIKeyCount += len(cfg.ClaudeKey)
	}
	if len(cfg.CodexKey) > 0 {
		codexAPIKeyCount += len(cfg.CodexKey)
	}
	if len(cfg.BedrockKey) > 0 {
		bedrockAPIKeyCount += len(cfg.BedrockKey)
	}
	if len(cfg.OpenCodeGoKey) > 0 {
		openCodeGoAPIKeyCount += len(cfg.OpenCodeGoKey)
	}
	if len(cfg.OpenAICompatibility) > 0 {
		for _, compatConfig := range cfg.OpenAICompatibility {
			openAICompatCount += len(compatConfig.APIKeyEntries)
		}
	}
	return geminiAPIKeyCount, vertexCompatAPIKeyCount, claudeAPIKeyCount, codexAPIKeyCount, bedrockAPIKeyCount, openCodeGoAPIKeyCount, openAICompatCount
}
