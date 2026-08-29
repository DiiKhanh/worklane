package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
	"github.com/duykhanh/worklane/services/otp-api/internal/app"
)

// tenantCtxKey is where the resolved tenant id is stashed for handlers to read.
const tenantCtxKey = "tenant_id"

// authorCtxKey holds the authenticated user's email on the JWT path. It is empty for
// API-key callers, which is how template management is restricted to human logins.
const authorCtxKey = "actor_email"

// authenticate resolves a Bearer token that is either a user JWT (dashboard) or a tenant
// API key (machine). A JWT has three dot-separated parts and is verified locally with the
// public key. Anything else is an opaque API key, resolved by asking the identity service
// (introspection, Redis-cached). Both paths set tenant_id for downstream handlers.
func authenticate(v *security.Verifier, intro app.Introspector) gin.HandlerFunc {
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
			c.Set(authorCtxKey, claims.Email)
			c.Next()
			return
		}

		tenantID, err := intro.Introspect(c.Request.Context(), token)
		switch {
		case errors.Is(err, app.ErrInvalidAPIKey):
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		case err != nil:
			// The identity service is unreachable or errored: fail closed but distinctly,
			// so a transient auth-svc outage does not look like a bad key to the caller.
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "auth temporarily unavailable"})
			return
		}
		c.Set(tenantCtxKey, tenantID)
		c.Next()
	}
}
