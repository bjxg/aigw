// Package synthesizer provides auth synthesis strategies for the watcher package.
// ConfigSynthesizer generates Auth entries from config API keys.
package synthesizer

import (
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// AuthSynthesizer defines the interface for generating Auth entries from various sources.
type AuthSynthesizer interface {
	// Synthesize generates Auth entries from the given context.
	// Returns a slice of Auth pointers and any error encountered.
	Synthesize(ctx *SynthesisContext) ([]*coreauth.Auth, error)
}
