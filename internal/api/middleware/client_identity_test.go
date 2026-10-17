package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestClientIdentityMiddlewarePreservesInboundSnapshot(t *testing.T) {
	engine := gin.New()
	engine.Use(ClientIdentityMiddleware())
	engine.GET("/", func(c *gin.Context) {
		c.Request.Header.Set("User-Agent", "claude-cli/99.0")
		identity, ok := usage.ClientIdentityFromContext(c.Request.Context())
		if !ok || identity.ClientName != "codex" || identity.ClientVersion != "0.125.0" || identity.ClientUserAgent != "codex_cli_rs/0.125.0" {
			t.Fatalf("入站快照受改写影响: %#v", identity)
		}
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "codex_cli_rs/0.125.0")
	engine.ServeHTTP(httptest.NewRecorder(), req)
}

func TestClientIdentityMiddlewareMainstreamClients(t *testing.T) {
	for _, tc := range []struct{ ua, originator, name, version string }{
		{"pi (darwin 25; arm64)", "", "pi", ""},
		{"OpenAI/JS 4.0", "pi", "pi", ""},
		{"opencode/latest/2.0.0/cli", "", "opencode", "2.0.0"},
		{"WorkBuddy/5.8.3 WorkBuddy/5.8.3 CLI/2.168.2", "", "workbuddy", "5.8.3"},
	} {
		t.Run(tc.name+tc.ua, func(t *testing.T) {
			engine := gin.New()
			engine.Use(ClientIdentityMiddleware())
			engine.POST("/v1/responses", func(c *gin.Context) {
				identity, ok := usage.ClientIdentityFromContext(c.Request.Context())
				if !ok || identity.ClientName != tc.name || identity.ClientVersion != tc.version || identity.ClientUserAgent != tc.ua {
					t.Fatalf("快照错误: %#v", identity)
				}
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			req.Header.Set("User-Agent", tc.ua)
			req.Header.Set("Originator", tc.originator)
			engine.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
