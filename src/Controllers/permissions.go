package Controllers

import (
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetPermissionMatrix(c *gin.Context, db *sql.DB) {
	endpointMatrix, err := Services.GetEndpointPermissionMatrix(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get endpoint permissions: " + err.Error()})
		c.Abort()
		return
	}

	subforumMatrix, err := Services.GetSubforumPermissionMatrix(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get subforum permissions: " + err.Error()})
		c.Abort()
		return
	}

	response := map[Services.PermissionType]Services.PermissionMatrixObject{
		Services.EndpointPermission: endpointMatrix,
		Services.SubforumPermission: subforumMatrix,
	}

	c.JSON(http.StatusOK, response)
}
