package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetCharacterTemplate(c *gin.Context, db *sql.DB) {
	var config string
	err := db.QueryRow("SELECT config FROM custom_field_config WHERE entity_type = 'character'").Scan(&config)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"config": "{}"}) // Return empty JSON object if no config
			return
		}
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get character template: " + err.Error()})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": config})
}

func UpdateCharacterTemplate(c *gin.Context, db *sql.DB) {
	jsonData, err := c.GetRawData()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body"})
		c.Abort()
		return
	}

	// First, try to insert the config. If it already exists, update it.
	// This handles the case where the config might not exist yet.
	_, err = db.Exec("INSERT INTO custom_field_config (entity_type, config) VALUES (?, ?) ON DUPLICATE KEY UPDATE config = ?", "character", string(jsonData), string(jsonData))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update character template config: " + err.Error()})
		c.Abort()
		return
	}

	var tableExists int
	err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", "character_flattened").Scan(&tableExists)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to check character_flattened table existence: " + err.Error()})
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
		if err := Entities.GenerateEntityTables(customFieldEntity, "character", db); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to generate entity tables: " + err.Error()})
			c.Abort()
			return
		}
	} else {
		if err := Entities.UpdateFlattenedTable(customFieldEntity, "character", db); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update flattened table: " + err.Error()})
			c.Abort()
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Character template updated successfully"})
}
