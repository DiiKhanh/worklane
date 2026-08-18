package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// tenantCtxKey is where the resolved tenant id is stashed for handlers to read.
const tenantCtxKey = "tenant_id"

// authenticate resolves a Bearer token that is either a user JWT (dashboard) or a tenant
// API key (machine). A JWT has three dot-separated parts; anything else is treated as an
// opaque API key and looked up by hash. Both paths set tenant_id for downstream handlers.
func authenticate(v *security.Verifier, repo app.Repo) gin.HandlerFunc {
	const prefix = "Bearer "
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing credentials"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))

		if strings.Count(token, ".") == 2 {
			claims, err := v.Parse(token)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
			c.Set(tenantCtxKey, claims.TenantID)
			c.Next()
			return
		}

		ak, err := repo.FindAPIKey(c.Request.Context(), security.HashKey(token))
		if err != nil || ak.Status != "active" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Set(tenantCtxKey, ak.TenantID)
		c.Next()
	}
}
