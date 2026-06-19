package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

// formatAwareExecutor is a mock ProviderExecutor that declares support for a
// fixed set of source formats, mirroring how the real executors implement
// SupportsSourceFormat. It records whether it was invoked.
type formatAwareExecutor struct {
	providerID string
	supports   map[string]bool // sourceFormat -> supported (alt-agnostic for this mock)
	invoked    bool
}

func (e *formatAwareExecutor) Identifier() string { return e.providerID }

func (e *formatAwareExecutor) Execute(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	e.invoked = true
	return coreexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
}

func (e *formatAwareExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	e.invoked = true
	ch := make(chan coreexecutor.StreamChunk, 1)
	ch <- coreexecutor.StreamChunk{Payload: []byte(`{"ok":true}`)}
	close(ch)
	return &coreexecutor.StreamResult{Chunks: ch}, nil
}

func (e *formatAwareExecutor) Refresh(context.Context, *coreauth.Auth) (*coreauth.Auth, error) {
	return nil, nil
}

func (e *formatAwareExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	e.invoked = true
	return coreexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
}

func (e *formatAwareExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

// SupportsSourceFormat mirrors the duck-typed capability used by
// filterProvidersBySourceFormat.
func (e *formatAwareExecutor) SupportsSourceFormat(sourceFormat, alt string) bool {
	return e.supports[sourceFormat]
}

// newFilterTestManager builds a manager with codex (openai-response only) and
// an OpenAI-compatible provider (openai) executors registered, plus matching
// auths and a shared model registration so getRequestDetails returns both.
func newFilterTestManager(t *testing.T, model string) (*coreauth.Manager, *formatAwareExecutor, *formatAwareExecutor) {
	t.Helper()
	manager := coreauth.NewManager(nil, nil, nil)

	codexExec := &formatAwareExecutor{providerID: "codex", supports: map[string]bool{"openai-response": true}}
	openaiExec := &formatAwareExecutor{providerID: "openai", supports: map[string]bool{"openai": true}}
	manager.RegisterExecutor(codexExec)
	manager.RegisterExecutor(openaiExec)

	for _, p := range []string{"codex", "openai"} {
		auth := &coreauth.Auth{ID: "auth-" + p, Provider: p, Status: coreauth.StatusActive}
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatalf("Register auth for %s: %v", p, err)
		}
		registry.GetGlobalRegistry().RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{ID: model}})
		t.Cleanup(func() {
			registry.GetGlobalRegistry().UnregisterClient(auth.ID)
		})
	}
	return manager, codexExec, openaiExec
}

// TestFilterProvidersBySourceFormat_ChatCompletionsExcludesCodex verifies that
// a chat-completions request (sourceFormat="openai") filters out the Codex
// provider (which only speaks the Responses wire format), leaving only the
// OpenAI-compatible provider.
func TestFilterProvidersBySourceFormat_ChatCompletionsExcludesCodex(t *testing.T) {
	const model = "shared-model"
	manager, codexExec, _ := newFilterTestManager(t, model)
	base := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)

	got := base.filterProvidersBySourceFormat([]string{"codex", "openai"}, "openai", "")
	if len(got) != 1 || got[0] != "openai" {
		t.Fatalf("filterProvidersBySourceFormat(openai) = %v, want [openai]", got)
	}
	// Sanity: the filter itself does not invoke executors.
	if codexExec.invoked {
		t.Fatalf("codex executor should not be invoked during filtering")
	}
}

// TestFilterProvidersBySourceFormat_ResponsesExcludesOpenAICompat verifies that
// a /v1/responses request (sourceFormat="openai-response") filters out the
// OpenAI-compatible provider, leaving only Codex.
func TestFilterProvidersBySourceFormat_ResponsesExcludesOpenAICompat(t *testing.T) {
	const model = "shared-model"
	manager, _, openaiExec := newFilterTestManager(t, model)
	base := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)

	got := base.filterProvidersBySourceFormat([]string{"codex", "openai"}, "openai-response", "")
	if len(got) != 1 || got[0] != "codex" {
		t.Fatalf("filterProvidersBySourceFormat(openai-response) = %v, want [codex]", got)
	}
	if openaiExec.invoked {
		t.Fatalf("openai executor should not be invoked during filtering")
	}
}

// TestFilterProvidersBySourceFormat_CompactKeepsOpenAICompat verifies that the
// /responses/compact alt is still served by the OpenAI-compatible provider.
func TestFilterProvidersBySourceFormat_CompactKeepsOpenAICompat(t *testing.T) {
	const model = "shared-model"
	manager, _, _ := newFilterTestManager(t, model)
	base := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)

	// openai exec supports "openai"; here we test the compact path where the
	// real OpenAICompatExecutor also accepts (openai-response, responses/compact).
	// The mock openai exec only declares "openai", so register a compact-aware
	// executor to emulate the real behaviour.
	compactExec := &formatAwareExecutor{
		providerID: "openai-compat",
		supports:   map[string]bool{"openai-response": true},
	}
	manager.RegisterExecutor(compactExec)

	got := base.filterProvidersBySourceFormat([]string{"codex", "openai-compat"}, "openai-response", "responses/compact")
	// Both Codex and the OpenAI-compat executor accept openai-response for compact.
	if len(got) != 2 {
		t.Fatalf("filterProvidersBySourceFormat(compact) = %v, want both providers kept", got)
	}
}

// TestFilterProvidersBySourceFormat_NoMatchReturnsEmpty verifies that when no
// registered executor supports the requested source format, the filter returns
// an empty list (the caller then surfaces an unsupported-source-format error).
func TestFilterProvidersBySourceFormat_NoMatchReturnsEmpty(t *testing.T) {
	const model = "shared-model"
	manager, _, _ := newFilterTestManager(t, model)
	base := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)

	got := base.filterProvidersBySourceFormat([]string{"codex"}, "openai", "")
	if len(got) != 0 {
		t.Fatalf("filterProvidersBySourceFormat(codex, openai) = %v, want empty", got)
	}
}

// TestFilterProvidersBySourceFormat_NoFormatProviderKept verifies backwards
// compatibility: an executor that does NOT implement sourceFormatProvider is
// always kept by the filter.
func TestFilterProvidersBySourceFormat_NoFormatProviderKept(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	legacyExec := &legacyExecutor{providerID: "legacy"}
	manager.RegisterExecutor(legacyExec)
	base := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)

	got := base.filterProvidersBySourceFormat([]string{"legacy"}, "openai", "")
	if len(got) != 1 || got[0] != "legacy" {
		t.Fatalf("filterProvidersBySourceFormat(legacy) = %v, want [legacy]", got)
	}
}

// legacyExecutor implements ProviderExecutor but intentionally does NOT
// implement SupportsSourceFormat, emulating executors from before this change.
type legacyExecutor struct {
	providerID string
}

func (e *legacyExecutor) Identifier() string { return e.providerID }
func (e *legacyExecutor) Execute(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}
func (e *legacyExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, nil
}
func (e *legacyExecutor) Refresh(context.Context, *coreauth.Auth) (*coreauth.Auth, error) {
	return nil, nil
}
func (e *legacyExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}
func (e *legacyExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}
