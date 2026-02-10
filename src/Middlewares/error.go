package Middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AppError is a custom error type to hold status codes.
type AppError struct {
	Code    int    `json:"-"` // Hide from JSON response
	Message string `json:"error"`
}

func (e *AppError) Error() string {
	return e.Message
}

// ErrorMiddleware catches errors added to the context and sends a JSON response.
func ErrorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next() // Process request first

		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err // Get the last error

			// Check if it's our custom AppError
			if appErr, ok := err.(*AppError); ok {
				c.JSON(appErr.Code, appErr)
				return
			}

			// Handle other generic errors as a fallback
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "An unexpected server error occurred",
			})
		}
	}
}
