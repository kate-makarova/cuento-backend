package Controllers

import (
	"cuento-backend/src/Router"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetEndpoints(c *gin.Context) {
	c.JSON(http.StatusOK, Router.AllRoutes)
}
