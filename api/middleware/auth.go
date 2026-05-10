package middleware

import (
	"net/http"
	"strings"

	"github.com/openseedr/api/observability"
	"github.com/openseedr/api/services"
	"github.com/gin-gonic/gin"
)

const userIDKey = "userID"
const userEmailKey = "userEmail"
const userUsernameKey = "userUsername"

// Auth validates the Bearer JWT from the Authorization header.
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":    "missing or malformed authorization header",
				"trace_id": observability.TraceID(c.Request.Context()),
			})
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := services.ValidateJWT(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":    "invalid or expired token",
				"trace_id": observability.TraceID(c.Request.Context()),
			})
			return
		}

		c.Set(userIDKey, claims.UserID.String())
		c.Set(userEmailKey, claims.Email)
		c.Set(userUsernameKey, claims.Username)
		c.Next()
	}
}

// AuthFlexible is like Auth but also accepts the JWT via the ?token= query
// parameter.  Only use this for endpoints (like file downloads) where the
// browser must navigate directly to the URL without the ability to set custom
// request headers.
func AuthFlexible() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""

		// Prefer Authorization header; fall back to ?token= query param.
		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			tokenStr = strings.TrimPrefix(h, "Bearer ")
		} else if qt := c.Query("token"); qt != "" {
			tokenStr = qt
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":    "missing or malformed authorization",
				"trace_id": observability.TraceID(c.Request.Context()),
			})
			return
		}

		claims, err := services.ValidateJWT(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":    "invalid or expired token",
				"trace_id": observability.TraceID(c.Request.Context()),
			})
			return
		}

		c.Set(userIDKey, claims.UserID.String())
		c.Set(userEmailKey, claims.Email)
		c.Set(userUsernameKey, claims.Username)
		c.Next()
	}
}

// GetUserID extracts the authenticated user ID from gin context.
func GetUserID(c *gin.Context) string {
	id, _ := c.Get(userIDKey)
	if s, ok := id.(string); ok {
		return s
	}
	return ""
}
