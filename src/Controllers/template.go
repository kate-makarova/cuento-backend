package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

var safeIdentifier = regexp.MustCompile(`^[a-z0-9_]+$`)

func GetTemplate(c *gin.Context, db *sql.DB) {
	entityType := c.Param("type")
	var configJSON string
	var blocksJSON, blockSettingsJSON sql.NullString
	err := db.QueryRow("SELECT config, blocks, block_settings FROM custom_field_config WHERE entity_type = ?", entityType).
		Scan(&configJSON, &blocksJSON, &blockSettingsJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{"fields": []interface{}{}, "blocks": []interface{}{}, "block_settings": []interface{}{}})
			return
		}
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get template: " + err.Error()})
		c.Abort()
		return
	}

	var fields interface{}
	if err := json.Unmarshal([]byte(configJSON), &fields); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to parse template config: " + err.Error()})
		c.Abort()
		return
	}

	var blocks interface{} = []interface{}{}
	if blocksJSON.Valid && blocksJSON.String != "" && blocksJSON.String != "null" {
		if err := json.Unmarshal([]byte(blocksJSON.String), &blocks); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to parse blocks config: " + err.Error()})
			c.Abort()
			return
		}
	}

	var blockSettings interface{} = []interface{}{}
	if blockSettingsJSON.Valid && blockSettingsJSON.String != "" && blockSettingsJSON.String != "null" {
		if err := json.Unmarshal([]byte(blockSettingsJSON.String), &blockSettings); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to parse block_settings config: " + err.Error()})
			c.Abort()
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"fields":         fields,
		"blocks":         blocks,
		"block_settings": blockSettings,
	})
}

func UpdateTemplate(c *gin.Context, db *sql.DB) {
	entityType := c.Param("type")

	var req struct {
		Fields        json.RawMessage `json:"fields"`
		Blocks        json.RawMessage `json:"blocks"`
		BlockSettings json.RawMessage `json:"block_settings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	fieldsJSON := req.Fields
	if fieldsJSON == nil {
		fieldsJSON = json.RawMessage("[]")
	}
	blocksJSON := req.Blocks
	if blocksJSON == nil {
		blocksJSON = json.RawMessage("[]")
	}
	blockSettingsJSON := req.BlockSettings
	if blockSettingsJSON == nil {
		blockSettingsJSON = json.RawMessage("[]")
	}

	_, err := db.Exec(
		"INSERT INTO custom_field_config (entity_type, config, blocks, block_settings) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE config = ?, blocks = ?, block_settings = ?",
		entityType, string(fieldsJSON), string(blocksJSON), string(blockSettingsJSON),
		string(fieldsJSON), string(blocksJSON), string(blockSettingsJSON),
	)
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
	if err := json.Unmarshal(fieldsJSON, &customConfig); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid fields JSON: " + err.Error()})
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

func CustomFieldAutocomplete(c *gin.Context, db *sql.DB) {
	entityType := c.Query("entity_type")
	fieldName := c.Query("field")
	term := c.Query("term")

	if !safeIdentifier.MatchString(entityType) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid entity_type"})
		c.Abort()
		return
	}
	if !safeIdentifier.MatchString(fieldName) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid field name"})
		c.Abort()
		return
	}

	fieldConfigs, err := getFieldConfigs(entityType, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to load field config: " + err.Error()})
		c.Abort()
		return
	}

	var matched *Entities.CustomFieldConfig
	for i := range fieldConfigs {
		if fieldConfigs[i].MachineFieldName == fieldName {
			matched = &fieldConfigs[i]
			break
		}
	}
	if matched == nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Custom field not found"})
		c.Abort()
		return
	}
	if matched.FieldType != "string" {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Field is not of type string"})
		c.Abort()
		return
	}

	query := fmt.Sprintf(
		"SELECT DISTINCT `%s` FROM `%s_flattened` WHERE `%s` IS NOT NULL AND `%s` != '' AND `%s` LIKE ? ORDER BY `%s` ASC LIMIT 20",
		fieldName, entityType, fieldName, fieldName, fieldName, fieldName,
	)
	rows, err := db.Query(query, "%"+term+"%")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to query field values: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	values := []string{}
	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err == nil {
			values = append(values, val)
		}
	}

	c.JSON(http.StatusOK, values)
}

func getFieldConfigs(entityType string, db *sql.DB) ([]Entities.CustomFieldConfig, error) {
	var configJSON string
	if err := db.QueryRow("SELECT config FROM custom_field_config WHERE entity_type = ?", entityType).Scan(&configJSON); err != nil {
		return nil, err
	}
	var configs []Entities.CustomFieldConfig
	if err := json.Unmarshal([]byte(configJSON), &configs); err != nil {
		return nil, err
	}
	return configs, nil
}
