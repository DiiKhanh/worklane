package http

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duykhanh/worklane/pkg/security"
)

const (
	userCtxKey   = "user_id"
	tenantCtxKey = "tenant_id"
	emailCtxKey  = "email"
)

// jwtAuth verifies a Bearer JWT with the public key and stashes the claims.
func jwtAuth(v *security.Verifier) gin.HandlerFunc {
	const prefix = "Bearer "
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		claims, err := v.Parse(strings.TrimSpace(strings.TrimPrefix(auth, prefix)))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(userCtxKey, claims.UserID)
		c.Set(tenantCtxKey, claims.TenantID)
		c.Set(emailCtxKey, claims.Email)
		c.Next()
	}
}

// internalAuth guards service-to-service routes with a shared secret.
func internalAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader("X-Internal-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}
