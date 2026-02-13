package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CreateEpisodeRequest struct {
	SubforumID   int                    `json:"subforum_id" binding:"required"`
	Name         string                 `json:"name" binding:"required"`
	CharacterIDs []int                  `json:"character_ids"`
	CustomFields map[string]interface{} `json:"custom_fields"`
}

func CreateEpisode(c *gin.Context, db *sql.DB) {
	var req CreateEpisodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	userIDVal, exists := c.Get("userID")
	if !exists {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	var userID int
	switch v := userIDVal.(type) {
	case int:
		userID = v
	case float64:
		userID = int(v)
	default:
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Invalid user ID type"})
		c.Abort()
		return
	}

	// Fetch Custom Field Config for "episode"
	var configJSON string
	var fieldConfigs []Entities.CustomFieldConfig
	err := db.QueryRow("SELECT config FROM custom_field_config WHERE entity_type = 'episode'").Scan(&configJSON)
	if err != nil {
		if err != sql.ErrNoRows {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get episode config: " + err.Error()})
			c.Abort()
			return
		}
		// If no config, fieldConfigs remains empty, which is fine.
	} else {
		if err := json.Unmarshal([]byte(configJSON), &fieldConfigs); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to parse episode config: " + err.Error()})
			c.Abort()
			return
		}
	}

	tx, err := db.Begin()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction"})
		c.Abort()
		return
	}
	defer tx.Rollback()

	// 1. Insert Topic (without first post)
	// Note: post_number = 0.
	res, err := tx.Exec("INSERT INTO topics (subforum_id, name, author_user_id, date_created, date_last_post, status, type, post_number, last_post_author_user_id) VALUES (?, ?, ?, NOW(), NOW(), 0, 0, 0, ?)",
		req.SubforumID, req.Name, userID, userID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert topic: " + err.Error()})
		c.Abort()
		return
	}
	topicID, err := res.LastInsertId()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get topic ID"})
		c.Abort()
		return
	}

	// 2. Insert Episode
	// Assuming 'episodes' table has columns: topic_id, name.
	res, err = tx.Exec("INSERT INTO episode_base (topic_id, name) VALUES (?, ?)", topicID, req.Name)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert episode: " + err.Error()})
		c.Abort()
		return
	}
	episodeID, err := res.LastInsertId()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get episode ID"})
		c.Abort()
		return
	}

	// 3. Insert Custom Fields
	for _, fieldConfig := range fieldConfigs {
		if val, ok := req.CustomFields[fieldConfig.MachineFieldName]; ok {
			var colName string
			var dbVal interface{} = val

			switch fieldConfig.FieldType {
			case "int":
				colName = "value_int"
				if f, ok := val.(float64); ok {
					dbVal = int(f)
				}
			case "decimal":
				colName = "value_decimal"
			case "string":
				colName = "value_string"
			case "text":
				colName = "value_text"
			case "date":
				colName = "value_date"
			default:
				colName = "value_string"
			}

			query := fmt.Sprintf("INSERT INTO episode_main (entity_id, field_machine_name, field_type, %s) VALUES (?, ?, ?, ?)", colName)
			_, err := tx.Exec(query, episodeID, fieldConfig.MachineFieldName, fieldConfig.FieldType, dbVal)
			if err != nil {
				_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert custom field " + fieldConfig.MachineFieldName + ": " + err.Error()})
				c.Abort()
				return
			}
		}
	}

	// 4. Insert Episode-Character Relations
	if len(req.CharacterIDs) > 0 {
		stmt, err := tx.Prepare("INSERT INTO episode_character (episode_id, character_id) VALUES (?, ?)")
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to prepare character relation statement"})
			c.Abort()
			return
		}
		defer stmt.Close()
		for _, charID := range req.CharacterIDs {
			_, err := stmt.Exec(episodeID, charID)
			if err != nil {
				_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert character relation: " + err.Error()})
				c.Abort()
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit transaction"})
		c.Abort()
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Episode created successfully", "episode_id": episodeID, "topic_id": topicID})
}
