package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

// ClientIdentityMiddleware 在任何上游指纹修改前采集入站快照。
func ClientIdentityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := usage.ParseClientHeaders(c.Request.Header)
		c.Request = c.Request.WithContext(usage.WithClientIdentity(c.Request.Context(), identity))
		c.Next()
	}
}
