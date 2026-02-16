package Services

import (
	"github.com/gin-gonic/gin"
)

func GetUserIdFromContext(c *gin.Context) int {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		return 0
	}

	var userID int
	switch v := userIDVal.(type) {
	case int:
		userID = v
	case float64:
		userID = int(v)
	default:
		return 0
	}
	return userID
}
