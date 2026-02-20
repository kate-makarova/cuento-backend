package Middlewares

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// WebSocketAuthMiddleware extracts the JWT from the "token" query parameter for WebSocket connections.
func WebSocketAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.Query("token")
		if tokenString == "" {
			fmt.Println("WebSocket Auth Failed: No token provided")
			// Abort if no token is provided. A 401 response is a "bad response" for websockets.
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return JwtKey, nil
		})

		if err != nil || !token.Valid {
			fmt.Printf("WebSocket Auth Failed: %v\n", err)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Set("user_id", claims.UserID)
		c.Next()
	}
}
