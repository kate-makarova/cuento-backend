package Middlewares

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// NewRelicMiddleware wraps each request in a New Relic transaction and logs
// failed requests (HTTP 4xx/5xx) with a stack trace to both the application
// log and New Relic.
func NewRelicMiddleware(app *newrelic.Application) gin.HandlerFunc {
	instance := os.Getenv("PROJECT_NAME")

	return func(c *gin.Context) {
		if app == nil {
			c.Next()
			return
		}

		txn := app.StartTransaction(c.Request.Method + " " + c.FullPath())
		txn.SetWebRequestHTTP(c.Request)
		c.Request = newrelic.RequestWithTransactionContext(c.Request, txn)
		defer txn.End()

		c.Next()

		status := c.Writer.Status()
		if status >= 400 {
			stack := debug.Stack()

			errMsg := fmt.Sprintf("HTTP %d on %s %s", status, c.Request.Method, c.Request.URL.Path)

			// Include gin errors in the message if present.
			if len(c.Errors) > 0 {
				errMsg += ": " + c.Errors.String()
			}

			log.Printf("[NewRelic] app_instance=%s Failed request — %s\n%s", instance, errMsg, stack)

			txn.NoticeError(newrelic.Error{
				Message: errMsg,
				Class:   fmt.Sprintf("HTTPError%d", status),
				Stack:   newrelic.NewStackTrace(),
				Attributes: map[string]interface{}{
					"app_instance": instance,
				},
			})
		}
	}
}
