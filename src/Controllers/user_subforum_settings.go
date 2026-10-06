package Controllers

import (
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserSubforumSetting struct {
	SubforumID             int64 `json:"subforum_id"`
	HideNewPostsIndex      bool  `json:"hide_new_posts_index"`
	HideNewPostsActivePage bool  `json:"hide_new_posts_active_page"`
}

type UpsertUserSubforumSettingRequest struct {
	SubforumID             int64 `json:"subforum_id" binding:"required"`
	HideNewPostsIndex      bool  `json:"hide_new_posts_index"`
	HideNewPostsActivePage bool  `json:"hide_new_posts_active_page"`
}

func GetUserSubforumSettings(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)

	rows, err := db.Query(
		"SELECT subforum_id, hide_new_posts_index, hide_new_posts_active_page FROM user_subforum_settings WHERE user_id = ?",
		userID,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch settings: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	settings := []UserSubforumSetting{}
	for rows.Next() {
		var s UserSubforumSetting
		if err := rows.Scan(&s.SubforumID, &s.HideNewPostsIndex, &s.HideNewPostsActivePage); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan setting: " + err.Error()})
			c.Abort()
			return
		}
		settings = append(settings, s)
	}

	c.JSON(http.StatusOK, settings)
}

func UpsertUserSubforumSetting(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)

	var req UpsertUserSubforumSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	// If both settings are off, the row serves no purpose — remove it.
	if !req.HideNewPostsIndex && !req.HideNewPostsActivePage {
		db.Exec(
			"DELETE FROM user_subforum_settings WHERE user_id = ? AND subforum_id = ?",
			userID, req.SubforumID,
		)
		c.JSON(http.StatusOK, gin.H{"message": "Settings cleared"})
		return
	}

	_, err := db.Exec(`
		INSERT INTO user_subforum_settings (user_id, subforum_id, hide_new_posts_index, hide_new_posts_active_page)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			hide_new_posts_index = VALUES(hide_new_posts_index),
			hide_new_posts_active_page = VALUES(hide_new_posts_active_page)`,
		userID, req.SubforumID, req.HideNewPostsIndex, req.HideNewPostsActivePage,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to save settings: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, UserSubforumSetting{
		SubforumID:             req.SubforumID,
		HideNewPostsIndex:      req.HideNewPostsIndex,
		HideNewPostsActivePage: req.HideNewPostsActivePage,
	})
}
