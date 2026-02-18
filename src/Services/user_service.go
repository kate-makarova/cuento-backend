package Services

import (
	"github.com/gin-gonic/gin"
)

// GetUserIdFromContext safely retrieves the user ID from the Gin context.
// It returns the user ID if it exists and is a valid integer.
// It returns 0 if the user ID does not exist or is not a valid type,
// indicating a guest user or an unauthenticated session.
func GetUserIdFromContext(c *gin.Context) int {
	userID := 0
	if id, exists := c.Get("user_id"); exists {
		userID = id.(int)
	}
	return userID
}
