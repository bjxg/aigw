package cmd

import (
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v6/sdk/auth"
)

// newAuthManager creates a new authentication manager instance.
// OAuth authenticators have been removed; only the file-based token store remains.
//
// Returns:
//   - *sdkAuth.Manager: A configured authentication manager instance
func newAuthManager() *sdkAuth.Manager {
	return sdkAuth.NewManager(sdkAuth.GetTokenStore())
}
