package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares" // Add this import
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CreateCharacterRequest struct {
	SubforumID   int                    `json:"subforum_id" binding:"required"`
	Name         string                 `json:"name" binding:"required"`
	Avatar       *string                `json:"avatar"`
	CustomFields map[string]interface{} `json:"custom_fields"`
}

func GetCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid Id"})
		c.Abort()
		return
	}

	entity, err := Services.GetEntity(int64(id), "character", db)
	if err != nil {
		if err == sql.ErrNoRows {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Character not found"})
		} else {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get character: " + err.Error()})
		}
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, entity)
}

func CreateCharacter(c *gin.Context, db *sql.DB) {
	var req CreateCharacterRequest
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

	// Insert Topic (without first post)
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

	character := Entities.Character{
		UserId:  userID,
		TopicId: int(topicID),
		Name:    req.Name,
		Avatar:  req.Avatar,
		CustomFields: Entities.CustomFieldEntity{
			CustomFields: req.CustomFields,
		},
	}

	createdEntity, err := Services.CreateEntity("character", &character, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create character: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusCreated, createdEntity)
}

func PatchCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid Id"})
		c.Abort()
		return
	}

	var jsonMap map[string]interface{}
	if err := c.ShouldBindJSON(&jsonMap); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	updatedEntity, err := Services.PatchEntity(int64(id), "character", jsonMap, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to patch character: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, updatedEntity)
}
