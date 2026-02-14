package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetTemplate(c *gin.Context, db *sql.DB) {
	entityType := c.Param("type")
	var configJSON string
	err := db.QueryRow("SELECT config FROM custom_field_config WHERE entity_type = ?", entityType).Scan(&configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			// If no config exists, return an empty JSON array, as the config is a list of fields.
			c.JSON(http.StatusOK, []gin.H{})
			return
		}
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get template: " + err.Error()})
		c.Abort()
		return
	}

	var configData interface{}
	if err := json.Unmarshal([]byte(configJSON), &configData); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to parse template config: " + err.Error()})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, configData)
}

func UpdateTemplate(c *gin.Context, db *sql.DB) {
	entityType := c.Param("type")
	jsonData, err := c.GetRawData()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		c.Abort()
		return
	}

	// First, try to insert the config. If it already exists, update it.
	// This handles the case where the config might not exist yet.
	_, err = db.Exec("INSERT INTO custom_field_config (entity_type, config) VALUES (?, ?) ON DUPLICATE KEY UPDATE config = ?", entityType, string(jsonData), string(jsonData))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update template config: " + err.Error()})
		c.Abort()
		return
	}

	var tableExists int
	err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", entityType+"_flattened").Scan(&tableExists)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to check flattened table existence: " + err.Error()})
		c.Abort()
		return
	}

	var customConfig []Entities.CustomFieldConfig
	err = json.Unmarshal(jsonData, &customConfig)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid config JSON: " + err.Error()})
		c.Abort()
		return
	}
	customFieldEntity := Entities.CustomFieldEntity{FieldConfig: customConfig}

	if tableExists == 0 {
		if err := Entities.GenerateEntityTables(customFieldEntity, entityType, db); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to generate entity tables: " + err.Error()})
			c.Abort()
			return
		}
	} else {
		if err := Entities.UpdateFlattenedTable(customFieldEntity, entityType, db); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update flattened table: " + err.Error()})
			c.Abort()
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template updated successfully"})
}
