package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
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

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit transaction"})
		c.Abort()
		return
	}

	// 2. Create Episode Entity using Service
	episode := Entities.Episode{
		TopicId:      int(topicID),
		Name:         req.Name,
		CharacterIds: req.CharacterIDs,
		CustomFields: Entities.CustomFieldEntity{
			CustomFields: req.CustomFields,
		},
	}

	createdEntity, err := Services.CreateEntity("episode", &episode, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create episode entity: " + err.Error()})
		c.Abort()
		return
	}

	createdEpisode, ok := createdEntity.(*Entities.Episode)
	if !ok {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to cast created entity"})
		c.Abort()
		return
	}

	// 3. Insert Episode-Character Relations
	if len(req.CharacterIDs) > 0 {
		// Start a new transaction for relations
		txRel, err := db.Begin()
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction for relations"})
			c.Abort()
			return
		}

		stmt, err := txRel.Prepare("INSERT INTO episode_character (episode_id, character_id) VALUES (?, ?)")
		if err != nil {
			txRel.Rollback()
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to prepare character relation statement"})
			c.Abort()
			return
		}
		defer stmt.Close()

		for _, charID := range req.CharacterIDs {
			_, err := stmt.Exec(createdEpisode.Id, charID)
			if err != nil {
				txRel.Rollback()
				_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert character relation: " + err.Error()})
				c.Abort()
				return
			}
		}

		if err := txRel.Commit(); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit relation transaction"})
			c.Abort()
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Episode created successfully", "episode_id": createdEpisode.Id, "topic_id": topicID})
}
