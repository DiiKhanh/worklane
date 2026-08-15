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

// apiKeyAuth resolves `Authorization: Bearer <key>`, hashes the key, looks it up, and
// injects the tenant id. Keys are stored hashed (never plaintext); we hash the incoming
// key the same way the seed CLI did (security.HashKey) and compare.
//
// We do the auth here in the service rather than at the gateway (Traefik has no built-in
// key-auth plugin), which also keeps the logic visible and unit-testable.
func apiKeyAuth(repo app.Repo) gin.HandlerFunc {
	const prefix = "Bearer "
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing api key"})
			return
		}
		key := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
		ak, err := repo.FindAPIKey(c.Request.Context(), security.HashKey(key))
		if err != nil || ak.Status != "active" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Set(tenantCtxKey, ak.TenantID)
		c.Next()
	}
}
