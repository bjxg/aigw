package auth

import (
	"context"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestLookupAPIKeyUpstreamModel(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{
				APIKey:  "k",
				BaseURL: "https://example.com",
				Models: []internalconfig.GeminiModel{
					{Name: "gemini-2.5-pro-exp-03-25", Alias: "g25p"},
					{Name: "gemini-2.5-flash(low)", Alias: "g25f"},
				},
			},
		},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "a1", Provider: "gemini", Attributes: map[string]string{"api_key": "k", "base_url": "https://example.com"}})

	tests := []struct {
		name   string
		authID string
		input  string
		want   string
	}{
		// Fast path + suffix preservation
		{"alias with suffix", "a1", "g25p(8192)", "gemini-2.5-pro-exp-03-25(8192)"},
		{"alias without suffix", "a1", "g25p", "gemini-2.5-pro-exp-03-25"},

		// Config suffix takes priority
		{"config suffix priority", "a1", "g25f(high)", "gemini-2.5-flash(low)"},
		{"config suffix no user suffix", "a1", "g25f", "gemini-2.5-flash(low)"},

		// Case insensitive
		{"uppercase alias", "a1", "G25P", "gemini-2.5-pro-exp-03-25"},
		{"mixed case with suffix", "a1", "G25p(4096)", "gemini-2.5-pro-exp-03-25(4096)"},

		// Direct name lookup
		{"upstream name direct", "a1", "gemini-2.5-pro-exp-03-25", "gemini-2.5-pro-exp-03-25"},
		{"upstream name with suffix", "a1", "gemini-2.5-pro-exp-03-25(8192)", "gemini-2.5-pro-exp-03-25(8192)"},

		// Cache miss scenarios
		{"non-existent auth", "non-existent", "g25p", ""},
		{"unknown alias", "a1", "unknown-alias", ""},
		{"empty auth ID", "", "g25p", ""},
		{"empty model", "a1", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := mgr.lookupAPIKeyUpstreamModel(tt.authID, tt.input)
			if resolved != tt.want {
				t.Errorf("lookupAPIKeyUpstreamModel(%q, %q) = %q, want %q", tt.authID, tt.input, resolved, tt.want)
			}
		})
	}
}

func TestAPIKeyModelAlias_ConfigHotReload(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{
				APIKey: "k",
				Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-pro-exp-03-25", Alias: "g25p"}},
			},
		},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "a1", Provider: "gemini", Attributes: map[string]string{"api_key": "k"}})

	// Initial alias
	if resolved := mgr.lookupAPIKeyUpstreamModel("a1", "g25p"); resolved != "gemini-2.5-pro-exp-03-25" {
		t.Fatalf("before reload: got %q, want %q", resolved, "gemini-2.5-pro-exp-03-25")
	}

	// Hot reload with new alias
	mgr.SetConfig(&internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{
				APIKey: "k",
				Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-flash", Alias: "g25p"}},
			},
		},
	})

	// New alias should take effect
	if resolved := mgr.lookupAPIKeyUpstreamModel("a1", "g25p"); resolved != "gemini-2.5-flash" {
		t.Fatalf("after reload: got %q, want %q", resolved, "gemini-2.5-flash")
	}
}

