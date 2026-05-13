package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"cuento-backend/src/Websockets"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func SendMessage(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	if activeAgent == nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusServiceUnavailable, Message: "AI is not configured"})
		c.Abort()
		return
	}

	// Save user message
	_, err := db.Exec(
		"INSERT INTO ai_chat_messages (user_id, role, content, date_created) VALUES (?, 'user', ?, NOW())",
		userID, req.Content,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to save message"})
		c.Abort()
		return
	}

	// Load history for context (chronological order)
	rows, err := db.Query(
		"SELECT role, content FROM ai_chat_messages WHERE user_id = ? ORDER BY date_created ASC",
		userID,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to load history"})
		c.Abort()
		return
	}
	defer rows.Close()

	var history []ChatMessage
	history = append(history, ChatMessage{
		Role:    "user",
		Content: fmt.Sprintf("[system] The current user's ID is %d. Use this when calling tools that require a user_id.", userID),
	})
	history = append(history, ChatMessage{
		Role:    "assistant",
		Content: "Understood.",
	})
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			continue
		}
		history = append(history, msg)
	}

	// Call AI
	reply, err := activeAgent.Chat(context.Background(), history)
	if err != nil {
		code := http.StatusInternalServerError
		if strings.Contains(err.Error(), "high demand") || strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "overloaded") {
			code = http.StatusServiceUnavailable
		}
		_ = c.Error(&Middlewares.AppError{Code: code, Message: "AI error: " + err.Error()})
		c.Abort()
		return
	}

	// Save assistant reply
	var replyID int64
	res, err := db.Exec(
		"INSERT INTO ai_chat_messages (user_id, role, content, date_created) VALUES (?, 'assistant', ?, NOW())",
		userID, reply,
	)
	if err == nil {
		replyID, _ = res.LastInsertId()
	}

	// Push reply to user over WebSocket
	Websockets.MainHub.SendNotification(userID, map[string]interface{}{
		"type": "ai_message",
		"data": Entities.AIChatMessage{
			Id:          int(replyID),
			UserId:      userID,
			Role:        "assistant",
			Content:     reply,
			DateCreated: time.Now(),
		},
	})

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func GetAvailableModels(c *gin.Context, db *sql.DB) {
	models, err := ListAvailableModels(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch models: " + err.Error()})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, models)
}

func GetAIChatHistory(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rows, err := db.Query(
		"SELECT id, user_id, role, content, date_created FROM ai_chat_messages WHERE user_id = ? ORDER BY date_created ASC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch history"})
		c.Abort()
		return
	}
	defer rows.Close()

	messages := make([]Entities.AIChatMessage, 0)
	for rows.Next() {
		var msg Entities.AIChatMessage
		if err := rows.Scan(&msg.Id, &msg.UserId, &msg.Role, &msg.Content, &msg.DateCreated); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	c.JSON(http.StatusOK, messages)
}