func TestAPIKeyModelAlias_MultipleProviders(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{{APIKey: "gemini-key", Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-pro", Alias: "gp"}}}},
		ClaudeKey: []internalconfig.ClaudeKey{{APIKey: "claude-key", Models: []internalconfig.ClaudeModel{{Name: "claude-sonnet-4", Alias: "cs4"}}}},
		CodexKey:  []internalconfig.CodexKey{{APIKey: "codex-key", Models: []internalconfig.CodexModel{{Name: "o3", Alias: "o"}}}},
		BedrockKey: []internalconfig.BedrockKey{
			{AuthMode: "sigv4", AccessKeyID: "AKIA", SecretAccessKey: "SECRET", Models: []internalconfig.BedrockModel{{Name: "claude-sonnet-4-5", Alias: "aws-sonnet"}}},
		},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "gemini-auth", Provider: "gemini", Attributes: map[string]string{"api_key": "gemini-key"}})
	_, _ = mgr.Register(ctx, &Auth{ID: "claude-auth", Provider: "claude", Attributes: map[string]string{"api_key": "claude-key"}})
	_, _ = mgr.Register(ctx, &Auth{ID: "codex-auth", Provider: "codex", Attributes: map[string]string{"api_key": "codex-key"}})
	_, _ = mgr.Register(ctx, &Auth{ID: "bedrock-auth", Provider: "bedrock", Attributes: map[string]string{"auth_mode": "sigv4", "api_key": "AKIA", "access_key_id": "AKIA"}})

	tests := []struct {
		authID, input, want string
	}{
		{"gemini-auth", "gp", "gemini-2.5-pro"},
		{"claude-auth", "cs4", "claude-sonnet-4"},
		{"codex-auth", "o", "o3"},
		{"bedrock-auth", "aws-sonnet", "claude-sonnet-4-5"},
	}

	for _, tt := range tests {
		if resolved := mgr.lookupAPIKeyUpstreamModel(tt.authID, tt.input); resolved != tt.want {
			t.Errorf("lookupAPIKeyUpstreamModel(%q, %q) = %q, want %q", tt.authID, tt.input, resolved, tt.want)
		}
	}
}

func TestApplyAPIKeyModelAlias(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{APIKey: "k", Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-pro-exp-03-25", Alias: "g25p"}}},
		},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	apiKeyAuth := &Auth{ID: "a1", Provider: "gemini", Attributes: map[string]string{"api_key": "k"}}
	oauthAuth := &Auth{ID: "oauth-auth", Provider: "gemini", Attributes: map[string]string{"auth_kind": "oauth"}}
	_, _ = mgr.Register(ctx, apiKeyAuth)

	tests := []struct {
		name       string
		auth       *Auth
		inputModel string
		wantModel  string
	}{
		{
			name:       "api_key auth with alias",
			auth:       apiKeyAuth,
			inputModel: "g25p(8192)",
			wantModel:  "gemini-2.5-pro-exp-03-25(8192)",
		},
		{
			name:       "oauth auth passthrough",
			auth:       oauthAuth,
			inputModel: "some-model",
			wantModel:  "some-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedModel := mgr.applyAPIKeyModelAlias(tt.auth, tt.inputModel)

			if resolvedModel != tt.wantModel {
				t.Errorf("model = %q, want %q", resolvedModel, tt.wantModel)
			}
		})
	}
}

type testModelAliasEntry struct {
	name  string
	alias string
}

func (e testModelAliasEntry) GetName() string  { return e.name }
func (e testModelAliasEntry) GetAlias() string { return e.alias }

func TestResolveModelAliasFromConfigModels_SlowPath(t *testing.T) {
	models := []modelAliasEntry{
		testModelAliasEntry{name: "moonshotai/kimi-k2:free", alias: "kimi-k2"},
		testModelAliasEntry{name: "gemini-2.5-pro", alias: "g25p"},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"alias to name", "kimi-k2", "moonshotai/kimi-k2:free"},
		{"alias with suffix", "kimi-k2(8192)", "moonshotai/kimi-k2:free(8192)"},
		{"name stays name", "moonshotai/kimi-k2:free", "moonshotai/kimi-k2:free"},
		{"name with suffix stays name", "moonshotai/kimi-k2:free(4096)", "moonshotai/kimi-k2:free(4096)"},
		{"uppercase alias", "KIMI-K2", "moonshotai/kimi-k2:free"},
		{"uppercase name", "MOONSHOTAI/KIMI-K2:FREE", "moonshotai/kimi-k2:free"},
		{"unknown model", "unknown", ""},
		{"empty model", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveModelAliasFromConfigModels(tt.input, models)
			if got != tt.expected {
				t.Errorf("resolveModelAliasFromConfigModels(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
